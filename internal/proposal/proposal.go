package proposal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/finalreview"
	"github.com/Mawar2/Kaimi/internal/kobs"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
	"github.com/Mawar2/Kaimi/internal/writer"
)

// ProposalStatus values this service writes to Opportunity.ProposalStatus,
// using a "{stage}:{status}" vocabulary with in-progress markers, the human
// gate, and the terminal human-submitted state.
const (
	// StatusOutlineRunning means Noa is building the document skeleton.
	StatusOutlineRunning = "outline:in_progress"
	// StatusWriterRunning means Tomás is drafting (or revising) sections.
	StatusWriterRunning = "writer:in_progress"
	// StatusGate is the single human review gate: the draft is paused for
	// the human to read, edit, and decide.
	StatusGate = "writer:needs_human"
	// StatusReviewRunning means Vera is running the final pass.
	StatusReviewRunning = "final-review:in_progress"
	// StatusReadyToSubmit means the final pass passed; a human may submit.
	StatusReadyToSubmit = "final-review:ready_to_submit"
	// StatusReviewNeedsHuman means the final pass found issues; the human
	// is back at the gate with flags.
	StatusReviewNeedsHuman = "final-review:needs_human"
	// StatusSubmitted is the terminal state, set only by the human Submit
	// action. Agents stand down.
	StatusSubmitted = "submitted"
)

// Sentinel errors for the Select bridge so callers (e.g. the JSON API) can map
// them to precise HTTP status codes with errors.Is, rather than matching on
// message text. Both are "already in flight" conflicts but are distinguished so
// a caller can tell "you already pursued this" from "a stage is mid-run".
var (
	// ErrAlreadySelected means the opportunity has already been selected into
	// the proposals pipeline (a duplicate Select).
	ErrAlreadySelected = errors.New("opportunity is already in your proposals")
	// ErrStageRunning means a background stage is currently running for the
	// opportunity, so a new one cannot be claimed.
	ErrStageRunning = errors.New("opportunity already has a stage running")
)

// stageTimeout bounds each background agent stage.
const stageTimeout = 10 * time.Minute

// Deps wires the service. The agent fields take the Zone 2 agent interfaces
// defined in agents.go, so the real agents (and their stubs/mocks) drop in
// unchanged.
type Deps struct {
	Opportunities store.Store
	Documents     *document.Store
	Outline       OutlineRunner
	Writer        WriterRunner
	Review        Reviewer
	Profile       *scorer.CapabilityProfile
	// Ingest is optional. When set, the draft pipeline ingests the solicitation
	// attachments before outlining, attaches the resulting SolicitationDocs to the
	// Opportunity, and threads the extracted text into the Writer and Final Review
	// (so the live compliance pass has documents to vet against). When nil,
	// ingestion is skipped and the pipeline behaves exactly as before.
	Ingest Ingestor
}

// Service drives the gated Zone 2 proposal lifecycle: Select starts the
// real agents and pauses at the single human gate; the human edits the
// document; Approve resumes with the real Final Review on the human-edited
// revision; Submit is always a human act. Both the web dashboard and the
// desktop app call this service — it is the shared backend of epic #153.
type Service struct {
	deps *Deps
	// Now is injected for deterministic tests; defaults to time.Now.
	Now func() time.Time

	mu      sync.Mutex
	running map[string]bool
	wg      sync.WaitGroup
	// docText caches the ingested filename->text per opportunity, populated by the
	// draft pipeline and read back at the (separately triggered) Final Review so
	// the compliance pass has the documents without re-fetching. A cache miss
	// (e.g. a server restart across the human gate) degrades to a review with no
	// documents, never an error.
	docText map[string]map[string]string
}

// NewService validates deps and returns a Service.
func NewService(deps *Deps) *Service {
	return &Service{
		deps:    deps,
		Now:     time.Now,
		running: make(map[string]bool),
		docText: make(map[string]map[string]string),
	}
}

// Wait blocks until all background agent stages have finished. Tests and
// graceful shutdown use it; handlers never need to.
func (s *Service) Wait() { s.wg.Wait() }

// Document returns the current proposal document for the opportunity.
func (s *Service) Document(oppID string) (*document.Document, error) {
	return s.deps.Documents.Get(oppID)
}

// Select is the bridge event from Zone 1: the human chooses to pursue an
// opportunity. It marks the selection and starts the draft pipeline (real
// Outline agent, then real Writer agent) in the background; the pipeline
// pauses at the human gate, never running Final Review on its own.
func (s *Service) Select(ctx context.Context, oppID string) error {
	opp, err := s.deps.Opportunities.Get(ctx, oppID)
	if err != nil {
		return fmt.Errorf("select %s: %w", oppID, err)
	}
	if opp.Selected {
		return fmt.Errorf("select %s: %w", oppID, ErrAlreadySelected)
	}
	if !s.claim(oppID) {
		return fmt.Errorf("select %s: %w", oppID, ErrStageRunning)
	}

	now := s.Now()
	opp.Selected = true
	opp.SelectedAt = &now
	opp.ProposalStatus = StatusOutlineRunning
	opp.UpdatedAt = now
	if err := s.deps.Opportunities.Save(ctx, opp); err != nil {
		s.release(oppID)
		return fmt.Errorf("select %s: %w", oppID, err)
	}

	// Telemetry: the bridge event from Zone 1. trace_id is the opportunity ID,
	// set explicitly because the stage spawned below runs on a severed context.
	kobs.EmitProposal("proposal.selected", kobs.TenantID(opp, ""), opp.ID, "", "", 0,
		kobs.ProposalID(opp.ID), kobs.ProposalStatus(opp.ProposalStatus))
	s.spawn(oppID, s.runDraftPipeline)
	return nil
}

// RecordOutcome records the human's award decision for a submitted proposal:
// "won", "lost", or "" (back to pending award). Only a submitted proposal can
// carry an outcome; it feeds the Submitted archive's pipeline value stats.
func (s *Service) RecordOutcome(ctx context.Context, oppID, outcome string) error {
	switch outcome {
	case "", "won", "lost":
	default:
		return fmt.Errorf("record outcome %s: invalid outcome %q", oppID, outcome)
	}
	opp, err := s.deps.Opportunities.Get(ctx, oppID)
	if err != nil {
		return fmt.Errorf("record outcome %s: %w", oppID, err)
	}
	if opp.ProposalStatus != StatusSubmitted {
		return fmt.Errorf("record outcome %s: proposal is not submitted", oppID)
	}
	opp.AwardOutcome = outcome
	opp.UpdatedAt = s.Now()
	if err := s.deps.Opportunities.Save(ctx, opp); err != nil {
		return fmt.Errorf("record outcome %s: %w", oppID, err)
	}
	return nil
}

// UpdateSection records a human edit to one document section. Edits are
// only meaningful while the proposal is paused at a gate.
func (s *Service) UpdateSection(ctx context.Context, oppID, sectionID, body string) (*document.Document, error) {
	opp, err := s.deps.Opportunities.Get(ctx, oppID)
	if err != nil {
		return nil, err
	}
	if !atGate(opp.ProposalStatus) {
		return nil, fmt.Errorf("draft for %s is not at the review gate (status %q)", oppID, opp.ProposalStatus)
	}
	return s.deps.Documents.UpdateSection(oppID, sectionID, body, "human")
}

// Approve is the gate's go decision: the human is done editing, and the
// real Final Review agent runs on the document exactly as the human left
// it. The verdict lands back on the opportunity status and, for issues, as
// document flags.
func (s *Service) Approve(ctx context.Context, oppID string) error {
	opp, err := s.deps.Opportunities.Get(ctx, oppID)
	if err != nil {
		return err
	}
	if !atGate(opp.ProposalStatus) {
		return fmt.Errorf("proposal %s is not at the review gate (status %q)", oppID, opp.ProposalStatus)
	}
	if !s.claim(oppID) {
		return fmt.Errorf("proposal %s already has a stage running", oppID)
	}
	if err := s.setStatus(ctx, oppID, StatusReviewRunning); err != nil {
		s.release(oppID)
		return err
	}
	// Telemetry: the human's go decision at the gate, before Final Review runs.
	kobs.EmitProposal("proposal.approved", kobs.TenantID(opp, ""), oppID, "", "", 0,
		kobs.ProposalID(oppID), kobs.ProposalStatus(StatusReviewRunning))
	s.spawn(oppID, s.runFinalReview)
	return nil
}

// RequestChanges is the gate's other decision: the draft goes back to the
// Writer with the human's note recorded in the document history.
func (s *Service) RequestChanges(ctx context.Context, oppID, note string) error {
	opp, err := s.deps.Opportunities.Get(ctx, oppID)
	if err != nil {
		return err
	}
	if !atGate(opp.ProposalStatus) {
		return fmt.Errorf("proposal %s is not at the review gate (status %q)", oppID, opp.ProposalStatus)
	}
	if !s.claim(oppID) {
		return fmt.Errorf("proposal %s already has a stage running", oppID)
	}
	if note = strings.TrimSpace(note); note != "" {
		if _, err := s.deps.Documents.AppendRevisionNote(oppID, "human", "Request changes: "+note); err != nil {
			s.release(oppID)
			return err
		}
	}
	if err := s.setStatus(ctx, oppID, StatusWriterRunning); err != nil {
		s.release(oppID)
		return err
	}
	s.spawn(oppID, s.runRevision)
	return nil
}

// Submit is always a human act: it marks the proposal submitted. Kaimi
// never submits on its own.
func (s *Service) Submit(ctx context.Context, oppID string) error {
	opp, err := s.deps.Opportunities.Get(ctx, oppID)
	if err != nil {
		return err
	}
	if opp.ProposalStatus != StatusReadyToSubmit {
		return fmt.Errorf("proposal %s is not ready to submit (status %q)", oppID, opp.ProposalStatus)
	}
	if err := s.setStatus(ctx, oppID, StatusSubmitted); err != nil {
		return err
	}
	// Telemetry: the terminal human act. Emitted only on a successful transition.
	kobs.EmitProposal("proposal.submitted", kobs.TenantID(opp, ""), oppID, "", "", 0,
		kobs.ProposalID(oppID), kobs.ProposalStatus(StatusSubmitted))
	return nil
}

// atGate reports whether the proposal is paused for the human.
func atGate(status string) bool {
	return status == StatusGate || status == StatusReviewNeedsHuman
}

// DisplayState maps a persisted ProposalStatus to a coarse display state for
// clients: "progress" (an agent is working), "human" (paused at the review
// gate), "done" (ready to submit), "submitted" (terminal), or "failed" (a stage
// errored). The empty/unknown status reads as "progress" — selected but with no
// pipeline state yet. This is the API-facing twin of the HTML dashboard's
// proposalView state and keeps the web and desktop surfaces consistent.
func DisplayState(status string) string {
	switch status {
	case StatusGate, StatusReviewNeedsHuman:
		return "human"
	case StatusReadyToSubmit:
		return "done"
	case StatusSubmitted:
		return "submitted"
	case "outline:failed", "writer:failed", "final-review:failed":
		return "failed"
	default:
		// StatusOutlineRunning, StatusWriterRunning, StatusReviewRunning, the empty
		// status, and any legacy value all read as in-progress.
		return "progress"
	}
}

// claim/release guard against concurrent stages on one opportunity.
func (s *Service) claim(oppID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[oppID] {
		return false
	}
	s.running[oppID] = true
	return true
}

func (s *Service) release(oppID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, oppID)
}

// spawn runs one background stage with its own context: the HTTP request
// that triggered it ends immediately, while the agents keep working and
// persist status at every transition (the UI polls via auto-refresh).
func (s *Service) spawn(oppID string, stage func(ctx context.Context, oppID string)) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.release(oppID)
		ctx, cancel := context.WithTimeout(context.Background(), stageTimeout)
		defer cancel()
		stage(ctx, oppID)
	}()
}

// runDraftPipeline executes Outline then Writer, building the proposal
// document, and pauses at the human gate.
func (s *Service) runDraftPipeline(ctx context.Context, oppID string) {
	opp, err := s.deps.Opportunities.Get(ctx, oppID)
	if err != nil {
		return
	}

	// Step 0: ingest the solicitation documents (best-effort). Their text grounds
	// the Outline, the Writer, and the Final Review compliance pass.
	documents := s.ingestDocuments(ctx, opp)

	// Telemetry: open the outline span (agent Noa). trace_id is the opportunity
	// ID; the start time is captured off the injected clock for the duration.
	tenant := kobs.TenantID(opp, "")
	outlineStart := s.Now()
	kobs.EmitProposal("proposal.outline.started", tenant, oppID, "outline", kobs.AgentOutline, 0,
		kobs.ProposalID(oppID), kobs.ProposalStage("outline"))

	out, res, err := s.deps.Outline.Run(ctx, opp, documents)
	if err != nil || res == nil || res.IsFailed() || out == nil {
		kobs.EmitProposal("proposal.outline.failed", tenant, oppID, "outline", kobs.AgentOutline,
			s.Now().Sub(outlineStart).Milliseconds(),
			kobs.ProposalID(oppID), kobs.ProposalStage("outline"), kobs.ProposalStatus("failed"), kobs.ProposalErrorOf(err))
		resErr := ""
		if res != nil {
			resErr = res.Error
		}
		s.failStatus(ctx, oppID, "outline:failed", failCause(err, resErr, "outline produced no usable result"))
		return
	}

	doc := &document.Document{
		OpportunityID: opp.ID,
		Title:         out.Title,
		Sections:      sectionsFromOutline(out),
	}
	if err := s.deps.Documents.Create(doc, "outline",
		fmt.Sprintf("Outline skeleton: %d sections", len(out.Sections))); err != nil {
		kobs.EmitProposal("proposal.outline.failed", tenant, oppID, "outline", kobs.AgentOutline,
			s.Now().Sub(outlineStart).Milliseconds(),
			kobs.ProposalID(oppID), kobs.ProposalStage("outline"), kobs.ProposalStatus("failed"), kobs.ProposalErrorOf(err))
		s.failStatus(ctx, oppID, "outline:failed", fmt.Errorf("persisting outline skeleton: %w", err))
		return
	}

	// Telemetry: close the outline span on success.
	kobs.EmitProposal("proposal.outline.completed", tenant, oppID, "outline", kobs.AgentOutline,
		s.Now().Sub(outlineStart).Milliseconds(),
		kobs.ProposalID(oppID), kobs.ProposalStage("outline"), kobs.ProposalStatus("success"))

	if err := s.setStatus(ctx, oppID, StatusWriterRunning); err != nil {
		return
	}
	s.draftSections(ctx, oppID, opp, out, documents, "Draft from the technical writer", "", false)
}

// ingestDocuments runs the optional ingest stage for opp, attaches the resulting
// SolicitationDocs to the Opportunity, caches their extracted text for the later
// Final Review, and returns the filename->text map. It is best-effort: when no
// ingestor is configured it returns nil, and any ingest error degrades to nil
// (the pipeline continues without documents) rather than failing the proposal.
func (s *Service) ingestDocuments(ctx context.Context, opp *opportunity.Opportunity) map[string]string {
	if s.deps.Ingest == nil {
		return nil
	}
	docs, texts, _, err := s.deps.Ingest.Ingest(ctx, opp)
	if err != nil {
		// Ingestion is enrichment, not a gate here — the human still reviews every
		// draft. Continue without documents.
		return nil
	}
	if len(docs) > 0 {
		opp.Documents = docs
		opp.UpdatedAt = s.Now()
		// Persist the attached documents; ignore a save error (non-fatal to drafting).
		_ = s.deps.Opportunities.Save(ctx, opp)
	}
	s.cacheDocText(opp.ID, texts)
	return texts
}

// cacheDocText stores the per-opportunity extracted text for later stages.
func (s *Service) cacheDocText(oppID string, texts map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docText[oppID] = texts
}

// cachedDocText returns the cached extracted text for an opportunity, or nil.
func (s *Service) cachedDocText(oppID string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.docText[oppID]
}

// runRevision re-runs the Writer over the existing document after a
// change request, then returns to the gate.
func (s *Service) runRevision(ctx context.Context, oppID string) {
	opp, err := s.deps.Opportunities.Get(ctx, oppID)
	if err != nil {
		return
	}
	doc, err := s.deps.Documents.Get(oppID)
	if err != nil {
		s.failStatus(ctx, oppID, "writer:failed", fmt.Errorf("loading document for revision: %w", err))
		return
	}
	// Reuse the document text cached at draft time (revisions never re-ingest).
	// Thread the human's change request into the Writer so the revision actually
	// addresses it, instead of redrafting blind.
	s.draftSections(ctx, oppID, opp, outlineFromDocument(doc), s.cachedDocText(oppID),
		"Revised draft after change request", latestChangeRequest(doc), true)
}

// latestChangeRequest returns the most recent human "Request changes:" note on
// the document (stripped of its prefix), or "" if there is none. RequestChanges
// records the note via Documents.AppendRevisionNote with this exact prefix.
func latestChangeRequest(doc *document.Document) string {
	const prefix = "Request changes: "
	for i := len(doc.Revisions) - 1; i >= 0; i-- {
		r := doc.Revisions[i]
		if r.Actor == "human" && strings.HasPrefix(r.Note, prefix) {
			return strings.TrimPrefix(r.Note, prefix)
		}
	}
	return ""
}

// draftSections runs the Writer agent ONE SECTION AT A TIME (issue #158),
// applying each drafted section to the document as it completes so the
// human can review the outline — and watch the draft grow — while the
// writer works. It pauses at the gate when every section is drafted.
func (s *Service) draftSections(ctx context.Context, oppID string, opp *opportunity.Opportunity, out *outline.Outline, documents map[string]string, note, revisionNote string, revision bool) {
	// Telemetry: open the writer span (agent Tomás). revision=true marks the
	// re-run after a human change request; the start time uses the injected clock.
	tenant := kobs.TenantID(opp, "")
	writerStart := s.Now()
	kobs.EmitProposal("proposal.writer.started", tenant, oppID, "writer", kobs.AgentWriter, 0,
		kobs.ProposalID(oppID), kobs.ProposalStage("writer"), kobs.ProposalRevision(revision))
	for _, section := range out.Sections {
		// A one-section outline keeps the Writer's per-section prompting
		// identical while letting the document update incrementally.
		single := &outline.Outline{
			OpportunityID:   out.OpportunityID,
			Title:           out.Title,
			Sections:        []outline.Section{section},
			FormattingRules: out.FormattingRules,
			GeneratedAt:     out.GeneratedAt,
		}
		draft, res, err := s.deps.Writer.Run(ctx, writer.Input{
			Opportunity:  opp,
			Outline:      single,
			Profile:      s.deps.Profile,
			Documents:    documents,
			RevisionNote: revisionNote,
		})
		if err != nil || res == nil || res.IsFailed() {
			// Keep whatever sections already landed; surface the failure.
			kobs.EmitProposal("proposal.writer.failed", tenant, oppID, "writer", kobs.AgentWriter,
				s.Now().Sub(writerStart).Milliseconds(),
				kobs.ProposalID(oppID), kobs.ProposalStage("writer"), kobs.ProposalStatus("failed"),
				kobs.ProposalRevision(revision), kobs.ProposalErrorOf(err))
			resErr := ""
			if res != nil {
				resErr = res.Error
			}
			s.failStatus(ctx, oppID, "writer:failed",
				failCause(err, resErr, fmt.Sprintf("writer produced no usable draft for section %q", section.Title)))
			return
		}
		bodies := splitDraft(draft, single)
		if len(bodies) == 0 {
			continue
		}
		if _, err := s.deps.Documents.ReplaceSections(oppID, bodies, "writer",
			fmt.Sprintf("%s — %s", note, section.Title)); err != nil {
			kobs.EmitProposal("proposal.writer.failed", tenant, oppID, "writer", kobs.AgentWriter,
				s.Now().Sub(writerStart).Milliseconds(),
				kobs.ProposalID(oppID), kobs.ProposalStage("writer"), kobs.ProposalStatus("failed"),
				kobs.ProposalRevision(revision), kobs.ProposalErrorOf(err))
			s.failStatus(ctx, oppID, "writer:failed", fmt.Errorf("applying drafted section %q: %w", section.Title, err))
			return
		}
		// Telemetry: one section landed. The title is content-class.
		kobs.EmitProposal("proposal.section.updated", tenant, oppID, "writer", kobs.AgentWriter, 0,
			kobs.ProposalID(oppID), kobs.ProposalStage("writer"), kobs.ProposalRevision(revision),
			kobs.ProposalSection(section.Title))
	}
	// Telemetry: close the writer span, then record reaching the human gate.
	kobs.EmitProposal("proposal.writer.completed", tenant, oppID, "writer", kobs.AgentWriter,
		s.Now().Sub(writerStart).Milliseconds(),
		kobs.ProposalID(oppID), kobs.ProposalStage("writer"), kobs.ProposalStatus("success"),
		kobs.ProposalRevision(revision))
	_ = s.setStatus(ctx, oppID, StatusGate)
	kobs.EmitProposal("proposal.gate.reached", tenant, oppID, "", "", 0,
		kobs.ProposalID(oppID), kobs.ProposalStatus(StatusGate))
}

// runFinalReview renders the document as the human left it and runs the
// real Final Review agent on that revision.
func (s *Service) runFinalReview(ctx context.Context, oppID string) {
	opp, err := s.deps.Opportunities.Get(ctx, oppID)
	if err != nil {
		return
	}
	doc, err := s.deps.Documents.Get(oppID)
	if err != nil {
		s.failStatus(ctx, oppID, "final-review:failed", fmt.Errorf("loading document for final review: %w", err))
		return
	}

	// Telemetry: open the final-review span (agent Vera) around the real pass.
	tenant := kobs.TenantID(opp, "")
	reviewStart := s.Now()
	kobs.EmitProposal("proposal.finalreview.started", tenant, oppID, "finalreview", kobs.AgentReview, 0,
		kobs.ProposalID(oppID), kobs.ProposalStage("finalreview"))

	res, err := s.deps.Review.Review(ctx, finalreview.Input{
		Draft:       doc.Markdown(),
		Opportunity: opp,
		Outline:     outlineFromDocument(doc),
		Documents:   s.cachedDocText(oppID),
	})
	if err != nil || res == nil {
		s.failStatus(ctx, oppID, "final-review:failed", failCause(err, "", "final review returned no result"))
		return
	}

	switch {
	case res.Status == "ready_to_submit":
		// Resolve any outstanding flags: the final pass is clean.
		if len(doc.Flags) > 0 {
			flags := make([]document.Flag, len(doc.Flags))
			copy(flags, doc.Flags)
			for i := range flags {
				flags[i].Resolved = true
			}
			_, _ = s.deps.Documents.SetFlags(oppID, flags, "final-review", "Final pass clean — flags resolved")
		}
		_ = s.setStatus(ctx, oppID, StatusReadyToSubmit)
		// Telemetry: the pass was clean and the proposal is ready to submit.
		kobs.EmitProposal("proposal.finalreview.completed", tenant, oppID, "finalreview", kobs.AgentReview,
			s.Now().Sub(reviewStart).Milliseconds(),
			kobs.ProposalID(oppID), kobs.ProposalStage("finalreview"), kobs.ProposalStatus("ready_to_submit"))
	case res.NeedsHuman():
		flags := flagsFromResult(res.Flags, doc)
		if len(flags) > 0 {
			_, _ = s.deps.Documents.SetFlags(oppID, flags, "final-review", "Final review issues")
		}
		_ = s.setStatus(ctx, oppID, StatusReviewNeedsHuman)
		// Telemetry: the pass sent the human back to the gate with flags.
		kobs.EmitProposal("proposal.finalreview.needs_human", tenant, oppID, "finalreview", kobs.AgentReview,
			s.Now().Sub(reviewStart).Milliseconds(),
			kobs.ProposalID(oppID), kobs.ProposalStage("finalreview"), kobs.ProposalStatus("needs_human"))
	default:
		s.failStatus(ctx, oppID, "final-review:failed", fmt.Errorf("unexpected final-review status %q", res.Status))
	}
}

// setStatus persists a forward ProposalStatus transition and clears any stale
// failure reason left by a previous attempt.
func (s *Service) setStatus(ctx context.Context, oppID, status string) error {
	return s.persistStatus(ctx, oppID, status, "")
}

// persistStatus writes the ProposalStatus (and its optional failure reason) onto
// the stored Opportunity. reason is "" for normal transitions.
func (s *Service) persistStatus(ctx context.Context, oppID, status, reason string) error {
	opp, err := s.deps.Opportunities.Get(ctx, oppID)
	if err != nil {
		return err
	}
	opp.ProposalStatus = status
	opp.ProposalStatusReason = reason
	opp.UpdatedAt = s.Now()
	return s.deps.Opportunities.Save(ctx, opp)
}

// failStatus halts a stage: it logs the cause at ERROR with the opportunity id and
// status (so a live failure is diagnosable from stdout/Cloud Run logs, #56) and
// persists a short, single-line reason on the proposal so the dashboard/API can
// show WHY a proposal stalled instead of a bare "failed".
func (s *Service) failStatus(ctx context.Context, oppID, status string, cause error) {
	reason := shortReason(cause)
	log.Printf("ERROR: proposal %s %s: %s", oppID, status, reason)
	_ = s.persistStatus(ctx, oppID, status, reason)
}

// shortReason renders an error as a single-line, length-capped string safe to
// store on the record and surface in the UI. A nil cause yields a generic note.
func shortReason(cause error) string {
	if cause == nil {
		return "stage failed without a reported reason"
	}
	r := strings.Join(strings.Fields(cause.Error()), " ") // collapse newlines/runs of whitespace
	const maxLen = 240
	if len(r) > maxLen {
		r = r[:maxLen-1] + "…"
	}
	return r
}

// failCause picks the most informative cause for a failed stage: the returned
// error if any, else the agent Result's recorded error, else a stage fallback.
func failCause(err error, resultErr, fallback string) error {
	switch {
	case err != nil:
		return err
	case strings.TrimSpace(resultErr) != "":
		return errors.New(resultErr)
	default:
		return errors.New(fallback)
	}
}

// sectionsFromOutline converts the Outline agent's sections into document
// sections, preserving order.
func sectionsFromOutline(out *outline.Outline) []document.Section {
	secs := make([]document.Section, 0, len(out.Sections))
	for _, s := range out.Sections {
		secs = append(secs, document.Section{
			ID:      s.ID,
			Heading: s.Title,
			Status:  "outlined",
		})
	}
	return secs
}

// outlineFromDocument rebuilds a minimal Outline from the document so the
// Writer (revisions) and Final Review (section checks) can run against the
// document as it stands. Formatting rules are unspecified — agents must
// not invent defaults for them.
func outlineFromDocument(doc *document.Document) *outline.Outline {
	sections := make([]outline.Section, 0, len(doc.Sections))
	for _, s := range doc.Sections {
		sections = append(sections, outline.Section{
			ID:       s.ID,
			Title:    s.Heading,
			Required: true,
		})
	}
	return &outline.Outline{
		OpportunityID: doc.OpportunityID,
		Title:         doc.Title,
		Sections:      sections,
		FormattingRules: &outline.FormattingRules{
			PageLimit:   &outline.FormattingRule{},
			Font:        &outline.FormattingRule{},
			Margins:     &outline.FormattingRule{},
			LineSpacing: &outline.FormattingRule{},
			FileFormat:  &outline.FormattingRule{},
		},
	}
}

// splitDraft maps the Writer's draft text ("\n## Title\n body" per section)
// back onto document section ids by heading.
func splitDraft(draft string, out *outline.Outline) map[string]string {
	byTitle := make(map[string]string, len(out.Sections))
	for _, s := range out.Sections {
		byTitle[s.Title] = s.ID
	}
	bodies := make(map[string]string)
	chunks := strings.Split(draft, "\n## ")
	for _, chunk := range chunks {
		title, body, found := strings.Cut(chunk, "\n")
		if !found {
			continue
		}
		id, ok := byTitle[strings.TrimSpace(title)]
		if !ok {
			continue
		}
		if body = strings.TrimSpace(body); body != "" {
			bodies[id] = body
		}
	}
	return bodies
}

// flagsFromResult converts the Final Review agent's issue flags
// ("issue_1", "issue_2", …) into document flags. Unresolved-gap issues are
// anchored to the section holding the gap so the editor can point the human
// straight at it; every other issue keeps the generic document-level shape.
func flagsFromResult(resultFlags map[string]string, doc *document.Document) []document.Flag {
	var flags []document.Flag
	for i := 1; ; i++ {
		detail, ok := resultFlags[fmt.Sprintf("issue_%d", i)]
		if !ok {
			break
		}
		if section, missing, isGap := finalreview.ParseGapIssue(detail); isGap {
			flags = append(flags, document.Flag{
				SectionID: sectionIDByHeading(doc, section),
				Title:     "Unresolved gap",
				Detail:    missing,
			})
			continue
		}
		flags = append(flags, document.Flag{
			Title:  detail,
			Detail: "Flagged by the final review agent",
		})
	}
	return flags
}

// sectionIDByHeading finds the document section whose heading matches the one
// Vera reported for a gap. Empty when nothing matches (e.g. a gap she could
// not attribute to a section), which leaves the flag document-level.
func sectionIDByHeading(doc *document.Document, heading string) string {
	for i := range doc.Sections {
		if doc.Sections[i].Heading == heading {
			return doc.Sections[i].ID
		}
	}
	return ""
}

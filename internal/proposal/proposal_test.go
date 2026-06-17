package proposal

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/finalreview"
	"github.com/Mawar2/Kaimi/internal/googledocs"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
	"github.com/Mawar2/Kaimi/internal/writer"
)

// newTestService wires the REAL agents end to end: the real Outline agent
// with the cached (no-network) Google Docs client, the real Writer in stub
// mode, and the real Final Review agent. Only the LLM is absent.
func newTestService(t *testing.T) (*Service, store.Store) {
	t.Helper()
	dir := t.TempDir()
	opps, err := store.NewJSONStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	docs, err := document.NewStore(dir)
	if err != nil {
		t.Fatalf("document store: %v", err)
	}
	docsClient, err := googledocs.NewClient(context.Background(), googledocs.Config{UseCached: true})
	if err != nil {
		t.Fatalf("googledocs cached client: %v", err)
	}
	svc := NewService(&Deps{
		Opportunities: opps,
		Documents:     docs,
		Outline:       outline.New(docsClient),
		Writer:        writer.New(), // stub mode: deterministic, no LLM
		Review:        finalreview.New(),
		Profile:       &scorer.CapabilityProfile{},
	})
	return svc, opps
}

func seedOpp(t *testing.T, s store.Store) *opportunity.Opportunity {
	t.Helper()
	now := time.Now()
	opp := &opportunity.Opportunity{
		ID:               "zta-1",
		Title:            "Zero Trust Architecture Modernization",
		Agency:           "DHS CISA",
		NAICSCode:        "541512",
		Description:      "Modernize zero trust architecture.",
		ResponseDeadline: now.Add(30 * 24 * time.Hour),
		Score:            0.87,
		Recommendation:   "BID",
		Requirements:     []string{"FedRAMP High"},
		ScoredAt:         &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.Save(context.Background(), opp); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return opp
}

func TestSelectRunsRealAgentsToTheGate(t *testing.T) {
	svc, opps := newTestService(t)
	seedOpp(t, opps)

	if err := svc.Select(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Select: %v", err)
	}
	svc.Wait()

	opp, err := opps.Get(context.Background(), "zta-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !opp.Selected || opp.SelectedAt == nil {
		t.Errorf("opportunity not marked selected")
	}
	if opp.ProposalStatus != StatusGate {
		t.Fatalf("ProposalStatus = %q, want %q (the pipeline must PAUSE at the human gate, never run final review)", opp.ProposalStatus, StatusGate)
	}

	doc, err := svc.Document("zta-1")
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if len(doc.Sections) < 5 {
		t.Errorf("outline should produce at least the five standard volumes, got %d", len(doc.Sections))
	}
	for _, sec := range doc.Sections {
		if strings.TrimSpace(sec.Body) == "" {
			t.Errorf("section %q has no drafted body", sec.ID)
		}
	}
	actors := []string{}
	for _, r := range doc.Revisions {
		actors = append(actors, r.Actor)
	}
	if len(actors) < 2 || actors[0] != "outline" || actors[1] != "writer" {
		t.Errorf("revision trail should be outline then writer, got %v", actors)
	}
}

func TestSelectTwiceFails(t *testing.T) {
	svc, opps := newTestService(t)
	seedOpp(t, opps)
	if err := svc.Select(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Select: %v", err)
	}
	svc.Wait()
	if err := svc.Select(context.Background(), "zta-1"); err == nil {
		t.Errorf("second Select must fail")
	}
}

func TestApproveRunsRealFinalReview_FindsGaps(t *testing.T) {
	svc, opps := newTestService(t)
	seedOpp(t, opps)
	if err := svc.Select(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Select: %v", err)
	}
	svc.Wait()

	// The stub draft does not mention "FedRAMP High", so the real Final
	// Review agent must send it back to the human with flags.
	if err := svc.Approve(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	svc.Wait()

	opp, _ := opps.Get(context.Background(), "zta-1")
	if opp.ProposalStatus != "final-review:needs_human" {
		t.Fatalf("ProposalStatus = %q, want final-review:needs_human", opp.ProposalStatus)
	}
	doc, _ := svc.Document("zta-1")
	if len(doc.Flags) == 0 {
		t.Errorf("final review issues should land as document flags")
	}
}

func TestHumanEditsAreWhatVeraReviews(t *testing.T) {
	svc, opps := newTestService(t)
	seedOpp(t, opps)
	if err := svc.Select(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Select: %v", err)
	}
	svc.Wait()

	// Human edits the draft at the gate to satisfy the must-have
	// requirement; Final Review must run on THIS revision (INTENT.md).
	doc, _ := svc.Document("zta-1")
	if _, err := svc.UpdateSection(context.Background(), "zta-1", doc.Sections[0].ID,
		"We will use FedRAMP High authorized tooling throughout."); err != nil {
		t.Fatalf("UpdateSection: %v", err)
	}

	if err := svc.Approve(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	svc.Wait()

	opp, _ := opps.Get(context.Background(), "zta-1")
	if opp.ProposalStatus != "final-review:ready_to_submit" {
		t.Fatalf("ProposalStatus = %q, want final-review:ready_to_submit (review must pass on the human-edited revision)", opp.ProposalStatus)
	}

	// Submit is a human act and only valid from ready_to_submit.
	if err := svc.Submit(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	opp, _ = opps.Get(context.Background(), "zta-1")
	if opp.ProposalStatus != StatusSubmitted {
		t.Errorf("ProposalStatus = %q, want %q", opp.ProposalStatus, StatusSubmitted)
	}
}

func TestRequestChangesLoopsBackToGate(t *testing.T) {
	svc, opps := newTestService(t)
	seedOpp(t, opps)
	if err := svc.Select(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Select: %v", err)
	}
	svc.Wait()

	if err := svc.RequestChanges(context.Background(), "zta-1", "Tighten the technical approach."); err != nil {
		t.Fatalf("RequestChanges: %v", err)
	}
	svc.Wait()

	opp, _ := opps.Get(context.Background(), "zta-1")
	if opp.ProposalStatus != StatusGate {
		t.Fatalf("ProposalStatus = %q, want back at %q", opp.ProposalStatus, StatusGate)
	}
	doc, _ := svc.Document("zta-1")
	found := false
	for _, r := range doc.Revisions {
		if strings.Contains(r.Note, "Tighten the technical approach.") {
			found = true
		}
	}
	if !found {
		t.Errorf("the human's change-request note must be recorded in the revision history")
	}
}

func TestGuards(t *testing.T) {
	svc, opps := newTestService(t)
	seedOpp(t, opps)

	if err := svc.Approve(context.Background(), "zta-1"); err == nil {
		t.Errorf("Approve before the gate must fail")
	}
	if err := svc.Submit(context.Background(), "zta-1"); err == nil {
		t.Errorf("Submit before ready_to_submit must fail")
	}
	if _, err := svc.UpdateSection(context.Background(), "zta-1", "x", "y"); err == nil {
		t.Errorf("UpdateSection without a document must fail")
	}
	if err := svc.Select(context.Background(), "missing"); err == nil {
		t.Errorf("Select on unknown opportunity must fail")
	}
}

// recordingWriter records every Run call so tests can prove section-by-
// section drafting (issue #158).
type recordingWriter struct {
	mu    sync.Mutex
	calls []writerCall
}

type writerCall struct {
	sectionCount int
	title        string
	documents    map[string]string
	revisionNote string
}

func (r *recordingWriter) Run(_ context.Context, in writer.Input) (string, *agent.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	title := ""
	if len(in.Outline.Sections) > 0 {
		title = in.Outline.Sections[0].Title
	}
	r.calls = append(r.calls, writerCall{sectionCount: len(in.Outline.Sections), title: title, documents: in.Documents, revisionNote: in.RevisionNote})
	draft := "\n## " + title + "\nDrafted body for " + title + "\n"
	return draft, &agent.Result{AgentName: "writer", Status: agent.StatusSuccess, CompletedAt: time.Now()}, nil
}

// fakeIngestor returns canned documents and extracted text.
type fakeIngestor struct {
	docs  []opportunity.SolicitationDoc
	texts map[string]string
}

func (f *fakeIngestor) Ingest(_ context.Context, _ *opportunity.Opportunity) ([]opportunity.SolicitationDoc, map[string]string, *agent.Result, error) {
	return f.docs, f.texts, &agent.Result{AgentName: "ingest", Status: agent.StatusSuccess, CompletedAt: time.Now()}, nil
}

// recordingReviewer captures the finalreview.Input it receives.
type recordingReviewer struct {
	mu        sync.Mutex
	gotDocs   map[string]string
	gotCalled bool
}

func (r *recordingReviewer) Review(_ context.Context, in finalreview.Input) (*agent.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gotCalled = true
	r.gotDocs = in.Documents
	return &agent.Result{AgentName: "final-review", Status: agent.StatusReadyToSubmit, CompletedAt: time.Now()}, nil
}

// TestIngestion_ThreadsDocumentTextToWriterAndReview proves that when an Ingestor
// is configured, its extracted text reaches both the Writer (at draft time) and
// the Final Review (at the separately-triggered Approve step, from the cache).
func TestIngestion_ThreadsDocumentTextToWriterAndReview(t *testing.T) {
	dir := t.TempDir()
	opps, err := store.NewJSONStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	docs, err := document.NewStore(dir)
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	docsClient, err := googledocs.NewClient(context.Background(), googledocs.Config{UseCached: true})
	if err != nil {
		t.Fatalf("docs client: %v", err)
	}
	rec := &recordingWriter{}
	rev := &recordingReviewer{}
	ing := &fakeIngestor{
		docs:  []opportunity.SolicitationDoc{{Filename: "rfp.pdf", TextObject: "gs://b/zta-1/text/rfp.pdf.txt"}},
		texts: map[string]string{"rfp.pdf": "Offerors shall provide a FedRAMP High authorization."},
	}
	svc := NewService(&Deps{
		Opportunities: opps,
		Documents:     docs,
		Outline:       outline.New(docsClient),
		Writer:        rec,
		Review:        rev,
		Profile:       &scorer.CapabilityProfile{},
		Ingest:        ing,
	})
	seedOpp(t, opps)

	// Draft pipeline: ingestion runs, Writer receives the document text.
	if err := svc.Select(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Select: %v", err)
	}
	svc.Wait()

	if len(rec.calls) == 0 {
		t.Fatal("writer was never called")
	}
	if got := rec.calls[0].documents["rfp.pdf"]; got != "Offerors shall provide a FedRAMP High authorization." {
		t.Errorf("writer did not receive ingested document text: %q", got)
	}
	// Documents were attached to the persisted opportunity.
	saved, err := opps.Get(context.Background(), "zta-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(saved.Documents) != 1 || saved.Documents[0].Filename != "rfp.pdf" {
		t.Errorf("ingested documents not attached to opportunity: %+v", saved.Documents)
	}

	// Approve: Final Review receives the same text from the cache across the gate.
	if err := svc.Approve(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	svc.Wait()

	if !rev.gotCalled {
		t.Fatal("final review was never called")
	}
	if got := rev.gotDocs["rfp.pdf"]; got != "Offerors shall provide a FedRAMP High authorization." {
		t.Errorf("final review did not receive ingested document text: %q", got)
	}
}

// TestRequestChangesThreadsNoteToWriter proves the human's change request reaches
// the Writer on a revision (tester-reported: request-changes appeared to do
// nothing because the note was recorded in history but never passed to the
// Writer, so it redrafted blind).
func TestRequestChangesThreadsNoteToWriter(t *testing.T) {
	dir := t.TempDir()
	opps, err := store.NewJSONStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	docs, err := document.NewStore(dir)
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	docsClient, err := googledocs.NewClient(context.Background(), googledocs.Config{UseCached: true})
	if err != nil {
		t.Fatalf("docs client: %v", err)
	}
	rec := &recordingWriter{}
	svc := NewService(&Deps{
		Opportunities: opps, Documents: docs,
		Outline: outline.New(docsClient), Writer: rec, Review: &recordingReviewer{},
		Profile: &scorer.CapabilityProfile{},
	})
	seedOpp(t, opps)

	if err := svc.Select(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Select: %v", err)
	}
	svc.Wait()
	initial := len(rec.calls)
	if initial == 0 {
		t.Fatal("writer not called for the initial draft")
	}
	if rec.calls[0].revisionNote != "" {
		t.Errorf("initial draft should carry no revision note, got %q", rec.calls[0].revisionNote)
	}

	const note = "Add a teaming partner for past performance at this scale."
	if err := svc.RequestChanges(context.Background(), "zta-1", note); err != nil {
		t.Fatalf("RequestChanges: %v", err)
	}
	svc.Wait()

	if len(rec.calls) <= initial {
		t.Fatal("writer was not re-run on request-changes")
	}
	last := rec.calls[len(rec.calls)-1]
	if last.revisionNote != note {
		t.Errorf("revision writer call revisionNote = %q, want %q", last.revisionNote, note)
	}
}

// TestNoIngestor_NoDocumentsThreaded confirms the pipeline is unchanged without
// an ingestor: the Writer receives nil Documents.
func TestNoIngestor_NoDocumentsThreaded(t *testing.T) {
	dir := t.TempDir()
	opps, err := store.NewJSONStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	docs, err := document.NewStore(dir)
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	docsClient, err := googledocs.NewClient(context.Background(), googledocs.Config{UseCached: true})
	if err != nil {
		t.Fatalf("docs client: %v", err)
	}
	rec := &recordingWriter{}
	svc := NewService(&Deps{
		Opportunities: opps,
		Documents:     docs,
		Outline:       outline.New(docsClient),
		Writer:        rec,
		Review:        finalreview.New(),
		Profile:       &scorer.CapabilityProfile{},
		// no Ingest
	})
	seedOpp(t, opps)

	if err := svc.Select(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Select: %v", err)
	}
	svc.Wait()

	if len(rec.calls) == 0 {
		t.Fatal("writer was never called")
	}
	if rec.calls[0].documents != nil {
		t.Errorf("expected nil Documents without an ingestor, got %v", rec.calls[0].documents)
	}
}

// TestWriterDraftsSectionBySection proves the document grows incrementally:
// one Writer run per outline section, applied as each completes, so the
// human can review the outline (and early sections) while drafting runs.
func TestWriterDraftsSectionBySection(t *testing.T) {
	dir := t.TempDir()
	opps, err := store.NewJSONStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	docs, err := document.NewStore(dir)
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	docsClient, err := googledocs.NewClient(context.Background(), googledocs.Config{UseCached: true})
	if err != nil {
		t.Fatalf("docs client: %v", err)
	}
	rec := &recordingWriter{}
	svc := NewService(&Deps{
		Opportunities: opps,
		Documents:     docs,
		Outline:       outline.New(docsClient),
		Writer:        rec,
		Review:        finalreview.New(),
		Profile:       &scorer.CapabilityProfile{},
	})
	seedOpp(t, opps)

	if err := svc.Select(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Select: %v", err)
	}
	svc.Wait()

	doc, err := svc.Document("zta-1")
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	if len(rec.calls) != len(doc.Sections) {
		t.Fatalf("writer ran %d times for %d sections — want one run per section", len(rec.calls), len(doc.Sections))
	}
	for _, c := range rec.calls {
		if c.sectionCount != 1 {
			t.Errorf("each writer run must receive a single-section outline, got %d (%q)", c.sectionCount, c.title)
		}
	}
	for _, sec := range doc.Sections {
		if !strings.Contains(sec.Body, "Drafted body for "+sec.Heading) {
			t.Errorf("section %q body not applied from its own run", sec.ID)
		}
	}
	// Incremental application means one writer revision per section.
	writerRevs := 0
	for _, r := range doc.Revisions {
		if r.Actor == "writer" {
			writerRevs++
		}
	}
	if writerRevs != len(doc.Sections) {
		t.Errorf("want %d writer revisions (one per section), got %d", len(doc.Sections), writerRevs)
	}
}

func TestGapFlagsAnchorToSections(t *testing.T) {
	svc, opps := newTestService(t)
	seedOpp(t, opps)
	if err := svc.Select(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Select: %v", err)
	}
	svc.Wait()

	// The human edit satisfies the must-have requirement but leaves a Writer
	// gap marker in place, so the unresolved gap is the ONLY issue: it alone
	// must keep the proposal from reaching ready_to_submit.
	doc, _ := svc.Document("zta-1")
	secID := doc.Sections[0].ID
	if _, err := svc.UpdateSection(context.Background(), "zta-1", secID,
		"We will use FedRAMP High authorized tooling, staffed by [GAP: number of cleared staff] engineers."); err != nil {
		t.Fatalf("UpdateSection: %v", err)
	}

	if err := svc.Approve(context.Background(), "zta-1"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	svc.Wait()

	opp, _ := opps.Get(context.Background(), "zta-1")
	if opp.ProposalStatus != StatusReviewNeedsHuman {
		t.Fatalf("ProposalStatus = %q, want %q (an unresolved gap must block ready_to_submit)",
			opp.ProposalStatus, StatusReviewNeedsHuman)
	}

	doc, _ = svc.Document("zta-1")
	var gap *document.Flag
	for i := range doc.Flags {
		if doc.Flags[i].Title == "Unresolved gap" {
			gap = &doc.Flags[i]
		}
	}
	if gap == nil {
		t.Fatalf("no \"Unresolved gap\" flag landed on the document; flags: %+v", doc.Flags)
	}
	if gap.SectionID != secID {
		t.Errorf("gap flag SectionID = %q, want %q (must anchor to the section holding the gap)", gap.SectionID, secID)
	}
	if !strings.Contains(gap.Detail, "number of cleared staff") {
		t.Errorf("gap flag Detail = %q, want the missing-fact text", gap.Detail)
	}
}

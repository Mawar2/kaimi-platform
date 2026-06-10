// Package manager implements the Zone 2 per-proposal orchestrator (KAI-M5).
//
// Given one eligible, scored Opportunity, the Manager threads it through the Zone 2
// chain in order — (Ingest) -> Outline -> Writer -> Final Review — recording each
// stage's agent.Result and persisting progress to the Store. It halts and surfaces
// clearly on any stage that fails or needs a human, and it never auto-submits: the
// best terminal state is ready_to_submit, awaiting a human.
//
// Ingestion is an optional step 0, enabled via WithIngestor: it fetches the
// solicitation attachments, attaches the resulting SolicitationDocs to the
// Opportunity, and threads their extracted text to the Writer and Final Review so
// those stages ground on the real documents rather than the SAM.gov summary alone.
// A failed or needs_human ingest halts the chain — the Manager does not draft
// against documents it could not read.
//
// Persistence note: the Store is Opportunity-centric (Save(*Opportunity)), so each
// stage's outcome is persisted by updating Opportunity.ProposalStatus and saving —
// forward-compatible with the existing schema. The full per-stage agent.Result
// trail is returned in the Outcome for callers and logs.
package manager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/finalreview"
	"github.com/Mawar2/Kaimi/internal/ingest"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
	"github.com/Mawar2/Kaimi/internal/writer"
)

// Stage names, also used as the ProposalStatus prefix persisted to the Store.
const (
	stageIngest  = "ingest"
	stageOutline = "outline"
	stageWriter  = "writer"
	stageReview  = "final-review"
)

// Ingestor fetches an opportunity's solicitation attachments, stores them, and
// returns the resulting SolicitationDocs together with a filename -> extracted
// text map for downstream grounding. The concrete *ingest.Agent satisfies this.
type Ingestor interface {
	Ingest(ctx context.Context, opp *opportunity.Opportunity) ([]opportunity.SolicitationDoc, map[string]string, *agent.Result, error)
}

// OutlineRunner produces an outline and a Result for an opportunity.
// The concrete *outline.Agent satisfies this.
type OutlineRunner interface {
	Run(ctx context.Context, opp *opportunity.Opportunity) (*outline.Outline, *agent.Result, error)
}

// WriterRunner produces a draft and a Result from a writer.Input.
// The concrete *writer.Agent satisfies this.
type WriterRunner interface {
	Run(ctx context.Context, in writer.Input) (string, *agent.Result, error)
}

// Reviewer runs the final pre-submission review.
// The concrete *finalreview.Agent satisfies this.
type Reviewer interface {
	Review(ctx context.Context, in finalreview.Input) (*agent.Result, error)
}

// Compile-time checks that the real Zone 2 agents satisfy the Manager's interfaces,
// so the Manager can be wired with the production agents.
var (
	_ Ingestor      = (*ingest.Agent)(nil)
	_ OutlineRunner = (*outline.Agent)(nil)
	_ WriterRunner  = (*writer.Agent)(nil)
	_ Reviewer      = (*finalreview.Agent)(nil)
)

// Manager threads one Opportunity through the Zone 2 chain.
type Manager struct {
	ingest  Ingestor // optional step 0; when nil, ingestion is skipped
	outline OutlineRunner
	writer  WriterRunner
	review  Reviewer
	store   store.Store
}

// New constructs a Manager from the three Zone 2 agents and a Store. Document
// ingestion (step 0) is opt-in via WithIngestor.
func New(o OutlineRunner, w WriterRunner, r Reviewer, s store.Store) *Manager {
	return &Manager{outline: o, writer: w, review: r, store: s}
}

// WithIngestor configures the Manager to run document ingestion as step 0 before
// Outline: the ingestor's SolicitationDocs are attached to the Opportunity and
// its extracted text is threaded to the Writer and Final Review. Returns the
// Manager for chaining.
func (m *Manager) WithIngestor(i Ingestor) *Manager {
	m.ingest = i
	return m
}

// Outcome is the result of running the Zone 2 chain for one opportunity.
type Outcome struct {
	// Status is the terminal status: StatusReadyToSubmit on a clean run,
	// StatusNeedsHuman when a stage needs a human, or StatusFailed on failure.
	Status agent.Status
	// Stage names the stage that produced the terminal status.
	Stage string
	// Results is the ordered per-stage agent.Result trail.
	Results []*agent.Result
	// Documents are the ingested solicitation documents (nil if ingestion is not
	// configured or the chain halts before it completes).
	Documents []opportunity.SolicitationDoc
	// Outline and Draft are intermediate artifacts (empty if the chain halts early).
	Outline *outline.Outline
	Draft   string
}

// Run threads the opportunity through Outline -> Writer -> Final Review.
//
// It halts on the first stage that returns a Go error, a failed Result, or a
// needs_human Result, persisting progress at each step. On a clean run the terminal
// status is whatever Final Review returns — ready_to_submit when the draft passes —
// and the Manager never submits.
func (m *Manager) Run(ctx context.Context, opp *opportunity.Opportunity, profile *scorer.CapabilityProfile) (*Outcome, error) {
	if m.outline == nil || m.writer == nil || m.review == nil || m.store == nil {
		return nil, fmt.Errorf("manager: outline, writer, review, and store are all required")
	}
	if opp == nil {
		return nil, fmt.Errorf("manager: opportunity must not be nil")
	}
	if profile == nil {
		return nil, fmt.Errorf("manager: capability profile must not be nil")
	}

	out := &Outcome{}

	// docText carries the filename -> extracted text produced by ingestion so the
	// Writer and Final Review can ground on the real solicitation documents. It
	// stays nil when ingestion is not configured; agents treat that as "no docs".
	var docText map[string]string

	// Stage 0: Ingest (optional). Fetch, store, and extract the solicitation
	// attachments, attach them to the Opportunity, and capture their text. A
	// failed or needs_human ingest halts the chain — we do not draft against
	// documents we could not read.
	if m.ingest != nil {
		docs, texts, res, err := m.ingest.Ingest(ctx, opp)
		opp.Documents = docs
		out.Documents = docs
		docText = texts
		if stop, e := m.after(ctx, out, stageIngest, res, err, opp); stop {
			return out, e
		}
	}

	// Stage 1: Outline. Capture the artifact before evaluating the halt so a human
	// reviewing a halted run still sees whatever was produced.
	ol, res, err := m.outline.Run(ctx, opp)
	out.Outline = ol
	if stop, e := m.after(ctx, out, stageOutline, res, err, opp); stop {
		return out, e
	}

	// Stage 2: Writer.
	draft, res, err := m.writer.Run(ctx, writer.Input{Opportunity: opp, Outline: ol, Profile: profile, Documents: docText})
	out.Draft = draft
	if stop, e := m.after(ctx, out, stageWriter, res, err, opp); stop {
		return out, e
	}

	// Stage 3: Final Review (terminal). A clean review returns ready_to_submit; the
	// Manager surfaces that and stops — submission is always a human action.
	res, err = m.review.Review(ctx, finalreview.Input{Draft: draft, Opportunity: opp, Documents: docText})
	if stop, e := m.after(ctx, out, stageReview, res, err, opp); stop {
		return out, e
	}
	out.Status = res.Status
	out.Stage = stageReview
	return out, nil
}

// after records the stage result and persists progress. It returns stop=true when
// the chain must halt: a Go error from the agent, a Store persistence failure, or a
// result Status of failed or needs_human. A successful intermediate result returns
// stop=false so the chain continues.
func (m *Manager) after(ctx context.Context, out *Outcome, stage string, res *agent.Result, err error, opp *opportunity.Opportunity) (bool, error) {
	// A Go error from the agent is terminal.
	if err != nil {
		if res != nil {
			out.Results = append(out.Results, res)
		}
		return m.haltFailed(ctx, out, stage, opp, fmt.Errorf("manager: %s stage error: %w", stage, err))
	}
	// A nil result with no error is a contract violation; treat it as terminal
	// rather than dereferencing it below.
	if res == nil {
		return m.haltFailed(ctx, out, stage, opp, fmt.Errorf("manager: %s stage returned a nil result", stage))
	}

	out.Results = append(out.Results, res)
	if perr := m.persist(ctx, opp, stage, res.Status); perr != nil {
		return m.haltFailed(ctx, out, stage, opp, fmt.Errorf("manager: persist %s stage: %w", stage, perr))
	}

	switch res.Status {
	case agent.StatusSuccess, agent.StatusReadyToSubmit:
		// Healthy: continue the chain (the review stage's ready_to_submit becomes
		// the terminal status in Run).
		return false, nil
	case agent.StatusFailed, agent.StatusNeedsHuman:
		out.Status = res.Status
		out.Stage = stage
		return true, nil
	default:
		// Unknown/uninitialized status: halt rather than continue with bad data.
		out.Status = agent.StatusFailed
		out.Stage = stage
		return true, fmt.Errorf("manager: %s stage returned unexpected status %q", stage, res.Status)
	}
}

// haltFailed marks the outcome failed at a stage, best-effort persists the failed
// status (joining any persistence error so it is never swallowed), and returns the
// terminal error.
func (m *Manager) haltFailed(ctx context.Context, out *Outcome, stage string, opp *opportunity.Opportunity, cause error) (bool, error) {
	out.Status = agent.StatusFailed
	out.Stage = stage
	perr := m.persist(ctx, opp, stage, agent.StatusFailed)
	return true, errors.Join(cause, perr)
}

// persist records pipeline progress to the Store via the Opportunity's
// ProposalStatus (existing schema, forward-compatible).
func (m *Manager) persist(ctx context.Context, opp *opportunity.Opportunity, stage string, status agent.Status) error {
	opp.ProposalStatus = fmt.Sprintf("%s:%s", stage, status)
	opp.UpdatedAt = time.Now().UTC()
	if err := m.store.Save(ctx, opp); err != nil {
		return fmt.Errorf("save opportunity %s: %w", opp.ID, err)
	}
	return nil
}

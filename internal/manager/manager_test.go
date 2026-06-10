package manager

import (
	"context"
	"errors"
	"testing"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/finalreview"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
	"github.com/Mawar2/Kaimi/internal/writer"
)

// --- mocks ---

type mockIngest struct {
	docs   []opportunity.SolicitationDoc
	texts  map[string]string
	res    *agent.Result
	err    error
	called bool
}

func (m *mockIngest) Ingest(_ context.Context, _ *opportunity.Opportunity) ([]opportunity.SolicitationDoc, map[string]string, *agent.Result, error) {
	m.called = true
	return m.docs, m.texts, m.res, m.err
}

type mockOutline struct {
	res    *agent.Result
	err    error
	called bool
}

func (m *mockOutline) Run(_ context.Context, _ *opportunity.Opportunity) (*outline.Outline, *agent.Result, error) {
	m.called = true
	return &outline.Outline{OpportunityID: "opp-1", Sections: []outline.Section{{ID: "s1", Title: "Approach"}}}, m.res, m.err
}

type mockWriter struct {
	res      *agent.Result
	err      error
	called   bool
	gotInput writer.Input
}

func (m *mockWriter) Run(_ context.Context, in writer.Input) (string, *agent.Result, error) {
	m.called = true
	m.gotInput = in
	return "draft text", m.res, m.err
}

type mockReview struct {
	res      *agent.Result
	err      error
	called   bool
	gotInput finalreview.Input
}

func (m *mockReview) Review(_ context.Context, in finalreview.Input) (*agent.Result, error) {
	m.called = true
	m.gotInput = in
	return m.res, m.err
}

func ok(name string) *agent.Result {
	return &agent.Result{AgentName: name, Status: agent.StatusSuccess}
}
func ready() *agent.Result {
	return &agent.Result{AgentName: "final-review", Status: agent.StatusReadyToSubmit}
}
func needsHuman() *agent.Result {
	return &agent.Result{AgentName: "final-review", Status: agent.StatusNeedsHuman}
}
func failedRes(n string) *agent.Result {
	return &agent.Result{AgentName: n, Status: agent.StatusFailed}
}

func testOpp() *opportunity.Opportunity {
	return &opportunity.Opportunity{ID: "opp-1", Title: "Cloud project"}
}

func testProfile() *scorer.CapabilityProfile {
	return &scorer.CapabilityProfile{PrimaryNAICS: []string{"541512"}}
}

func newStore(t *testing.T) store.Store {
	t.Helper()
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}
	return s
}

// --- tests ---

func TestRun_HappyPath_ReadyToSubmit(t *testing.T) {
	st := newStore(t)
	rev := &mockReview{res: ready()}
	m := New(&mockOutline{res: ok("outline")}, &mockWriter{res: ok("writer")}, rev, st)

	opp := testOpp()
	out, err := m.Run(context.Background(), opp, testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %s, want ready_to_submit", out.Status)
	}
	if len(out.Results) != 3 {
		t.Errorf("Results len = %d, want 3 (one per stage)", len(out.Results))
	}
	if out.Draft == "" || out.Outline == nil {
		t.Error("expected outline and draft artifacts on a clean run")
	}
	// Each stage was persisted; the Store reflects the terminal stage.
	saved, err := st.Get(context.Background(), opp.ID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if saved.ProposalStatus != "final-review:ready_to_submit" {
		t.Errorf("persisted ProposalStatus = %q, want final-review:ready_to_submit", saved.ProposalStatus)
	}
}

func TestRun_NeverAutoSubmits(t *testing.T) {
	st := newStore(t)
	m := New(&mockOutline{res: ok("outline")}, &mockWriter{res: ok("writer")}, &mockReview{res: ready()}, st)
	out, err := m.Run(context.Background(), testOpp(), testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Terminal success is ready_to_submit and nothing stronger — never "submitted"/complete.
	if out.Status != agent.StatusReadyToSubmit {
		t.Errorf("terminal Status = %s, want ready_to_submit (never auto-submit)", out.Status)
	}
}

func TestRun_MidChainFailed_Halts(t *testing.T) {
	st := newStore(t)
	rev := &mockReview{res: ready()}
	m := New(&mockOutline{res: ok("outline")}, &mockWriter{res: failedRes("writer")}, rev, st)

	out, err := m.Run(context.Background(), testOpp(), testProfile())
	if err != nil {
		t.Fatalf("a failed Result should halt without a Go error: %v", err)
	}
	if out.Status != agent.StatusFailed {
		t.Errorf("Status = %s, want failed", out.Status)
	}
	if out.Stage != stageWriter {
		t.Errorf("Stage = %s, want writer", out.Stage)
	}
	if len(out.Results) != 2 {
		t.Errorf("Results len = %d, want 2 (outline + writer)", len(out.Results))
	}
	if rev.called {
		t.Error("Final Review must not run after the Writer fails")
	}
}

func TestRun_NeedsHuman_Halts(t *testing.T) {
	st := newStore(t)
	m := New(&mockOutline{res: ok("outline")}, &mockWriter{res: ok("writer")}, &mockReview{res: needsHuman()}, st)

	out, err := m.Run(context.Background(), testOpp(), testProfile())
	if err != nil {
		t.Fatalf("needs_human is a clean halt, not a Go error: %v", err)
	}
	if out.Status != agent.StatusNeedsHuman {
		t.Errorf("Status = %s, want needs_human", out.Status)
	}
	if out.Stage != stageReview {
		t.Errorf("Stage = %s, want final-review", out.Stage)
	}
}

func TestRun_OutlineError_PropagatesFailed(t *testing.T) {
	st := newStore(t)
	w := &mockWriter{res: ok("writer")}
	m := New(&mockOutline{err: errors.New("boom")}, w, &mockReview{res: ready()}, st)

	out, err := m.Run(context.Background(), testOpp(), testProfile())
	if err == nil {
		t.Error("expected a Go error to propagate from the outline stage")
	}
	if out.Status != agent.StatusFailed {
		t.Errorf("Status = %s, want failed", out.Status)
	}
	if w.called {
		t.Error("Writer must not run after the Outline stage errors")
	}
}

func TestRun_MissingArgs_Error(t *testing.T) {
	st := newStore(t)
	full := New(&mockOutline{res: ok("o")}, &mockWriter{res: ok("w")}, &mockReview{res: ready()}, st)
	if _, err := full.Run(context.Background(), nil, testProfile()); err == nil {
		t.Error("expected error for nil opportunity")
	}
	if _, err := full.Run(context.Background(), testOpp(), nil); err == nil {
		t.Error("expected error for nil profile")
	}
	bare := New(nil, nil, nil, nil)
	if _, err := bare.Run(context.Background(), testOpp(), testProfile()); err == nil {
		t.Error("expected error for missing dependencies")
	}
}

func TestRun_NilResult_Failed(t *testing.T) {
	st := newStore(t)
	// Outline returns no error but a nil Result — a contract violation.
	m := New(&mockOutline{res: nil, err: nil}, &mockWriter{res: ok("writer")}, &mockReview{res: ready()}, st)

	out, err := m.Run(context.Background(), testOpp(), testProfile())
	if err == nil {
		t.Error("expected an error when a stage returns a nil result")
	}
	if out.Status != agent.StatusFailed {
		t.Errorf("Status = %s, want failed", out.Status)
	}
}

func TestRun_UnexpectedStatus_Halts(t *testing.T) {
	st := newStore(t)
	bogus := &agent.Result{AgentName: "outline", Status: agent.Status("weird")}
	m := New(&mockOutline{res: bogus}, &mockWriter{res: ok("writer")}, &mockReview{res: ready()}, st)

	out, err := m.Run(context.Background(), testOpp(), testProfile())
	if err == nil {
		t.Error("expected an error for an unexpected stage status")
	}
	if out.Status != agent.StatusFailed {
		t.Errorf("Status = %s, want failed", out.Status)
	}
}

// --- ingestion (step 0) tests ---

func ingestDocs() []opportunity.SolicitationDoc {
	return []opportunity.SolicitationDoc{{
		Filename:   "rfp.pdf",
		RawObject:  "gs://b/opp-1/raw/rfp.pdf",
		TextObject: "gs://b/opp-1/text/rfp.pdf.txt",
	}}
}

func TestRun_WithIngestor_ThreadsDocumentTextDownstream(t *testing.T) {
	st := newStore(t)
	ing := &mockIngest{
		docs:  ingestDocs(),
		texts: map[string]string{"rfp.pdf": "Section L instructions"},
		res:   ok("ingest"),
	}
	w := &mockWriter{res: ok("writer")}
	rev := &mockReview{res: ready()}
	m := New(&mockOutline{res: ok("outline")}, w, rev, st).WithIngestor(ing)

	opp := testOpp()
	out, err := m.Run(context.Background(), opp, testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ing.called {
		t.Fatal("ingestor was not called")
	}
	if out.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %s, want ready_to_submit", out.Status)
	}
	// Ingest adds a fourth stage to the trail.
	if len(out.Results) != 4 {
		t.Errorf("Results len = %d, want 4 (ingest+outline+writer+review)", len(out.Results))
	}
	// Documents are attached to the Opportunity and surfaced on the Outcome.
	if len(opp.Documents) != 1 || len(out.Documents) != 1 {
		t.Errorf("documents not attached: opp=%d out=%d, want 1/1", len(opp.Documents), len(out.Documents))
	}
	// The extracted text is threaded to both downstream agents.
	if got := w.gotInput.Documents["rfp.pdf"]; got != "Section L instructions" {
		t.Errorf("writer did not receive document text: %q", got)
	}
	if got := rev.gotInput.Documents["rfp.pdf"]; got != "Section L instructions" {
		t.Errorf("final review did not receive document text: %q", got)
	}
}

func TestRun_IngestNeedsHuman_HaltsBeforeOutline(t *testing.T) {
	st := newStore(t)
	ing := &mockIngest{
		docs: ingestDocs(),
		res:  &agent.Result{AgentName: "ingest", Status: agent.StatusNeedsHuman},
	}
	o := &mockOutline{res: ok("outline")}
	m := New(o, &mockWriter{res: ok("writer")}, &mockReview{res: ready()}, st).WithIngestor(ing)

	opp := testOpp()
	out, err := m.Run(context.Background(), opp, testProfile())
	if err != nil {
		t.Fatalf("needs_human is a clean halt, not a Go error: %v", err)
	}
	if out.Status != agent.StatusNeedsHuman {
		t.Errorf("Status = %s, want needs_human", out.Status)
	}
	if out.Stage != stageIngest {
		t.Errorf("Stage = %s, want ingest", out.Stage)
	}
	if o.called {
		t.Error("Outline must not run after ingestion needs a human")
	}
	// Documents fetched so far are still attached for the human to review.
	if len(opp.Documents) != 1 {
		t.Errorf("documents should be attached even on halt: got %d", len(opp.Documents))
	}
	saved, err := st.Get(context.Background(), opp.ID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if saved.ProposalStatus != "ingest:needs_human" {
		t.Errorf("persisted ProposalStatus = %q, want ingest:needs_human", saved.ProposalStatus)
	}
}

func TestRun_IngestFailed_Halts(t *testing.T) {
	st := newStore(t)
	ing := &mockIngest{res: failedRes("ingest")}
	o := &mockOutline{res: ok("outline")}
	m := New(o, &mockWriter{res: ok("writer")}, &mockReview{res: ready()}, st).WithIngestor(ing)

	out, err := m.Run(context.Background(), testOpp(), testProfile())
	if err != nil {
		t.Fatalf("a failed Result should halt without a Go error: %v", err)
	}
	if out.Status != agent.StatusFailed || out.Stage != stageIngest {
		t.Errorf("got %s/%s, want failed/ingest", out.Status, out.Stage)
	}
	if o.called {
		t.Error("Outline must not run after ingestion fails")
	}
}

func TestRun_NoIngestor_SkipsStep0(t *testing.T) {
	st := newStore(t)
	w := &mockWriter{res: ok("writer")}
	m := New(&mockOutline{res: ok("outline")}, w, &mockReview{res: ready()}, st)

	opp := testOpp()
	out, err := m.Run(context.Background(), opp, testProfile())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Without an ingestor the chain is the original three stages.
	if len(out.Results) != 3 {
		t.Errorf("Results len = %d, want 3 (no ingest stage)", len(out.Results))
	}
	if len(out.Documents) != 0 {
		t.Errorf("expected no documents without an ingestor, got %d", len(out.Documents))
	}
	if w.gotInput.Documents != nil {
		t.Errorf("writer should receive nil Documents without an ingestor, got %v", w.gotInput.Documents)
	}
}

// failingStore fails every Save, to prove a persistence failure halts the chain.
type failingStore struct{ store.Store }

func (failingStore) Save(_ context.Context, _ *opportunity.Opportunity) error {
	return errors.New("disk full")
}

func TestRun_PersistFailure_Halts(t *testing.T) {
	rev := &mockReview{res: ready()}
	m := New(&mockOutline{res: ok("outline")}, &mockWriter{res: ok("writer")}, rev, failingStore{})

	out, err := m.Run(context.Background(), testOpp(), testProfile())
	if err == nil {
		t.Error("expected an error when persistence fails")
	}
	if out.Status != agent.StatusFailed {
		t.Errorf("Status = %s, want failed on persistence failure", out.Status)
	}
	if out.Stage != stageOutline {
		t.Errorf("Stage = %s, want outline (first persist fails)", out.Stage)
	}
}

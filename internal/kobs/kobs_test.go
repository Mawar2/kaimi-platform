package kobs

import (
	"context"
	"testing"

	"github.com/Mawar2/kaimi-telemetry/emit"
	"github.com/Mawar2/kaimi-telemetry/event"

	"github.com/Mawar2/Kaimi/internal/opportunity"
)

func TestTenantIDPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		opp       *opportunity.Opportunity
		cfgTenant string
		want      string
	}{
		{
			name:      "record wins over config",
			opp:       &opportunity.Opportunity{TenantID: "acme"},
			cfgTenant: "default-tenant",
			want:      "acme",
		},
		{
			name:      "config used when record empty",
			opp:       &opportunity.Opportunity{TenantID: ""},
			cfgTenant: "default-tenant",
			want:      "default-tenant",
		},
		{
			name:      "empty when neither set",
			opp:       &opportunity.Opportunity{},
			cfgTenant: "",
			want:      "",
		},
		{
			name:      "nil record falls back to config",
			opp:       nil,
			cfgTenant: "default-tenant",
			want:      "default-tenant",
		},
		{
			name:      "nil record and empty config is empty",
			opp:       nil,
			cfgTenant: "",
			want:      "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := TenantID(tc.opp, tc.cfgTenant); got != tc.want {
				t.Errorf("TenantID() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEmitUninitializedIsNoOp asserts that emitting before Init never panics and
// simply drops the event. This is the property that makes instrumentation safe to
// add anywhere, including in tests and binaries that never wire telemetry.
func TestEmitUninitializedIsNoOp(t *testing.T) {
	// Ensure no emitter is installed for this test, restoring any prior handle
	// afterward so test ordering cannot leak state.
	prev := handle.Swap(nil)
	t.Cleanup(func() { handle.Store(prev) })

	// Must not panic.
	Emit(LLMEvent("gemini.generate", "acme", LLMModel("gemini-2.5-pro")))
}

// recordingSink captures delivered events so a test can confirm Emit reaches an
// installed emitter.
type recordingSink struct{ events []event.Event }

func (s *recordingSink) Emit(_ context.Context, ev event.Event) error { //nolint:gocritic // Event passed by value to satisfy sink.EventSink.Emit
	s.events = append(s.events, ev)
	return nil
}

func (s *recordingSink) Flush(_ context.Context) error { return nil }
func (s *recordingSink) Close() error                  { return nil }

func TestEmitReachesInstalledEmitter(t *testing.T) {
	sink := &recordingSink{}
	// Single worker keeps delivery ordering deterministic for the test.
	em := emit.New(sink, emit.Config{Workers: 1})

	prev := handle.Swap(em)
	t.Cleanup(func() {
		handle.Store(prev)
		if err := em.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	Init(em)

	Emit(ProposalEvent("proposal.stage.completed", "acme",
		ProposalStage("writer"),
		ProposalStatus("success"),
	))

	if err := em.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if len(sink.events) != 1 {
		t.Fatalf("delivered %d events, want 1", len(sink.events))
	}
	got := sink.events[0]
	if got.Category != event.CategoryProposal {
		t.Errorf("Category = %q, want %q", got.Category, event.CategoryProposal)
	}
	if got.TenantID != "acme" {
		t.Errorf("TenantID = %q, want %q", got.TenantID, "acme")
	}
}

// TestContentClassification guards the redaction-class contract: prompt/response
// helpers produce content-class attributes (kept in-deployment) while metadata
// helpers produce usage-class attributes (forwardable).
func TestContentClassification(t *testing.T) {
	if got := LLMPrompt("secret prompt").Class; got != event.ClassContent {
		t.Errorf("LLMPrompt class = %d, want ClassContent(%d)", got, event.ClassContent)
	}
	if got := LLMResponse("secret response").Class; got != event.ClassContent {
		t.Errorf("LLMResponse class = %d, want ClassContent(%d)", got, event.ClassContent)
	}
	if got := LLMModel("gemini-2.5-pro").Class; got != event.ClassUsage {
		t.Errorf("LLMModel class = %d, want ClassUsage(%d)", got, event.ClassUsage)
	}
	if got := ProposalStatus("success").Class; got != event.ClassUsage {
		t.Errorf("ProposalStatus class = %d, want ClassUsage(%d)", got, event.ClassUsage)
	}
	// Section titles can echo solicitation wording, so they are content-class.
	if got := ProposalSection("Technical Approach").Class; got != event.ClassContent {
		t.Errorf("ProposalSection class = %d, want ClassContent(%d)", got, event.ClassContent)
	}
	if got := ProposalErrorOf(errSentinel).Class; got != event.ClassContent {
		t.Errorf("ProposalErrorOf class = %d, want ClassContent(%d)", got, event.ClassContent)
	}
	// Revision is a usage-class label.
	if got := ProposalRevision(true).Class; got != event.ClassUsage {
		t.Errorf("ProposalRevision class = %d, want ClassUsage(%d)", got, event.ClassUsage)
	}
}

// errSentinel is a fixed error for classification/value assertions.
var errSentinel = errSentinelType("boom")

type errSentinelType string

func (e errSentinelType) Error() string { return string(e) }

// TestProposalErrorOfNilIsEmpty confirms ProposalErrorOf tolerates a nil error,
// which lets a failure branch emit unconditionally as a single statement.
func TestProposalErrorOfNilIsEmpty(t *testing.T) {
	if got := ProposalErrorOf(nil).Value; got != "" {
		t.Errorf("ProposalErrorOf(nil).Value = %v, want empty string", got)
	}
	if got := ProposalErrorOf(errSentinel).Value; got != "boom" {
		t.Errorf("ProposalErrorOf(err).Value = %v, want %q", got, "boom")
	}
}

// TestEmitProposalEnvelope asserts EmitProposal stamps the trace, span, agent
// Actor, and duration onto the event exactly as the proposal lifecycle needs.
func TestEmitProposalEnvelope(t *testing.T) {
	capture, restore := NewCapture()
	defer restore()

	EmitProposal("proposal.outline.completed", "acme", "zta-1", "outline", AgentOutline, 1500,
		ProposalID("zta-1"), ProposalStage("outline"), ProposalStatus("success"))

	events := capture.Drain()
	if len(events) != 1 {
		t.Fatalf("captured %d events, want 1", len(events))
	}
	got := events[0]
	if got.Category != event.CategoryProposal {
		t.Errorf("Category = %q, want %q", got.Category, event.CategoryProposal)
	}
	if got.Name != "proposal.outline.completed" {
		t.Errorf("Name = %q, want proposal.outline.completed", got.Name)
	}
	if got.TenantID != "acme" {
		t.Errorf("TenantID = %q, want acme", got.TenantID)
	}
	if got.TraceID != "zta-1" {
		t.Errorf("TraceID = %q, want zta-1 (the opportunity ID, set explicitly)", got.TraceID)
	}
	if got.SpanID != "outline" {
		t.Errorf("SpanID = %q, want outline", got.SpanID)
	}
	if got.Actor.Name != AgentOutline || got.Actor.Kind != "agent" {
		t.Errorf("Actor = %+v, want agent %q", got.Actor, AgentOutline)
	}
	if got.DurationMS != 1500 {
		t.Errorf("DurationMS = %d, want 1500", got.DurationMS)
	}
}

// TestEmitProposalPointEvent confirms a point event (no span, no agent, no
// duration) omits those fields.
func TestEmitProposalPointEvent(t *testing.T) {
	capture, restore := NewCapture()
	defer restore()

	EmitProposal("proposal.selected", "acme", "zta-1", "", "", 0, ProposalID("zta-1"))

	events := capture.Drain()
	if len(events) != 1 {
		t.Fatalf("captured %d events, want 1", len(events))
	}
	got := events[0]
	if got.SpanID != "" {
		t.Errorf("SpanID = %q, want empty for a point event", got.SpanID)
	}
	if got.Actor != (event.Actor{}) {
		t.Errorf("Actor = %+v, want zero for a non-agent event", got.Actor)
	}
	if got.DurationMS != 0 {
		t.Errorf("DurationMS = %d, want 0 for a point event", got.DurationMS)
	}
}

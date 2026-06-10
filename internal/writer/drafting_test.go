package writer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/scorer"
)

// mockGenerator records the system instructions and prompts it receives and
// returns canned output or an error.
type mockGenerator struct {
	systems []string
	prompts []string
	out     string
	err     error
}

func (m *mockGenerator) GenerateSection(_ context.Context, systemInstruction, prompt string) (string, error) {
	m.systems = append(m.systems, systemInstruction)
	m.prompts = append(m.prompts, prompt)
	if m.err != nil {
		return "", m.err
	}
	return m.out, nil
}

func groundedInput() Input {
	in := newValidInput() // opportunity + outline with 3 sections
	in.Opportunity.Agency = "Department of Homeland Security"
	in.Opportunity.NAICSCode = "541512"
	in.Outline.FormattingRules = &outline.FormattingRules{
		PageLimit: &outline.FormattingRule{Value: "25 pages", Specified: true},
	}
	in.Profile = &scorer.CapabilityProfile{
		PrimaryNAICS:    []string{"541512"},
		CompetencyTags:  []string{"cloud migration", "Zero Trust"},
		PastPerformance: []string{"DHS CDM program", "VA cloud modernization"},
		SDBStatus:       true,
	}
	return in
}

func TestRun_WithGenerator_GroundedDraft(t *testing.T) {
	gen := &mockGenerator{out: "Grounded section prose."}
	a := NewWithGenerator(gen)
	in := groundedInput()

	draft, res, err := a.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != agent.StatusSuccess {
		t.Errorf("Status = %s, want success", res.Status)
	}
	if res.Flags["stub"] != "false" {
		t.Errorf("Flags[stub] = %q, want false in generation mode", res.Flags["stub"])
	}
	if got := res.Flags["section_count"]; got != "3" {
		t.Errorf("Flags[section_count] = %q, want 3", got)
	}
	// One generator call per section, and the generated prose must appear in the draft.
	if len(gen.prompts) != len(in.Outline.Sections) {
		t.Errorf("generator called %d times, want %d", len(gen.prompts), len(in.Outline.Sections))
	}
	if !strings.Contains(draft, "Grounded section prose.") {
		t.Error("draft does not contain the generated section prose")
	}
	for _, s := range in.Outline.Sections {
		if !strings.Contains(draft, s.Title) {
			t.Errorf("draft missing section heading %q", s.Title)
		}
	}
}

func TestRun_WithGenerator_PromptIsGroundedAndAntiFabrication(t *testing.T) {
	gen := &mockGenerator{out: "ok"}
	a := NewWithGenerator(gen)
	in := groundedInput()

	if _, _, err := a.Run(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gen.prompts) == 0 {
		t.Fatal("generator was never called")
	}
	sys := gen.systems[0]
	p := gen.prompts[0]

	// Anti-fabrication directive must be present in the system instruction.
	if !strings.Contains(sys, "Do NOT invent past performance") {
		t.Error("system instruction is missing the anti-fabrication directive")
	}
	// Gap-flagging instruction must be present in the system instruction.
	if !strings.Contains(sys, gapMarker) {
		t.Errorf("system instruction is missing the gap marker %q", gapMarker)
	}
	// Real company facts must be injected into the user prompt (grounding), not
	// left to the model to guess.
	if !strings.Contains(p, "DHS CDM program") {
		t.Error("prompt does not include the profile's past performance facts")
	}
	if !strings.Contains(p, "cloud migration") {
		t.Error("prompt does not include the profile's competency facts")
	}
}

func TestRun_WithGenerator_GroundsOnSolicitationDocuments(t *testing.T) {
	gen := &mockGenerator{out: "ok"}
	a := NewWithGenerator(gen)
	in := groundedInput()
	in.Documents = map[string]string{
		"RFP_Section_L.pdf": "Offerors shall submit a 10-page technical volume.",
	}

	if _, _, err := a.Run(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gen.prompts) == 0 {
		t.Fatal("generator was never called")
	}
	p := gen.prompts[0]
	if !strings.Contains(p, "RFP_Section_L.pdf") {
		t.Error("prompt does not reference the solicitation document filename")
	}
	if !strings.Contains(p, "Offerors shall submit a 10-page technical volume.") {
		t.Error("prompt does not include the extracted solicitation document text")
	}
}

func TestRun_WithGenerator_NoDocuments_OmitsDocumentSection(t *testing.T) {
	gen := &mockGenerator{out: "ok"}
	a := NewWithGenerator(gen)
	in := groundedInput() // no Documents set

	if _, _, err := a.Run(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(gen.prompts[0], "Solicitation documents") {
		t.Error("prompt should not include a documents section when none were ingested")
	}
}

func TestRun_WithGenerator_WhitespaceSection_Failed(t *testing.T) {
	gen := &mockGenerator{out: "   \n\t  "} // model returns only whitespace
	a := NewWithGenerator(gen)

	draft, res, err := a.Run(context.Background(), groundedInput())
	if err == nil {
		t.Error("expected error when a section comes back empty/whitespace")
	}
	if draft != "" {
		t.Errorf("expected empty draft (no silent blank section), got %q", draft)
	}
	if res == nil || res.Status != agent.StatusFailed {
		t.Errorf("expected failed result, got %+v", res)
	}
}

func TestRun_WithGenerator_NoPastPerformance_NotFabricated(t *testing.T) {
	gen := &mockGenerator{out: "ok"}
	a := NewWithGenerator(gen)
	in := groundedInput()
	in.Profile.PastPerformance = nil // no past performance available

	if _, _, err := a.Run(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := gen.prompts[0]
	// The prompt must explicitly say none is provided rather than omit the line,
	// so the model cannot quietly invent past performance.
	if !strings.Contains(p, "Past performance: (none provided)") {
		t.Errorf("prompt should mark past performance as none provided; got:\n%s", p)
	}
}

func TestRun_WithGenerator_NilProfile_Failed(t *testing.T) {
	gen := &mockGenerator{out: "ok"}
	a := NewWithGenerator(gen)
	in := groundedInput()
	in.Profile = nil

	draft, res, err := a.Run(context.Background(), in)
	if err == nil {
		t.Error("expected error when profile is nil in generation mode")
	}
	if draft != "" {
		t.Errorf("expected empty draft on failure, got %q", draft)
	}
	if res == nil || res.Status != agent.StatusFailed {
		t.Errorf("expected failed result, got %+v", res)
	}
	if len(gen.prompts) != 0 {
		t.Error("generator should not be called when profile is nil")
	}
}

func TestRun_WithGenerator_GenerationError_NotSilentEmpty(t *testing.T) {
	gen := &mockGenerator{err: errors.New("model unavailable")}
	a := NewWithGenerator(gen)
	in := groundedInput()

	draft, res, err := a.Run(context.Background(), in)
	if err == nil {
		t.Error("expected error when generation fails")
	}
	if draft != "" {
		t.Errorf("expected empty draft on generation failure (no silent empty draft), got %q", draft)
	}
	if res == nil || res.Status != agent.StatusFailed {
		t.Errorf("expected failed result, got %+v", res)
	}
}

func TestRun_StubMode_BackCompat(t *testing.T) {
	a := New() // no generator -> stub mode
	draft, res, err := a.Run(context.Background(), newValidInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Flags["stub"] != "true" {
		t.Errorf("Flags[stub] = %q, want true in stub mode", res.Flags["stub"])
	}
	if !strings.Contains(draft, "Stub draft") {
		t.Error("stub mode should still emit the placeholder draft")
	}
}

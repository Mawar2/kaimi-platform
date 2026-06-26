package outline

import (
	"context"
	"errors"
	"testing"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/scorer"
)

// fakePlanner is a colocated test double for SectionPlanner so the agent's
// planner wiring can be tested without a live Gemini call.
type fakePlanner struct {
	sections []Section
	err      error
	gotOpp   *opportunity.Opportunity
	gotSrc   string
}

func (f *fakePlanner) PlanSections(_ context.Context, opp *opportunity.Opportunity, _ *scorer.CapabilityProfile, source string) ([]Section, error) {
	f.gotOpp = opp
	f.gotSrc = source
	return f.sections, f.err
}

func plannerTestOpp() *opportunity.Opportunity {
	return &opportunity.Opportunity{
		ID:               "TEST-001",
		Title:            "Cyber Support Services",
		Agency:           "DISA",
		NAICSCode:        "541512",
		NAICSDescription: "Computer Systems Design Services",
		Description:      "The contractor shall provide cybersecurity engineering support.",
	}
}

// TestOutlineAgent_UsesInjectedPlanner verifies NewWithPlanner routes section
// generation through the planner (not the deterministic rules) and saves a Doc.
func TestOutlineAgent_UsesInjectedPlanner(t *testing.T) {
	planner := &fakePlanner{sections: []Section{
		{ID: "approach", Title: "Our Approach", Required: true, Rationale: "from the model"},
	}}
	a := NewWithPlanner(succeedingDocsClient(), planner)

	outline, res, err := a.Run(context.Background(), plannerTestOpp(), nil, nil)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.Status != agent.StatusSuccess {
		t.Fatalf("status = %v, want success", res.Status)
	}
	if len(outline.Sections) != 1 || outline.Sections[0].Title != "Our Approach" {
		t.Errorf("sections = %+v, want the planner's single section", outline.Sections)
	}
	if planner.gotOpp == nil || planner.gotSrc == "" {
		t.Error("planner was not called with the opportunity and source text")
	}
	// Formatting extraction stays deterministic regardless of the planner.
	if outline.FormattingRules == nil {
		t.Error("FormattingRules must still be populated deterministically")
	}
}

// TestOutlineAgent_PlannerError surfaces a planner failure as a failed Result with
// no outline — never a silently empty one.
func TestOutlineAgent_PlannerError(t *testing.T) {
	a := NewWithPlanner(succeedingDocsClient(), &fakePlanner{err: errors.New("model unavailable")})

	outline, res, err := a.Run(context.Background(), plannerTestOpp(), nil, nil)
	if err == nil {
		t.Fatal("expected an error when the planner fails, got nil")
	}
	if outline != nil {
		t.Errorf("outline = %+v, want nil on planner failure", outline)
	}
	if res.Status != agent.StatusFailed {
		t.Errorf("status = %v, want failed", res.Status)
	}
}

// TestOutlineAgent_PlannerReturnsNoSections treats a section-less plan as a failure
// rather than saving an empty outline.
func TestOutlineAgent_PlannerReturnsNoSections(t *testing.T) {
	a := NewWithPlanner(succeedingDocsClient(), &fakePlanner{sections: nil})

	outline, res, err := a.Run(context.Background(), plannerTestOpp(), nil, nil)
	if err == nil {
		t.Fatal("expected an error when the planner returns zero sections, got nil")
	}
	if outline != nil || res.Status != agent.StatusFailed {
		t.Errorf("want nil outline + failed status, got outline=%v status=%v", outline, res.Status)
	}
}

// TestDeterministicPlanner_MatchesBuildSections confirms the default planner is a
// faithful wrapper of buildSections, so New(...) preserves existing behavior.
func TestDeterministicPlanner_MatchesBuildSections(t *testing.T) {
	opp := plannerTestOpp()
	source := combinedSource(opp, nil)

	got, err := deterministicPlanner{}.PlanSections(context.Background(), opp, nil, source)
	if err != nil {
		t.Fatalf("deterministic planner error: %v", err)
	}
	want := buildSections(opp, source)
	if len(got) != len(want) {
		t.Fatalf("planner returned %d sections, buildSections returned %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("section[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

package outline

import (
	"strings"
	"testing"

	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/scorer"
)

// TestBuildPlannerPrompt_IncludesCompanyProfile proves the Gemini section planner is
// told the bidding company's competencies/NAICS so it can emphasize sections the
// company can actually support, while still planning the structure from the
// solicitation. A nil profile must not panic and must not fabricate company facts (#135).
func TestBuildPlannerPrompt_IncludesCompanyProfile(t *testing.T) {
	opp := &opportunity.Opportunity{Title: "T", Agency: "A"}
	prof := &scorer.CapabilityProfile{
		Company:        "Acme Federal",
		CompetencyTags: []string{"cybersecurity", "cloud migration"},
		PrimaryNAICS:   []string{"541512"},
	}
	got := buildPlannerPrompt(opp, prof, "the solicitation body")
	if !strings.Contains(got, "cybersecurity") {
		t.Errorf("planner prompt must include company competencies:\n%s", got)
	}
	if !strings.Contains(got, "the solicitation body") {
		t.Errorf("planner prompt must still include the solicitation source")
	}

	none := buildPlannerPrompt(opp, nil, "the solicitation body")
	if !strings.Contains(none, "the solicitation body") {
		t.Errorf("nil-profile planner prompt must include the solicitation source")
	}
	if strings.Contains(none, "Acme Federal") {
		t.Errorf("nil-profile prompt must not fabricate company facts:\n%s", none)
	}
}

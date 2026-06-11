//go:build live

// Live test for the gemini-3.5-flash section planner. Excluded from the default
// `make test` run (which must never hit live models). Run explicitly:
//
//	GCP_PROJECT_ID=kaimi-seeker \
//	  go test -tags live -run TestLivePlanSections ./internal/outline
//
// Note: the Gemini 3.x family is served only from the GLOBAL Vertex endpoint, so
// this test uses location "global". Requires Application Default Credentials.
package outline

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/opportunity"
)

func TestLivePlanSections(t *testing.T) {
	project := os.Getenv("GCP_PROJECT_ID")
	if project == "" {
		t.Skip("set GCP_PROJECT_ID to run the live Gemini Flash planner test")
	}
	model := os.Getenv("OUTLINE_MODEL")
	if model == "" {
		model = "gemini-3.5-flash"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	planner, err := NewGeminiSectionPlanner(ctx, project, "global", model)
	if err != nil {
		t.Fatalf("NewGeminiSectionPlanner: %v", err)
	}

	opp := &opportunity.Opportunity{
		ID:               "LIVE-OUTLINE-001",
		Title:            "Cybersecurity Engineering Support Services",
		Agency:           "Defense Information Systems Agency",
		NAICSCode:        "541512",
		NAICSDescription: "Computer Systems Design Services",
	}
	source := "Section L: Offerors shall submit a Technical Approach, Management Plan, " +
		"and Past Performance volume. Section M: Proposals are evaluated on technical " +
		"merit, management approach, and past performance relevance."

	sections, err := planner.PlanSections(ctx, opp, source)
	if err != nil {
		t.Fatalf("PlanSections (%s): %v", model, err)
	}
	if len(sections) == 0 {
		t.Fatalf("%s returned zero sections", model)
	}
	for _, s := range sections {
		if s.ID == "" || s.Title == "" {
			t.Errorf("section with blank id/title: %+v", s)
		}
	}
	t.Logf("%s planned %d sections: %+v", model, len(sections), sections)
}

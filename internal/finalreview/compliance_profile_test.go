package finalreview

import (
	"strings"
	"testing"

	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/scorer"
)

// TestCompliancePrompts_IncludeCompanyProfile proves the LLM compliance pass is told
// the bidding company's facts so it can flag draft claims the company cannot support —
// for both the documents-backed prompt and the opportunity-only fallback prompt. A nil
// profile must not panic (#135).
func TestCompliancePrompts_IncludeCompanyProfile(t *testing.T) {
	prof := &scorer.CapabilityProfile{
		Company:         "Acme Federal",
		PastPerformance: []string{"DHS SOC modernization"},
	}
	opp := &opportunity.Opportunity{Title: "T", Agency: "A"}
	docs := map[string]string{"rfp.txt": "requirements"}

	withDocs := buildCompliancePrompt("draft", prof, docs)
	if !strings.Contains(withDocs, "Acme Federal") {
		t.Errorf("documents prompt must include the bidding company facts:\n%s", withDocs)
	}
	fallback := buildOpportunityCompliancePrompt("draft", prof, opp)
	if !strings.Contains(fallback, "DHS SOC modernization") {
		t.Errorf("fallback prompt must include the bidding company past performance:\n%s", fallback)
	}

	// A nil profile must not panic and must still produce a usable prompt.
	if got := buildCompliancePrompt("draft", nil, docs); !strings.Contains(got, "draft") {
		t.Errorf("nil-profile documents prompt must still include the draft")
	}
	if got := buildOpportunityCompliancePrompt("draft", nil, opp); !strings.Contains(got, "draft") {
		t.Errorf("nil-profile fallback prompt must still include the draft")
	}
}

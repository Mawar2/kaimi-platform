package scorer

import (
	"context"
	"testing"

	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// detProfile returns a representative scoring profile used across deterministic tests.
func detProfile() *CapabilityProfile {
	return &CapabilityProfile{
		PrimaryNAICS:        []string{"541512"},
		SecondaryNAICS:      []string{"518210"},
		CompetencyTags:      []string{"cloud migration", "Zero Trust", "DevSecOps"},
		PastPerformance:     []string{"DHS", "VA"},
		SDBStatus:           true,
		QualifyingSetAsides: []string{"SBA"},
	}
}

// TestDeterministicScorer_ImplementsScorer fails to compile if the type drifts
// from the Scorer interface â€” the whole point is an offline drop-in for GeminiScorer.
func TestDeterministicScorer_ImplementsScorer(t *testing.T) {
	var _ Scorer = NewDeterministicScorer()
}

func TestDeterministicScorer_StrongFit_BID(t *testing.T) {
	s := NewDeterministicScorer()
	opp := &opportunity.Opportunity{
		ID:           "strong-1",
		Title:        "Cloud Migration and Zero Trust modernization",
		Agency:       "DHS",
		NAICSCode:    "541512", // primary match
		SetAsideCode: "SBA",
		Description:  "DevSecOps cloud migration for DHS.",
	}
	res, err := s.Score(context.Background(), opp, detProfile())
	if err != nil {
		t.Fatalf("Score returned error: %v", err)
	}
	if res.RawScore < 60 {
		t.Errorf("RawScore = %d, want >= 60 for a strong-fit opportunity", res.RawScore)
	}
	if res.Recommendation != RecommendationBID {
		t.Errorf("Recommendation = %q, want BID for a strong-fit opportunity", res.Recommendation)
	}
	if res.Reasoning == "" {
		t.Error("Reasoning is empty; deterministic scorer must explain the score")
	}
	if res.Requirements == nil {
		t.Error("Requirements is nil; want non-nil (possibly empty) slice")
	}
}

func TestDeterministicScorer_NoFit_NoBid(t *testing.T) {
	s := NewDeterministicScorer()
	opp := &opportunity.Opportunity{
		ID:          "weak-1",
		Title:       "Janitorial services for federal building",
		Agency:      "GSA",
		NAICSCode:   "561720", // not in profile
		Description: "Custodial and cleaning services.",
	}
	res, err := s.Score(context.Background(), opp, detProfile())
	if err != nil {
		t.Fatalf("Score returned error: %v", err)
	}
	if res.RawScore >= 40 {
		t.Errorf("RawScore = %d, want < 40 for a no-fit opportunity", res.RawScore)
	}
	if res.Recommendation != RecommendationNoBid {
		t.Errorf("Recommendation = %q, want NO_BID for a no-fit opportunity", res.Recommendation)
	}
}

func TestDeterministicScorer_ScoreAlwaysInRange(t *testing.T) {
	s := NewDeterministicScorer()
	p := detProfile()
	opps := []*opportunity.Opportunity{
		{ID: "a", NAICSCode: "541512", SetAsideCode: "SBA", Title: "cloud migration Zero Trust DevSecOps", Agency: "DHS VA", Description: "DHS VA cloud migration Zero Trust DevSecOps"},
		{ID: "b", NAICSCode: "999999", Title: "", Description: ""},
		{ID: "c", NAICSCode: "518210", Title: "Zero Trust", Description: ""},
	}
	for _, opp := range opps {
		res, err := s.Score(context.Background(), opp, p)
		if err != nil {
			t.Fatalf("Score(%s) error: %v", opp.ID, err)
		}
		if res.RawScore < 0 || res.RawScore > 100 {
			t.Errorf("Score(%s) = %d, want within [0,100]", opp.ID, res.RawScore)
		}
	}
}

func TestDeterministicScorer_Deterministic(t *testing.T) {
	s := NewDeterministicScorer()
	p := detProfile()
	opp := &opportunity.Opportunity{ID: "d", NAICSCode: "541512", Title: "Zero Trust", Description: "cloud migration"}
	first, err := s.Score(context.Background(), opp, p)
	if err != nil {
		t.Fatalf("first Score error: %v", err)
	}
	second, err := s.Score(context.Background(), opp, p)
	if err != nil {
		t.Fatalf("second Score error: %v", err)
	}
	if first.RawScore != second.RawScore || first.Recommendation != second.Recommendation {
		t.Errorf("non-deterministic: first=(%d,%s) second=(%d,%s)", first.RawScore, first.Recommendation, second.RawScore, second.Recommendation)
	}
}

func TestDeterministicScorer_NilInputs_Error(t *testing.T) {
	s := NewDeterministicScorer()
	if _, err := s.Score(context.Background(), nil, detProfile()); err == nil {
		t.Error("expected error for nil opportunity, got nil")
	}
	opp := &opportunity.Opportunity{ID: "x", NAICSCode: "541512"}
	if _, err := s.Score(context.Background(), opp, nil); err == nil {
		t.Error("expected error for nil profile, got nil")
	}
}

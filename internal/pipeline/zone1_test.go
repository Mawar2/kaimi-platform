package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/profile"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
)

// mockSam is a stand-in for samgov.Client so the Zone-1 pipeline can be exercised
// without fixtures or live SAM.gov calls.
type mockSam struct {
	opps []*opportunity.Opportunity
	err  error
}

func (m *mockSam) FetchByNAICS(_ context.Context, _ []string) ([]*opportunity.Opportunity, error) {
	return m.opps, m.err
}

func (m *mockSam) FetchByID(_ context.Context, _ string) (*opportunity.Opportunity, error) {
	return nil, errors.New("not implemented")
}

// failingScorer errors on a specific opportunity ID and succeeds otherwise,
// so we can assert the pipeline counts a failure and keeps going.
type failingScorer struct {
	failID string
}

func (f *failingScorer) Score(_ context.Context, opp *opportunity.Opportunity, _ *scorer.CapabilityProfile) (*scorer.ScoreResult, error) {
	if opp.ID == f.failID {
		return nil, errors.New("synthetic scoring failure")
	}
	return &scorer.ScoreResult{RawScore: 75, Recommendation: scorer.RecommendationBID, Reasoning: "ok", Requirements: []string{}}, nil
}

func testScoringProfile() *scorer.CapabilityProfile {
	return &scorer.CapabilityProfile{
		PrimaryNAICS:   []string{"541512"},
		SecondaryNAICS: []string{"518210"},
		CompetencyTags: []string{"cloud migration", "Zero Trust"},
	}
}

// testEligibilityProfile returns a minimal profile.CapabilityProfile for unit tests.
// The NAICS codes cover the codes used by threeOpps(), and the set-aside flags reflect
// BlueMeta's actual certifications (small business, SDB; not 8A, SDVOSB, WOSB, HUBZone).
// IsEligible will therefore pass "" and "SBA" and drop "8A" — matching threeOpps expectations.
func testEligibilityProfile() *profile.CapabilityProfile {
	return &profile.CapabilityProfile{
		Company: "BlueMeta Technologies (test)",
		NAICSCodes: []profile.NAICSCode{
			{Code: "541512", Description: "Computer Systems Design Services", Tier: profile.TierPrimary},
			{Code: "518210", Description: "Computing Infrastructure Providers", Tier: profile.TierSecondary},
		},
		SetAside: profile.SetAsideStatus{
			SmallBusiness: true,
			SDB:           true,
		},
	}
}

// threeOpps returns two eligible opportunities and one set-aside (8A) opportunity
// that the eligibility gate must drop before scoring.
func threeOpps() []*opportunity.Opportunity {
	return []*opportunity.Opportunity{
		{ID: "elig-primary", Title: "Cloud Migration", Agency: "DHS", NAICSCode: "541512", SetAsideCode: "", Description: "Zero Trust cloud migration"},
		{ID: "elig-sba", Title: "Data hosting", Agency: "VA", NAICSCode: "518210", SetAsideCode: "SBA", Description: "hosting services"},
		{ID: "ineligible-8a", Title: "Reserved work", Agency: "GSA", NAICSCode: "541512", SetAsideCode: "8A", Description: "8a only"},
	}
}

func TestRunZone1_CachedFullRun_ProducesScoredOpportunities(t *testing.T) {
	st, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}
	report, err := RunZone1(context.Background(), &Zone1Deps{
		Sam:         &mockSam{opps: threeOpps()},
		Scorer:      scorer.NewDeterministicScorer(),
		Store:       st,
		Profile:     testScoringProfile(),
		Eligibility: testEligibilityProfile(),
	})
	if err != nil {
		t.Fatalf("RunZone1: %v", err)
	}

	if report.Fetched != 3 {
		t.Errorf("Fetched = %d, want 3", report.Fetched)
	}
	if report.Dropped != 1 {
		t.Errorf("Dropped = %d, want 1 (the 8A set-aside)", report.Dropped)
	}
	if report.Eligible != 2 {
		t.Errorf("Eligible = %d, want 2", report.Eligible)
	}
	if report.Scored != 2 {
		t.Errorf("Scored = %d, want 2", report.Scored)
	}
	if report.Failed != 0 {
		t.Errorf("Failed = %d, want 0", report.Failed)
	}

	// The store must hold exactly the two eligible, *scored* opportunities.
	saved, err := st.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("store.List: %v", err)
	}
	if len(saved) != 2 {
		t.Fatalf("store holds %d opportunities, want 2", len(saved))
	}
	for _, opp := range saved {
		if opp.ID == "ineligible-8a" {
			t.Errorf("ineligible opportunity %q was persisted; it should have been dropped", opp.ID)
		}
		if opp.ScoredAt == nil {
			t.Errorf("opportunity %q has nil ScoredAt; pipeline must persist scored records", opp.ID)
		}
		if opp.Recommendation == "" {
			t.Errorf("opportunity %q has empty Recommendation after scoring", opp.ID)
		}
	}
}

func TestRunZone1_ScorerError_CountsFailedAndContinues(t *testing.T) {
	st, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}
	report, err := RunZone1(context.Background(), &Zone1Deps{
		Sam:         &mockSam{opps: threeOpps()},
		Scorer:      &failingScorer{failID: "elig-sba"},
		Store:       st,
		Profile:     testScoringProfile(),
		Eligibility: testEligibilityProfile(),
	})
	if err != nil {
		t.Fatalf("RunZone1 should not abort on a single scoring failure: %v", err)
	}
	if report.Scored != 1 {
		t.Errorf("Scored = %d, want 1", report.Scored)
	}
	if report.Failed != 1 {
		t.Errorf("Failed = %d, want 1", report.Failed)
	}
	// The failure must be surfaced (not swallowed) so operators have observability.
	if len(report.Errors) != 1 {
		t.Fatalf("Errors = %v, want exactly 1 captured failure", report.Errors)
	}
	if !strings.Contains(report.Errors[0], "elig-sba") {
		t.Errorf("Errors[0] = %q, want it to reference the failing opportunity id", report.Errors[0])
	}
}

func TestRunZone1_ContextCancelled_StopsEarly(t *testing.T) {
	st, _ := store.NewJSONStore(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the run so the loop bails on the first iteration

	_, err := RunZone1(ctx, &Zone1Deps{
		Sam:         &mockSam{opps: threeOpps()},
		Scorer:      scorer.NewDeterministicScorer(),
		Store:       st,
		Profile:     testScoringProfile(),
		Eligibility: testEligibilityProfile(),
	})
	if err == nil {
		t.Error("expected a context error when ctx is cancelled, got nil")
	}
}

func TestRunZone1_FetchError_Propagates(t *testing.T) {
	st, _ := store.NewJSONStore(t.TempDir())
	_, err := RunZone1(context.Background(), &Zone1Deps{
		Sam:         &mockSam{err: errors.New("sam down")},
		Scorer:      scorer.NewDeterministicScorer(),
		Store:       st,
		Profile:     testScoringProfile(),
		Eligibility: testEligibilityProfile(),
	})
	if err == nil {
		t.Error("expected error when SAM.gov fetch fails, got nil")
	}
}

func TestRunZone1_MissingDeps_Error(t *testing.T) {
	st, _ := store.NewJSONStore(t.TempDir())
	ep := testEligibilityProfile()
	cases := map[string]Zone1Deps{
		"no sam":         {Scorer: scorer.NewDeterministicScorer(), Store: st, Profile: testScoringProfile(), Eligibility: ep},
		"no scorer":      {Sam: &mockSam{}, Store: st, Profile: testScoringProfile(), Eligibility: ep},
		"no store":       {Sam: &mockSam{}, Scorer: scorer.NewDeterministicScorer(), Profile: testScoringProfile(), Eligibility: ep},
		"no profile":     {Sam: &mockSam{}, Scorer: scorer.NewDeterministicScorer(), Store: st, Eligibility: ep},
		"no eligibility": {Sam: &mockSam{}, Scorer: scorer.NewDeterministicScorer(), Store: st, Profile: testScoringProfile()},
	}
	for name, deps := range cases {
		deps := deps
		if _, err := RunZone1(context.Background(), &deps); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

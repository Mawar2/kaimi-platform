// Package e2e holds the Kaimi full-chain integration tests (KAI-10): proof that
// Hunter (eligibility) -> Scorer -> Manager (Outline -> Writer -> Final Review)
// works end to end, not just the parts.
//
// Two layers:
//   - Contract (every commit): mocked SAM.gov + the offline deterministic scorer +
//     the skeleton agents, against fixtures. Fast and deterministic.
//   - Live (run separately): real SAM.gov + real Gemini, gated behind KAIMI_E2E.
//
// Run the contract layer with `go test ./internal/e2e`. Run the live layer with
// `KAIMI_E2E=1 SAM_API_KEY=... GCP_PROJECT_ID=... go test -run E2E_Live ./internal/e2e`.
package e2e

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/finalreview"
	"github.com/Mawar2/Kaimi/internal/manager"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/pipeline"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
	"github.com/Mawar2/Kaimi/internal/writer"
)

type mockSam struct {
	opps []*opportunity.Opportunity
}

func (m *mockSam) FetchByNAICS(_ context.Context, _ []string) ([]*opportunity.Opportunity, error) {
	return m.opps, nil
}

func (m *mockSam) FetchByID(_ context.Context, _ string) (*opportunity.Opportunity, error) {
	return nil, errors.New("not implemented")
}

func newOpp(id, naics, setAside string, deadline time.Time) *opportunity.Opportunity {
	return &opportunity.Opportunity{
		ID:               id,
		Title:            "Cloud project " + id,
		Agency:           "DHS",
		NAICSCode:        naics,
		SetAsideCode:     setAside,
		Description:      "cloud migration",
		ResponseDeadline: deadline,
	}
}

func profile() *scorer.CapabilityProfile {
	return &scorer.CapabilityProfile{
		PrimaryNAICS:   []string{"541512"},
		CompetencyTags: []string{"cloud migration"},
	}
}

// TestE2E_Contract_FullChain runs the whole chain against mocks and fixtures and
// covers a happy path, an ineligible opportunity dropped by Hunter, and a draft
// that fails Final Review.
func TestE2E_Contract_FullChain(t *testing.T) {
	future := time.Now().Add(720 * time.Hour)
	past := time.Now().Add(-24 * time.Hour)

	good := newOpp("good", "541512", "", future)
	ineligible := newOpp("bad8a", "541512", "8A", future)
	stale := newOpp("stale", "541512", "", past)

	st, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Zone 1: Hunter eligibility gate + Scorer.
	report, err := pipeline.RunZone1(ctx, &pipeline.Zone1Deps{
		Sam:     &mockSam{opps: []*opportunity.Opportunity{good, ineligible, stale}},
		Scorer:  scorer.NewDeterministicScorer(),
		Store:   st,
		Profile: profile(),
	})
	if err != nil {
		t.Fatalf("run zone 1 failed: %v", err)
	}

	if report.Dropped != 1 {
		t.Errorf("expected 1 dropped (the 8A set-aside), got %d", report.Dropped)
	}
	if report.Scored != 2 {
		t.Errorf("expected 2 scored, got %d", report.Scored)
	}
	// The ineligible opportunity must never have been persisted.
	if _, err := st.Get(ctx, "bad8a"); err == nil {
		t.Error("ineligible opportunity bad8a should not be in the store")
	}

	// Zone 2: Manager threads a scored opportunity through Outline -> Writer -> Final Review.
	m := manager.New(outline.New(), writer.New(), finalreview.New(), st)

	// Happy path: a future deadline yields a ready_to_submit proposal.
	out, err := m.Run(ctx, good, profile())
	if err != nil {
		t.Fatalf("manager run (good) failed: %v", err)
	}
	if out.Status != agent.StatusReadyToSubmit {
		t.Errorf("good: status = %v, want ready_to_submit", out.Status)
	}
	if out.Draft == "" {
		t.Error("good: expected a non-empty draft")
	}

	// A passed deadline fails Final Review and halts the chain there.
	out2, _ := m.Run(ctx, stale, profile())
	if out2.Status != agent.StatusFailed {
		t.Errorf("stale: status = %v, want failed", out2.Status)
	}
	if out2.Stage != "final-review" {
		t.Errorf("stale: stage = %s, want final-review", out2.Stage)
	}
}

func TestE2E_Live(t *testing.T) {
	if os.Getenv("KAIMI_E2E") == "" {
		t.Skip("set KAIMI_E2E=1 (with SAM_API_KEY + GCP creds) to run the live full-chain E2E")
	}
	// TODO(KAI-10): wire live samgov + GeminiScorer + writer.NewWithGenerator(gemini)
	// and assert structure/behavior (valid scored + drafted + reviewed Opportunity).
}

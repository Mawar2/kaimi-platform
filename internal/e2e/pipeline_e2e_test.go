// Package e2e holds the Kaimi full-chain integration tests (KAI-10): proof that
// Hunter (eligibility) -> Scorer -> the gated proposal service (Outline -> Writer
// -> [human gate] -> Final Review) works end to end, not just the parts. The
// Zone-2 layer drives proposal.Service — the single orchestrator the dashboard
// also uses (issue #174 retired the parallel manager.Manager).
//
// Two layers:
//   - Contract (every commit): mocked SAM.gov + the offline deterministic scorer +
//     the skeleton agents, against fixtures. Fast and deterministic.
//   - Live (run separately): real SAM.gov + real Gemini, gated behind KAIMI_E2E.
//
// Run the contract layer with `go test ./internal/e2e`. Run the live layer with:
//
//	KAIMI_E2E=1 SAM_API_KEY=<key> GCP_PROJECT_ID=<project> \
//	  go test ./internal/e2e -run TestE2E_Live -v -timeout 10m
//
// Optional live-layer env: GCP_REGION (default us-east4) and GEMINI_MODEL
// (default gemini-2.5-pro). The live layer also requires GCP Application Default
// Credentials (`gcloud auth application-default login`) for Vertex AI.
package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/finalreview"
	"github.com/Mawar2/Kaimi/internal/googledocs"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/pipeline"
	"github.com/Mawar2/Kaimi/internal/profile"
	"github.com/Mawar2/Kaimi/internal/proposal"
	"github.com/Mawar2/Kaimi/internal/samgov"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
	"github.com/Mawar2/Kaimi/internal/writer"
)

// stubDocsClient satisfies googledocs.Client for the contract layer: Outline
// gets a deterministic doc without any network call. Mirrors the colocated
// fake in internal/outline's tests (test doubles aren't exported across
// packages).
type stubDocsClient struct{}

func (stubDocsClient) CreateDoc(_ context.Context, _ googledocs.Document) (*googledocs.CreatedDoc, error) {
	return &googledocs.CreatedDoc{
		ID:  "e2e-fixture-doc-001",
		URL: "https://docs.google.com/document/d/e2e-fixture-doc-001/edit",
	}, nil
}

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

func scoringProfile() *scorer.CapabilityProfile {
	return &scorer.CapabilityProfile{
		PrimaryNAICS:   []string{"541512"},
		CompetencyTags: []string{"cloud migration"},
	}
}

// eligibilityProfile mirrors the eligibility fixture in internal/pipeline's
// tests: a small business not 8(a)-certified, so the 8A set-aside opportunity
// is dropped by the Hunter gate.
func eligibilityProfile() *profile.CapabilityProfile {
	return &profile.CapabilityProfile{
		Company: "BlueMeta Technologies (e2e)",
		NAICSCodes: []profile.NAICSCode{
			{Code: "541512", Description: "Computer Systems Design Services", Tier: profile.TierPrimary},
		},
		SetAside: profile.SetAsideStatus{
			SmallBusiness: true,
			SDB:           true,
		},
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
		Sam:         &mockSam{opps: []*opportunity.Opportunity{good, ineligible, stale}},
		Scorer:      scorer.NewDeterministicScorer(),
		Store:       st,
		Profile:     scoringProfile(),
		Eligibility: eligibilityProfile(),
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

	// Zone 2: the gated proposal service threads a scored opportunity through
	// Outline -> Writer -> [human gate] -> Final Review. (Issue #174 retired the
	// parallel manager.Manager; the dashboard and this test share this one path.)
	svc := newZone2Service(t, st, outline.New(stubDocsClient{}), writer.New())

	// Happy path: a future deadline yields a ready_to_submit proposal.
	status, draft := driveZone2(t, svc, st, good.ID)
	if status != proposal.StatusReadyToSubmit {
		t.Errorf("good: status = %q, want %q", status, proposal.StatusReadyToSubmit)
	}
	if draft == "" {
		t.Error("good: expected a non-empty draft")
	}

	// A passed deadline fails Final Review at the Approve step.
	staleStatus, _ := driveZone2(t, svc, st, stale.ID)
	if staleStatus != "final-review:failed" {
		t.Errorf("stale: status = %q, want final-review:failed", staleStatus)
	}
}

// newZone2Service builds the gated proposal service for the e2e tests with a
// fresh document store and the offline deterministic Final Review agent.
func newZone2Service(t *testing.T, opps store.Store, ol proposal.OutlineRunner, w proposal.WriterRunner) *proposal.Service {
	t.Helper()
	docs, err := document.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("document store: %v", err)
	}
	return proposal.NewService(&proposal.Deps{
		Opportunities: opps,
		Documents:     docs,
		Outline:       ol,
		Writer:        w,
		Review:        finalreview.New(),
		Profile:       scoringProfile(),
	})
}

// driveZone2 runs the gated lifecycle for one opportunity to a terminal state,
// exactly as a human would: Select drafts to the gate, then Approve runs Final
// Review. It returns the final ProposalStatus and the rendered draft (empty when
// the draft pipeline halts before the gate).
func driveZone2(t *testing.T, svc *proposal.Service, opps store.Store, oppID string) (status, draft string) {
	t.Helper()
	ctx := context.Background()

	if err := svc.Select(ctx, oppID); err != nil {
		t.Fatalf("select %s: %v", oppID, err)
	}
	svc.Wait()

	opp, err := opps.Get(ctx, oppID)
	if err != nil {
		t.Fatalf("get %s after draft: %v", oppID, err)
	}
	// The draft pipeline either paused at the gate or failed before it.
	if opp.ProposalStatus != proposal.StatusGate {
		return opp.ProposalStatus, ""
	}

	if err := svc.Approve(ctx, oppID); err != nil {
		t.Fatalf("approve %s: %v", oppID, err)
	}
	svc.Wait()

	opp, err = opps.Get(ctx, oppID)
	if err != nil {
		t.Fatalf("get %s after review: %v", oppID, err)
	}
	doc, err := svc.Document(oppID)
	if err != nil {
		t.Fatalf("document %s: %v", oppID, err)
	}
	return opp.ProposalStatus, doc.Markdown()
}

// liveFetchCap bounds how many live opportunities one TestE2E_Live run feeds
// into scoring. SAM.gov can return a month of results for a popular NAICS code;
// scoring them all would blow the test deadline and burn Gemini quota without
// proving anything the first few don't already prove.
const liveFetchCap = 5

// cappedSam wraps a live samgov.Client and truncates FetchByNAICS results to a
// fixed cap. It exists only so the live E2E run stays fast and cheap — the real
// client still performs the full live fetch underneath.
type cappedSam struct {
	inner samgov.Client
	max   int
}

func (c *cappedSam) FetchByNAICS(ctx context.Context, naicsCodes []string) ([]*opportunity.Opportunity, error) {
	opps, err := c.inner.FetchByNAICS(ctx, naicsCodes)
	if err != nil {
		return nil, fmt.Errorf("capped live fetch: %w", err)
	}
	if len(opps) > c.max {
		opps = opps[:c.max]
	}
	return opps, nil
}

func (c *cappedSam) FetchByID(ctx context.Context, noticeID string) (*opportunity.Opportunity, error) {
	return c.inner.FetchByID(ctx, noticeID)
}

// TestE2E_Live runs the full chain against the real SAM.gov API and real Gemini
// via Vertex AI: live fetch -> Hunter eligibility gate -> Gemini scoring ->
// store persistence -> Manager Zone-2 chain (Outline -> Writer -> Final Review).
//
// Gated behind KAIMI_E2E=1 so it never runs on the fast unit path. Requires
// SAM_API_KEY, GCP_PROJECT_ID, and GCP Application Default Credentials
// (gcloud auth application-default login). GCP_REGION defaults to us-east4 and
// GEMINI_MODEL defaults to gemini-2.5-pro. See the package comment for the
// exact command line.
//
// Per WORKFLOW.md, assertions check structure and behavior — a validly scored,
// drafted, and reviewed Opportunity — never exact LLM output strings. Live data
// is whatever SAM.gov has today, so "no eligible opportunities" is a skip, not
// a failure.
func TestE2E_Live(t *testing.T) {
	if os.Getenv("KAIMI_E2E") == "" {
		t.Skip("set KAIMI_E2E=1 (with SAM_API_KEY + GCP_PROJECT_ID + GCP credentials) to run the live full-chain E2E")
	}

	// KAIMI_E2E=1 is an explicit opt-in, so missing required credentials is a
	// loud misconfiguration failure rather than a silent skip.
	samKey := os.Getenv("SAM_API_KEY")
	if samKey == "" {
		t.Fatal("SAM_API_KEY is required when KAIMI_E2E=1")
	}
	project := os.Getenv("GCP_PROJECT_ID")
	if project == "" {
		t.Fatal("GCP_PROJECT_ID is required when KAIMI_E2E=1")
	}
	region := os.Getenv("GCP_REGION")
	if region == "" {
		region = "us-east4" // Kaimi's home region
	}
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-2.5-pro"
	}

	// One overall deadline for the whole chain: a live SAM.gov fetch plus several
	// Gemini calls legitimately takes minutes, but a hang should fail the test
	// rather than stall the run indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	sam, err := samgov.NewClient(samgov.Config{APIKey: samKey})
	if err != nil {
		t.Fatalf("samgov.NewClient (live): %v", err)
	}

	gemini, err := scorer.NewGeminiScorer(ctx, project, region, model)
	if err != nil {
		t.Fatalf("scorer.NewGeminiScorer: %v", err)
	}

	st, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("store.NewJSONStore: %v", err)
	}

	// Zone 1: live SAM.gov fetch -> eligibility gate -> Gemini scoring -> Store.
	// The eligibility profile's NAICS list (541512) drives the fetch, same as the
	// contract layer, and cappedSam keeps the scored batch small.
	report, err := pipeline.RunZone1(ctx, &pipeline.Zone1Deps{
		Sam:         &cappedSam{inner: sam, max: liveFetchCap},
		Scorer:      gemini,
		Store:       st,
		Profile:     scoringProfile(),
		Eligibility: eligibilityProfile(),
	})
	if err != nil {
		t.Fatalf("run zone 1 (live) failed: %v", err)
	}
	t.Logf("zone 1: fetched=%d eligible=%d dropped=%d scored=%d failed=%d",
		report.Fetched, report.Eligible, report.Dropped, report.Scored, report.Failed)

	// An empty live result set is a fact about the world, not a code failure.
	if report.Eligible == 0 {
		t.Skip("no eligible opportunities live today")
	}
	// But eligible opportunities that ALL failed to score means the scoring or
	// persistence path is broken — that is a real failure.
	if report.Scored == 0 {
		t.Fatalf("eligible opportunities found but none scored; errors: %v", report.Errors)
	}
	if len(report.SavedIDs) != report.Scored {
		t.Fatalf("scored %d opportunities but persisted %d IDs", report.Scored, len(report.SavedIDs))
	}

	// Structural validity of what Zone 1 persisted: a real score in range, with
	// reasoning and a scored-at timestamp — never an assertion on the LLM's words.
	scored, err := st.Get(ctx, report.SavedIDs[0])
	if err != nil {
		t.Fatalf("get scored opportunity %s from store: %v", report.SavedIDs[0], err)
	}
	if scored.Score < 0 || scored.Score > 1 {
		t.Errorf("scored.Score = %v, want in [0, 1]", scored.Score)
	}
	if strings.TrimSpace(scored.ScoreReasoning) == "" {
		t.Error("scored opportunity is missing ScoreReasoning")
	}
	if scored.Recommendation == "" {
		t.Error("scored opportunity is missing a Recommendation")
	}
	if scored.ScoredAt == nil {
		t.Error("scored opportunity is missing ScoredAt")
	}

	// Zone 2: Manager threads the first scored opportunity through
	// Outline -> Writer (live Gemini) -> Final Review.
	//
	// The Docs client stays cached: live Docs needs GOOGLE_DRIVE_SHARED_DRIVE_ID
	// plus Drive write credentials, which is more auth than this test's required
	// env (SAM_API_KEY + GCP_PROJECT_ID + ADC for Vertex AI) provides. Live Doc
	// creation has its own dedicated test (internal/outline, -tags=live).
	docsClient, err := googledocs.NewClient(ctx, googledocs.Config{UseCached: true})
	if err != nil {
		t.Fatalf("googledocs.NewClient (cached): %v", err)
	}

	gen, err := writer.NewGeminiGenerator(ctx, project, region, model)
	if err != nil {
		t.Fatalf("writer.NewGeminiGenerator: %v", err)
	}

	svc := newZone2Service(t, st, outline.New(docsClient), writer.NewWithGenerator(gen))

	status, draft := driveZone2(t, svc, st, scored.ID)
	t.Logf("zone 2: status=%s draftLen=%d", status, len(draft))

	// Behavior, not strings: the lifecycle must land on a terminal status. A clean
	// run reaches ready_to_submit; failed/needs_human are legitimate live outcomes
	// (e.g. Final Review flags a passed deadline on a real solicitation), so they
	// are reported, not failed.
	switch status {
	case proposal.StatusReadyToSubmit:
		if strings.TrimSpace(draft) == "" {
			t.Error("ready_to_submit outcome must include a non-empty draft")
		}
	case proposal.StatusReviewNeedsHuman, "final-review:failed", "writer:failed", "outline:failed":
		t.Logf("chain halted at status %q — a legitimate live outcome", status)
	default:
		t.Errorf("status = %q, want a terminal status (ready_to_submit, review/draft failure, or needs_human)", status)
	}

	// Progress must be persisted: ProposalStatus records the last stage outcome on
	// the stored Opportunity.
	persisted, err := st.Get(ctx, scored.ID)
	if err != nil {
		t.Fatalf("get persisted opportunity %s from store: %v", scored.ID, err)
	}
	if persisted.ProposalStatus == "" {
		t.Error("the proposal service did not persist a ProposalStatus to the store")
	}
}

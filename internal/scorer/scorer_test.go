package scorer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/capabilitymap"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/store"
)

// TestComputeSignalsWithCapabilityMap: when a capability map is wired, computeSignals adds
// capability coverage from the map matched against the (resolved) solicitation text; a nil
// map leaves coverage at zero (the legacy profile-only behavior).
func TestComputeSignalsWithCapabilityMap(t *testing.T) {
	opp := &opportunity.Opportunity{
		Title:               "Zero Trust Architecture Implementation",
		ResolvedDescription: "Cybersecurity and continuous monitoring services for DHS networks.",
	}
	profile := &CapabilityProfile{}
	m := &capabilitymap.CapabilityMap{
		CoreCompetencies: []capabilitymap.Competency{{Name: "Zero Trust Architecture"}},
		Domains:          []string{"Cybersecurity"},
		Keywords:         []string{"Continuous Monitoring"},
	}

	s := computeSignals(opp, profile, m)
	if s.CapabilityCoverage < 3 {
		t.Errorf("CapabilityCoverage = %d, want >= 3 (competency + domain + keyword)", s.CapabilityCoverage)
	}
	if !strings.Contains(strings.Join(s.CapabilityMatches, "|"), "Zero Trust Architecture") {
		t.Errorf("CapabilityMatches missing competency: %v", s.CapabilityMatches)
	}

	if s0 := computeSignals(opp, profile, nil); s0.CapabilityCoverage != 0 || s0.CapabilityMatches != nil {
		t.Errorf("nil map should yield zero coverage, got %d / %v", s0.CapabilityCoverage, s0.CapabilityMatches)
	}
}

// mockScorer is a test double for the Scorer interface.
type mockScorer struct {
	result *ScoreResult
	err    error
}

func (m *mockScorer) Score(_ context.Context, _ *opportunity.Opportunity, _ *CapabilityProfile) (*ScoreResult, error) {
	return m.result, m.err
}

// mockStore is a test double for the store.Store interface.
type mockStore struct {
	saved []*opportunity.Opportunity
	err   error // If set, Save returns this error.
}

func (m *mockStore) Save(_ context.Context, opp *opportunity.Opportunity) error {
	if m.err != nil {
		return m.err
	}
	m.saved = append(m.saved, opp)
	return nil
}

func (m *mockStore) Get(_ context.Context, _ string) (*opportunity.Opportunity, error) {
	return nil, fmt.Errorf("not implemented in test double")
}

func (m *mockStore) List(_ context.Context, _ *store.Filter) ([]*opportunity.Opportunity, error) {
	return nil, fmt.Errorf("not implemented in test double")
}

func (m *mockStore) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("not implemented in test double")
}

// testProfile returns a CapabilityProfile suitable for most tests.
func testProfile() *CapabilityProfile {
	return &CapabilityProfile{
		PrimaryNAICS:        []string{"541512", "541519"},
		SecondaryNAICS:      []string{"541511", "541513"},
		CompetencyTags:      []string{"cloud migration", "Zero Trust", "DevSecOps"},
		PastPerformance:     []string{"Department of Defense", "DHS"},
		SDBStatus:           true,
		QualifyingSetAsides: []string{"SDB", "SBA"},
	}
}

// testOpportunity returns an Opportunity suitable for most tests.
func testOpportunity() *opportunity.Opportunity {
	deadline := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	return &opportunity.Opportunity{
		ID:               "TEST-OPP-001",
		Title:            "Cloud Migration Services",
		Agency:           "Department of Defense",
		NAICSCode:        "541512",
		NAICSDescription: "Computer Systems Design Services",
		SetAsideCode:     "SBA",
		Description:      "Zero Trust cloud migration and DevSecOps support.",
		ResponseDeadline: deadline,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
}

// --- TestRecommendation_Values ---

// TestRecommendation_Values verifies the three Recommendation constants have the
// expected string values.
func TestRecommendation_Values(t *testing.T) {
	t.Run("BID", func(t *testing.T) {
		if RecommendationBID != "BID" {
			t.Errorf("RecommendationBID = %q, want %q", RecommendationBID, "BID")
		}
	})
	t.Run("NO_BID", func(t *testing.T) {
		if RecommendationNoBid != "NO_BID" {
			t.Errorf("RecommendationNoBid = %q, want %q", RecommendationNoBid, "NO_BID")
		}
	})
	t.Run("REVIEW", func(t *testing.T) {
		if RecommendationReview != "REVIEW" {
			t.Errorf("RecommendationReview = %q, want %q", RecommendationReview, "REVIEW")
		}
	})
}

// --- TestComputeSignals ---

// TestComputeSignals_PrimaryNAICSMatch verifies a match on a primary NAICS code sets
// PrimaryNAICSMatch and does not set SecondaryNAICSMatch.
func TestComputeSignals_PrimaryNAICSMatch(t *testing.T) {
	opp := testOpportunity()
	opp.NAICSCode = "541512"
	profile := testProfile()

	got := computeSignals(opp, profile, nil)

	if !got.PrimaryNAICSMatch {
		t.Error("expected PrimaryNAICSMatch true for primary code 541512")
	}
	if got.SecondaryNAICSMatch {
		t.Error("expected SecondaryNAICSMatch false when primary matched")
	}
}

// TestComputeSignals_SecondaryNAICSMatch verifies a secondary NAICS match sets
// SecondaryNAICSMatch and leaves PrimaryNAICSMatch false.
func TestComputeSignals_SecondaryNAICSMatch(t *testing.T) {
	opp := testOpportunity()
	opp.NAICSCode = "541511" // secondary, not primary
	profile := testProfile()

	got := computeSignals(opp, profile, nil)

	if got.PrimaryNAICSMatch {
		t.Error("expected PrimaryNAICSMatch false for secondary code 541511")
	}
	if !got.SecondaryNAICSMatch {
		t.Error("expected SecondaryNAICSMatch true for secondary code 541511")
	}
}

// TestComputeSignals_NoNAICSMatch verifies that an unrecognized NAICS code yields
// no NAICS match signals.
func TestComputeSignals_NoNAICSMatch(t *testing.T) {
	opp := testOpportunity()
	opp.NAICSCode = "999999" // not in primary or secondary
	profile := testProfile()

	got := computeSignals(opp, profile, nil)

	if got.PrimaryNAICSMatch || got.SecondaryNAICSMatch {
		t.Errorf("expected no NAICS match for code 999999; got primary=%v secondary=%v",
			got.PrimaryNAICSMatch, got.SecondaryNAICSMatch)
	}
}

// TestComputeSignals_TagOverlap verifies that competency tags present in the
// opportunity title and description are counted correctly.
func TestComputeSignals_TagOverlap(t *testing.T) {
	opp := testOpportunity()
	// Title has "cloud migration", description has "Zero Trust" and "DevSecOps"
	opp.Title = "Cloud Migration Services"
	opp.Description = "Zero Trust architecture and DevSecOps support required."
	profile := testProfile() // tags: cloud migration, Zero Trust, DevSecOps

	got := computeSignals(opp, profile, nil)

	if got.TagOverlapCount != 3 {
		t.Errorf("TagOverlapCount = %d, want 3", got.TagOverlapCount)
	}
}

// TestComputeSignals_NoTagOverlap verifies that when no tags match, TagOverlapCount is zero.
func TestComputeSignals_NoTagOverlap(t *testing.T) {
	opp := testOpportunity()
	opp.Title = "Bridge Construction Maintenance"
	opp.Description = "Structural inspection of federal highway bridges."
	profile := testProfile() // tags unrelated to construction

	got := computeSignals(opp, profile, nil)

	if got.TagOverlapCount != 0 {
		t.Errorf("TagOverlapCount = %d, want 0 for unrelated opportunity", got.TagOverlapCount)
	}
}

// TestComputeSignals_SDBApplies verifies that SDBApplies is true when the company
// has SDB status and the opportunity's set-aside code matches a qualifying code.
func TestComputeSignals_SDBApplies(t *testing.T) {
	opp := testOpportunity()
	opp.SetAsideCode = "SDB"
	profile := testProfile() // SDBStatus true, QualifyingSetAsides includes SDB

	got := computeSignals(opp, profile, nil)

	if !got.SDBApplies {
		t.Error("expected SDBApplies true for SDB set-aside when company has SDB status")
	}
}

// TestComputeSignals_SDBNotApplies_NoStatus verifies that SDBApplies is false when
// the company does not have SDB status, even if the code would otherwise qualify.
func TestComputeSignals_SDBNotApplies_NoStatus(t *testing.T) {
	opp := testOpportunity()
	opp.SetAsideCode = "SDB"
	profile := testProfile()
	profile.SDBStatus = false // company is not SDB

	got := computeSignals(opp, profile, nil)

	if got.SDBApplies {
		t.Error("expected SDBApplies false when SDBStatus is false")
	}
}

// TestComputeSignals_PastPerformanceOverlap verifies that past performance terms
// found in the agency name and description are counted correctly.
func TestComputeSignals_PastPerformanceOverlap(t *testing.T) {
	opp := testOpportunity()
	opp.Agency = "Department of Defense"      // matches "Department of Defense"
	opp.Description = "DHS enterprise cloud." // matches "DHS"
	profile := testProfile()                  // past performance: Department of Defense, DHS

	got := computeSignals(opp, profile, nil)

	if got.PastPerfOverlapCount != 2 {
		t.Errorf("PastPerfOverlapCount = %d, want 2", got.PastPerfOverlapCount)
	}
}

// --- TestBuildScoringPrompt ---

// TestBuildScoringPrompt_ContainsRequiredElements verifies the prompt includes the
// opportunity title, agency, NAICS code, and scoring instructions.
func TestBuildScoringPrompt_ContainsRequiredElements(t *testing.T) {
	opp := testOpportunity()
	profile := testProfile()
	signals := computeSignals(opp, profile, nil)

	prompt := buildScoringPrompt(opp, profile, signals)

	required := []string{
		opp.Title,
		opp.Agency,
		opp.NAICSCode,
		"BID",
		"NO_BID",
		"REVIEW",
		"Scoring Rubric",
	}
	for _, want := range required {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing required element %q", want)
		}
	}
}

// TestBuildScoringPrompt_ReflectsSignals verifies the prompt includes the
// pre-computed signal values so the LLM can reference them.
func TestBuildScoringPrompt_ReflectsSignals(t *testing.T) {
	opp := testOpportunity()
	profile := testProfile()
	signals := Signals{
		PrimaryNAICSMatch:    true,
		SecondaryNAICSMatch:  false,
		TagOverlapCount:      3,
		PastPerfOverlapCount: 2,
		SDBApplies:           true,
	}

	prompt := buildScoringPrompt(opp, profile, signals)

	wantPhrases := []string{
		"Primary NAICS Match: true",
		"Secondary NAICS Match: false",
		"Competency Tag Overlap: 3",
		"Past Performance Overlap: 2",
		"SDB Set-Aside Applies: true",
	}
	for _, phrase := range wantPhrases {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("prompt missing signal phrase %q", phrase)
		}
	}
}

// --- TestValidateAndConvert ---

// TestValidateAndConvert_ValidBID verifies a valid BID response is parsed correctly.
func TestValidateAndConvert_ValidBID(t *testing.T) {
	input := `{"score":75,"recommendation":"BID","reasoning":"Strong NAICS match with multiple tag overlaps.","requirements":["SECRET clearance","5 years experience"]}`

	got, err := validateAndConvert(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RawScore != 75 {
		t.Errorf("RawScore = %d, want 75", got.RawScore)
	}
	if got.Recommendation != RecommendationBID {
		t.Errorf("Recommendation = %q, want %q", got.Recommendation, RecommendationBID)
	}
	if len(got.Requirements) != 2 {
		t.Errorf("Requirements len = %d, want 2", len(got.Requirements))
	}
}

// TestValidateAndConvert_ValidNoBid verifies a valid NO_BID response is parsed correctly.
func TestValidateAndConvert_ValidNoBid(t *testing.T) {
	input := `{"score":20,"recommendation":"NO_BID","reasoning":"No NAICS match and zero tag overlap.","requirements":[]}`

	got, err := validateAndConvert(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Recommendation != RecommendationNoBid {
		t.Errorf("Recommendation = %q, want %q", got.Recommendation, RecommendationNoBid)
	}
	if got.RawScore != 20 {
		t.Errorf("RawScore = %d, want 20", got.RawScore)
	}
}

// TestValidateAndConvert_ValidReview verifies a valid REVIEW response is parsed correctly.
func TestValidateAndConvert_ValidReview(t *testing.T) {
	input := `{"score":50,"recommendation":"REVIEW","reasoning":"Secondary NAICS match but unclear scope.","requirements":null}`

	got, err := validateAndConvert(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Recommendation != RecommendationReview {
		t.Errorf("Recommendation = %q, want %q", got.Recommendation, RecommendationReview)
	}
}

// TestValidateAndConvert_BoundaryScoreZero verifies score 0 is accepted.
func TestValidateAndConvert_BoundaryScoreZero(t *testing.T) {
	input := `{"score":0,"recommendation":"NO_BID","reasoning":"Completely out of scope for this company.","requirements":[]}`

	got, err := validateAndConvert(input)

	if err != nil {
		t.Fatalf("unexpected error for boundary score 0: %v", err)
	}
	if got.RawScore != 0 {
		t.Errorf("RawScore = %d, want 0", got.RawScore)
	}
}

// TestValidateAndConvert_BoundaryScoreHundred verifies score 100 is accepted.
func TestValidateAndConvert_BoundaryScoreHundred(t *testing.T) {
	input := `{"score":100,"recommendation":"BID","reasoning":"Perfect match on all signals.","requirements":[]}`

	got, err := validateAndConvert(input)

	if err != nil {
		t.Fatalf("unexpected error for boundary score 100: %v", err)
	}
	if got.RawScore != 100 {
		t.Errorf("RawScore = %d, want 100", got.RawScore)
	}
}

// TestValidateAndConvert_ScoreOutOfRange verifies that a score outside [0, 100] returns an error.
func TestValidateAndConvert_ScoreOutOfRange(t *testing.T) {
	input := `{"score":101,"recommendation":"BID","reasoning":"Too high.","requirements":[]}`

	_, err := validateAndConvert(input)

	if err == nil {
		t.Error("expected error for score 101, got nil")
	}
}

// TestValidateAndConvert_InvalidRecommendation verifies that an unrecognized recommendation
// value returns an error.
func TestValidateAndConvert_InvalidRecommendation(t *testing.T) {
	input := `{"score":50,"recommendation":"MAYBE","reasoning":"Not sure.","requirements":[]}`

	_, err := validateAndConvert(input)

	if err == nil {
		t.Error("expected error for recommendation MAYBE, got nil")
	}
}

// TestValidateAndConvert_NilRequirementsNormalized verifies that a null requirements
// field in the JSON is normalized to an empty (non-nil) slice.
func TestValidateAndConvert_NilRequirementsNormalized(t *testing.T) {
	input := `{"score":60,"recommendation":"BID","reasoning":"Good fit.","requirements":null}`

	got, err := validateAndConvert(input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Requirements == nil {
		t.Error("expected Requirements to be normalized to empty slice, got nil")
	}
	if len(got.Requirements) != 0 {
		t.Errorf("expected empty Requirements slice, got length %d", len(got.Requirements))
	}
}

// --- TestScoreAndSave ---

// TestScoreAndSave_WriteBack verifies all scored fields are written to the opportunity
// and the store is called with the updated opportunity.
func TestScoreAndSave_WriteBack(t *testing.T) {
	opp := testOpportunity()
	profile := testProfile()
	scorer := &mockScorer{
		result: &ScoreResult{
			RawScore:       75,
			Recommendation: RecommendationBID,
			Reasoning:      "Strong NAICS and tag overlap.",
			Requirements:   []string{"SECRET clearance"},
		},
	}
	ms := &mockStore{}

	err := ScoreAndSave(context.Background(), scorer, ms, opp, profile)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Score converts 75/100 → 0.75
	if opp.Score != 0.75 {
		t.Errorf("Score = %f, want 0.75", opp.Score)
	}
	if opp.ScoreReasoning != "Strong NAICS and tag overlap." {
		t.Errorf("ScoreReasoning = %q, want %q", opp.ScoreReasoning, "Strong NAICS and tag overlap.")
	}
	if opp.Recommendation != "BID" {
		t.Errorf("Recommendation = %q, want %q", opp.Recommendation, "BID")
	}
	if len(opp.Requirements) != 1 || opp.Requirements[0] != "SECRET clearance" {
		t.Errorf("Requirements = %v, want [SECRET clearance]", opp.Requirements)
	}
	if len(ms.saved) != 1 {
		t.Errorf("store.Save called %d times, want 1", len(ms.saved))
	}
}

// TestScoreAndSave_NilOpportunity verifies that a nil opportunity returns an error.
func TestScoreAndSave_NilOpportunity(t *testing.T) {
	err := ScoreAndSave(context.Background(), &mockScorer{}, &mockStore{}, nil, testProfile())

	if err == nil {
		t.Error("expected error for nil opportunity, got nil")
	}
}

// TestScoreAndSave_NilProfile verifies that a nil capability profile returns an error.
func TestScoreAndSave_NilProfile(t *testing.T) {
	err := ScoreAndSave(context.Background(), &mockScorer{}, &mockStore{}, testOpportunity(), nil)

	if err == nil {
		t.Error("expected error for nil profile, got nil")
	}
}

// TestScoreAndSave_ScoringError verifies that a scorer error is propagated.
func TestScoreAndSave_ScoringError(t *testing.T) {
	scorer := &mockScorer{err: errors.New("gemini unavailable")}

	err := ScoreAndSave(context.Background(), scorer, &mockStore{}, testOpportunity(), testProfile())

	if err == nil {
		t.Error("expected error from scorer, got nil")
	}
	if !strings.Contains(err.Error(), "gemini unavailable") {
		t.Errorf("error %q does not contain %q", err.Error(), "gemini unavailable")
	}
}

// TestScoreAndSave_StoreError verifies that a store.Save error is propagated.
func TestScoreAndSave_StoreError(t *testing.T) {
	scorer := &mockScorer{
		result: &ScoreResult{
			RawScore:       80,
			Recommendation: RecommendationBID,
			Reasoning:      "Good fit.",
			Requirements:   []string{},
		},
	}
	ms := &mockStore{err: errors.New("disk full")}

	err := ScoreAndSave(context.Background(), scorer, ms, testOpportunity(), testProfile())

	if err == nil {
		t.Error("expected store error, got nil")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error %q does not contain %q", err.Error(), "disk full")
	}
}

// TestScoreAndSave_Timestamps verifies that ScoredAt and UpdatedAt are set to
// non-nil / non-zero values after scoring.
func TestScoreAndSave_Timestamps(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	opp := testOpportunity()
	scorer := &mockScorer{
		result: &ScoreResult{
			RawScore:       60,
			Recommendation: RecommendationBID,
			Reasoning:      "Reasonable fit.",
			Requirements:   []string{},
		},
	}

	_ = ScoreAndSave(context.Background(), scorer, &mockStore{}, opp, testProfile())

	if opp.ScoredAt == nil {
		t.Error("expected ScoredAt to be set, got nil")
	}
	if opp.ScoredAt.Before(before) {
		t.Errorf("ScoredAt %v is before test start %v", opp.ScoredAt, before)
	}
	if opp.UpdatedAt.Before(before) {
		t.Errorf("UpdatedAt %v is before test start %v", opp.UpdatedAt, before)
	}
}

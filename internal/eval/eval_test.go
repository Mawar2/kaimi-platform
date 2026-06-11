package eval

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/scorer"
)

// floatEq compares two floats within a small tolerance. Metric math produces
// repeating decimals (e.g. 2/3), so an exact == comparison would be brittle.
func floatEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// --- Mock agents (fast layer: deterministic, no network) ---

// mockScorer returns a canned ScoreResult keyed by opportunity ID, satisfying the
// eval.Scorer consumer interface without any LLM call.
type mockScorer struct {
	byID map[string]*scorer.ScoreResult
	err  error
}

func (m *mockScorer) Score(_ context.Context, opp *opportunity.Opportunity, _ *scorer.CapabilityProfile) (*scorer.ScoreResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	res, ok := m.byID[opp.ID]
	if !ok {
		return nil, errors.New("no canned result for " + opp.ID)
	}
	return res, nil
}

// mockDrafter returns canned section text, satisfying the eval.SectionDrafter
// consumer interface without any LLM call.
type mockDrafter struct {
	text string
	err  error
}

func (m *mockDrafter) GenerateSection(_ context.Context, _, _ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.text, nil
}

// --- Scorer metrics tests ---

// TestScorerMetrics_KnownConfusionMatrix builds a labeled set whose confusion matrix
// is hand-computed, then asserts the runner reproduces the exact accuracy / precision /
// recall numbers. Treating BID as the positive class:
//
//	case  expected  predicted   outcome
//	1     BID       BID         true positive
//	2     BID       NO_BID      false negative
//	3     NO_BID    BID         false positive
//	4     NO_BID    NO_BID      true negative
//	5     BID       BID         true positive
//
// TP=2, FP=1, FN=1, TN=1  ->  accuracy=3/5=0.6, precision=2/3, recall=2/3.
func TestScorerMetrics_KnownConfusionMatrix(t *testing.T) {
	cases := []ScorerCase{
		{Name: "tp1", Opportunity: &opportunity.Opportunity{ID: "o1"}, ExpectedRecommendation: scorer.RecommendationBID},
		{Name: "fn1", Opportunity: &opportunity.Opportunity{ID: "o2"}, ExpectedRecommendation: scorer.RecommendationBID},
		{Name: "fp1", Opportunity: &opportunity.Opportunity{ID: "o3"}, ExpectedRecommendation: scorer.RecommendationNoBid},
		{Name: "tn1", Opportunity: &opportunity.Opportunity{ID: "o4"}, ExpectedRecommendation: scorer.RecommendationNoBid},
		{Name: "tp2", Opportunity: &opportunity.Opportunity{ID: "o5"}, ExpectedRecommendation: scorer.RecommendationBID},
	}
	ms := &mockScorer{byID: map[string]*scorer.ScoreResult{
		"o1": {Recommendation: scorer.RecommendationBID, Reasoning: "x"},
		"o2": {Recommendation: scorer.RecommendationNoBid, Reasoning: "x"},
		"o3": {Recommendation: scorer.RecommendationBID, Reasoning: "x"},
		"o4": {Recommendation: scorer.RecommendationNoBid, Reasoning: "x"},
		"o5": {Recommendation: scorer.RecommendationBID, Reasoning: "x"},
	}}

	rep, err := EvaluateScorer(context.Background(), ms, &scorer.CapabilityProfile{}, cases)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rep.Total != 5 {
		t.Errorf("Total = %d, want 5", rep.Total)
	}
	if rep.TruePositives != 2 || rep.FalsePositives != 1 || rep.FalseNegatives != 1 || rep.TrueNegatives != 1 {
		t.Errorf("confusion matrix = TP%d FP%d FN%d TN%d, want TP2 FP1 FN1 TN1",
			rep.TruePositives, rep.FalsePositives, rep.FalseNegatives, rep.TrueNegatives)
	}
	if !floatEq(rep.Accuracy, 0.6) {
		t.Errorf("Accuracy = %v, want 0.6", rep.Accuracy)
	}
	if !floatEq(rep.Precision, 2.0/3.0) {
		t.Errorf("Precision = %v, want 2/3", rep.Precision)
	}
	if !floatEq(rep.Recall, 2.0/3.0) {
		t.Errorf("Recall = %v, want 2/3", rep.Recall)
	}
}

// TestScorerMetrics_ReasonKeywords asserts that missing expected-reason keywords are
// recorded per case without affecting the bid/no-bid confusion matrix.
func TestScorerMetrics_ReasonKeywords(t *testing.T) {
	cases := []ScorerCase{
		{
			Name:                   "keyword-hit",
			Opportunity:            &opportunity.Opportunity{ID: "k1"},
			ExpectedRecommendation: scorer.RecommendationBID,
			ExpectedReasonKeywords: []string{"NAICS", "cloud"},
		},
		{
			Name:                   "keyword-miss",
			Opportunity:            &opportunity.Opportunity{ID: "k2"},
			ExpectedRecommendation: scorer.RecommendationBID,
			ExpectedReasonKeywords: []string{"NAICS", "Zero Trust"},
		},
	}
	ms := &mockScorer{byID: map[string]*scorer.ScoreResult{
		// "NAICS" and "cloud" both present (case-insensitive).
		"k1": {Recommendation: scorer.RecommendationBID, Reasoning: "Strong naics match for CLOUD migration."},
		// "NAICS" present, "Zero Trust" absent.
		"k2": {Recommendation: scorer.RecommendationBID, Reasoning: "Strong NAICS match only."},
	}}

	rep, err := EvaluateScorer(context.Background(), ms, &scorer.CapabilityProfile{}, cases)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rep.Cases) != 2 {
		t.Fatalf("len(Cases) = %d, want 2", len(rep.Cases))
	}
	if len(rep.Cases[0].MissingKeywords) != 0 {
		t.Errorf("case k1 MissingKeywords = %v, want none", rep.Cases[0].MissingKeywords)
	}
	if len(rep.Cases[1].MissingKeywords) != 1 || rep.Cases[1].MissingKeywords[0] != "Zero Trust" {
		t.Errorf("case k2 MissingKeywords = %v, want [Zero Trust]", rep.Cases[1].MissingKeywords)
	}
}

// TestScorerMetrics_ScorerError surfaces the error rather than silently dropping the case.
func TestScorerMetrics_ScorerError(t *testing.T) {
	cases := []ScorerCase{
		{Name: "boom", Opportunity: &opportunity.Opportunity{ID: "e1"}, ExpectedRecommendation: scorer.RecommendationBID},
	}
	ms := &mockScorer{err: errors.New("model unavailable")}

	if _, err := EvaluateScorer(context.Background(), ms, &scorer.CapabilityProfile{}, cases); err == nil {
		t.Fatal("expected error when scorer fails, got nil")
	}
}

// TestScorerMetrics_EmptySet rejects an empty dataset rather than dividing by zero.
func TestScorerMetrics_EmptySet(t *testing.T) {
	ms := &mockScorer{byID: map[string]*scorer.ScoreResult{}}
	if _, err := EvaluateScorer(context.Background(), ms, &scorer.CapabilityProfile{}, nil); err == nil {
		t.Fatal("expected error for empty dataset, got nil")
	}
}

// --- Writer groundedness tests ---

// TestWriterGroundedness_AllGrounded: every sentence's significant tokens appear in the
// supplied facts, so groundedness is 1.0 with no flagged claims.
func TestWriterGroundedness_AllGrounded(t *testing.T) {
	c := WriterCase{
		Name:          "grounded",
		SectionPrompt: "Describe the technical approach.",
		Facts:         []string{"cloud migration", "Zero Trust", "DevSecOps", "Example Federal Co"},
		MustNotFabricate: []string{
			"contract number",
		},
	}
	// Every significant token below appears in Facts.
	md := &mockDrafter{text: "Example Federal Co delivers cloud migration. Zero Trust and DevSecOps are core."}

	rep, err := EvaluateWriter(context.Background(), md, []WriterCase{c})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rep.Cases) != 1 {
		t.Fatalf("len(Cases) = %d, want 1", len(rep.Cases))
	}
	cr := rep.Cases[0]
	if !floatEq(cr.Groundedness, 1.0) {
		t.Errorf("Groundedness = %v, want 1.0 (ungrounded: %v)", cr.Groundedness, cr.UngroundedClaims)
	}
	if len(cr.UngroundedClaims) != 0 {
		t.Errorf("UngroundedClaims = %v, want none", cr.UngroundedClaims)
	}
	if cr.FabricationDetected {
		t.Errorf("FabricationDetected = true, want false")
	}
	if !floatEq(rep.MeanGroundedness, 1.0) {
		t.Errorf("MeanGroundedness = %v, want 1.0", rep.MeanGroundedness)
	}
}

// TestWriterGroundedness_HalfGrounded: two sentences, one fully supported by the facts and
// one introducing an unsupported claim, so groundedness is exactly 0.5 and the bad
// sentence is flagged.
func TestWriterGroundedness_HalfGrounded(t *testing.T) {
	c := WriterCase{
		Name:          "half",
		SectionPrompt: "Describe past performance.",
		Facts:         []string{"cloud migration", "General Services Administration"},
	}
	// Sentence 1 grounded (tokens in facts). Sentence 2 ungrounded (Pentagon/$5 million absent).
	md := &mockDrafter{text: "We performed cloud migration for the General Services Administration. We won a $5 million Pentagon award."}

	rep, err := EvaluateWriter(context.Background(), md, []WriterCase{c})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cr := rep.Cases[0]
	if !floatEq(cr.Groundedness, 0.5) {
		t.Errorf("Groundedness = %v, want 0.5 (ungrounded: %v)", cr.Groundedness, cr.UngroundedClaims)
	}
	if len(cr.UngroundedClaims) != 1 {
		t.Errorf("UngroundedClaims = %v, want exactly 1", cr.UngroundedClaims)
	}
}

// TestWriterGroundedness_FabricationMarker: the must-not-fabricate term shows up in the
// draft as an invented fact, so FabricationDetected is true.
func TestWriterGroundedness_FabricationMarker(t *testing.T) {
	c := WriterCase{
		Name:             "fab",
		SectionPrompt:    "Describe certifications.",
		Facts:            []string{"cloud migration"},
		MustNotFabricate: []string{"ISO 9001"},
	}
	md := &mockDrafter{text: "We hold ISO 9001 certification."}

	rep, err := EvaluateWriter(context.Background(), md, []WriterCase{c})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rep.Cases[0].FabricationDetected {
		t.Errorf("FabricationDetected = false, want true (draft asserted a must-not-fabricate term)")
	}
}

// TestWriterGroundedness_GapMarkerIsGrounded: a section that emits the writer's [GAP: ...]
// placeholder instead of inventing a fact must not be penalized — a gap is the correct,
// honest behavior, so the sentence counts as grounded.
func TestWriterGroundedness_GapMarkerIsGrounded(t *testing.T) {
	c := WriterCase{
		Name:          "gap",
		SectionPrompt: "Describe staffing.",
		Facts:         []string{"cloud migration"},
	}
	md := &mockDrafter{text: "We provide cloud migration. [GAP: number of cleared staff is not provided]."}

	rep, err := EvaluateWriter(context.Background(), md, []WriterCase{c})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cr := rep.Cases[0]
	if !floatEq(cr.Groundedness, 1.0) {
		t.Errorf("Groundedness = %v, want 1.0 (gap markers are honest, not ungrounded). ungrounded=%v", cr.Groundedness, cr.UngroundedClaims)
	}
}

// TestWriterGroundedness_DrafterError surfaces a generation error.
func TestWriterGroundedness_DrafterError(t *testing.T) {
	c := WriterCase{Name: "err", SectionPrompt: "x", Facts: []string{"y"}}
	md := &mockDrafter{err: errors.New("safety blocked")}
	if _, err := EvaluateWriter(context.Background(), md, []WriterCase{c}); err == nil {
		t.Fatal("expected error when drafter fails, got nil")
	}
}

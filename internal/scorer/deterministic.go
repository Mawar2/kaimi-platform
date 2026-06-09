package scorer

import (
	"context"
	"fmt"
	"strings"

	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// DeterministicScorer is an offline, rule-based implementation of Scorer that
// produces a bid-fit score from the same pre-computed Signals the GeminiScorer
// feeds to the LLM — but without any model call, API key, or network access.
//
// It exists so the Zone-1 pipeline (KAI-M6) can run end to end in cached mode:
// fetch from fixtures, gate eligibility, and still produce *scored* opportunities
// in the Store with zero credentials. It is intentionally explainable and stable
// (same input → same output), which also makes it ideal for tests.
//
// It is not a replacement for GeminiScorer in live mode: it cannot read the
// solicitation prose to extract must-have Requirements (that needs the LLM), so
// Requirements is always returned empty. Live mode should use GeminiScorer.
type DeterministicScorer struct{}

// NewDeterministicScorer returns an offline rule-based Scorer.
func NewDeterministicScorer() *DeterministicScorer {
	return &DeterministicScorer{}
}

// Scoring weights for the deterministic rubric. They mirror the weighting order
// documented for the LLM (Primary NAICS > Secondary NAICS > tags > past perf > SDB)
// and are tuned so a primary-NAICS match alone clears the REVIEW band while a
// no-signal opportunity lands firmly in NO_BID.
const (
	weightPrimaryNAICS   = 55
	weightSecondaryNAICS = 30
	weightPerTag         = 6
	maxTagPoints         = 24
	weightPerPastPerf    = 5
	maxPastPerfPoints    = 15
	weightSDB            = 6

	thresholdBID   = 60 // score >= 60 → BID
	thresholdNoBid = 40 // score < 40 → NO_BID; otherwise REVIEW
)

// Score implements the Scorer interface using a deterministic rubric over the
// pre-computed Signals. It never calls an LLM and never returns Requirements.
func (d *DeterministicScorer) Score(_ context.Context, opp *opportunity.Opportunity, profile *CapabilityProfile) (*ScoreResult, error) {
	if opp == nil {
		return nil, fmt.Errorf("opportunity cannot be nil")
	}
	if profile == nil {
		return nil, fmt.Errorf("capability profile cannot be nil")
	}

	signals := computeSignals(opp, profile)
	score := scoreFromSignals(signals)

	var rec Recommendation
	switch {
	case score >= thresholdBID:
		rec = RecommendationBID
	case score < thresholdNoBid:
		rec = RecommendationNoBid
	default:
		rec = RecommendationReview
	}

	return &ScoreResult{
		RawScore:       score,
		Recommendation: rec,
		Reasoning:      reasoningFromSignals(signals, score),
		Requirements:   []string{}, // deterministic scorer does not extract must-haves
	}, nil
}

// scoreFromSignals applies the deterministic rubric and clamps to [0, 100].
func scoreFromSignals(s Signals) int {
	score := 0
	// NAICS contributes either the primary OR the secondary weight, never both:
	// computeSignals guarantees SecondaryNAICSMatch is false when PrimaryNAICSMatch
	// is true, so this mirrors the documented "Primary > Secondary" hierarchy.
	switch {
	case s.PrimaryNAICSMatch:
		score += weightPrimaryNAICS
	case s.SecondaryNAICSMatch:
		score += weightSecondaryNAICS
	}
	score += capPoints(s.TagOverlapCount*weightPerTag, maxTagPoints)
	score += capPoints(s.PastPerfOverlapCount*weightPerPastPerf, maxPastPerfPoints)
	if s.SDBApplies {
		score += weightSDB
	}
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return score
}

func capPoints(points, limit int) int {
	if points > limit {
		return limit
	}
	return points
}

// reasoningFromSignals builds a human-readable explanation of the score, citing
// which signals fired — the explainable trail the pipeline persists.
func reasoningFromSignals(s Signals, score int) string {
	var reasons []string
	switch {
	case s.PrimaryNAICSMatch:
		reasons = append(reasons, "primary NAICS match")
	case s.SecondaryNAICSMatch:
		reasons = append(reasons, "secondary NAICS match")
	default:
		reasons = append(reasons, "no NAICS match")
	}
	if s.TagOverlapCount > 0 {
		reasons = append(reasons, fmt.Sprintf("%d competency tag overlap(s)", s.TagOverlapCount))
	}
	if s.PastPerfOverlapCount > 0 {
		reasons = append(reasons, fmt.Sprintf("%d past-performance overlap(s)", s.PastPerfOverlapCount))
	}
	if s.SDBApplies {
		reasons = append(reasons, "qualifying SDB set-aside")
	}
	return fmt.Sprintf("Deterministic score %d/100 (offline rubric): %s.", score, strings.Join(reasons, ", "))
}

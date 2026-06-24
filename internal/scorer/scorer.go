package scorer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/store"
)

// Recommendation is the bid/no-bid recommendation produced by the Scorer.
type Recommendation string

const (
	// RecommendationBID indicates the company should bid on this opportunity.
	RecommendationBID Recommendation = "BID"

	// RecommendationNoBid indicates the company should not bid.
	RecommendationNoBid Recommendation = "NO_BID"

	// RecommendationReview indicates the opportunity needs human review before
	// a bid/no-bid decision can be made (score in uncertain range).
	RecommendationReview Recommendation = "REVIEW"
)

// Signals holds pre-computed deterministic bid-fit signals passed to the LLM.
//
// Signals are computed from the CapabilityProfile and Opportunity without calling
// the LLM, making them fast and consistent. Passing them explicitly to the prompt
// makes scoring more explainable and less dependent on the LLM extracting facts
// from unstructured text.
type Signals struct {
	// PrimaryNAICSMatch is true when the opportunity's NAICS code matches one of
	// the profile's primary codes (highest weight).
	PrimaryNAICSMatch bool

	// SecondaryNAICSMatch is true when the opportunity's NAICS code matches one of
	// the profile's secondary codes (moderate weight). False when PrimaryNAICSMatch is true.
	SecondaryNAICSMatch bool

	// TagOverlapCount is the number of profile competency tags found (case-insensitive
	// substring match) in the opportunity's title or description.
	TagOverlapCount int

	// PastPerfOverlapCount is the number of profile past-performance terms found
	// in the opportunity's agency name or description.
	PastPerfOverlapCount int

	// SDBApplies is true when the company has SDB status and the opportunity's
	// set-aside code matches one of the profile's qualifying set-aside codes.
	SDBApplies bool
}

// ScoreResult holds the structured output from the LLM scorer.
type ScoreResult struct {
	// RawScore is the 0–100 integer bid fit score returned by the LLM.
	RawScore int `json:"score"`

	// Recommendation is BID, NO_BID, or REVIEW.
	Recommendation Recommendation `json:"recommendation"`

	// Reasoning is 2–4 sentences of human-readable reasoning for the score,
	// citing specific signals from the opportunity and profile.
	Reasoning string `json:"reasoning"`

	// Requirements are must-have requirements extracted from the solicitation.
	// May be empty if the solicitation has no extractable requirements.
	Requirements []string `json:"requirements"`
}

// Scorer scores an opportunity for bid/no-bid fit against a CapabilityProfile.
//
// Implementations must be safe for concurrent use. The interface is designed to be
// mockable in unit tests — GeminiScorer is the production implementation.
type Scorer interface {
	// Score computes a bid/no-bid ScoreResult for the given opportunity.
	Score(ctx context.Context, opp *opportunity.Opportunity, profile *CapabilityProfile) (*ScoreResult, error)
}

// GeminiScorer is the Vertex AI / Gemini 2.5 Pro implementation of Scorer.
//
// It pre-computes deterministic signals via computeSignals, then calls Gemini with a
// structured prompt and ResponseSchema to get a consistent JSON ScoreResult.
// Uses BackendEnterprise (Vertex AI) with Application Default Credentials.
type GeminiScorer struct {
	client    *genai.Client
	modelName string
}

// NewGeminiScorer creates a new GeminiScorer backed by Vertex AI.
//
// Requires Application Default Credentials:
//
//	gcloud auth application-default login
//
// Parameters:
//   - ctx: context for client initialization
//   - projectID: GCP project ID (e.g., "your-gcp-project")
//   - location: GCP region (e.g., "us-east4")
//   - modelName: Gemini model name (e.g., "gemini-2.5-pro")
func NewGeminiScorer(ctx context.Context, projectID, location, modelName string) (*GeminiScorer, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend:  genai.BackendEnterprise,
		Project:  projectID,
		Location: location,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	return &GeminiScorer{
		client:    client,
		modelName: modelName,
	}, nil
}

// Score implements the Scorer interface using Gemini 2.5 Pro via Vertex AI.
//
// Pre-computes deterministic signals, builds a structured prompt, calls Gemini with
// a ResponseSchema, and parses the JSON output into a ScoreResult.
func (gs *GeminiScorer) Score(ctx context.Context, opp *opportunity.Opportunity, profile *CapabilityProfile) (*ScoreResult, error) {
	if opp == nil {
		return nil, fmt.Errorf("opportunity cannot be nil")
	}
	if profile == nil {
		return nil, fmt.Errorf("capability profile cannot be nil")
	}

	signals := computeSignals(opp, profile)
	prompt := buildScoringPrompt(opp, profile, signals)

	contents := []*genai.Content{
		genai.NewContentFromText(prompt, genai.RoleUser),
	}

	config := scoringGenerateConfig()

	resp, err := gs.client.Models.GenerateContent(ctx, gs.modelName, contents, config)
	if err != nil {
		return nil, fmt.Errorf("gemini API call failed: %w", err)
	}

	text := resp.Text()
	if text == "" {
		return nil, fmt.Errorf("gemini returned empty response")
	}

	result, err := validateAndConvert(text)
	if err != nil {
		return nil, fmt.Errorf("invalid Gemini response: %w", err)
	}
	return result, nil
}

// Output-budget sizing for the scoring call. gemini-2.5-pro is a thinking model:
// its internal reasoning tokens are billed against MaxOutputTokens. The old
// 1024 cap was consumed by thinking, leaving the JSON output empty or truncated
// (finishReason MAX_TOKENS) — see issue #192. We raise the cap and bound the
// thinking budget so reasoning can never starve the structured output. Values
// are deliberately generous, tunable headroom, not tight estimates.
const (
	maxScoringOutputTokens = 8192
	scoringThinkingBudget  = 2048
)

// scoringGenerateConfig builds the Gemini config for a scoring call, with
// thinking-token headroom (see #192) and the structured-output schema.
func scoringGenerateConfig() *genai.GenerateContentConfig {
	temp := float32(0.2) // Low temperature for consistent, deterministic scoring
	budget := int32(scoringThinkingBudget)
	return &genai.GenerateContentConfig{
		Temperature:      &temp,
		MaxOutputTokens:  maxScoringOutputTokens,
		ResponseMIMEType: "application/json",
		ResponseSchema:   scoringResponseSchema(),
		ThinkingConfig:   &genai.ThinkingConfig{ThinkingBudget: &budget},
	}
}

// scoringResponseSchema returns the JSON schema for Gemini structured output.
//
// Using ResponseSchema instead of free-text JSON parsing eliminates the need for
// regex or prompt engineering to extract structured data from Gemini's response.
func scoringResponseSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"score": {
				Type:        genai.TypeInteger,
				Description: "Bid fit score from 0 to 100. 0 = no fit, 100 = perfect fit.",
			},
			"recommendation": {
				Type:        genai.TypeString,
				Enum:        []string{"BID", "NO_BID", "REVIEW"},
				Description: "BID if score >= 60, NO_BID if score < 40, REVIEW if 40–59 or uncertain.",
			},
			"reasoning": {
				Type:        genai.TypeString,
				Description: "2–4 sentences explaining the score, citing specific signals from the opportunity.",
			},
			"requirements": {
				Type:        genai.TypeArray,
				Items:       &genai.Schema{Type: genai.TypeString},
				Description: "Must-have requirements extracted from the solicitation.",
			},
		},
		Required: []string{"score", "recommendation", "reasoning"},
	}
}

// computeSignals pre-computes deterministic bid-fit signals from the opportunity
// and capability profile without calling the LLM.
func computeSignals(opp *opportunity.Opportunity, profile *CapabilityProfile) Signals {
	var signals Signals

	// NAICS match — primary codes (highest weight)
	for _, code := range profile.PrimaryNAICS {
		if code == opp.NAICSCode {
			signals.PrimaryNAICSMatch = true
			break
		}
	}

	// NAICS match — secondary codes (moderate weight); skip if primary already matched
	if !signals.PrimaryNAICSMatch {
		for _, code := range profile.SecondaryNAICS {
			if code == opp.NAICSCode {
				signals.SecondaryNAICSMatch = true
				break
			}
		}
	}

	// Competency tag overlap — case-insensitive substring match against title + description.
	// EffectiveDescription is the resolved solicitation text when available (else the raw
	// Description, which may be a noticedesc URL), so signals reflect real prose once resolved.
	searchText := strings.ToLower(opp.Title + " " + opp.EffectiveDescription())
	for _, tag := range profile.CompetencyTags {
		if strings.Contains(searchText, strings.ToLower(tag)) {
			signals.TagOverlapCount++
		}
	}

	// Past performance overlap — keyword match in agency name or description
	agencyAndDesc := strings.ToLower(opp.Agency + " " + opp.EffectiveDescription())
	for _, term := range profile.PastPerformance {
		if strings.Contains(agencyAndDesc, strings.ToLower(term)) {
			signals.PastPerfOverlapCount++
		}
	}

	// SDB set-aside factor — applies only when company has SDB status and the
	// opportunity's set-aside code matches a qualifying code
	if profile.SDBStatus {
		for _, code := range profile.QualifyingSetAsides {
			if strings.EqualFold(code, opp.SetAsideCode) {
				signals.SDBApplies = true
				break
			}
		}
	}

	return signals
}

// buildScoringPrompt constructs the LLM prompt from the opportunity, profile, and signals.
func buildScoringPrompt(opp *opportunity.Opportunity, profile *CapabilityProfile, signals Signals) string {
	var sb strings.Builder

	sb.WriteString("You are a federal contracting bid/no-bid analyst.\n\n")
	sb.WriteString("Score the following opportunity for bid/no-bid fit using the company's ")
	sb.WriteString("capability profile and pre-computed signals.\n\n")

	sb.WriteString("## Opportunity\n")
	fmt.Fprintf(&sb, "Title: %s\n", opp.Title)
	fmt.Fprintf(&sb, "Agency: %s\n", opp.Agency)
	fmt.Fprintf(&sb, "NAICS: %s (%s)\n", opp.NAICSCode, opp.NAICSDescription)
	fmt.Fprintf(&sb, "Set-Aside: %s\n", opp.SetAsideCode)
	fmt.Fprintf(&sb, "Response Deadline: %s\n", opp.ResponseDeadline.Format("2006-01-02"))
	fmt.Fprintf(&sb, "Description:\n%s\n\n", opp.EffectiveDescription())

	sb.WriteString("## Capability Profile\n")
	fmt.Fprintf(&sb, "Primary NAICS: %s\n", strings.Join(profile.PrimaryNAICS, ", "))
	fmt.Fprintf(&sb, "Secondary NAICS: %s\n", strings.Join(profile.SecondaryNAICS, ", "))
	fmt.Fprintf(&sb, "Competency Tags: %s\n", strings.Join(profile.CompetencyTags, ", "))
	fmt.Fprintf(&sb, "Past Performance: %s\n", strings.Join(profile.PastPerformance, ", "))
	fmt.Fprintf(&sb, "SDB Status: %v\n\n", profile.SDBStatus)

	sb.WriteString("## Pre-Computed Signals\n")
	fmt.Fprintf(&sb, "Primary NAICS Match: %v\n", signals.PrimaryNAICSMatch)
	fmt.Fprintf(&sb, "Secondary NAICS Match: %v\n", signals.SecondaryNAICSMatch)
	fmt.Fprintf(&sb, "Competency Tag Overlap: %d\n", signals.TagOverlapCount)
	fmt.Fprintf(&sb, "Past Performance Overlap: %d\n", signals.PastPerfOverlapCount)
	fmt.Fprintf(&sb, "SDB Set-Aside Applies: %v\n\n", signals.SDBApplies)

	sb.WriteString("## Scoring Rubric\n")
	sb.WriteString("Score 0–100 where: 80–100 = excellent fit (BID), 60–79 = good fit (BID), ")
	sb.WriteString("40–59 = uncertain (REVIEW), 20–39 = poor fit (NO_BID), 0–19 = no fit (NO_BID).\n\n")
	sb.WriteString("Signal weights: Primary NAICS > Secondary NAICS > Tag overlap > Past performance > SDB factor.\n\n")
	sb.WriteString("Return score, BID/NO_BID/REVIEW recommendation, 2–4 sentence reasoning citing ")
	sb.WriteString("specific signals, and must-have requirements extracted from the solicitation.\n")

	return sb.String()
}

// validateAndConvert parses and validates the JSON response from Gemini.
//
// Returns an error if the JSON is malformed, the score is out of [0, 100],
// or the recommendation is not one of BID / NO_BID / REVIEW.
// Normalizes nil Requirements to an empty slice.
func validateAndConvert(jsonText string) (*ScoreResult, error) {
	var result ScoreResult
	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	if result.RawScore < 0 || result.RawScore > 100 {
		return nil, fmt.Errorf("score %d out of range [0, 100]", result.RawScore)
	}

	switch result.Recommendation {
	case RecommendationBID, RecommendationNoBid, RecommendationReview:
		// valid
	default:
		return nil, fmt.Errorf("invalid recommendation %q: must be BID, NO_BID, or REVIEW", result.Recommendation)
	}

	if result.Reasoning == "" {
		return nil, fmt.Errorf("reasoning cannot be empty")
	}

	// Normalize nil Requirements so callers don't need to handle nil vs empty
	if result.Requirements == nil {
		result.Requirements = []string{}
	}

	return &result, nil
}

// ScoreAndSave scores an opportunity and writes all scored fields back to the store.
//
// Converts the 0–100 RawScore to a 0.0–1.0 Score field and populates
// ScoreReasoning, Recommendation, Requirements, ScoredAt, and UpdatedAt.
// The updated opportunity is saved to the store after scoring completes.
//
// Returns an error if opp or profile is nil, scoring fails, or the store write fails.
func ScoreAndSave(ctx context.Context, scorer Scorer, s store.Store, opp *opportunity.Opportunity, profile *CapabilityProfile) error {
	if opp == nil {
		return fmt.Errorf("opportunity cannot be nil")
	}
	if profile == nil {
		return fmt.Errorf("capability profile cannot be nil")
	}

	result, err := scorer.Score(ctx, opp, profile)
	if err != nil {
		return fmt.Errorf("scoring failed for %s: %w", opp.ID, err)
	}

	// Convert 0–100 raw score to 0.0–1.0
	opp.Score = float64(result.RawScore) / 100.0
	opp.ScoreReasoning = result.Reasoning
	opp.Recommendation = string(result.Recommendation)
	opp.Requirements = result.Requirements

	now := time.Now().UTC()
	opp.ScoredAt = &now
	opp.UpdatedAt = now

	if err := s.Save(ctx, opp); err != nil {
		return fmt.Errorf("failed to save scored opportunity %s: %w", opp.ID, err)
	}
	return nil
}

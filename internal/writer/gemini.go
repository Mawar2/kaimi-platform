package writer

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/Mawar2/Kaimi/internal/kobs"
)

// GeminiGenerator is the Vertex AI / Gemini implementation of Generator.
//
// It mirrors scorer.GeminiScorer: a Vertex AI (BackendEnterprise) client with
// Application Default Credentials, called at low temperature for consistent,
// grounded prose. The anti-fabrication grounding lives in the prompt the Writer
// builds (buildSectionPrompt); this type only performs the model call.
type GeminiGenerator struct {
	client    *genai.Client
	modelName string
}

// NewGeminiGenerator creates a GeminiGenerator backed by Vertex AI.
//
// Requires Application Default Credentials (gcloud auth application-default login).
func NewGeminiGenerator(ctx context.Context, projectID, location, modelName string) (*GeminiGenerator, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend:  genai.BackendEnterprise,
		Project:  projectID,
		Location: location,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	return &GeminiGenerator{client: client, modelName: modelName}, nil
}

// Output-budget sizing for a section draft. Gemini thinking models (2.5-pro and
// the 3.x family) bill their internal reasoning against MaxOutputTokens, so a low
// cap is consumed by thinking and the section comes back truncated or empty — the
// 2048 cap did this on 2.5-pro (#192) and the bake-off (#229) confirmed 3.x is
// even hungrier, returning empty drafts. Full proposal sections are long prose, so
// we give them more room than the scorer's JSON: a 16K cap with a bounded thinking
// budget that can never starve the output. Values are generous, tunable headroom.
const (
	maxSectionOutputTokens = 16384
	sectionThinkingBudget  = 4096
)

// sectionGenerateConfig builds the Gemini config for one section draft, with
// thinking-token headroom (see #192) and the anti-fabrication system instruction.
func sectionGenerateConfig(systemInstruction string) *genai.GenerateContentConfig {
	temp := float32(0.3) // low temperature: grounded, consistent prose
	budget := int32(sectionThinkingBudget)
	return &genai.GenerateContentConfig{
		Temperature:       &temp,
		MaxOutputTokens:   maxSectionOutputTokens,
		SystemInstruction: genai.NewContentFromText(systemInstruction, genai.RoleUser),
		ThinkingConfig:    &genai.ThinkingConfig{ThinkingBudget: &budget},
	}
}

// GenerateSection implements Generator using Gemini via Vertex AI.
//
// The anti-fabrication rules are passed as a system instruction (not in the user
// prompt) so they resist instruction drift on long opportunity text.
func (g *GeminiGenerator) GenerateSection(ctx context.Context, systemInstruction, prompt string) (string, error) {
	contents := []*genai.Content{
		genai.NewContentFromText(prompt, genai.RoleUser),
	}

	config := sectionGenerateConfig(systemInstruction)

	resp, err := kobs.GenerateContent(ctx, g.client, g.modelName, contents, config)
	if err != nil {
		return "", fmt.Errorf("gemini API call failed: %w", err)
	}

	// A safety-blocked or otherwise empty response can have zero candidates;
	// guard before reading text so a blocked generation surfaces as an error.
	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("gemini returned no candidates (possibly safety-blocked)")
	}

	text := resp.Text()
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("gemini returned empty response")
	}
	return text, nil
}

package finalreview

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/Mawar2/Kaimi/internal/kobs"
)

// GeminiComplianceChecker is the Vertex AI / Gemini implementation of
// ComplianceChecker. It mirrors writer.GeminiGenerator: a Vertex AI
// (BackendEnterprise) client using Application Default Credentials, called at a
// low temperature for a consistent, grounded compliance verdict. The grounding
// and output-shape discipline live in the prompt and system instruction the
// Final Review agent builds; this type only performs the model call.
type GeminiComplianceChecker struct {
	client    *genai.Client
	modelName string
}

// NewGeminiComplianceChecker creates a GeminiComplianceChecker backed by Vertex AI.
//
// Requires Application Default Credentials (gcloud auth application-default login).
func NewGeminiComplianceChecker(ctx context.Context, projectID, location, modelName string) (*GeminiComplianceChecker, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend:  genai.BackendEnterprise,
		Project:  projectID,
		Location: location,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	return &GeminiComplianceChecker{client: client, modelName: modelName}, nil
}

// Output-budget sizing for a compliance check. Gemini thinking models (2.5-pro
// and the 3.x family) bill internal reasoning against MaxOutputTokens, so the old
// 4096 cap with no thinking budget could be consumed by reasoning and return an
// empty verdict on 3.x — the same failure the scorer (#192) and Writer (#229) hit.
// We raise the cap and bound the thinking budget so reasoning can never starve the
// JSON verdict. The verdict is compact JSON, so it needs less room than the
// Writer's prose.
const (
	maxComplianceOutputTokens = 8192
	complianceThinkingBudget  = 2048
)

// complianceGenerateConfig builds the Gemini config for a compliance check, with
// thinking-token headroom (see #192/#229) and JSON output so a 3.x model returns a
// verdict rather than spending the whole budget on hidden reasoning.
func complianceGenerateConfig(systemInstruction string) *genai.GenerateContentConfig {
	temp := float32(0.1) // low temperature: deterministic, grounded compliance verdict
	budget := int32(complianceThinkingBudget)
	return &genai.GenerateContentConfig{
		Temperature:       &temp,
		MaxOutputTokens:   maxComplianceOutputTokens,
		ResponseMIMEType:  "application/json",
		SystemInstruction: genai.NewContentFromText(systemInstruction, genai.RoleUser),
		ThinkingConfig:    &genai.ThinkingConfig{ThinkingBudget: &budget},
	}
}

// CheckCompliance implements ComplianceChecker using Gemini via Vertex AI. It
// requests JSON output and returns the raw response for the agent to parse.
func (g *GeminiComplianceChecker) CheckCompliance(ctx context.Context, systemInstruction, prompt string) (string, error) {
	contents := []*genai.Content{
		genai.NewContentFromText(prompt, genai.RoleUser),
	}

	config := complianceGenerateConfig(systemInstruction)

	resp, err := kobs.GenerateContent(ctx, g.client, g.modelName, contents, config)
	if err != nil {
		return "", fmt.Errorf("gemini API call failed: %w", err)
	}

	// A safety-blocked or otherwise empty response can have zero candidates;
	// guard before reading text so a blocked review surfaces as an error.
	if len(resp.Candidates) == 0 {
		return "", fmt.Errorf("gemini returned no candidates (possibly safety-blocked)")
	}

	text := resp.Text()
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("gemini returned empty response")
	}
	return text, nil
}

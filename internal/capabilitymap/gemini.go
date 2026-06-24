package capabilitymap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/Mawar2/Kaimi/internal/profile"
)

// GeminiBuilder is the Vertex AI / Gemini implementation of Builder. It reads the
// onboarding profile AND the company's context documents (capability statements, CPARS,
// past proposals) and synthesizes a rich, evidence-backed CapabilityMap — the deep
// business understanding that profile-only fields can't capture. It mirrors
// internal/scorer's Vertex client setup (BackendEnterprise + ADC, structured output).
type GeminiBuilder struct {
	client    *genai.Client
	modelName string
}

// NewGeminiBuilder creates a builder backed by Vertex AI (ADC: gcloud auth
// application-default login). location is the GCP region; modelName e.g. "gemini-2.5-pro".
func NewGeminiBuilder(ctx context.Context, projectID, location, modelName string) (*GeminiBuilder, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend:  genai.BackendEnterprise,
		Project:  projectID,
		Location: location,
	})
	if err != nil {
		return nil, fmt.Errorf("capabilitymap: create Gemini client: %w", err)
	}
	return &GeminiBuilder{client: client, modelName: modelName}, nil
}

// maxDocChars bounds how much of each context document is sent to the model, so a large
// upload can't blow the token budget. The most signal is near the top of a capability
// statement / CPARS, so a generous head slice is a sound, simple bound.
const maxDocChars = 12000

// budget sizing — Gemini 2.5 Pro is a thinking model (reasoning tokens bill against
// MaxOutputTokens), so leave generous headroom plus a bounded thinking budget, matching
// the scorer's approach (issue #192).
const (
	maxMapOutputTokens = 8192
	mapThinkingBudget  = 2048
)

// Build calls Gemini with the profile + document text and returns a synthesized map.
// Company, NAICS, Sources, Model, and GeneratedAt are stamped in code (authoritative
// facts / provenance); the model fills the synthesized fields (summary, competencies
// with evidence, differentiators, domains, certifications, keywords, past-performance
// relevance). On any model/parse failure the caller can fall back to DeterministicBuilder.
func (b *GeminiBuilder) Build(ctx context.Context, p *profile.CapabilityProfile, docs []ContextDoc) (*CapabilityMap, error) {
	if p == nil {
		return nil, fmt.Errorf("capabilitymap: profile cannot be nil")
	}
	prompt := buildPrompt(p, docs)
	contents := []*genai.Content{genai.NewContentFromText(prompt, genai.RoleUser)}

	resp, err := b.client.Models.GenerateContent(ctx, b.modelName, contents, generateConfig())
	if err != nil {
		return nil, fmt.Errorf("capabilitymap: gemini call failed: %w", err)
	}
	text := resp.Text()
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("capabilitymap: gemini returned empty response")
	}

	var synth synthesized
	if err := json.Unmarshal([]byte(text), &synth); err != nil {
		return nil, fmt.Errorf("capabilitymap: decode model output: %w", err)
	}

	m := &CapabilityMap{
		Company:          p.Company,
		Summary:          strings.TrimSpace(synth.Summary),
		CoreCompetencies: synth.CoreCompetencies,
		Differentiators:  synth.Differentiators,
		Domains:          synth.Domains,
		PastPerformance:  synth.PastPerformance,
		Certifications:   mergeCerts(setAsideCertifications(p.SetAside), synth.Certifications),
		NAICS:            naicsCodes(p),
		Keywords:         synth.Keywords,
		Model:            b.modelName,
		GeneratedAt:      time.Now().UTC(),
	}
	m.Sources = append(m.Sources, "onboarding profile")
	for _, d := range docs {
		if name := strings.TrimSpace(d.Name); name != "" {
			m.Sources = append(m.Sources, name)
		}
	}
	return m, nil
}

// synthesized is the model's structured output (the fields it generates; the rest are
// stamped in code).
type synthesized struct {
	Summary          string               `json:"summary"`
	CoreCompetencies []Competency         `json:"core_competencies"`
	Differentiators  []string             `json:"differentiators"`
	Domains          []string             `json:"domains"`
	PastPerformance  []PastPerformanceRef `json:"past_performance"`
	Certifications   []string             `json:"certifications"`
	Keywords         []string             `json:"keywords"`
}

// mergeCerts unions the profile-derived set-aside certs with any the model inferred from
// documents (e.g. ISO 27001, CMMC, clearances), de-duplicated case-insensitively. The
// profile-derived ones come first as the authoritative set-aside facts.
func mergeCerts(base, extra []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, base...), extra...) {
		v := strings.TrimSpace(s)
		if v == "" {
			continue
		}
		k := strings.ToLower(v)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
	}
	return out
}

// buildPrompt assembles the capability-analyst prompt from the profile + document text.
func buildPrompt(p *profile.CapabilityProfile, docs []ContextDoc) string {
	var b strings.Builder
	b.WriteString("You are a federal business-development capability analyst. Build a structured ")
	b.WriteString("capability map for the company below, used to qualify and score federal opportunities ")
	b.WriteString("and to ground proposal drafting. Synthesize from BOTH the profile and the context ")
	b.WriteString("documents. Where a claim comes from a document, cite the document name in evidence. ")
	b.WriteString("Be concrete and specific; do not invent capabilities the company has not demonstrated.\n\n")

	b.WriteString("## Company profile\n")
	fmt.Fprintf(&b, "Company: %s\n", p.Company)
	if codes := naicsCodes(p); len(codes) > 0 {
		fmt.Fprintf(&b, "NAICS: %s\n", strings.Join(codes, ", "))
	}
	if len(p.Competencies) > 0 {
		fmt.Fprintf(&b, "Stated competencies: %s\n", strings.Join(p.Competencies, "; "))
	}
	if len(p.Scoring.CompetencyTags) > 0 {
		fmt.Fprintf(&b, "Competency tags: %s\n", strings.Join(p.Scoring.CompetencyTags, ", "))
	}
	for _, pp := range p.PastPerformance {
		fmt.Fprintf(&b, "Past performance: %s — %s (%s)\n", pp.Client, pp.Scope, pp.Value)
	}
	if certs := setAsideCertifications(p.SetAside); len(certs) > 0 {
		fmt.Fprintf(&b, "Set-aside status: %s\n", strings.Join(certs, ", "))
	}

	if len(docs) > 0 {
		b.WriteString("\n## Context documents\n")
		for _, d := range docs {
			text := strings.TrimSpace(d.Text)
			if text == "" {
				continue
			}
			if len(text) > maxDocChars {
				text = text[:maxDocChars] + "\n…[truncated]"
			}
			fmt.Fprintf(&b, "\n### %s\n%s\n", d.Name, text)
		}
	}

	b.WriteString("\n## Output\nReturn a JSON capability map: a 2–3 sentence summary; core competencies ")
	b.WriteString("(name, description, and evidence citing document names); differentiators; mission/customer ")
	b.WriteString("domains served; relevant past performance with a one-line relevance note; certifications ")
	b.WriteString("(ISO/CMMC/clearances inferred from documents); and an expanded keyword vocabulary for matching.")
	return b.String()
}

// generateConfig builds the Gemini config with structured output + thinking headroom.
func generateConfig() *genai.GenerateContentConfig {
	temp := float32(0.3)
	budget := int32(mapThinkingBudget)
	return &genai.GenerateContentConfig{
		Temperature:      &temp,
		MaxOutputTokens:  maxMapOutputTokens,
		ResponseMIMEType: "application/json",
		ResponseSchema:   responseSchema(),
		ThinkingConfig:   &genai.ThinkingConfig{ThinkingBudget: &budget},
	}
}

// responseSchema is the structured-output schema for the synthesized fields.
func responseSchema() *genai.Schema {
	strArr := &genai.Schema{Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}}
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"summary": {Type: genai.TypeString, Description: "2–3 sentence business summary."},
			"core_competencies": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"name":        {Type: genai.TypeString},
						"description": {Type: genai.TypeString},
						"evidence":    strArr,
					},
					Required: []string{"name"},
				},
			},
			"differentiators": strArr,
			"domains":         strArr,
			"past_performance": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"client":    {Type: genai.TypeString},
						"scope":     {Type: genai.TypeString},
						"value":     {Type: genai.TypeString},
						"relevance": {Type: genai.TypeString},
					},
				},
			},
			"certifications": strArr,
			"keywords":       strArr,
		},
		Required: []string{"summary", "core_competencies"},
	}
}

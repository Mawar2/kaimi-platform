package outline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"github.com/Mawar2/Kaimi/internal/kobs"
	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// outlinePlannerSystemInstruction tells the model its job is to identify the
// required proposal sections from the solicitation — structure only. The Writer
// drafts the prose; the planner must not write section bodies or invent
// requirements the solicitation does not state.
const outlinePlannerSystemInstruction = "You are planning the section structure of a U.S. federal proposal. " +
	"From the solicitation text, identify the proposal sections an offeror must include — typically driven by " +
	"the Section L (instructions) and Section M (evaluation factors) requirements. " +
	"Return ONLY the section structure as JSON: for each section give a short snake_case id, a display title, " +
	"whether it is required, and a one-sentence rationale grounded in the solicitation. " +
	"Do NOT draft section content, and do NOT invent sections or requirements the solicitation does not support."

// Output-budget sizing for the planner call. Gemini 3.x (incl. gemini-3.5-flash)
// is a thinking model: its reasoning tokens are billed against MaxOutputTokens, so
// a low cap is consumed by thinking and the JSON comes back empty (the scorer
// #192 / Writer #229 failure mode). We raise the cap and bound the thinking budget
// so reasoning can never starve the structured output. The section list is compact
// JSON, so it needs less room than the Writer's prose.
const (
	maxPlannerOutputTokens = 8192
	plannerThinkingBudget  = 2048
)

// GeminiSectionPlanner is the gemini-3.5-flash implementation of SectionPlanner.
// It mirrors writer.GeminiGenerator: a Vertex AI (BackendEnterprise) client using
// Application Default Credentials, called at low temperature for a consistent,
// grounded section structure.
type GeminiSectionPlanner struct {
	client    *genai.Client
	modelName string
}

// NewGeminiSectionPlanner creates a GeminiSectionPlanner backed by Vertex AI.
//
// Requires Application Default Credentials (gcloud auth application-default login).
// Note: the Gemini 3.x family — including gemini-3.5-flash — is served only from
// the GLOBAL Vertex endpoint, so callers must pass location "global" (not a
// regional location like us-east4).
func NewGeminiSectionPlanner(ctx context.Context, projectID, location, modelName string) (*GeminiSectionPlanner, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend:  genai.BackendEnterprise,
		Project:  projectID,
		Location: location,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	return &GeminiSectionPlanner{client: client, modelName: modelName}, nil
}

// PlanSections implements SectionPlanner using gemini-3.5-flash via Vertex AI. It
// requests structured JSON and parses it into the Section list. It never returns
// an empty, error-free result: a blocked response or a section-less plan is an
// error so the agent halts rather than producing a silently empty outline.
func (g *GeminiSectionPlanner) PlanSections(ctx context.Context, opp *opportunity.Opportunity, source string) ([]Section, error) {
	contents := []*genai.Content{
		genai.NewContentFromText(buildPlannerPrompt(opp, source), genai.RoleUser),
	}

	resp, err := kobs.GenerateContent(ctx, g.client, g.modelName, contents, outlineGenerateConfig())
	if err != nil {
		return nil, fmt.Errorf("gemini API call failed: %w", err)
	}
	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("gemini returned no candidates (possibly safety-blocked)")
	}

	text := resp.Text()
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("gemini returned empty response")
	}

	sections, err := parsePlannedSections(text)
	if err != nil {
		return nil, err
	}
	if len(sections) == 0 {
		return nil, fmt.Errorf("gemini planner returned zero sections")
	}
	return sections, nil
}

// outlineGenerateConfig builds the Gemini config for a planning call, with
// thinking-token headroom (see #192/#229) and the structured-output schema so the
// model returns parseable JSON rather than free text.
func outlineGenerateConfig() *genai.GenerateContentConfig {
	temp := float32(0.2) // low temperature: consistent, grounded structure
	budget := int32(plannerThinkingBudget)
	return &genai.GenerateContentConfig{
		Temperature:       &temp,
		MaxOutputTokens:   maxPlannerOutputTokens,
		ResponseMIMEType:  "application/json",
		ResponseSchema:    outlineResponseSchema(),
		SystemInstruction: genai.NewContentFromText(outlinePlannerSystemInstruction, genai.RoleUser),
		ThinkingConfig:    &genai.ThinkingConfig{ThinkingBudget: &budget},
	}
}

// outlineResponseSchema returns the JSON schema for the planned section list.
func outlineResponseSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"sections": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"id":        {Type: genai.TypeString, Description: "Short snake_case identifier, e.g. technical_approach."},
						"title":     {Type: genai.TypeString, Description: "Display title, e.g. Technical Approach."},
						"required":  {Type: genai.TypeBoolean, Description: "Whether the solicitation makes this section mandatory."},
						"rationale": {Type: genai.TypeString, Description: "One sentence grounded in the solicitation explaining why the section is included."},
					},
					Required: []string{"id", "title", "required", "rationale"},
				},
				Description: "Ordered proposal sections the offeror must include.",
			},
		},
		Required: []string{"sections"},
	}
}

// buildPlannerPrompt assembles the grounded user prompt: the opportunity facts and
// the combined solicitation text the model plans against.
func buildPlannerPrompt(opp *opportunity.Opportunity, source string) string {
	var sb strings.Builder
	sb.WriteString("## Opportunity\n")
	fmt.Fprintf(&sb, "Title: %s\n", opp.Title)
	fmt.Fprintf(&sb, "Agency: %s\n", opp.Agency)
	fmt.Fprintf(&sb, "NAICS: %s (%s)\n", opp.NAICSCode, opp.NAICSDescription)
	if opp.SetAsideCode != "" {
		fmt.Fprintf(&sb, "Set-aside: %s\n", opp.SetAsideCode)
	}
	sb.WriteString("\n## Solicitation text (plan the sections from this)\n")
	sb.WriteString(source)
	return sb.String()
}

// plannedSections is the JSON shape the model returns under outlineResponseSchema.
type plannedSections struct {
	Sections []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Required  bool   `json:"required"`
		Rationale string `json:"rationale"`
	} `json:"sections"`
}

// parsePlannedSections decodes the model's JSON into the Section list, skipping
// any entry missing an id or title (which would render as a blank heading).
func parsePlannedSections(jsonText string) ([]Section, error) {
	var parsed plannedSections
	if err := json.Unmarshal([]byte(jsonText), &parsed); err != nil {
		return nil, fmt.Errorf("outline planner: decode sections JSON: %w", err)
	}

	sections := make([]Section, 0, len(parsed.Sections))
	for _, s := range parsed.Sections {
		id := strings.TrimSpace(s.ID)
		title := strings.TrimSpace(s.Title)
		if id == "" || title == "" {
			continue
		}
		sections = append(sections, Section{
			ID:        id,
			Title:     title,
			Required:  s.Required,
			Rationale: strings.TrimSpace(s.Rationale),
		})
	}
	return sections, nil
}

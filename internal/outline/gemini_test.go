package outline

import "testing"

// TestOutlineGenerateConfig_ThinkingHeadroom guards against #192/#229: a Gemini
// 3.x planner returns empty JSON if its thinking tokens consume the whole output
// cap. The config must raise the cap, bound the thinking budget so it cannot
// starve the output, and keep the structured-JSON schema.
func TestOutlineGenerateConfig_ThinkingHeadroom(t *testing.T) {
	cfg := outlineGenerateConfig()

	if cfg.MaxOutputTokens <= 2048 {
		t.Errorf("MaxOutputTokens = %d; must exceed a cap that 3.x thinking could consume (#229)", cfg.MaxOutputTokens)
	}
	if cfg.ThinkingConfig == nil || cfg.ThinkingConfig.ThinkingBudget == nil {
		t.Fatal("ThinkingConfig.ThinkingBudget must be set to bound thinking tokens")
	}
	if *cfg.ThinkingConfig.ThinkingBudget >= cfg.MaxOutputTokens {
		t.Errorf("thinking budget %d must leave room for output under MaxOutputTokens %d",
			*cfg.ThinkingConfig.ThinkingBudget, cfg.MaxOutputTokens)
	}
	if cfg.ResponseSchema == nil {
		t.Error("ResponseSchema must be set for structured JSON output")
	}
	if cfg.ResponseMIMEType != "application/json" {
		t.Errorf("ResponseMIMEType = %q, want application/json", cfg.ResponseMIMEType)
	}
	if cfg.SystemInstruction == nil {
		t.Error("SystemInstruction must be set")
	}
}

func TestParsePlannedSections_Valid(t *testing.T) {
	js := `{"sections":[
		{"id":"technical_approach","title":"Technical Approach","required":true,"rationale":"Section L requires it."},
		{"id":"past_performance","title":"Past Performance","required":false,"rationale":"Section M evaluation factor."}
	]}`

	got, err := parsePlannedSections(js)
	if err != nil {
		t.Fatalf("parsePlannedSections error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "technical_approach" || got[0].Title != "Technical Approach" || !got[0].Required {
		t.Errorf("section[0] = %+v, want technical_approach/required", got[0])
	}
	if got[1].Required {
		t.Errorf("section[1].Required = true, want false")
	}
	if got[0].Rationale != "Section L requires it." {
		t.Errorf("section[0].Rationale = %q", got[0].Rationale)
	}
}

func TestParsePlannedSections_SkipsBlankIDOrTitle(t *testing.T) {
	js := `{"sections":[
		{"id":"","title":"No ID","required":true,"rationale":"x"},
		{"id":"ok","title":"   ","required":true,"rationale":"y"},
		{"id":"keep","title":"Keep Me","required":true,"rationale":"z"}
	]}`

	got, err := parsePlannedSections(js)
	if err != nil {
		t.Fatalf("parsePlannedSections error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "keep" {
		t.Fatalf("got %+v, want only the well-formed section", got)
	}
}

func TestParsePlannedSections_Empty(t *testing.T) {
	got, err := parsePlannedSections(`{"sections":[]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestParsePlannedSections_BadJSON(t *testing.T) {
	if _, err := parsePlannedSections(`not json`); err == nil {
		t.Fatal("expected an error on malformed JSON, got nil")
	}
}

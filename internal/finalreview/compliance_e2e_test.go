package finalreview

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestCompliance_E2E_LiveGemini exercises the LLM compliance pass against a real
// Gemini model. It is skipped unless KAIMI_FINALREVIEW_E2E=1 is set (and GCP
// Application Default Credentials are available), so it never runs on the fast
// unit path.
//
// Per WORKFLOW.md, LLM-dependent assertions check structure and behavior, not
// exact output strings: the model returns parseable findings, and a clearly
// unmet requirement is reported as not addressed.
//
// Run with:
//
//	KAIMI_FINALREVIEW_E2E=1 GCP_PROJECT_ID=<proj> go test ./internal/finalreview -run E2E -v
func TestCompliance_E2E_LiveGemini(t *testing.T) {
	if os.Getenv("KAIMI_FINALREVIEW_E2E") == "" {
		t.Skip("set KAIMI_FINALREVIEW_E2E=1 (and GCP credentials) to run the live Gemini E2E")
	}
	project := os.Getenv("GCP_PROJECT_ID")
	if project == "" {
		t.Skip("GCP_PROJECT_ID required for the live Gemini E2E")
	}
	region := os.Getenv("GCP_REGION")
	if region == "" {
		region = "us-east4"
	}
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-2.5-pro"
	}

	ctx := context.Background()
	checker, err := NewGeminiComplianceChecker(ctx, project, region, model)
	if err != nil {
		t.Fatalf("NewGeminiComplianceChecker: %v", err)
	}

	// A draft that plainly omits the cybersecurity plan the solicitation requires.
	draft := "## Technical Approach\nWe will modernize the agency's IT systems using a phased cloud migration."
	documents := map[string]string{
		"Section_L.txt": "Offerors shall submit a cybersecurity plan describing NIST 800-171 controls. " +
			"Offerors shall provide a 5-page technical approach.",
	}

	prompt := buildCompliancePrompt(draft, nil, documents)
	raw, err := checker.CheckCompliance(ctx, complianceSystemInstruction, prompt)
	if err != nil {
		t.Fatalf("CheckCompliance: %v", err)
	}

	findings, err := parseComplianceResponse(raw)
	if err != nil {
		t.Fatalf("parseComplianceResponse: %v\nraw: %s", err, raw)
	}
	if len(findings) == 0 {
		t.Fatal("expected at least one compliance finding from the live model")
	}

	// Structure/behavior assertion: the cybersecurity requirement should be
	// flagged as not addressed somewhere in the findings.
	var sawUnmetCyber bool
	for _, f := range findings {
		if !f.Addressed && strings.Contains(strings.ToLower(f.Requirement+f.Note), "cyber") {
			sawUnmetCyber = true
		}
	}
	if !sawUnmetCyber {
		t.Errorf("expected the missing cybersecurity plan to be flagged unmet; findings=%+v", findings)
	}
}

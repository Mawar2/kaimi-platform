package writer

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Mawar2/Kaimi/internal/agent"
)

// TestRun_E2E_LiveGemini exercises the full grounded-drafting path against a real
// Gemini model. It is skipped unless KAIMI_WRITER_E2E=1 is set (and GCP Application
// Default Credentials are available), so it never runs on the fast unit path.
//
// Per WORKFLOW.md, LLM-dependent assertions check structure and behavior, not exact
// output strings: a non-empty draft that contains every section heading.
//
// Run with:
//
//	KAIMI_WRITER_E2E=1 GCP_PROJECT_ID=<proj> go test ./internal/writer -run E2E -v
func TestRun_E2E_LiveGemini(t *testing.T) {
	if os.Getenv("KAIMI_WRITER_E2E") == "" {
		t.Skip("set KAIMI_WRITER_E2E=1 (and GCP credentials) to run the live Gemini E2E")
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
	gen, err := NewGeminiGenerator(ctx, project, region, model)
	if err != nil {
		t.Fatalf("NewGeminiGenerator: %v", err)
	}

	draft, res, err := NewWithGenerator(gen).Run(ctx, groundedInput())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Status != agent.StatusSuccess {
		t.Fatalf("Status = %s, want success", res.Status)
	}
	if strings.TrimSpace(draft) == "" {
		t.Fatal("draft is empty")
	}
	for _, s := range groundedInput().Outline.Sections {
		if !strings.Contains(draft, s.Title) {
			t.Errorf("draft missing section heading %q", s.Title)
		}
	}
}

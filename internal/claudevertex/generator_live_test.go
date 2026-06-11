//go:build live

// Live test for the Claude-on-Vertex generator. It makes a real rawPredict call
// and is excluded from the default `make test` run (which must never hit live
// models). Run it explicitly once Anthropic MaaS + quota are enabled on
// us-east5 (Task 0):
//
//	GCP_PROJECT_ID=your-gcp-project CLAUDE_MODEL=claude-opus-4-8 \
//	  go test -tags live -run TestLive ./internal/claudevertex
//
// Requires Application Default Credentials (gcloud auth application-default
// login) against the configured GCP project.
package claudevertex

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLiveGenerateSection(t *testing.T) {
	project := os.Getenv("GCP_PROJECT_ID")
	if project == "" {
		t.Skip("set GCP_PROJECT_ID to run the live Claude-on-Vertex test")
	}
	region := os.Getenv("CLAUDE_REGION")
	if region == "" {
		region = "us-east5"
	}
	model := os.Getenv("CLAUDE_MODEL")
	if model == "" {
		model = "claude-opus-4-8"
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	gen, err := New(ctx, project, region, model)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	start := time.Now()
	got, err := gen.GenerateSection(ctx,
		"You write one short sentence and nothing else.",
		"Write a single sentence confirming the proposal drafting pipeline is online.",
	)
	if err != nil {
		t.Fatalf("GenerateSection (%s): %v", model, err)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatalf("%s returned empty text", model)
	}
	t.Logf("%s responded in %s: %q", model, time.Since(start).Round(time.Millisecond), got)
}

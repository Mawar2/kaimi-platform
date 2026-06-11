// Command eval is the live reliability harness entry point for Kaimi's two
// LLM-backed agents. It wires the real Gemini-backed Scorer and Writer to the
// internal/eval harness, runs the labeled seed dataset, and prints a structured
// reliability report (scorer accuracy/precision/recall and writer groundedness).
//
// This is the LIVE layer: it calls Gemini via Vertex AI and therefore needs GCP
// Application Default Credentials. It is NOT run in CI — CI only runs the fast,
// mocked unit tests in internal/eval. The metric math itself is proven there.
//
// Usage:
//
//	gcloud auth application-default login
//	GCP_PROJECT_ID=your-gcp-project go run ./cmd/eval
//
// Flags (with env fallbacks):
//
//	--project   GCP project ID         (env GCP_PROJECT_ID, required)
//	--region    GCP region             (env GCP_REGION, default us-east4)
//	--model     Gemini model name      (env GEMINI_MODEL, default gemini-2.5-pro)
//	--profile   capability profile JSON (env PROFILE_PATH, default test/fixtures/capability_profile.json)
//	--dataset   eval fixtures directory (env EVAL_DATASET_DIR, default test/fixtures/eval)
//	--agent     which agent to evaluate: scorer | writer | both (default both)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Mawar2/Kaimi/internal/eval"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/writer"
)

// Compile-time assertions that the real Gemini-backed agents satisfy the harness's
// consumer interfaces. If a future change to scorer/writer breaks this contract, the
// build fails here rather than at runtime.
var (
	_ eval.Scorer         = (*scorer.GeminiScorer)(nil)
	_ eval.SectionDrafter = (*writer.GeminiGenerator)(nil)
)

// config holds the resolved CLI configuration.
type config struct {
	project     string
	region      string
	model       string
	profilePath string
	datasetDir  string
	agent       string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "eval error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := parseConfig()
	if cfg.project == "" {
		return fmt.Errorf("GCP project ID is required (set --project or GCP_PROJECT_ID)")
	}

	ctx := context.Background()

	switch cfg.agent {
	case "scorer":
		return runScorer(ctx, &cfg)
	case "writer":
		return runWriter(ctx, &cfg)
	case "both":
		if err := runScorer(ctx, &cfg); err != nil {
			return err
		}
		return runWriter(ctx, &cfg)
	default:
		return fmt.Errorf("--agent must be scorer, writer, or both, got %q", cfg.agent)
	}
}

// runScorer evaluates the live Gemini Scorer against the labeled scorer dataset.
func runScorer(ctx context.Context, cfg *config) error {
	profile, err := loadProfile(cfg.profilePath)
	if err != nil {
		return err
	}

	cases, err := eval.LoadScorerCases(filepath.Join(cfg.datasetDir, "scorer_cases.json"), cfg.datasetDir)
	if err != nil {
		return err
	}

	gs, err := scorer.NewGeminiScorer(ctx, cfg.project, cfg.region, cfg.model)
	if err != nil {
		return fmt.Errorf("create Gemini scorer: %w", err)
	}

	report, err := eval.EvaluateScorer(ctx, gs, profile, cases)
	if err != nil {
		return fmt.Errorf("evaluate scorer: %w", err)
	}

	fmt.Println("=== Scorer reliability report ===")
	return printJSON(report)
}

// runWriter evaluates the live Gemini section drafter against the groundedness dataset.
func runWriter(ctx context.Context, cfg *config) error {
	cases, err := eval.LoadWriterCases(filepath.Join(cfg.datasetDir, "writer_cases.json"))
	if err != nil {
		return err
	}

	gen, err := writer.NewGeminiGenerator(ctx, cfg.project, cfg.region, cfg.model)
	if err != nil {
		return fmt.Errorf("create Gemini generator: %w", err)
	}

	report, err := eval.EvaluateWriter(ctx, gen, cases)
	if err != nil {
		return fmt.Errorf("evaluate writer: %w", err)
	}

	fmt.Println("=== Writer groundedness report ===")
	return printJSON(report)
}

// loadProfile reads and parses a CapabilityProfile from a JSON file.
func loadProfile(path string) (*scorer.CapabilityProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile %s: %w", path, err)
	}
	var profile scorer.CapabilityProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("parse profile %s: %w", path, err)
	}
	return &profile, nil
}

// printJSON writes v to stdout as indented JSON.
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	return nil
}

// parseConfig reads configuration from flags with environment-variable fallbacks.
func parseConfig() config {
	project := flag.String("project", getEnv("GCP_PROJECT_ID", ""), "GCP project ID (required)")
	region := flag.String("region", getEnv("GCP_REGION", "us-east4"), "GCP region")
	model := flag.String("model", getEnv("GEMINI_MODEL", "gemini-2.5-pro"), "Gemini model name")
	profilePath := flag.String("profile", getEnv("PROFILE_PATH", filepath.Join("test", "fixtures", "capability_profile.json")), "Capability profile JSON path")
	datasetDir := flag.String("dataset", getEnv("EVAL_DATASET_DIR", filepath.Join("test", "fixtures", "eval")), "Eval dataset directory")
	agent := flag.String("agent", "both", "Which agent to evaluate: scorer, writer, or both")

	flag.Parse()

	return config{
		project:     *project,
		region:      *region,
		model:       *model,
		profilePath: *profilePath,
		datasetDir:  *datasetDir,
		agent:       *agent,
	}
}

// getEnv returns the value of an environment variable or a default value.
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

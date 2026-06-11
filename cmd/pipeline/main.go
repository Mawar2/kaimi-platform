// Package main is the entry point for the Kaimi Zone-1 pipeline runner (KAI-M6).
//
// It runs the Hunter eligibility gate + Scorer as a single flow and persists
// scored opportunities to the Store. It is the first thing an operator actually
// runs to populate the queue end to end.
//
// Two modes:
//   - cached (default): reads opportunities from test fixtures and scores them with
//     the offline DeterministicScorer. Needs no API key and no GCP credentials.
//   - live: fetches from SAM.gov (SAM_API_KEY) and scores with Gemini via Vertex AI
//     (GCP Application Default Credentials).
//
// Configuration is read from flags or environment variables:
//   - MODE:                  "cached" or "live"         (default: cached)
//   - STORE_PATH:            store directory            (default: ./queue)
//   - PROFILE_PATH:          scoring profile JSON        (default: ./test/fixtures/capability_profile.json)
//   - ELIGIBILITY_PROFILE_PATH: eligibility profile JSON (default: config/profile.json)
//   - NAICS_CODES:           comma-separated overrides   (default: eligibility profile's codes)
//   - SAM_API_KEY:           required for live mode
//   - GCP_PROJECT_ID:        required for live mode
//   - GCP_REGION:            GCP region                  (default: us-east4)
//   - GEMINI_MODEL:          model name                  (default: gemini-2.5-pro)
//
// Example:
//
//	# Offline, no credentials — fixtures in, scored opportunities out
//	go run ./cmd/pipeline --mode=cached --store-path=/tmp/kaimi_queue
//
//	# Live
//	SAM_API_KEY=... GCP_PROJECT_ID=kaimi-seeker go run ./cmd/pipeline --mode=live
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/Mawar2/Kaimi/internal/config"
	"github.com/Mawar2/Kaimi/internal/pipeline"
	"github.com/Mawar2/Kaimi/internal/profile"
	"github.com/Mawar2/Kaimi/internal/samgov"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Pipeline error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := parseConfig()
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	fmt.Println("Kaimi Zone-1 pipeline starting...")
	fmt.Printf("Mode: %s\n", cfg.Mode)
	fmt.Printf("Store path: %s\n", cfg.Store.Path)
	fmt.Printf("Scoring profile: %s\n", cfg.Profile.ScoringPath)
	fmt.Printf("Eligibility profile: %s\n", cfg.Profile.EligibilityPath)

	// Abort gracefully on Ctrl+C rather than killing mid-write.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	scoringProfile, err := loadProfile(cfg.Profile.ScoringPath)
	if err != nil {
		return fmt.Errorf("failed to load scoring capability profile: %w", err)
	}

	// Load the eligibility profile used by the Hunter gate to filter set-asides.
	// This is separate from the scoring profile: it uses profile.CapabilityProfile
	// (with structured NAICS codes and set-aside flags) rather than scorer.CapabilityProfile.
	eligibilityProfile, err := profile.LoadProfile(cfg.Profile.EligibilityPath)
	if err != nil {
		return fmt.Errorf("failed to load eligibility profile: %w", err)
	}

	opportunityStore, err := store.NewJSONStore(cfg.Store.Path)
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}

	samClient, scoreEngine, err := buildBackends(ctx, &cfg)
	if err != nil {
		return err
	}

	report, err := pipeline.RunZone1(ctx, &pipeline.Zone1Deps{
		Sam:         samClient,
		Scorer:      scoreEngine,
		Store:       opportunityStore,
		Profile:     scoringProfile,
		Eligibility: eligibilityProfile,
		NAICSCodes:  cfg.Profile.NAICSCodes,
	})
	if err != nil {
		return fmt.Errorf("zone-1 run failed: %w", err)
	}

	fmt.Println("\n--- Zone-1 Summary ---")
	fmt.Printf("Fetched:   %d\n", report.Fetched)
	fmt.Printf("Eligible:  %d\n", report.Eligible)
	fmt.Printf("Dropped:   %d\n", report.Dropped)
	fmt.Printf("Scored:    %d\n", report.Scored)
	fmt.Printf("Failed:    %d\n", report.Failed)
	if report.Failed > 0 {
		fmt.Printf("\nWarning: %d opportunities could not be scored:\n", report.Failed)
		for _, e := range report.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}
	fmt.Println("\nZone-1 pipeline complete.")
	return nil
}

// buildBackends selects the SAM.gov client and Scorer for the configured mode.
// cached → fixtures + offline DeterministicScorer; live → SAM.gov + GeminiScorer.
func buildBackends(ctx context.Context, cfg *config.Config) (samgov.Client, scorer.Scorer, error) {
	switch cfg.Mode {
	case "cached":
		samClient, err := samgov.NewClient(samgov.Config{UseCached: true})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create cached SAM.gov client: %w", err)
		}
		return samClient, scorer.NewDeterministicScorer(), nil
	case "live":
		samClient, err := samgov.NewClient(samgov.Config{APIKey: cfg.SAM.APIKey, UseCached: false})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create live SAM.gov client: %w", err)
		}
		geminiScorer, err := scorer.NewGeminiScorer(ctx, cfg.GCP.ProjectID, cfg.GCP.Region, cfg.GCP.ScorerModel)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create Gemini scorer: %w", err)
		}
		return samClient, geminiScorer, nil
	default:
		return nil, nil, fmt.Errorf("unknown mode %q", cfg.Mode)
	}
}

// loadProfile reads and parses a scorer.CapabilityProfile from a JSON file.
func loadProfile(path string) (*scorer.CapabilityProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile file: %w", err)
	}
	var cp scorer.CapabilityProfile
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("parse profile JSON: %w", err)
	}
	return &cp, nil
}

// parseConfig defines the command-line surface and resolves the runner
// configuration through internal/config. The flag defaults are shown as the
// env-or-default value so `--help` reflects the effective default, but the flag
// is only treated as an override when the operator actually set it (detected
// via flag.Visit); otherwise config.Load applies the canonical env > file >
// default precedence. The set of flags, env var names, and defaults is
// unchanged from the previous hand-rolled version.
func parseConfig() (config.Config, error) {
	mode := flag.String("mode", getEnv("MODE", "cached"), "Mode: cached or live")
	storePath := flag.String("store-path", getEnv("STORE_PATH", "./queue"), "Store directory path")
	profilePath := flag.String("profile", getEnv("PROFILE_PATH", "./test/fixtures/capability_profile.json"), "Scoring capability profile JSON path (scorer.CapabilityProfile)")
	eligibilityProfilePath := flag.String("eligibility-profile", getEnv("ELIGIBILITY_PROFILE_PATH", "config/profile.json"), "Eligibility profile JSON/YAML path (profile.CapabilityProfile)")
	naics := flag.String("naics", getEnv("NAICS_CODES", ""), "Comma-separated NAICS codes (default: eligibility profile's codes)")
	project := flag.String("project", getEnv("GCP_PROJECT_ID", ""), "GCP project ID (required for live mode)")
	region := flag.String("region", getEnv("GCP_REGION", "us-east4"), "GCP region")
	model := flag.String("model", getEnv("GEMINI_MODEL", "gemini-2.5-pro"), "Gemini model name")

	flag.Parse()

	// Only forward flags the operator explicitly set so config.Load's precedence
	// (flag > env > file > default) holds; unset flags fall through to env/file.
	set := setFlags()
	f := &config.Flags{
		Mode:                   pick(set, "mode", mode),
		StorePath:              pick(set, "store-path", storePath),
		ScoringProfilePath:     pick(set, "profile", profilePath),
		EligibilityProfilePath: pick(set, "eligibility-profile", eligibilityProfilePath),
		NAICSCodes:             pick(set, "naics", naics),
		ProjectID:              pick(set, "project", project),
		Region:                 pick(set, "region", region),
		ScorerModel:            pick(set, "model", model),
	}

	cfg, err := config.Load(f)
	if err != nil {
		return config.Config{}, err
	}
	if err := cfg.ValidateMode(); err != nil {
		return config.Config{}, err
	}
	if err := cfg.ValidateLive(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

// setFlags returns the names of flags the operator explicitly set on the
// command line, so unset flags don't shadow env/file values in config.Load.
func setFlags() map[string]bool {
	set := map[string]bool{}
	flag.Visit(func(fl *flag.Flag) { set[fl.Name] = true })
	return set
}

// pick returns the flag's value pointer only when the flag was set; otherwise
// nil so config.Load falls through to env/file/default.
func pick(set map[string]bool, name string, val *string) *string {
	if set[name] {
		return val
	}
	return nil
}

// getEnv returns the value of an environment variable or a default value.
func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

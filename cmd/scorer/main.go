// Package main is the entry point for the Scorer agent.
//
// Scorer is the second agent in the Kaimi Zone 1 pipeline. It reads unscored
// opportunities from the queue, calls Gemini 2.5 Pro via Vertex AI to produce
// a 0–100 bid fit score with BID/NO_BID/REVIEW recommendation, and writes the
// scored fields back to the store. Already-scored opportunities (ScoredAt != nil)
// are skipped.
//
// Configuration is read from environment variables or command-line flags:
//   - MODE: "dry-run" or "live" (default: dry-run)
//   - GCP_PROJECT_ID: GCP project ID (required for live mode)
//   - GCP_REGION: GCP region (default: "us-east4")
//   - GEMINI_MODEL: Gemini model name (default: "gemini-2.5-pro")
//   - STORE_PATH: Path to store directory (default: "./queue")
//   - PROFILE_PATH: Path to capability profile JSON
//
// Example usage:
//
//	# Dry-run (lists opportunities that would be scored)
//	go run cmd/scorer/main.go --mode=dry-run
//
//	# Score live against Vertex AI
//	GCP_PROJECT_ID=kaimi-seeker go run cmd/scorer/main.go --mode=live
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
)

// Config holds the Scorer agent configuration.
type Config struct {
	Mode        string // "dry-run" or "live"
	ProjectID   string // GCP project ID
	Region      string // GCP region
	GeminiModel string // Gemini model name
	StorePath   string // Path to store directory
	ProfilePath string // Path to capability profile JSON
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Scorer error: %v\n", err)
		os.Exit(1)
	}
}

// run contains the main logic for the Scorer agent.
func run() error {
	config := parseConfig()
	if err := validateConfig(&config); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	fmt.Println("Scorer agent starting...")
	fmt.Printf("Mode: %s\n", config.Mode)
	fmt.Printf("Store path: %s\n", config.StorePath)
	fmt.Printf("Profile: %s\n", config.ProfilePath)

	// Load capability profile
	profile, err := loadProfile(config.ProfilePath)
	if err != nil {
		return fmt.Errorf("failed to load capability profile: %w", err)
	}

	// Initialize store
	opportunityStore, err := store.NewJSONStore(config.StorePath)
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}

	// List all opportunities
	ctx := context.Background()
	all, err := opportunityStore.List(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to list opportunities: %w", err)
	}

	// Separate unscored from already-scored
	var unscored []*opportunity.Opportunity
	for _, opp := range all {
		if opp.ScoredAt == nil {
			unscored = append(unscored, opp)
		}
	}

	fmt.Printf("Opportunities in queue: %d (unscored: %d)\n", len(all), len(unscored))

	if config.Mode == "dry-run" {
		fmt.Println("\nDry-run mode — no scoring performed. Unscored opportunities:")
		for _, opp := range unscored {
			fmt.Printf("  [%s] %s\n", opp.ID, opp.Title)
		}
		fmt.Println("\nScorer dry-run complete.")
		return nil
	}

	// Live mode: score each unscored opportunity via Gemini
	geminiScorer, err := scorer.NewGeminiScorer(ctx, config.ProjectID, config.Region, config.GeminiModel)
	if err != nil {
		return fmt.Errorf("failed to create Gemini scorer: %w", err)
	}

	scoredCount := 0
	errorCount := 0

	for _, opp := range unscored {
		fmt.Printf("Scoring: [%s] %s\n", opp.ID, opp.Title)

		if err := scorer.ScoreAndSave(ctx, geminiScorer, opportunityStore, opp, profile); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to score %s: %v\n", opp.ID, err)
			errorCount++
			continue
		}

		fmt.Printf("  Score: %.2f | %s\n", opp.Score, opp.Recommendation)
		scoredCount++
	}

	fmt.Println("\n--- Scorer Summary ---")
	fmt.Printf("Opportunities scored:  %d\n", scoredCount)
	fmt.Printf("Errors:                %d\n", errorCount)

	if errorCount > 0 {
		fmt.Printf("\nWarning: %d opportunities could not be scored\n", errorCount)
	}

	fmt.Println("\nScorer complete.")
	return nil
}

// loadProfile reads and parses a CapabilityProfile from a JSON file.
func loadProfile(path string) (*scorer.CapabilityProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read profile file: %w", err)
	}
	var profile scorer.CapabilityProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("parse profile JSON: %w", err)
	}
	return &profile, nil
}

// parseConfig reads configuration from environment variables and command-line flags.
func parseConfig() Config {
	mode := flag.String("mode", getEnv("MODE", "dry-run"), "Mode: dry-run or live")
	project := flag.String("project", getEnv("GCP_PROJECT_ID", ""), "GCP project ID (required for live mode)")
	region := flag.String("region", getEnv("GCP_REGION", "us-east4"), "GCP region")
	model := flag.String("model", getEnv("GEMINI_MODEL", "gemini-2.5-pro"), "Gemini model name")
	storePath := flag.String("store-path", getEnv("STORE_PATH", "./queue"), "Store directory path")
	profilePath := flag.String("profile", getEnv("PROFILE_PATH", "./test/fixtures/capability_profile.json"), "Capability profile JSON path")

	flag.Parse()

	return Config{
		Mode:        *mode,
		ProjectID:   *project,
		Region:      *region,
		GeminiModel: *model,
		StorePath:   *storePath,
		ProfilePath: *profilePath,
	}
}

// validateConfig validates the Scorer configuration.
func validateConfig(config *Config) error {
	switch config.Mode {
	case "dry-run", "live":
		// valid
	default:
		return fmt.Errorf("mode must be 'dry-run' or 'live', got: %s", config.Mode)
	}

	if config.Mode == "live" && config.ProjectID == "" {
		return fmt.Errorf("GCP_PROJECT_ID is required for live mode")
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

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/profile"
	"github.com/Mawar2/Kaimi/internal/samgov"
	"github.com/Mawar2/Kaimi/internal/store"
)

// TestParseNAICSCodes verifies NAICS code parsing from comma-separated strings.
func TestParseNAICSCodes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single code",
			input:    "541512",
			expected: []string{"541512"},
		},
		{
			name:     "multiple codes",
			input:    "541512,541519,541330",
			expected: []string{"541512", "541519", "541330"},
		},
		{
			name:     "codes with spaces",
			input:    "541512, 541519, 541330",
			expected: []string{"541512", "541519", "541330"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "trailing comma",
			input:    "541512,541519,",
			expected: []string{"541512", "541519"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseNAICSCodes(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d codes, got %d", len(tt.expected), len(result))
				return
			}
			for i, code := range result {
				if code != tt.expected[i] {
					t.Errorf("Expected code %q at index %d, got %q", tt.expected[i], i, code)
				}
			}
		})
	}
}

// TestValidateConfig verifies configuration validation.
func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		shouldError bool
	}{
		{
			name: "valid cached config",
			config: Config{
				Mode:       "cached",
				NAICSCodes: []string{"541512"},
				StoreType:  "json",
				StorePath:  "./queue",
			},
			shouldError: false,
		},
		{
			name: "valid live config with API key",
			config: Config{
				Mode:       "live",
				APIKey:     "test-api-key",
				NAICSCodes: []string{"541512"},
				StoreType:  "json",
				StorePath:  "./queue",
			},
			shouldError: false,
		},
		{
			name: "invalid mode",
			config: Config{
				Mode:       "invalid",
				NAICSCodes: []string{"541512"},
				StoreType:  "json",
			},
			shouldError: true,
		},
		{
			name: "live mode without API key",
			config: Config{
				Mode:       "live",
				NAICSCodes: []string{"541512"},
				StoreType:  "json",
			},
			shouldError: true,
		},
		{
			name: "no NAICS codes",
			config: Config{
				Mode:       "cached",
				NAICSCodes: []string{},
				StoreType:  "json",
			},
			shouldError: true,
		},
		{
			name: "unsupported store type",
			config: Config{
				Mode:       "cached",
				NAICSCodes: []string{"541512"},
				StoreType:  "firestore",
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(&tt.config)
			if tt.shouldError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestGetEnv verifies environment variable reading with defaults.
func TestGetEnv(t *testing.T) {
	// Set a test environment variable
	testKey := "TEST_HUNTER_VAR"
	testValue := "test-value"
	if err := os.Setenv(testKey, testValue); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer func() {
		if err := os.Unsetenv(testKey); err != nil {
			t.Errorf("Failed to unset environment variable: %v", err)
		}
	}()

	tests := []struct {
		name         string
		key          string
		defaultValue string
		expected     string
	}{
		{
			name:         "existing variable",
			key:          testKey,
			defaultValue: "default",
			expected:     testValue,
		},
		{
			name:         "non-existent variable",
			key:          "NONEXISTENT_VAR",
			defaultValue: "default",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getEnv(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestHunterIntegration is an end-to-end integration test for the Hunter agent.
//
// This test runs the complete Hunter workflow in cached mode:
// 1. Initialize SAM.gov client in cached mode
// 2. Initialize JSON store
// 3. Fetch opportunities from cached fixtures
// 4. Save opportunities to store
// 5. Verify opportunities were saved correctly
func TestHunterIntegration(t *testing.T) {
	ctx := context.Background()

	// Create temporary directory for store
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "queue")

	// Initialize SAM.gov client in cached mode
	samClient, err := samgov.NewClient(samgov.Config{
		UseCached: true,
	})
	if err != nil {
		t.Fatalf("Failed to create SAM.gov client: %v", err)
	}

	// Initialize JSON store
	opportunityStore, err := store.NewJSONStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create JSON store: %v", err)
	}

	// Fetch opportunities
	naicsCodes := []string{"541512", "541519"}
	opportunities, err := samClient.FetchByNAICS(ctx, naicsCodes)
	if err != nil {
		t.Fatalf("Failed to fetch opportunities: %v", err)
	}

	// Verify we got opportunities
	if len(opportunities) == 0 {
		t.Fatal("Expected to fetch at least one opportunity")
	}

	t.Logf("Fetched %d opportunities", len(opportunities))

	// Save opportunities to store
	savedCount := 0
	for _, opp := range opportunities {
		if err := opportunityStore.Save(ctx, opp); err != nil {
			t.Errorf("Failed to save opportunity %s: %v", opp.ID, err)
			continue
		}
		savedCount++
	}

	// Verify all opportunities were saved
	if savedCount != len(opportunities) {
		t.Errorf("Expected to save %d opportunities, saved %d", len(opportunities), savedCount)
	}

	// Verify opportunities can be retrieved from store
	for _, opp := range opportunities {
		retrieved, err := opportunityStore.Get(ctx, opp.ID)
		if err != nil {
			t.Errorf("Failed to retrieve opportunity %s: %v", opp.ID, err)
			continue
		}

		// Verify key fields match
		if retrieved.ID != opp.ID {
			t.Errorf("ID mismatch: expected %q, got %q", opp.ID, retrieved.ID)
		}
		if retrieved.Title != opp.Title {
			t.Errorf("Title mismatch for %s: expected %q, got %q", opp.ID, opp.Title, retrieved.Title)
		}
		if retrieved.Agency != opp.Agency {
			t.Errorf("Agency mismatch for %s: expected %q, got %q", opp.ID, opp.Agency, retrieved.Agency)
		}
	}

	// Verify JSON files were created
	entries, err := os.ReadDir(storePath)
	if err != nil {
		t.Fatalf("Failed to read store directory: %v", err)
	}

	jsonFileCount := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			jsonFileCount++
		}
	}

	if jsonFileCount != len(opportunities) {
		t.Errorf("Expected %d JSON files, found %d", len(opportunities), jsonFileCount)
	}

	t.Logf("Integration test complete: %d opportunities saved and verified", savedCount)
}

// TestHunterIntegration_EmptyNAICS verifies Hunter behavior with NAICS codes that return no results.
func TestHunterIntegration_EmptyNAICS(t *testing.T) {
	ctx := context.Background()

	// Create temporary directory for store
	tempDir := t.TempDir()

	// Initialize SAM.gov client in cached mode
	samClient, err := samgov.NewClient(samgov.Config{
		UseCached: true,
	})
	if err != nil {
		t.Fatalf("Failed to create SAM.gov client: %v", err)
	}

	// Initialize JSON store
	opportunityStore, err := store.NewJSONStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create JSON store: %v", err)
	}

	// Fetch opportunities with non-matching NAICS code
	naicsCodes := []string{"999999"} // This NAICS code doesn't exist in fixtures
	opportunities, err := samClient.FetchByNAICS(ctx, naicsCodes)
	if err != nil {
		t.Fatalf("Failed to fetch opportunities: %v", err)
	}

	// Verify no opportunities were found
	if len(opportunities) != 0 {
		t.Errorf("Expected 0 opportunities for non-matching NAICS, got %d", len(opportunities))
	}

	// Verify store is empty
	allOpportunities, err := opportunityStore.List(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list opportunities: %v", err)
	}

	if len(allOpportunities) != 0 {
		t.Errorf("Expected empty store, found %d opportunities", len(allOpportunities))
	}
}

// TestFilterEligible verifies that filterEligible applies the profile gate correctly.
func TestFilterEligible(t *testing.T) {
	now := time.Now().UTC()

	makeOpp := func(id, setAside string) *opportunity.Opportunity {
		return &opportunity.Opportunity{
			ID:           id,
			SetAsideCode: setAside,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
	}

	tests := []struct {
		name          string
		opportunities []*opportunity.Opportunity
		wantEligible  int
		wantDropped   int
	}{
		{
			name: "eligible kept, ineligible dropped",
			opportunities: []*opportunity.Opportunity{
				makeOpp("sba-opp", "SBA"),   // eligible
				makeOpp("8a-opp", "8A"),     // ineligible
				makeOpp("open-opp", ""),     // eligible (full-and-open)
				makeOpp("wosb-opp", "WOSB"), // ineligible
				makeOpp("hub-opp", "HZC"),   // ineligible
			},
			wantEligible: 2,
			wantDropped:  3,
		},
		{
			name: "all eligible",
			opportunities: []*opportunity.Opportunity{
				makeOpp("open1", ""),
				makeOpp("sba1", "SBA"),
				makeOpp("sbp1", "SBP"),
			},
			wantEligible: 3,
			wantDropped:  0,
		},
		{
			name: "all ineligible",
			opportunities: []*opportunity.Opportunity{
				makeOpp("8a1", "8A"),
				makeOpp("sdvosb1", "SDVOSB"),
			},
			wantEligible: 0,
			wantDropped:  2,
		},
		{
			name:          "empty input",
			opportunities: []*opportunity.Opportunity{},
			wantEligible:  0,
			wantDropped:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eligible, dropped := filterEligible(tt.opportunities, profile.BlueMeta)
			if len(eligible) != tt.wantEligible {
				t.Errorf("eligible count = %d, want %d", len(eligible), tt.wantEligible)
			}
			if dropped != tt.wantDropped {
				t.Errorf("dropped count = %d, want %d", dropped, tt.wantDropped)
			}
		})
	}
}

// TestHunterIntegration_EligibilityGate tests that the cached fixture returns the
// correct eligible subset. Two of three fixture opportunities are eligible:
//   - a1b2c3d4e5f6 (SBA): eligible
//   - f6e5d4c3b2a1 (8A):  ineligible — dropped
//   - 9z8y7x6w5v4u (""): eligible (full-and-open)
func TestHunterIntegration_EligibilityGate(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	samClient, err := samgov.NewClient(samgov.Config{UseCached: true})
	if err != nil {
		t.Fatalf("Failed to create SAM.gov client: %v", err)
	}

	opportunityStore, err := store.NewJSONStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create JSON store: %v", err)
	}

	all, err := samClient.FetchByNAICS(ctx, []string{"541512", "541519"})
	if err != nil {
		t.Fatalf("Failed to fetch opportunities: %v", err)
	}

	eligible, dropped := filterEligible(all, profile.BlueMeta)

	// Fixture has exactly one 8(a) opportunity — verify it was dropped
	if dropped != 1 {
		t.Errorf("Expected 1 dropped opportunity (8(a)), got %d", dropped)
	}
	if len(eligible) != len(all)-1 {
		t.Errorf("Expected %d eligible opportunities, got %d", len(all)-1, len(eligible))
	}

	// Save eligible opportunities and verify
	for _, opp := range eligible {
		if err := opportunityStore.Save(ctx, opp); err != nil {
			t.Errorf("Failed to save opportunity %s: %v", opp.ID, err)
		}
	}

	saved, err := opportunityStore.List(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list saved opportunities: %v", err)
	}
	if len(saved) != len(eligible) {
		t.Errorf("Expected %d saved opportunities, got %d", len(eligible), len(saved))
	}

	// Verify the 8(a) opportunity was NOT saved
	for _, opp := range saved {
		if opp.SetAsideCode == "8A" {
			t.Errorf("Ineligible 8(a) opportunity %s was saved to store", opp.ID)
		}
	}

	t.Logf("Eligibility gate test: %d fetched, %d dropped, %d saved", len(all), dropped, len(saved))
}

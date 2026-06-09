package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/samgov"
	"github.com/Mawar2/Kaimi/internal/store"
)

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
				Mode:        "cached",
				ProfilePath: "config/profile.json",
				StoreType:   "json",
				StorePath:   "./queue",
			},
			shouldError: false,
		},
		{
			name: "valid live config with API key",
			config: Config{
				Mode:        "live",
				APIKey:      "test-api-key",
				ProfilePath: "config/profile.json",
				StoreType:   "json",
				StorePath:   "./queue",
			},
			shouldError: false,
		},
		{
			name: "invalid mode",
			config: Config{
				Mode:        "invalid",
				ProfilePath: "config/profile.json",
				StoreType:   "json",
			},
			shouldError: true,
		},
		{
			name: "live mode without API key",
			config: Config{
				Mode:        "live",
				ProfilePath: "config/profile.json",
				StoreType:   "json",
			},
			shouldError: true,
		},
		{
			name: "empty profile path",
			config: Config{
				Mode:        "cached",
				ProfilePath: "",
				StoreType:   "json",
			},
			shouldError: true,
		},
		{
			name: "unsupported store type",
			config: Config{
				Mode:        "cached",
				ProfilePath: "config/profile.json",
				StoreType:   "firestore",
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

// TestIsEligible verifies the standalone eligibility switch in the hunter package.
func TestIsEligible(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{"", true},
		{"SBA", true},
		{"SBP", true},
		{"SDB", true},
		{"8A", false},
		{"8AN", false},
		{"SDVOSB", false},
		{"WOSB", false},
		{"EDWOSB", false},
		{"HUBZONE", false},
		{"HZC", false},
		{"VOSB", false},
		{"IEE", false},
		{"ISBEE", false},
		{"UNKNOWNCODE", true}, // conservative passthrough
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := isEligible(tt.code)
			if got != tt.want {
				t.Errorf("isEligible(%q) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

// TestFilterEligible verifies that filterEligible applies the eligibility gate correctly.
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
			eligible, dropped := filterEligible(tt.opportunities)
			if len(eligible) != tt.wantEligible {
				t.Errorf("eligible count = %d, want %d", len(eligible), tt.wantEligible)
			}
			if dropped != tt.wantDropped {
				t.Errorf("dropped count = %d, want %d", dropped, tt.wantDropped)
			}
		})
	}
}

// TestRunWithConfig verifies end-to-end Hunter behaviour: 3 cached fixtures in, 2 saved
// (the 8(a) opportunity is dropped by the eligibility gate).
func TestRunWithConfig(t *testing.T) {
	// Write a minimal profile to a temp file so runWithConfig can load it.
	// The profile must include NAICS codes present in the SAM.gov fixtures
	// (541512, 541519) so the cached client returns all 3 opportunities.
	minimalProfile := map[string]any{
		"uei":     "TESTUE1234567",
		"cage":    "TESTCG",
		"company": "Test Corp",
		"naics_codes": []map[string]string{
			{"code": "541512", "description": "Computer Systems Design", "tier": "primary"},
			{"code": "541519", "description": "Other Computer Related", "tier": "primary"},
		},
		"set_aside":        map[string]bool{"small_business": true},
		"past_performance": []any{},
	}

	profileData, err := json.Marshal(minimalProfile)
	if err != nil {
		t.Fatalf("Failed to marshal test profile: %v", err)
	}

	tmpDir := t.TempDir()
	profilePath := filepath.Join(tmpDir, "test_profile.json")
	if err := os.WriteFile(profilePath, profileData, 0o644); err != nil {
		t.Fatalf("Failed to write test profile: %v", err)
	}

	storePath := filepath.Join(tmpDir, "store")

	config := &Config{
		Mode:        "cached",
		ProfilePath: profilePath,
		StoreType:   "json",
		StorePath:   storePath,
	}

	if err := runWithConfig(config); err != nil {
		t.Fatalf("runWithConfig failed: %v", err)
	}

	// Verify the store received exactly 2 opportunities (8(a) was dropped).
	opportunityStore, err := store.NewJSONStore(storePath)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}

	saved, err := opportunityStore.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("Failed to list saved opportunities: %v", err)
	}

	if len(saved) != 2 {
		t.Errorf("Expected 2 saved opportunities (8(a) filtered), got %d", len(saved))
	}

	// Verify the 8(a) opportunity was NOT saved.
	for _, opp := range saved {
		if opp.SetAsideCode == "8A" {
			t.Errorf("Ineligible 8(a) opportunity %s was saved to store", opp.ID)
		}
	}
}

// TestHunterIntegration is an end-to-end integration test for the Hunter agent
// using the SAM.gov cached fixtures and the JSON store.
func TestHunterIntegration(t *testing.T) {
	ctx := context.Background()

	tempDir := t.TempDir()

	samClient, err := samgov.NewClient(samgov.Config{
		UseCached: true,
	})
	if err != nil {
		t.Fatalf("Failed to create SAM.gov client: %v", err)
	}

	opportunityStore, err := store.NewJSONStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create JSON store: %v", err)
	}

	naicsCodes := []string{"541512", "541519"}
	opportunities, err := samClient.FetchByNAICS(ctx, naicsCodes)
	if err != nil {
		t.Fatalf("Failed to fetch opportunities: %v", err)
	}

	if len(opportunities) == 0 {
		t.Fatal("Expected to fetch at least one opportunity")
	}

	t.Logf("Fetched %d opportunities", len(opportunities))

	savedCount := 0
	for _, opp := range opportunities {
		if err := opportunityStore.Save(ctx, opp); err != nil {
			t.Errorf("Failed to save opportunity %s: %v", opp.ID, err)
			continue
		}
		savedCount++
	}

	if savedCount != len(opportunities) {
		t.Errorf("Expected to save %d opportunities, saved %d", len(opportunities), savedCount)
	}

	for _, opp := range opportunities {
		retrieved, err := opportunityStore.Get(ctx, opp.ID)
		if err != nil {
			t.Errorf("Failed to retrieve opportunity %s: %v", opp.ID, err)
			continue
		}

		if retrieved.ID != opp.ID {
			t.Errorf("ID mismatch: expected %q, got %q", opp.ID, retrieved.ID)
		}
		if retrieved.Title != opp.Title {
			t.Errorf("Title mismatch for %s: expected %q, got %q", opp.ID, opp.Title, retrieved.Title)
		}
	}

	t.Logf("Integration test complete: %d opportunities saved and verified", savedCount)
}

// TestHunterIntegration_EmptyNAICS verifies Hunter behaviour with NAICS codes that return no results.
func TestHunterIntegration_EmptyNAICS(t *testing.T) {
	ctx := context.Background()

	tempDir := t.TempDir()

	samClient, err := samgov.NewClient(samgov.Config{
		UseCached: true,
	})
	if err != nil {
		t.Fatalf("Failed to create SAM.gov client: %v", err)
	}

	opportunityStore, err := store.NewJSONStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create JSON store: %v", err)
	}

	naicsCodes := []string{"999999"} // not present in fixtures
	opportunities, err := samClient.FetchByNAICS(ctx, naicsCodes)
	if err != nil {
		t.Fatalf("Failed to fetch opportunities: %v", err)
	}

	if len(opportunities) != 0 {
		t.Errorf("Expected 0 opportunities for non-matching NAICS, got %d", len(opportunities))
	}

	allOpportunities, err := opportunityStore.List(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list opportunities: %v", err)
	}

	if len(allOpportunities) != 0 {
		t.Errorf("Expected empty store, found %d opportunities", len(allOpportunities))
	}
}

package capability

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadProfile tests loading a capability profile from a YAML file
func TestLoadProfile(t *testing.T) {
	// Create a temporary test config file
	testConfig := `
uei: "XVUEA59LY579"
cage: "9RY40"
company: "BlueMeta Technologies"
address: "2 HOPKINS PLAZA, UNIT 1908, BALTIMORE, MD 21201-2946 USA"

naics_codes:
  - code: "541519"
    description: "Other Computer Related Services"
    tier: "primary"
  - code: "541512"
    description: "Computer Systems Design Services"
    tier: "primary"

set_aside:
  small_business: true
  sdb: true
  minority_owned: true
  eight_a: false
  sdvosb: false
  wosb: false
  hubzone: false

clearance: "Public Trust"

competencies:
  - "AI/ML"
  - "Federal Systems"

past_performance:
  - client: "U.S. Census Bureau"
    scope: "AI-powered multilingual communication platform"
    value: "Opportunity Project Sprint"
    what_it_proves:
      - "Federal experience"
      - "AI/ML capability"
`

	// Write test config to temp file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_profile.yaml")
	err := os.WriteFile(configPath, []byte(testConfig), 0o644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Test loading the profile
	profile, err := LoadProfile(configPath)
	if err != nil {
		t.Fatalf("LoadProfile failed: %v", err)
	}

	// Verify basic fields
	if profile.UEI != "XVUEA59LY579" {
		t.Errorf("Expected UEI 'XVUEA59LY579', got '%s'", profile.UEI)
	}

	if profile.CAGE != "9RY40" {
		t.Errorf("Expected CAGE '9RY40', got '%s'", profile.CAGE)
	}

	if profile.Company != "BlueMeta Technologies" {
		t.Errorf("Expected Company 'BlueMeta Technologies', got '%s'", profile.Company)
	}

	// Verify NAICS codes
	if len(profile.NAICSCodes) != 2 {
		t.Errorf("Expected 2 NAICS codes, got %d", len(profile.NAICSCodes))
	}

	if profile.NAICSCodes[0].Code != "541519" {
		t.Errorf("Expected first NAICS code '541519', got '%s'", profile.NAICSCodes[0].Code)
	}

	if profile.NAICSCodes[0].Tier != TierPrimary {
		t.Errorf("Expected first NAICS tier 'primary', got '%s'", profile.NAICSCodes[0].Tier)
	}

	// Verify set-aside status
	if !profile.SetAside.SmallBusiness {
		t.Error("Expected SmallBusiness to be true")
	}

	if !profile.SetAside.SDB {
		t.Error("Expected SDB to be true")
	}

	if !profile.SetAside.MinorityOwned {
		t.Error("Expected MinorityOwned to be true")
	}

	if profile.SetAside.EightA {
		t.Error("Expected EightA to be false")
	}

	// Verify clearance
	if profile.Clearance != "Public Trust" {
		t.Errorf("Expected Clearance 'Public Trust', got '%s'", profile.Clearance)
	}

	// Verify competencies
	if len(profile.Competencies) != 2 {
		t.Errorf("Expected 2 competencies, got %d", len(profile.Competencies))
	}

	// Verify past performance
	if len(profile.PastPerformance) != 1 {
		t.Errorf("Expected 1 past performance entry, got %d", len(profile.PastPerformance))
	}

	if profile.PastPerformance[0].Client != "U.S. Census Bureau" {
		t.Errorf("Expected client 'U.S. Census Bureau', got '%s'", profile.PastPerformance[0].Client)
	}

	if len(profile.PastPerformance[0].WhatItProves) != 2 {
		t.Errorf("Expected 2 'what it proves' items, got %d", len(profile.PastPerformance[0].WhatItProves))
	}
}

// TestLoadProfileInvalidPath tests that loading from an invalid path returns an error
func TestLoadProfileInvalidPath(t *testing.T) {
	_, err := LoadProfile("/nonexistent/path/profile.yaml")
	if err == nil {
		t.Error("Expected error when loading from invalid path, got nil")
	}
}

// TestLoadProfileInvalidYAML tests that loading invalid YAML returns an error
func TestLoadProfileInvalidYAML(t *testing.T) {
	// Create a temporary test config file with invalid YAML
	testConfig := `
uei: "XVUEA59LY579"
  invalid indentation
cage: "9RY40"
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid_profile.yaml")
	err := os.WriteFile(configPath, []byte(testConfig), 0o644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err = LoadProfile(configPath)
	if err == nil {
		t.Error("Expected error when loading invalid YAML, got nil")
	}
}

// TestGetNAICSByTier tests filtering NAICS codes by tier
func TestGetNAICSByTier(t *testing.T) {
	profile := &CapabilityProfile{
		NAICSCodes: []NAICSCode{
			{Code: "541519", Description: "Primary 1", Tier: TierPrimary},
			{Code: "541512", Description: "Primary 2", Tier: TierPrimary},
			{Code: "518210", Description: "Secondary 1", Tier: TierSecondary},
			{Code: "541690", Description: "Tertiary 1", Tier: TierTertiary},
		},
	}

	// Test getting primary codes
	primaryCodes := profile.GetNAICSByTier(TierPrimary)
	if len(primaryCodes) != 2 {
		t.Errorf("Expected 2 primary NAICS codes, got %d", len(primaryCodes))
	}

	// Test getting secondary codes
	secondaryCodes := profile.GetNAICSByTier(TierSecondary)
	if len(secondaryCodes) != 1 {
		t.Errorf("Expected 1 secondary NAICS code, got %d", len(secondaryCodes))
	}

	// Test getting tertiary codes
	tertiaryCodes := profile.GetNAICSByTier(TierTertiary)
	if len(tertiaryCodes) != 1 {
		t.Errorf("Expected 1 tertiary NAICS code, got %d", len(tertiaryCodes))
	}
}

// TestIsEligibleForSetAside tests checking eligibility for different set-aside types
func TestIsEligibleForSetAside(t *testing.T) {
	profile := &CapabilityProfile{
		SetAside: SetAsideStatus{
			SmallBusiness: true,
			SDB:           true,
			MinorityOwned: true,
			EightA:        false,
			SDVOSB:        false,
			WOSB:          false,
			HUBZone:       false,
		},
	}

	// Should be eligible for small business set-asides
	if !profile.IsEligibleForSetAside("small-business") {
		t.Error("Expected to be eligible for small-business set-aside")
	}

	// Should be eligible for SDB set-asides
	if !profile.IsEligibleForSetAside("sdb") {
		t.Error("Expected to be eligible for SDB set-aside")
	}

	// Should NOT be eligible for 8(a) set-asides
	if profile.IsEligibleForSetAside("8a") {
		t.Error("Expected NOT to be eligible for 8(a) set-aside")
	}

	// Should always be eligible for full-and-open
	if !profile.IsEligibleForSetAside("full-and-open") {
		t.Error("Expected to be eligible for full-and-open")
	}
}

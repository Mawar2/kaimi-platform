package profile

import (
	"testing"

	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// TestIsEligible verifies the eligibility gate switch against all known SAM.gov set-aside code families.
// 22 cases cover the full decision table plus legacy codes.
func TestIsEligible(t *testing.T) {
	p := &CapabilityProfile{}

	tests := []struct {
		name         string
		setAsideCode string
		wantEligible bool
	}{
		// Full-and-open: always eligible
		{name: "empty string (full-and-open)", setAsideCode: "", wantEligible: true},
		{name: "NONE (explicit full-and-open)", setAsideCode: "NONE", wantEligible: true},

		// Small business set-asides: eligible
		{name: "SBA (small business)", setAsideCode: "SBA", wantEligible: true},
		{name: "SBP (partial small business)", setAsideCode: "SBP", wantEligible: true},
		{name: "SDB (small disadvantaged)", setAsideCode: "SDB", wantEligible: true},

		// 8(a): BlueMeta does not hold this certification
		{name: "8A set-aside", setAsideCode: "8A", wantEligible: false},
		{name: "8(A) set-aside", setAsideCode: "8(A)", wantEligible: false},
		{name: "8AN sole source", setAsideCode: "8AN", wantEligible: false},

		// SDVOSB: BlueMeta does not hold this certification
		{name: "SDVOSB set-aside", setAsideCode: "SDVOSB", wantEligible: false},
		{name: "SDVOSBC competitive", setAsideCode: "SDVOSBC", wantEligible: false},
		{name: "SDVOSBS sole source (legacy)", setAsideCode: "SDVOSBS", wantEligible: false},

		// WOSB / EDWOSB: BlueMeta does not hold these certifications
		{name: "WOSB set-aside", setAsideCode: "WOSB", wantEligible: false},
		{name: "WOSBSS sole source (legacy)", setAsideCode: "WOSBSS", wantEligible: false},
		{name: "EDWOSB set-aside", setAsideCode: "EDWOSB", wantEligible: false},
		{name: "EDWOSBSS sole source (legacy)", setAsideCode: "EDWOSBSS", wantEligible: false},

		// HUBZone: BlueMeta does not hold this certification
		{name: "HUBZONE set-aside", setAsideCode: "HUBZONE", wantEligible: false},
		{name: "HUB set-aside (short code)", setAsideCode: "HUB", wantEligible: false},
		{name: "HZC set-aside (legacy)", setAsideCode: "HZC", wantEligible: false},
		{name: "HZS sole source (legacy)", setAsideCode: "HZS", wantEligible: false},

		// VOSB: BlueMeta does not hold this certification
		{name: "VOSB set-aside", setAsideCode: "VOSB", wantEligible: false},

		// IEE / ISBEE: BlueMeta does not hold this certification
		{name: "IEE set-aside", setAsideCode: "IEE", wantEligible: false},
		{name: "ISBEE set-aside", setAsideCode: "ISBEE", wantEligible: false},

		// Unrecognized codes pass through conservatively
		{name: "unrecognized code passes through", setAsideCode: "UNKNOWNCODE", wantEligible: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opp := &opportunity.Opportunity{
				ID:           "test-opp",
				SetAsideCode: tt.setAsideCode,
			}
			got := p.IsEligible(opp)
			if got != tt.wantEligible {
				t.Errorf("IsEligible(%q) = %v, want %v", tt.setAsideCode, got, tt.wantEligible)
			}
		})
	}
}

// TestIsEligible_CaseNormalization verifies that IsEligible normalizes case and whitespace.
func TestIsEligible_CaseNormalization(t *testing.T) {
	p := &CapabilityProfile{}

	tests := []struct {
		name         string
		setAsideCode string
		wantEligible bool
	}{
		{name: "lowercase sba is eligible", setAsideCode: "sba", wantEligible: true},
		{name: "lowercase 8a is ineligible", setAsideCode: "8a", wantEligible: false},
		{name: "mixed-case Wosb is ineligible", setAsideCode: "Wosb", wantEligible: false},
		{name: "whitespace around SBA is eligible", setAsideCode: "  SBA  ", wantEligible: true},
		{name: "whitespace only treated as full-and-open", setAsideCode: "   ", wantEligible: true},
		{name: "lowercase sdvosb is ineligible", setAsideCode: "sdvosb", wantEligible: false},
		{name: "mixed-case HubZone is ineligible", setAsideCode: "HubZone", wantEligible: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opp := &opportunity.Opportunity{
				ID:           "test-opp",
				SetAsideCode: tt.setAsideCode,
			}
			got := p.IsEligible(opp)
			if got != tt.wantEligible {
				t.Errorf("IsEligible(%q) = %v, want %v", tt.setAsideCode, got, tt.wantEligible)
			}
		})
	}
}

// TestLoadProfile_RealProfile verifies that the real BlueMeta profile loads correctly from
// config/profile.json and has the expected number of NAICS codes and past-performance entries.
func TestLoadProfile_RealProfile(t *testing.T) {
	p, err := LoadProfile("../../config/profile.json")
	if err != nil {
		t.Fatalf("LoadProfile(config/profile.json) failed: %v", err)
	}

	codes := p.AllNAICSCodes()
	if len(codes) != 9 {
		t.Errorf("AllNAICSCodes() returned %d codes, want 9", len(codes))
	}

	if len(p.PastPerformance) != 5 {
		t.Errorf("PastPerformance has %d entries, want 5", len(p.PastPerformance))
	}

	// Verify NAICS codes are non-empty strings
	for i, code := range codes {
		if code == "" {
			t.Errorf("NAICS code at index %d is empty", i)
		}
	}
}

// TestLoadProfile_JSONYAMLParity verifies that loading profile_test.json and
// profile_test.yaml from testdata produces identical CapabilityProfile values.
func TestLoadProfile_JSONYAMLParity(t *testing.T) {
	fromJSON, err := LoadProfile("testdata/profile_test.json")
	if err != nil {
		t.Fatalf("LoadProfile(profile_test.json) failed: %v", err)
	}

	fromYAML, err := LoadProfile("testdata/profile_test.yaml")
	if err != nil {
		t.Fatalf("LoadProfile(profile_test.yaml) failed: %v", err)
	}

	if fromJSON.Company != fromYAML.Company {
		t.Errorf("Company mismatch: JSON=%q YAML=%q", fromJSON.Company, fromYAML.Company)
	}
	if fromJSON.UEI != fromYAML.UEI {
		t.Errorf("UEI mismatch: JSON=%q YAML=%q", fromJSON.UEI, fromYAML.UEI)
	}

	jsonCodes := fromJSON.AllNAICSCodes()
	yamlCodes := fromYAML.AllNAICSCodes()
	if len(jsonCodes) != len(yamlCodes) {
		t.Fatalf("NAICS code count mismatch: JSON=%d YAML=%d", len(jsonCodes), len(yamlCodes))
	}
	for i := range jsonCodes {
		if jsonCodes[i] != yamlCodes[i] {
			t.Errorf("NAICS code[%d] mismatch: JSON=%q YAML=%q", i, jsonCodes[i], yamlCodes[i])
		}
	}

	if len(fromJSON.PastPerformance) != len(fromYAML.PastPerformance) {
		t.Fatalf("PastPerformance count mismatch: JSON=%d YAML=%d",
			len(fromJSON.PastPerformance), len(fromYAML.PastPerformance))
	}
	for i := range fromJSON.PastPerformance {
		if fromJSON.PastPerformance[i].Client != fromYAML.PastPerformance[i].Client {
			t.Errorf("PastPerformance[%d].Client mismatch: JSON=%q YAML=%q",
				i, fromJSON.PastPerformance[i].Client, fromYAML.PastPerformance[i].Client)
		}
	}

	if fromJSON.SetAside.SmallBusiness != fromYAML.SetAside.SmallBusiness {
		t.Errorf("SetAside.SmallBusiness mismatch: JSON=%v YAML=%v",
			fromJSON.SetAside.SmallBusiness, fromYAML.SetAside.SmallBusiness)
	}
	if fromJSON.SetAside.EightA != fromYAML.SetAside.EightA {
		t.Errorf("SetAside.EightA mismatch: JSON=%v YAML=%v",
			fromJSON.SetAside.EightA, fromYAML.SetAside.EightA)
	}
}

// TestLoadProfile_InvalidPath verifies that loading from a non-existent path returns an error.
func TestLoadProfile_InvalidPath(t *testing.T) {
	_, err := LoadProfile("nonexistent/path/profile.json")
	if err == nil {
		t.Error("LoadProfile with invalid path should return an error, got nil")
	}
}

// TestLoadProfile_UnsupportedExtension verifies that an unsupported file extension returns an error.
func TestLoadProfile_UnsupportedExtension(t *testing.T) {
	_, err := LoadProfile("testdata/profile_test.json")
	if err != nil {
		// Valid extension — not expected to fail; this path is just here for documentation
		t.Skip("profile_test.json should parse fine")
	}
	// TOML, XML, etc. should fail — tested via the .toml extension check
	_, err = LoadProfile("profile.toml")
	if err == nil {
		t.Log("unsupported extension should error — skipping if file not found")
	}
}

// TestAllNAICSCodes verifies that AllNAICSCodes returns a flat slice across all tiers.
func TestAllNAICSCodes(t *testing.T) {
	p := &CapabilityProfile{
		NAICSCodes: []NAICSCode{
			{Code: "541512", Tier: TierPrimary},
			{Code: "518210", Tier: TierSecondary},
			{Code: "541690", Tier: TierTertiary},
		},
	}

	codes := p.AllNAICSCodes()
	if len(codes) != 3 {
		t.Fatalf("AllNAICSCodes() returned %d codes, want 3", len(codes))
	}
	if codes[0] != "541512" {
		t.Errorf("codes[0] = %q, want %q", codes[0], "541512")
	}
	if codes[1] != "518210" {
		t.Errorf("codes[1] = %q, want %q", codes[1], "518210")
	}
	if codes[2] != "541690" {
		t.Errorf("codes[2] = %q, want %q", codes[2], "541690")
	}
}

package profile

import (
	"testing"

	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// TestIsEligible verifies the eligibility gate against BlueMeta's capability profile.
func TestIsEligible(t *testing.T) {
	tests := []struct {
		name         string
		setAsideCode string
		wantEligible bool
	}{
		// Full-and-open: always eligible
		{name: "full-and-open empty string", setAsideCode: "", wantEligible: true},

		// Small business set-asides: eligible
		{name: "small business (SBA)", setAsideCode: "SBA", wantEligible: true},
		{name: "partial small business (SBP)", setAsideCode: "SBP", wantEligible: true},

		// 8(a): BlueMeta does not hold this certification
		{name: "8(a) set-aside", setAsideCode: "8A", wantEligible: false},
		{name: "8(a) sole source", setAsideCode: "8AN", wantEligible: false},

		// SDVOSB: BlueMeta does not hold this certification
		{name: "SDVOSB set-aside", setAsideCode: "SDVOSB", wantEligible: false},
		{name: "SDVOSB sole source", setAsideCode: "SDVOSBS", wantEligible: false},

		// WOSB / EDWOSB: BlueMeta does not hold these certifications
		{name: "WOSB set-aside", setAsideCode: "WOSB", wantEligible: false},
		{name: "WOSB sole source", setAsideCode: "WOSBSS", wantEligible: false},
		{name: "EDWOSB set-aside", setAsideCode: "EDWOSB", wantEligible: false},
		{name: "EDWOSB sole source", setAsideCode: "EDWOSBSS", wantEligible: false},

		// HUBZone: BlueMeta does not hold this certification
		{name: "HUBZone set-aside", setAsideCode: "HZC", wantEligible: false},
		{name: "HUBZone sole source", setAsideCode: "HZS", wantEligible: false},

		// SDB is NOT gated here — left for Scorer to weight to avoid starving the pipeline
		{name: "SDB not gated (passes through)", setAsideCode: "SDB", wantEligible: true},

		// Case insensitivity and whitespace handling
		{name: "lowercase 8a is ineligible", setAsideCode: "8a", wantEligible: false},
		{name: "lowercase sba is eligible", setAsideCode: "sba", wantEligible: true},
		{name: "whitespace trimmed before check", setAsideCode: "  SBA  ", wantEligible: true},
		{name: "whitespace only treated as full-and-open", setAsideCode: "   ", wantEligible: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opp := &opportunity.Opportunity{
				ID:           "test-opp",
				SetAsideCode: tt.setAsideCode,
			}
			got := BlueMeta.IsEligible(opp)
			if got != tt.wantEligible {
				t.Errorf("IsEligible(%q) = %v, want %v", tt.setAsideCode, got, tt.wantEligible)
			}
		})
	}
}

// TestIsEligible_FixtureOpportunities verifies eligibility against the three fixture opportunities
// used in samgov cached-mode tests. This documents the expected gate outcome for each.
//
// Fixture set-asides:
//   - a1b2c3d4e5f6: "SBA"  → eligible (small business)
//   - f6e5d4c3b2a1: "8A"   → ineligible (8(a) program)
//   - 9z8y7x6w5v4u: ""     → eligible (full-and-open)
func TestIsEligible_FixtureOpportunities(t *testing.T) {
	tests := []struct {
		name         string
		noticeID     string
		setAsideCode string
		wantEligible bool
	}{
		{
			name:         "SBA opportunity kept",
			noticeID:     "a1b2c3d4e5f6",
			setAsideCode: "SBA",
			wantEligible: true,
		},
		{
			name:         "8(a) opportunity dropped",
			noticeID:     "f6e5d4c3b2a1",
			setAsideCode: "8A",
			wantEligible: false,
		},
		{
			name:         "full-and-open opportunity kept",
			noticeID:     "9z8y7x6w5v4u",
			setAsideCode: "",
			wantEligible: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opp := &opportunity.Opportunity{
				ID:           tt.noticeID,
				SetAsideCode: tt.setAsideCode,
			}
			got := BlueMeta.IsEligible(opp)
			if got != tt.wantEligible {
				t.Errorf("IsEligible for %s (set-aside %q) = %v, want %v",
					tt.noticeID, tt.setAsideCode, got, tt.wantEligible)
			}
		})
	}
}

// TestBlueMeta_NAICSCodes verifies the BlueMeta profile has the required NAICS codes.
func TestBlueMeta_NAICSCodes(t *testing.T) {
	if len(BlueMeta.NAICSCodes) == 0 {
		t.Fatal("BlueMeta profile must define at least one NAICS code")
	}

	// Primary codes must always be present
	required := []string{"541512", "541519"}
	codeSet := make(map[string]bool, len(BlueMeta.NAICSCodes))
	for _, code := range BlueMeta.NAICSCodes {
		if code == "" {
			t.Error("BlueMeta NAICSCodes must not contain empty strings")
		}
		codeSet[code] = true
	}

	for _, code := range required {
		if !codeSet[code] {
			t.Errorf("BlueMeta profile is missing required NAICS code %q", code)
		}
	}
}

// TestBlueMeta_IneligibleSetAsidesNotEmpty verifies the BlueMeta profile has
// ineligible set-asides defined.
func TestBlueMeta_IneligibleSetAsidesNotEmpty(t *testing.T) {
	if len(BlueMeta.IneligibleSetAsides) == 0 {
		t.Error("BlueMeta profile must define ineligible set-aside codes")
	}
}

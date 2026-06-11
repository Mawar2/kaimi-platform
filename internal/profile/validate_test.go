package profile

import "testing"

// TestValidate exercises the shared profile validation that both the WS-C1 JSON PUT
// and the WS-C3 SSR onboarding form rely on, so the two surfaces accept and reject
// the same profiles.
func TestValidate(t *testing.T) {
	valid := func() *CapabilityProfile {
		return &CapabilityProfile{
			Company:    "Acme Federal",
			NAICSCodes: []NAICSCode{{Code: "541512", Description: "Custom Programming", Tier: TierPrimary}},
		}
	}

	tests := []struct {
		name    string
		profile *CapabilityProfile
		wantErr bool
	}{
		{name: "valid minimal profile", profile: valid(), wantErr: false},
		{name: "nil profile", profile: nil, wantErr: true},
		{
			name:    "missing company name",
			profile: &CapabilityProfile{NAICSCodes: []NAICSCode{{Code: "541512"}}},
			wantErr: true,
		},
		{
			name:    "blank company name",
			profile: &CapabilityProfile{Company: "   ", NAICSCodes: []NAICSCode{{Code: "541512"}}},
			wantErr: true,
		},
		{
			name:    "no NAICS codes",
			profile: &CapabilityProfile{Company: "Acme"},
			wantErr: true,
		},
		{
			name:    "blank NAICS code entry",
			profile: &CapabilityProfile{Company: "Acme", NAICSCodes: []NAICSCode{{Code: " "}}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.profile)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

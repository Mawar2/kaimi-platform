package profile

import (
	"fmt"
	"strings"
)

// Validate enforces the minimal invariants a usable company profile must satisfy
// before it can ground the Hunter/Scorer/Writer. It returns nil when the profile
// is valid, or an error whose message is safe to surface to a human (the JSON API
// returns it as a 400 body; the SSR onboarding form re-renders it as a field error).
//
// It is the SINGLE source of truth for profile validation, shared by the WS-C1
// JSON PUT (internal/httpapi) and the WS-C3 SSR onboarding POST so the two
// configuration surfaces can never diverge on what counts as a valid profile.
//
// Minimal rules (kept deliberately small — richer validation belongs with the
// onboarding UI):
//   - Company name is required (the Writer addresses the proposal to it).
//   - At least one NAICS code is required, and every NAICS code string must be
//     non-blank (an empty code yields no SAM.gov query and breaks eligibility).
func Validate(p *CapabilityProfile) error {
	if p == nil {
		return fmt.Errorf("profile is nil")
	}
	if strings.TrimSpace(p.Company) == "" {
		return fmt.Errorf("profile is missing a company name")
	}
	if len(p.NAICSCodes) == 0 {
		return fmt.Errorf("profile must include at least one NAICS code")
	}
	for i, nc := range p.NAICSCodes {
		if strings.TrimSpace(nc.Code) == "" {
			return fmt.Errorf("NAICS code entries must have a non-empty code (index %d)", i)
		}
	}
	return nil
}

// Package profile defines BlueMeta Technologies' capability profile for evaluating
// federal contracting opportunities.
//
// The CapabilityProfile encodes what BlueMeta is legally eligible to bid on.
// Hunter uses this profile to gate out set-asides for programs BlueMeta does not
// hold, before opportunities reach the Scorer.
package profile

import (
	"strings"

	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// CapabilityProfile holds BlueMeta Technologies' certifications and NAICS codes,
// used to determine binary bid eligibility at the Hunter gate.
type CapabilityProfile struct {
	// NAICSCodes is the full tiered list of codes BlueMeta can perform work under.
	// Hunter uses this list when no NAICS override is specified.
	NAICSCodes []string

	// IneligibleSetAsides is the set of SAM.gov set-aside program codes BlueMeta does
	// not hold. Opportunities reserved for these programs are dropped before scoring.
	// Full-and-open (empty code) and SBA/SBP (small business) are eligible.
	// SDB (Small Disadvantaged Business) is intentionally absent — left for the Scorer
	// to weight rather than gate on, to avoid starving the pipeline.
	IneligibleSetAsides map[string]struct{}
}

// BlueMeta is the current operational capability profile for BlueMeta Technologies.
// Hunter uses this profile to gate SAM.gov opportunities by eligibility.
var BlueMeta = &CapabilityProfile{
	NAICSCodes: []string{
		"541511", // Custom Computer Programming Services
		"541512", // Computer Systems Design Services (primary)
		"541513", // Computer Facilities Management Services
		"541514", // Other Computer Related Services (misc)
		"541519", // Other Computer Related Services (primary)
		"518210", // Data Processing, Hosting, and Related Services
		"541330", // Engineering Services
		"541611", // Administrative Management and General Management Consulting
		"541618", // Other Management Consulting Services
		"541715", // Research and Development in the Physical, Engineering, and Life Sciences
	},

	IneligibleSetAsides: map[string]struct{}{
		"8A":       {}, // 8(a) Business Development Program
		"8AN":      {}, // 8(a) Sole Source
		"SDVOSB":   {}, // Service-Disabled Veteran-Owned Small Business Set-Aside
		"SDVOSBS":  {}, // SDVOSB Sole Source
		"WOSB":     {}, // Women-Owned Small Business Set-Aside
		"WOSBSS":   {}, // WOSB Sole Source
		"EDWOSB":   {}, // Economically Disadvantaged Women-Owned Small Business
		"EDWOSBSS": {}, // EDWOSB Sole Source
		"HZC":      {}, // HUBZone Set-Aside
		"HZS":      {}, // HUBZone Sole Source
	},
}

// IsEligible returns true if the opportunity passes BlueMeta's binary eligibility gate.
//
// An opportunity is eligible when its set-aside code is not reserved for a program
// BlueMeta does not hold. Full-and-open opportunities (empty set-aside) are always
// eligible. SBA and SBP (small business set-asides) are eligible. SDB is not gated here.
func (p *CapabilityProfile) IsEligible(opp *opportunity.Opportunity) bool {
	code := strings.ToUpper(strings.TrimSpace(opp.SetAsideCode))
	if code == "" {
		// Full-and-open: no set-aside restriction
		return true
	}
	_, ineligible := p.IneligibleSetAsides[code]
	return !ineligible
}

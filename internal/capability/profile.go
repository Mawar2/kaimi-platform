// Package capability provides the CapabilityProfile for BlueMeta Technologies.
//
// The CapabilityProfile is a structured representation of the company's capabilities,
// certifications, past performance, and eligibility for federal contracting set-asides.
// It is used by the Hunter agent (for hard eligibility gates) and the Scorer agent
// (for fit reasoning and bid/no-bid scoring).
//
// Design principles:
//   - Loads from YAML config file (not hardcoded) for easy updates
//   - Lightweight facts, not full narratives (Phase 3 will add knowledge base)
//   - Forward-compatible: designed to attach rich past-performance data later
//   - Self-documenting: struct tags and comments explain each field
package capability

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// NAICSTier represents the priority tier of a NAICS code.
// Primary codes are the strongest match, followed by Secondary and Tertiary.
type NAICSTier string

const (
	// TierPrimary indicates a primary NAICS code (strongest match)
	TierPrimary NAICSTier = "primary"

	// TierSecondary indicates a secondary NAICS code (moderate match)
	TierSecondary NAICSTier = "secondary"

	// TierTertiary indicates a tertiary NAICS code (weaker match)
	TierTertiary NAICSTier = "tertiary"
)

// NAICSCode represents a North American Industry Classification System code.
// NAICS codes are used to classify business types and match opportunities.
type NAICSCode struct {
	// Code is the 6-digit NAICS code (e.g., "541519")
	Code string `yaml:"code" json:"code"`

	// Description is the human-readable description of the NAICS code
	Description string `yaml:"description" json:"description"`

	// Tier indicates the priority level (primary, secondary, or tertiary)
	Tier NAICSTier `yaml:"tier" json:"tier"`
}

// SetAsideStatus represents eligibility for federal contracting set-aside programs.
// Set-asides restrict competition to certain business types (e.g., small businesses).
type SetAsideStatus struct {
	// SmallBusiness indicates eligibility for small business set-asides
	SmallBusiness bool `yaml:"small_business" json:"small_business"`

	// SDB indicates Small Disadvantaged Business certification (self-certified or SBA-certified)
	SDB bool `yaml:"sdb" json:"sdb"`

	// MinorityOwned indicates minority-owned business status (self-certified)
	MinorityOwned bool `yaml:"minority_owned" json:"minority_owned"`

	// EightA indicates 8(a) Business Development Program certification
	EightA bool `yaml:"eight_a" json:"eight_a"`

	// SDVOSB indicates Service-Disabled Veteran-Owned Small Business certification
	SDVOSB bool `yaml:"sdvosb" json:"sdvosb"`

	// WOSB indicates Women-Owned Small Business certification
	WOSB bool `yaml:"wosb" json:"wosb"`

	// HUBZone indicates Historically Underutilized Business Zone certification
	HUBZone bool `yaml:"hubzone" json:"hubzone"`
}

// PastPerformance represents a past project or contract.
// This is a lightweight fact representation (client, scope, value, capabilities proven).
// Full narratives, embeddings, and RAG will be added in Phase 3 knowledge base.
type PastPerformance struct {
	// Client is the client organization name (e.g., "U.S. Census Bureau")
	Client string `yaml:"client" json:"client"`

	// Scope is a brief description of the project/contract
	Scope string `yaml:"scope" json:"scope"`

	// Value is the contract value or engagement type (e.g., "$54,000" or "Research partnership")
	Value string `yaml:"value" json:"value"`

	// WhatItProves is a list of capabilities this project demonstrates
	// Examples: "Federal experience", "AI/ML capability", "Mobile development"
	WhatItProves []string `yaml:"what_it_proves" json:"what_it_proves"`
}

// CapabilityProfile represents BlueMeta Technologies' capabilities, certifications,
// and past performance for federal contracting.
//
// This profile is used by:
//   - Hunter agent: Hard eligibility gates (set-aside status, NAICS codes)
//   - Scorer agent: Fit reasoning (competencies, past performance, NAICS tiers)
//
// The profile is loaded from a YAML config file and designed to be forward-compatible
// with Phase 3 enhancements (knowledge base, embeddings, full past-performance narratives).
//
// Note: Named CapabilityProfile (not just Profile) per Issue #9 acceptance criteria,
// despite stuttering with package name. The explicit name improves clarity when reading
// agent code that uses this type.
type CapabilityProfile struct { //nolint:revive // Name specified in Issue #9 acceptance criteria
	// UEI is the Unique Entity Identifier (replaced DUNS in 2022)
	UEI string `yaml:"uei" json:"uei"`

	// CAGE is the Commercial and Government Entity code
	CAGE string `yaml:"cage" json:"cage"`

	// Company is the legal company name
	Company string `yaml:"company" json:"company"`

	// Address is the physical business address
	Address string `yaml:"address" json:"address"`

	// NAICSCodes is the list of NAICS codes organized by tier (primary, secondary, tertiary)
	NAICSCodes []NAICSCode `yaml:"naics_codes" json:"naics_codes"`

	// SetAside indicates eligibility for federal set-aside programs
	SetAside SetAsideStatus `yaml:"set_aside" json:"set_aside"`

	// Clearance is the security clearance level (e.g., "Public Trust", "Secret", "Top Secret")
	Clearance string `yaml:"clearance" json:"clearance"`

	// Competencies is a list of core technical and domain competencies
	// Examples: "AI/ML", "Federal Systems", "Identity Verification", "Mobile Development"
	Competencies []string `yaml:"competencies" json:"competencies"`

	// PastPerformance is a list of past projects/contracts (lightweight facts)
	// Full narratives and embeddings will be added in Phase 3
	PastPerformance []PastPerformance `yaml:"past_performance" json:"past_performance"`
}

// LoadProfile loads a CapabilityProfile from a YAML file.
//
// Example usage:
//
//	profile, err := capability.LoadProfile("config/bluemeta_profile.yaml")
//	if err != nil {
//	    log.Fatalf("Failed to load capability profile: %v", err)
//	}
func LoadProfile(path string) (*CapabilityProfile, error) {
	// Read the YAML file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile file: %w", err)
	}

	// Parse YAML into CapabilityProfile struct
	var profile CapabilityProfile
	err = yaml.Unmarshal(data, &profile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse profile YAML: %w", err)
	}

	return &profile, nil
}

// GetNAICSByTier returns all NAICS codes for a specific tier (primary, secondary, or tertiary).
//
// This is useful for the Scorer agent, which weights NAICS matches by tier:
// primary match > secondary match > tertiary match.
func (p *CapabilityProfile) GetNAICSByTier(tier NAICSTier) []NAICSCode {
	var codes []NAICSCode
	for _, code := range p.NAICSCodes {
		if code.Tier == tier {
			codes = append(codes, code)
		}
	}
	return codes
}

// IsEligibleForSetAside checks if BlueMeta is eligible for a specific set-aside type.
//
// Common set-aside types:
//   - "small-business" or "sb"
//   - "sdb" (Small Disadvantaged Business)
//   - "8a" (8(a) Business Development Program)
//   - "sdvosb" (Service-Disabled Veteran-Owned)
//   - "wosb" (Women-Owned Small Business)
//   - "hubzone" (Historically Underutilized Business Zone)
//   - "full-and-open" (unrestricted competition - always eligible)
//
// This is used by the Hunter agent to filter out opportunities we're not eligible for.
func (p *CapabilityProfile) IsEligibleForSetAside(setAsideType string) bool {
	// Normalize to lowercase for case-insensitive comparison
	setAsideType = strings.ToLower(strings.TrimSpace(setAsideType))

	switch setAsideType {
	case "small-business", "sb", "small business":
		return p.SetAside.SmallBusiness
	case "sdb", "small disadvantaged business":
		return p.SetAside.SDB
	case "minority-owned", "minority owned":
		return p.SetAside.MinorityOwned
	case "8a", "8(a)", "eight-a":
		return p.SetAside.EightA
	case "sdvosb", "service-disabled veteran-owned":
		return p.SetAside.SDVOSB
	case "wosb", "women-owned":
		return p.SetAside.WOSB
	case "hubzone", "hub zone":
		return p.SetAside.HUBZone
	case "full-and-open", "unrestricted", "":
		// Always eligible for unrestricted competition
		return true
	default:
		// Unknown set-aside type - default to not eligible (conservative)
		return false
	}
}

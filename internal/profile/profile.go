// Package profile defines BlueMeta Technologies' capability profile for evaluating
// federal contracting opportunities.
//
// The CapabilityProfile encodes what BlueMeta is legally eligible to bid on.
// Hunter uses this profile to gate out set-asides for programs BlueMeta does not
// hold, before opportunities reach the Scorer. Load a profile from a JSON or YAML
// file via LoadProfile; derive NAICS codes for SAM.gov queries via AllNAICSCodes.
package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// NAICSTier represents the priority tier of a NAICS code.
// Primary codes are the strongest match for BlueMeta's work; secondary and tertiary
// are weaker signals used by the Scorer for fit weighting.
type NAICSTier string

const (
	// TierPrimary indicates the strongest NAICS match.
	TierPrimary NAICSTier = "primary"
	// TierSecondary indicates a moderate NAICS match.
	TierSecondary NAICSTier = "secondary"
	// TierTertiary indicates a weaker NAICS match.
	TierTertiary NAICSTier = "tertiary"
)

// NAICSCode is a NAICS code with its human-readable description and tier.
type NAICSCode struct {
	Code        string    `json:"code"        yaml:"code"`
	Description string    `json:"description" yaml:"description"`
	Tier        NAICSTier `json:"tier"        yaml:"tier"`
}

// SetAsideStatus captures which federal set-aside programs BlueMeta holds certifications for.
type SetAsideStatus struct {
	SmallBusiness bool `json:"small_business" yaml:"small_business"`
	SDB           bool `json:"sdb"            yaml:"sdb"`
	MinorityOwned bool `json:"minority_owned" yaml:"minority_owned"`
	EightA        bool `json:"eight_a"        yaml:"eight_a"`
	SDVOSB        bool `json:"sdvosb"         yaml:"sdvosb"`
	WOSB          bool `json:"wosb"           yaml:"wosb"`
	HUBZone       bool `json:"hubzone"        yaml:"hubzone"`
}

// PastPerformance is a lightweight record of a prior project or contract.
// Full narratives and embeddings will be added in Phase 3.
type PastPerformance struct {
	Client       string   `json:"client"         yaml:"client"`
	Scope        string   `json:"scope"          yaml:"scope"`
	Value        string   `json:"value"          yaml:"value"`
	WhatItProves []string `json:"what_it_proves" yaml:"what_it_proves"`
}

// CapabilityProfile holds BlueMeta Technologies' certifications, NAICS codes,
// past performance, and competencies for federal contracting evaluation.
//
// Hunter uses it to:
//   - derive NAICS codes for SAM.gov queries (AllNAICSCodes)
//   - gate set-aside eligibility (IsEligible)
//
// Scorer will use it in Phase 1 for fit reasoning.
type CapabilityProfile struct {
	UEI             string            `json:"uei"              yaml:"uei"`
	CAGE            string            `json:"cage"             yaml:"cage"`
	Company         string            `json:"company"          yaml:"company"`
	Address         string            `json:"address"          yaml:"address"`
	NAICSCodes      []NAICSCode       `json:"naics_codes"      yaml:"naics_codes"`
	SetAside        SetAsideStatus    `json:"set_aside"        yaml:"set_aside"`
	Clearance       string            `json:"clearance"        yaml:"clearance"`
	Competencies    []string          `json:"competencies"     yaml:"competencies"`
	PastPerformance []PastPerformance `json:"past_performance" yaml:"past_performance"`

	// Scoring carries the curated, weighted signals the Scorer consumes that are
	// not mechanically derivable from the eligibility facts above. Folding them in
	// here lets one profile file feed both the Hunter gate and the Scorer; the
	// Scorer view is derived via ToScorerProfile. Optional: an empty Scoring block
	// yields an empty scorer profile (safe for eligibility-only deployments).
	Scoring ScoringHints `json:"scoring" yaml:"scoring"`
}

// LoadProfile reads a CapabilityProfile from path. The file format is determined
// by the extension: .json for JSON, .yaml or .yml for YAML.
func LoadProfile(path string) (*CapabilityProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile file %q: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	var p CapabilityProfile
	switch ext {
	case ".json":
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("failed to parse profile JSON %q: %w", path, err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("failed to parse profile YAML %q: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("unsupported profile file extension %q (use .json, .yaml, or .yml)", ext)
	}

	return &p, nil
}

// AllNAICSCodes returns a flat slice of all NAICS code strings across all tiers,
// in the order they appear in the profile. Use this to build SAM.gov query parameters.
func (p *CapabilityProfile) AllNAICSCodes() []string {
	codes := make([]string, 0, len(p.NAICSCodes))
	for _, nc := range p.NAICSCodes {
		codes = append(codes, nc.Code)
	}
	return codes
}

// IsEligible returns true if the opportunity is eligible for BlueMeta to bid on,
// based on its SAM.gov set-aside code.
//
// The decision table covers all known set-aside families with conservative passthrough
// for unrecognized codes (to avoid false negatives):
//
//	""  / "NONE"                → eligible (full-and-open)
//	"SBA" / "SBP" / "SDB"      → eligible (small-business)
//	"8A" / "8(A)" / "8AN"      → ineligible (8(a) cert not held)
//	"SDVOSB" / "SDVOSBC"       → ineligible (SDVOSB cert not held)
//	"WOSB" / "EDWOSB"          → ineligible (WOSB cert not held)
//	"HUBZONE" / "HUB"          → ineligible (HUBZone cert not held)
//	"VOSB"                     → ineligible (VOSB cert not held)
//	"IEE" / "ISBEE"            → ineligible (Indian enterprise cert not held)
//	legacy codes (HZC, HZS, …) → same as modern equivalents
//	unrecognized               → eligible (conservative pass-through)
func (p *CapabilityProfile) IsEligible(opp *opportunity.Opportunity) bool {
	code := strings.ToUpper(strings.TrimSpace(opp.SetAsideCode))
	switch code {
	case "", "NONE":
		return true
	case "SBA", "SBP", "SDB":
		return true
	case "8A", "8(A)", "8AN":
		return false
	case "SDVOSB", "SDVOSBC", "SDVOSBS":
		return false
	case "WOSB", "WOSBSS", "EDWOSB", "EDWOSBSS":
		return false
	case "HUBZONE", "HUB", "HZC", "HZS":
		return false
	case "VOSB":
		return false
	case "IEE", "ISBEE":
		return false
	default:
		// Unrecognized code: pass through conservatively to avoid false negatives.
		return true
	}
}

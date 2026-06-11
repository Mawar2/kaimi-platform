package scorer

import "github.com/Mawar2/Kaimi/internal/profile"

// CapabilityProfile describes a company's capabilities used for bid/no-bid scoring.
//
// This is distinct from the eligibility profile in internal/profile: the eligibility
// profile gates out legally ineligible set-asides at the Hunter; this profile provides
// weighted signals for the Scorer's LLM call so Gemini can synthesize a nuanced 0–100
// score rather than a binary pass/fail.
type CapabilityProfile struct {
	// Company is the legal/marketing name of the company the proposal is written
	// for (e.g., "Example Federal Co"). The Writer uses it to address the
	// proposal to the correct company instead of a hardcoded name. Optional: an
	// empty value falls back to a generic phrasing.
	Company string `json:"company"`

	// PrimaryNAICS are the company's core NAICS codes (highest scoring weight).
	// An exact match on these codes strongly favors a BID recommendation.
	PrimaryNAICS []string `json:"primary_naics"`

	// SecondaryNAICS are adjacent NAICS codes the company can compete in
	// (moderate weight). A match here provides positive signal but lower
	// confidence than a primary match.
	SecondaryNAICS []string `json:"secondary_naics"`

	// CompetencyTags are keywords describing the company's technical areas
	// (e.g., "cloud migration", "Zero Trust", "system integration").
	// Case-insensitive substring matches against opportunity title and description.
	CompetencyTags []string `json:"competency_tags"`

	// PastPerformance lists agency names and capability areas where the company
	// has prior contracts. Overlap with the opportunity's agency or description
	// signals strong relevance to the LLM.
	PastPerformance []string `json:"past_performance"`

	// SDBStatus indicates whether the company is a Small Disadvantaged Business.
	// When true and the opportunity's set-aside code matches QualifyingSetAsides,
	// the SDB factor is passed to the LLM as a positive signal.
	SDBStatus bool `json:"sdb_status"`

	// QualifyingSetAsides are the set-aside codes for which the company qualifies
	// when SDBStatus is true (e.g., "SDB", "SBA").
	// Case-insensitive comparison against the opportunity's SetAsideCode.
	QualifyingSetAsides []string `json:"qualifying_set_asides"`
}

// FromProfile projects the single company profile (profile.CapabilityProfile)
// into the flattened view the Scorer consumes (CapabilityProfile).
//
// This is the unification point for WS-A3: the Hunter eligibility gate and the
// Scorer now read ONE profile file; the Scorer's input is derived here rather
// than loaded from a second hand-maintained JSON. The projection is pinned by a
// golden-file parity test against the pre-unification scorer profile.
//
// The dependency points the correct way: the scorer engine imports the
// foundation profile package, not the reverse. SDBStatus is derived from the
// authoritative eligibility flag (SetAside.SDB) so the Scorer's SDB signal can
// never drift from the company's actual certification. The remaining
// scoring-only signals come from p.Scoring (see profile.ScoringHints).
func FromProfile(p *profile.CapabilityProfile) CapabilityProfile {
	return CapabilityProfile{
		Company:             p.Company,
		PrimaryNAICS:        p.Scoring.PrimaryNAICS,
		SecondaryNAICS:      p.Scoring.SecondaryNAICS,
		CompetencyTags:      p.Scoring.CompetencyTags,
		PastPerformance:     p.Scoring.PastPerformance,
		SDBStatus:           p.SetAside.SDB,
		QualifyingSetAsides: p.Scoring.QualifyingSetAsides,
	}
}

package scorer

// CapabilityProfile describes a company's capabilities used for bid/no-bid scoring.
//
// This is distinct from the eligibility profile in internal/profile: the eligibility
// profile gates out legally ineligible set-asides at the Hunter; this profile provides
// weighted signals for the Scorer's LLM call so Gemini can synthesize a nuanced 0–100
// score rather than a binary pass/fail.
type CapabilityProfile struct {
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

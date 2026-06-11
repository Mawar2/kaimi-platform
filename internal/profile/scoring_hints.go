package profile

// ScoringHints carries the curated, weighted signals the Scorer consumes that
// are NOT mechanically derivable from the eligibility facts in CapabilityProfile.
//
// Before WS-A3 these lived in a separate hand-maintained file
// (config/bluemeta_scorer_profile.json) that had to be kept in sync with the
// eligibility profile by hand. They are folded into the single CapabilityProfile
// so one file feeds both the Hunter gate and the Scorer. The Scorer derives its
// flattened view from this profile via scorer.FromProfile (which imports this
// package — the foundation profile package does NOT depend on the scorer engine).
//
// Why they are not derived from the eligibility fields:
//   - CompetencyTags are lowercase keyword expansions (e.g. "machine learning",
//     "Google Cloud") tuned for case-insensitive substring matching against
//     opportunity text — a different shape and intent than the title-case
//     Competencies used for human-facing display.
//   - PastPerformance here is one prose sentence per engagement (matched as a
//     substring signal), not the structured client/scope/value records.
//   - PrimaryNAICS/SecondaryNAICS are the Scorer's weighting buckets, which do
//     not map one-to-one onto the eligibility profile's primary/secondary/tertiary
//     tiers (the Scorer folds some tertiary codes into its secondary bucket and
//     omits others).
//
// Keeping them explicit makes the Scorer's inputs auditable and provably stable.
type ScoringHints struct {
	// PrimaryNAICS are the codes the Scorer weights most heavily (exact match
	// strongly favors BID).
	PrimaryNAICS []string `json:"primary_naics" yaml:"primary_naics"`

	// SecondaryNAICS are adjacent codes the Scorer weights moderately.
	SecondaryNAICS []string `json:"secondary_naics" yaml:"secondary_naics"`

	// CompetencyTags are case-insensitive keywords matched against the
	// opportunity's title and description.
	CompetencyTags []string `json:"competency_tags" yaml:"competency_tags"`

	// PastPerformance is one prose sentence per engagement, matched as a
	// substring signal against the opportunity's agency/description.
	PastPerformance []string `json:"past_performance" yaml:"past_performance"`

	// QualifyingSetAsides are the set-aside codes that activate the SDB signal
	// when the company is an SDB (see SetAsideStatus.SDB).
	QualifyingSetAsides []string `json:"qualifying_set_asides" yaml:"qualifying_set_asides"`
}

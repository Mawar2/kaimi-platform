package profile

import "github.com/Mawar2/Kaimi/internal/scorer"

// ScoringHints carries the curated, weighted signals the Scorer consumes that
// are NOT mechanically derivable from the eligibility facts above.
//
// Before WS-A3 these lived in a separate hand-maintained file
// (config/bluemeta_scorer_profile.json) that had to be kept in sync with the
// eligibility profile by hand. They are folded into the single CapabilityProfile
// so one file feeds both the Hunter gate and the Scorer.
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

// ToScorerProfile projects the single company profile into the flattened view the
// Scorer consumes (scorer.CapabilityProfile).
//
// This is the unification point for WS-A3: the Hunter eligibility gate and the
// Scorer now read ONE profile file; the Scorer's input is derived here rather than
// loaded from a second hand-maintained JSON. The projection is pinned by a
// golden-file parity test against the pre-unification scorer profile.
//
// SDBStatus is derived from the authoritative eligibility flag (SetAside.SDB) so
// the Scorer's SDB signal can never drift from the company's actual certification.
// The remaining scoring-only signals come from Scoring (see ScoringHints).
func (p *CapabilityProfile) ToScorerProfile() scorer.CapabilityProfile {
	return scorer.CapabilityProfile{
		PrimaryNAICS:        p.Scoring.PrimaryNAICS,
		SecondaryNAICS:      p.Scoring.SecondaryNAICS,
		CompetencyTags:      p.Scoring.CompetencyTags,
		PastPerformance:     p.Scoring.PastPerformance,
		SDBStatus:           p.SetAside.SDB,
		QualifyingSetAsides: p.Scoring.QualifyingSetAsides,
	}
}

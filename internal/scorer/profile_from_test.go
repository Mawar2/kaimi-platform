package scorer

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/Mawar2/Kaimi/internal/profile"
)

// TestFromProfile_GoldenParity is the WS-A3 parity guard. It proves the single
// company profile (config/profile.json) derives a scorer.CapabilityProfile that
// is semantically identical to the hand-maintained scorer view the Scorer
// consumed before unification (config/bluemeta_scorer_profile.json).
//
// Before WS-A3 the Scorer loaded config/bluemeta_scorer_profile.json directly and
// the Hunter loaded config/profile.json — two files kept in sync by hand. This
// test pins the derivation so that ONE file can feed both without changing the
// Scorer's inputs.
func TestFromProfile_GoldenParity(t *testing.T) {
	p, err := profile.LoadProfile("../../config/profile.json")
	if err != nil {
		t.Fatalf("LoadProfile(config/profile.json) failed: %v", err)
	}

	got := FromProfile(p)

	// The golden file is the exact scorer view that was hand-maintained and
	// consumed by the Scorer/Writer before unification.
	data, err := os.ReadFile("../../config/bluemeta_scorer_profile.json")
	if err != nil {
		t.Fatalf("read golden scorer profile: %v", err)
	}
	var want CapabilityProfile
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatalf("parse golden scorer profile: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("derived scorer profile does not match golden config/bluemeta_scorer_profile.json\n got: %#v\nwant: %#v", got, want)
	}
}

// TestFromProfile_FieldMapping documents the field-by-field mapping so a
// regression in any single field is reported precisely rather than as one opaque
// DeepEqual failure.
func TestFromProfile_FieldMapping(t *testing.T) {
	p := &profile.CapabilityProfile{
		NAICSCodes: []profile.NAICSCode{
			{Code: "541519", Tier: profile.TierPrimary},
			{Code: "518210", Tier: profile.TierSecondary},
		},
		SetAside: profile.SetAsideStatus{SDB: true},
		Scoring: profile.ScoringHints{
			PrimaryNAICS:        []string{"541519"},
			SecondaryNAICS:      []string{"518210", "519290"},
			CompetencyTags:      []string{"AI/ML", "cloud"},
			PastPerformance:     []string{"U.S. Census Bureau: built SpeakEase"},
			QualifyingSetAsides: []string{"SDB", "SBA"},
		},
	}

	got := FromProfile(p)

	if !reflect.DeepEqual(got.PrimaryNAICS, []string{"541519"}) {
		t.Errorf("PrimaryNAICS = %v", got.PrimaryNAICS)
	}
	if !reflect.DeepEqual(got.SecondaryNAICS, []string{"518210", "519290"}) {
		t.Errorf("SecondaryNAICS = %v", got.SecondaryNAICS)
	}
	if !reflect.DeepEqual(got.CompetencyTags, []string{"AI/ML", "cloud"}) {
		t.Errorf("CompetencyTags = %v", got.CompetencyTags)
	}
	if !reflect.DeepEqual(got.PastPerformance, []string{"U.S. Census Bureau: built SpeakEase"}) {
		t.Errorf("PastPerformance = %v", got.PastPerformance)
	}
	if !got.SDBStatus {
		t.Errorf("SDBStatus = false, want true (derived from SetAside.SDB)")
	}
	if !reflect.DeepEqual(got.QualifyingSetAsides, []string{"SDB", "SBA"}) {
		t.Errorf("QualifyingSetAsides = %v", got.QualifyingSetAsides)
	}
}

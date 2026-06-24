package capabilitymap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/profile"
)

func sampleProfile() *profile.CapabilityProfile {
	return &profile.CapabilityProfile{
		Company: "Ey3 Technologies",
		NAICSCodes: []profile.NAICSCode{
			{Code: "541512", Description: "Computer Systems Design", Tier: profile.TierPrimary},
			{Code: "541519", Description: "Other Computer Related Services", Tier: profile.TierSecondary},
		},
		Competencies: []string{"Zero Trust architecture", "Cloud migration"},
		SetAside:     profile.SetAsideStatus{SmallBusiness: true, SDB: true, WOSB: true},
		PastPerformance: []profile.PastPerformance{
			{Client: "DHS CISA", Scope: "Continuous monitoring", Value: "$6.8M"},
		},
		Scoring: profile.ScoringHints{
			CompetencyTags: []string{"zero trust", "DevSecOps", "Zero Trust architecture"}, // dup (case) of competency
		},
	}
}

// TestDeterministicBuilder: a profile-only map captures company, NAICS, certs,
// competencies (de-duped), past performance, keywords, and source provenance.
func TestDeterministicBuilder(t *testing.T) {
	b := NewDeterministicBuilder()
	fixed := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return fixed }

	m, err := b.Build(context.Background(), sampleProfile(), []ContextDoc{{Name: "capabilities.pdf", Text: "..."}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if m.Company != "Ey3 Technologies" {
		t.Errorf("company = %q", m.Company)
	}
	if m.Model != "deterministic" || !m.GeneratedAt.Equal(fixed) {
		t.Errorf("model/time = %q / %v", m.Model, m.GeneratedAt)
	}
	if len(m.NAICS) != 2 || m.NAICS[0] != "541512" {
		t.Errorf("NAICS = %v", m.NAICS)
	}
	// Competencies + tags merged, de-duped case-insensitively ("Zero Trust architecture"
	// appears in both Competencies and CompetencyTags → one entry).
	names := map[string]bool{}
	for _, c := range m.CoreCompetencies {
		if names[c.Name] {
			t.Errorf("duplicate competency %q", c.Name)
		}
		names[c.Name] = true
	}
	if !names["Zero Trust architecture"] || !names["DevSecOps"] {
		t.Errorf("competencies missing expected entries: %v", m.CoreCompetencies)
	}
	// Set-aside flags → certifications.
	if len(m.Certifications) != 3 {
		t.Errorf("certifications = %v, want 3 (SB, SDB, WOSB)", m.Certifications)
	}
	if len(m.PastPerformance) != 1 || m.PastPerformance[0].Client != "DHS CISA" {
		t.Errorf("past performance = %v", m.PastPerformance)
	}
	if len(m.Keywords) == 0 {
		t.Error("expected keywords")
	}
	// Provenance: onboarding profile + the doc name.
	if len(m.Sources) != 2 || m.Sources[0] != "onboarding profile" || m.Sources[1] != "capabilities.pdf" {
		t.Errorf("sources = %v", m.Sources)
	}
	if m.Summary == "" {
		t.Error("expected a summary")
	}
}

// TestDeterministicBuilderNilProfile: a nil profile yields an empty-but-valid map.
func TestDeterministicBuilderNilProfile(t *testing.T) {
	m, err := NewDeterministicBuilder().Build(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Build(nil): %v", err)
	}
	if m == nil || m.Model != "deterministic" {
		t.Fatalf("expected an empty deterministic map, got %+v", m)
	}
}

// TestJSONStoreRoundTrip: save then load returns the same map; missing is ErrNotFound.
func TestJSONStoreRoundTrip(t *testing.T) {
	s, err := NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}

	if _, err := s.Load(); !errors.Is(err, ErrNotFound) {
		t.Errorf("Load before save: got %v, want ErrNotFound", err)
	}

	m, _ := NewDeterministicBuilder().Build(context.Background(), sampleProfile(), nil)
	if err := s.Save(m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Company != m.Company || len(got.NAICS) != len(m.NAICS) || len(got.CoreCompetencies) != len(m.CoreCompetencies) {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

// TestJSONStoreSaveNil: saving nil errors rather than writing garbage.
func TestJSONStoreSaveNil(t *testing.T) {
	s, _ := NewJSONStore(t.TempDir())
	if err := s.Save(nil); err == nil {
		t.Error("Save(nil) should error")
	}
}

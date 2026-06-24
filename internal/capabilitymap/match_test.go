package capabilitymap

import (
	"slices"
	"testing"
)

func TestCapabilityMapMatch(t *testing.T) {
	m := &CapabilityMap{
		CoreCompetencies: []Competency{{Name: "Zero Trust Architecture"}, {Name: "Cloud Migration"}},
		Keywords:         []string{"DevSecOps", "Continuous Monitoring", "AI"}, // "AI" is too short to count
		Domains:          []string{"Cybersecurity", "Homeland Security"},
	}

	// A solicitation that clearly maps to the company's capabilities.
	got := m.Match("Zero Trust Architecture Implementation and Continuous Monitoring for Cybersecurity")
	if got.Coverage == 0 {
		t.Fatal("expected matches")
	}
	if !slices.Contains(got.Competencies, "Zero Trust Architecture") {
		t.Errorf("competencies = %v, want Zero Trust Architecture", got.Competencies)
	}
	if !slices.Contains(got.Keywords, "Continuous Monitoring") {
		t.Errorf("keywords = %v, want Continuous Monitoring", got.Keywords)
	}
	if !slices.Contains(got.Domains, "Cybersecurity") {
		t.Errorf("domains = %v, want Cybersecurity", got.Domains)
	}
	if slices.Contains(got.Keywords, "AI") {
		t.Errorf("short keyword 'AI' should be excluded as noise")
	}
	if got.Coverage != len(got.Competencies)+len(got.Keywords)+len(got.Domains) {
		t.Errorf("coverage = %d, want sum of matched terms", got.Coverage)
	}

	// An unrelated solicitation matches nothing.
	none := m.Match("Janitorial services for the Department of the Interior")
	if none.Coverage > 0 {
		t.Errorf("unrelated opportunity should not match: %+v", none)
	}

	// Case-insensitive.
	ci := m.Match("zero trust architecture upgrade")
	if !slices.Contains(ci.Competencies, "Zero Trust Architecture") {
		t.Errorf("match should be case-insensitive: %+v", ci)
	}

	// Nil map is safe.
	var nilMap *CapabilityMap
	if nilMap.Match("anything").Coverage > 0 {
		t.Error("nil map should match nothing")
	}

	// Word-boundary: a keyword must match as a whole word, not a bare substring.
	bm := &CapabilityMap{Keywords: []string{"Cloud"}}
	if bm.Match("partly cloudy skies").Coverage > 0 {
		t.Error("'Cloud' must not match inside 'cloudy' (word-boundary)")
	}
	if bm.Match("secure cloud migration").Coverage == 0 {
		t.Error("'Cloud' should match the whole word in 'secure cloud migration'")
	}
}

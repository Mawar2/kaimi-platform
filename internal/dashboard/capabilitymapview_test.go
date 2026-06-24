package dashboard_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/capabilitymap"
	"github.com/Mawar2/Kaimi/internal/dashboard"
)

// memCapMap is an in-memory capabilitymap.Store for the view tests.
type memCapMap struct{ m *capabilitymap.CapabilityMap }

func (s *memCapMap) Load() (*capabilitymap.CapabilityMap, error) {
	if s.m == nil {
		return nil, capabilitymap.ErrNotFound
	}
	return s.m, nil
}
func (s *memCapMap) Save(m *capabilitymap.CapabilityMap) error { s.m = m; return nil }

// TestCapabilityMapView covers the three states: a built map renders its content; an
// empty store shows "not built yet"; no reader wired shows the unavailable note.
func TestCapabilityMapView(t *testing.T) {
	// Built map.
	store := &memCapMap{m: &capabilitymap.CapabilityMap{
		Company:          "Ey3 Technologies",
		Summary:          "Ey3 is a small business specializing in zero-trust cybersecurity.",
		CoreCompetencies: []capabilitymap.Competency{{Name: "Zero Trust Architecture", Description: "ZTA for federal agencies", Evidence: []string{"cap.txt"}}},
		Certifications:   []string{"Small Business"},
		NAICS:            []string{"541512"},
		Model:            "gemini-2.5-pro",
		GeneratedAt:      time.Now(),
	}}
	h := newOnboardingHandler(t, dashboard.WithCapabilityMap(store))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/capability-map", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("built map status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Your capability map", "Ey3 is a small business", "Zero Trust Architecture", "cap.txt", "gemini-2.5-pro"} {
		if !strings.Contains(body, want) {
			t.Errorf("built-map view missing %q", want)
		}
	}

	// Not built yet (empty store).
	h2 := newOnboardingHandler(t, dashboard.WithCapabilityMap(&memCapMap{}))
	r2 := httptest.NewRecorder()
	h2.ServeHTTP(r2, httptest.NewRequest(http.MethodGet, "/capability-map", http.NoBody))
	if r2.Code != http.StatusOK || !strings.Contains(r2.Body.String(), "Not built yet") {
		t.Errorf("empty store: status=%d, want 200 + 'Not built yet'", r2.Code)
	}

	// Unavailable (no reader wired).
	h3 := newOnboardingHandler(t)
	r3 := httptest.NewRecorder()
	h3.ServeHTTP(r3, httptest.NewRequest(http.MethodGet, "/capability-map", http.NoBody))
	if r3.Code != http.StatusOK || !strings.Contains(r3.Body.String(), "not available") {
		t.Errorf("no reader: status=%d, want 200 + 'not available'", r3.Code)
	}
}

package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/capabilitymap"
	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/store"
)

// TestDetailShowsCapabilityMatch: the opportunity detail surfaces a "Why this fits your
// capabilities" section that matches the tenant's capability map against the solicitation,
// without changing the score. Unrelated opportunities show the no-direct-match note.
func TestDetailShowsCapabilityMatch(t *testing.T) {
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	now := time.Now()
	fit := &opportunity.Opportunity{ID: "0123456789abcdef0123456789abcdef", Title: "Zero Trust Architecture Implementation", Agency: "DHS", Score: 0.8, ScoredAt: &now}
	miss := &opportunity.Opportunity{ID: "fedcba9876543210fedcba9876543210", Title: "Grounds Maintenance Services", Agency: "GSA", Score: 0.3, ScoredAt: &now}
	for _, o := range []*opportunity.Opportunity{fit, miss} {
		if err := s.Save(context.Background(), o); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	capMap := &memCapMap{m: &capabilitymap.CapabilityMap{
		CoreCompetencies: []capabilitymap.Competency{{Name: "Zero Trust Architecture"}},
		Keywords:         []string{"DevSecOps"},
		Domains:          []string{"Cybersecurity"},
	}}
	h := dashboard.NewHandler(dashboard.NewService(s), dashboard.WithCapabilityMap(capMap))

	// Matching opportunity → section present with the matched competency.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/opportunity/"+fit.ID, http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Why this fits your capabilities") {
		t.Errorf("detail missing the capability-match section")
	}
	if !strings.Contains(body, "Zero Trust Architecture") {
		t.Errorf("detail did not surface the matched competency")
	}

	// Unrelated opportunity → the no-direct-match note.
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/opportunity/"+miss.ID, http.NoBody))
	if !strings.Contains(rec2.Body.String(), "No direct capability matches") {
		t.Errorf("unrelated opportunity should show the no-match note")
	}
}

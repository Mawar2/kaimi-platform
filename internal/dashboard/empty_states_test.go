package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/store"
)

// newSeededService builds a dashboard.Service over a JSON store holding two scored
// opportunities (both recommend BID), plus the fixed "now" used to seed them. It
// gives the empty-state tests a populated, non-NO_BID queue.
func newSeededService(t *testing.T) (*dashboard.Service, time.Time) {
	t.Helper()
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	opps := []*opportunity.Opportunity{
		{ID: "alpha", Title: "Alpha Opp", Agency: "GSA", Score: 0.9, Recommendation: "BID", ScoredAt: &now, UpdatedAt: now, CreatedAt: now},
		{ID: "beta", Title: "Beta Opp", Agency: "DOD", Score: 0.7, Recommendation: "BID", ScoredAt: &now, UpdatedAt: now, CreatedAt: now},
	}
	for _, opp := range opps {
		if err := s.Save(context.Background(), opp); err != nil {
			t.Fatalf("seed opportunity: %v", err)
		}
	}
	return dashboard.NewService(s), now
}

// WS-C4 first-run / empty-state copy. These literals must appear verbatim in the
// rendered Triage so the empty-state assertions stay coupled to the shipped copy.
const (
	// emptyConfiguredHeading / emptyConfiguredHint guide an operator whose profile
	// IS configured but who has no opportunities yet (a brand-new deployment before
	// the first scheduled hunt). It is friendlier than the filtered-empty copy.
	emptyConfiguredHeading = "No opportunities yet"
	emptyConfiguredHint    = "The pipeline runs on a schedule"

	// emptyFilteredHeading is the pre-existing filtered-empty copy: the queue has
	// rows, but none match the active recommendation filter.
	emptyFilteredHeading = "Nothing here right now"

	// onboardingPrompt is the WS-C3 first-run entry point shown when no company
	// profile is configured. WS-C4 must let this take priority over any
	// "no opportunities" empty state so a brand-new deployment shows ONE message.
	onboardingPrompt = "Complete onboarding"
)

// TestTriageEmptyStateProfileConfigured proves that with a configured profile and
// an empty store, the Triage renders the friendly first-run empty-state panel
// (not blank, not the filtered-empty copy) and emits no opportunity row markup.
func TestTriageEmptyStateProfileConfigured(t *testing.T) {
	svc := newEmptyService(t) // empty JSON store
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	h := dashboard.NewHandler(svc, dashboard.WithProfileStore(&memProfileStore{p: validProfile()}))
	h.Now = func() time.Time { return now }

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", http.NoBody))

	body := rr.Body.String()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !contains(body, "empty2") {
		t.Errorf("configured + empty store must render the designed empty-state panel")
	}
	if !contains(body, emptyConfiguredHeading) {
		t.Errorf("configured + empty store must show %q; got:\n%s", emptyConfiguredHeading, body)
	}
	if !contains(body, emptyConfiguredHint) {
		t.Errorf("configured + empty store must show the schedule hint %q", emptyConfiguredHint)
	}
	// The friendly first-run copy must not be the filtered-empty copy, and there
	// must be no row cards.
	if contains(body, emptyFilteredHeading) {
		t.Errorf("configured + empty store should NOT show the filtered-empty copy %q", emptyFilteredHeading)
	}
	if contains(body, `class="orow`) {
		t.Errorf("empty store must render no opportunity rows")
	}
	// No profile prompt — the profile IS configured.
	if contains(body, onboardingPrompt) {
		t.Errorf("configured profile must not show the onboarding prompt")
	}
}

// TestTriageEmptyStateNoProfile proves that with NO profile, hitting "/" first-run-
// redirects to the onboarding wizard rather than rendering the board — the tester
// completes setup before they ever reach the opportunities screen.
func TestTriageEmptyStateNoProfile(t *testing.T) {
	svc := newEmptyService(t) // empty JSON store
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	h := dashboard.NewHandler(svc, dashboard.WithProfileStore(&memProfileStore{})) // ErrProfileNotFound
	h.Now = func() time.Time { return now }

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", http.NoBody))

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (first-run redirect to onboarding)", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/onboarding" {
		t.Errorf("redirect = %q, want /onboarding", loc)
	}
}

// TestTriageNonEmptyNoEmptyState proves that a populated queue renders the normal
// table of row cards and none of the empty-state copy.
func TestTriageNonEmptyNoEmptyState(t *testing.T) {
	svc, now := newSeededService(t)
	h := dashboard.NewHandler(svc, dashboard.WithProfileStore(&memProfileStore{p: validProfile()}))
	h.Now = func() time.Time { return now }

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", http.NoBody))

	body := rr.Body.String()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !contains(body, `class="orow`) {
		t.Errorf("non-empty store must render opportunity row cards")
	}
	if contains(body, emptyConfiguredHeading) || contains(body, emptyFilteredHeading) {
		t.Errorf("non-empty store must not render any empty-state copy")
	}
}

// TestProposalsEmptyState proves the Proposals view renders a sensible empty
// state (not a blank page) when no opportunities have been selected into work.
func TestProposalsEmptyState(t *testing.T) {
	svc := newEmptyService(t)
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	h := dashboard.NewHandler(svc)
	h.Now = func() time.Time { return now }

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/proposals", http.NoBody))

	body := rr.Body.String()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !contains(body, "empty2") || !contains(body, "No active proposals") {
		t.Errorf("empty proposals view must render the designed empty state")
	}
}

// TestSubmittedEmptyState proves the Submitted archive renders a sensible empty
// state (not a blank list) when nothing has been submitted.
func TestSubmittedEmptyState(t *testing.T) {
	svc := newEmptyService(t)
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	h := dashboard.NewHandler(svc)
	h.Now = func() time.Time { return now }

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/submitted", http.NoBody))

	body := rr.Body.String()
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !contains(body, "empty2") || !contains(body, "Nothing submitted yet") {
		t.Errorf("empty submitted archive must render the designed empty state")
	}
}

// TestTriageFilteredEmptyKeepsFilteredCopy proves that when the queue HAS rows but
// none match the active recommendation filter, the existing filtered-empty copy
// shows (not the first-run "no opportunities yet" copy).
func TestTriageFilteredEmptyKeepsFilteredCopy(t *testing.T) {
	svc, now := newSeededService(t)
	h := dashboard.NewHandler(svc, dashboard.WithProfileStore(&memProfileStore{p: validProfile()}))
	h.Now = func() time.Time { return now }

	rr := httptest.NewRecorder()
	// NO_BID filter matches none of the seeded rows.
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/?rec=NO_BID", http.NoBody))

	body := rr.Body.String()
	if !contains(body, "empty2") {
		t.Errorf("filtered-empty must render the designed empty-state panel")
	}
	if !contains(body, emptyFilteredHeading) {
		t.Errorf("filtered-empty must keep the filter copy %q", emptyFilteredHeading)
	}
	if contains(body, emptyConfiguredHeading) {
		t.Errorf("filtered-empty (queue has rows) must NOT show the first-run no-opportunities copy")
	}
}

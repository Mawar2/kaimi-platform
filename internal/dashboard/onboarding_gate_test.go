package dashboard_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Mawar2/Kaimi/internal/dashboard"
)

// TestOnboardingGateRequiresProfile proves the app-wide onboarding gate: until a company
// profile is saved, every dashboard page redirects to /onboarding, while the onboarding flow
// and the public pages stay reachable. Once a profile exists, dashboard pages render.
func TestOnboardingGateRequiresProfile(t *testing.T) {
	noProfile := func() *dashboard.Handler {
		return dashboard.NewHandler(newEmptyService(t), dashboard.WithProfileStore(&memProfileStore{}))
	}

	// Gated dashboard pages bounce to onboarding when no profile is saved.
	for _, p := range []string{"/", "/proposals", "/submitted", "/team"} {
		rr := httptest.NewRecorder()
		noProfile().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, http.NoBody))
		if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/onboarding" {
			t.Errorf("%s without profile: status=%d loc=%q, want 303 -> /onboarding", p, rr.Code, rr.Header().Get("Location"))
		}
	}

	// Exempt pages render without a profile so setup (and the public site) stay reachable.
	for _, p := range []string{"/onboarding", "/home", "/help", "/privacy"} {
		rr := httptest.NewRecorder()
		noProfile().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, p, http.NoBody))
		if rr.Code != http.StatusOK {
			t.Errorf("%s without profile: status=%d, want 200 (exempt from the gate)", p, rr.Code)
		}
	}

	// With a profile saved, a dashboard page renders normally.
	h := dashboard.NewHandler(newEmptyService(t), dashboard.WithProfileStore(&memProfileStore{p: validProfile()}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if rr.Code != http.StatusOK {
		t.Errorf("/ with profile: status=%d, want 200", rr.Code)
	}
}

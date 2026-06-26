package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mawar2/Kaimi/internal/dashboard"
)

// TestOnboardingReflectsConfiguredSAMKey: when a SAM key is already configured for the
// deployment (the injected check reports true), a returning tester sees the "connected"
// state — NOT a "re-enter your key" prompt — even without the per-session ?sam_saved param.
// When no key is configured, it shows the pending state. This fixes the returning-tester
// dead end (the secret store is write-only, so onboarding previously couldn't tell).
func TestOnboardingReflectsConfiguredSAMKey(t *testing.T) {
	noopSaver := func(context.Context, string) error { return nil }

	render := func(configured bool) string {
		h := newOnboardingHandler(t,
			dashboard.WithProfileStore(&memProfileStore{}),
			dashboard.WithSAMKeySaver(noopSaver),
			dashboard.WithSAMKeyConfiguredCheck(func() bool { return configured }))
		rec := httptest.NewRecorder()
		// No ?sam_saved — this is a fresh page load, like a returning tester's.
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding", http.NoBody))
		return rec.Body.String()
	}

	if got := render(true); !strings.Contains(got, "SAM.gov connected") {
		t.Errorf("key configured: want 'SAM.gov connected' (returning tester not blocked)")
	}
	if got := render(false); !strings.Contains(got, "SAM.gov pending") {
		t.Errorf("no key configured: want 'SAM.gov pending'")
	}
}

// TestSAMSavedBannerOnlyAfterSave proves the "SAM.gov key saved" SUCCESS banner appears
// ONLY right after the user saves a key (the ?sam_saved=1 redirect), NOT on first load just
// because the deployment already has a key configured. The first-load banner read as the
// license being mistaken for a SAM key (a tester reported "it says SAM.gov key saved but
// that's my license key"). The "connected" summary state is unaffected (asserted above).
func TestSAMSavedBannerOnlyAfterSave(t *testing.T) {
	const banner = "SAM.gov key saved. Your next hunt will use it."
	noopSaver := func(context.Context, string) error { return nil }

	render := func(target string) string {
		h := newOnboardingHandler(t,
			dashboard.WithProfileStore(&memProfileStore{}),
			dashboard.WithSAMKeySaver(noopSaver),
			// Key IS configured for the deployment — the case that previously showed the
			// banner on first load.
			dashboard.WithSAMKeyConfiguredCheck(func() bool { return true }))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, http.NoBody))
		return rec.Body.String()
	}

	if got := render("/onboarding"); strings.Contains(got, banner) {
		t.Errorf("first load with a configured key must NOT show the 'key saved' success banner")
	}
	if got := render("/onboarding?sam_saved=1"); !strings.Contains(got, banner) {
		t.Errorf("after a save (?sam_saved=1) the success banner must show")
	}
}

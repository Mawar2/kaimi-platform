package dashboard_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Mawar2/Kaimi/internal/dashboard"
)

// settingsHandler wires a Handler with a saved profile (so the onboarding gate lets
// /settings through) and a fixed authenticated identity + CSRF token for the POST tests.
// It returns the handler and the underlying store so tests can assert persistence.
func settingsHandler(t *testing.T, csrf string) (*dashboard.Handler, *memProfileStore) {
	t.Helper()
	pstore := &memProfileStore{p: validProfile()}
	h := newOnboardingHandler(t,
		dashboard.WithProfileStore(pstore),
		identityOpt("owner@ey3.com", csrf))
	return h, pstore
}

// TestSettingsGETShowsCurrentProfile proves GET /settings renders (200) the editor
// pre-filled with the saved profile's values and the Settings chrome.
func TestSettingsGETShowsCurrentProfile(t *testing.T) {
	h, _ := settingsHandler(t, "tok")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/settings", http.NoBody))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Settings",           // page heading / nav
		`action="/settings"`, // the form posts back to /settings
		`name="company"`,     // the profile form is present
		"Acme Federal",       // the saved company name is pre-filled
		"541512",             // the saved NAICS code is pre-filled
		`value="tok"`,        // the CSRF token is embedded
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /settings body missing %q", want)
		}
	}
}

// TestSettingsPostValidSavesAndRedirects proves a valid, authenticated, CSRF-matched POST
// persists the updated profile and redirects (PRG, 303) to /settings?saved=1.
func TestSettingsPostValidSavesAndRedirects(t *testing.T) {
	const token = "tok"
	h, pstore := settingsHandler(t, token)

	form := url.Values{}
	form.Set("company", "Updated Co")
	form.Set("naics", "541511|Web Programming|primary\n541512")
	form.Set("sa_small_business", "on")
	form.Set("csrf_token", token)

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST /settings status = %d, want 303 (PRG); body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "saved=1") {
		t.Errorf("Location = %q, want a PRG redirect to /settings?saved=1", loc)
	}

	// The store now holds the updated profile.
	saved, err := pstore.Load()
	if err != nil {
		t.Fatalf("expected a saved profile, got error: %v", err)
	}
	if saved.Company != "Updated Co" {
		t.Errorf("saved company = %q, want Updated Co", saved.Company)
	}
	if len(saved.NAICSCodes) != 2 {
		t.Fatalf("saved NAICS count = %d, want 2", len(saved.NAICSCodes))
	}
	if !saved.SetAside.SmallBusiness {
		t.Errorf("set-aside small_business not persisted")
	}

	// The follow-up GET shows the success banner and the updated values.
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/settings?saved=1", http.NoBody))
	gb := getRec.Body.String()
	if !strings.Contains(gb, "Profile updated.") {
		t.Errorf("follow-up GET missing the success banner")
	}
	if !strings.Contains(gb, "Updated Co") {
		t.Errorf("follow-up GET did not reflect the saved company name")
	}
}

// TestSettingsPostInvalidReRendersAndPersistsNothing proves an invalid POST (empty
// company) re-renders the form with the error, returns 400, and leaves the store
// unchanged.
func TestSettingsPostInvalidReRendersAndPersistsNothing(t *testing.T) {
	const token = "tok"
	h, pstore := settingsHandler(t, token)

	form := url.Values{}
	form.Set("company", "") // empty company fails profile.Validate
	form.Set("naics", "541512")
	form.Set("csrf_token", token)

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid POST status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "company") {
		t.Errorf("re-rendered form missing a validation error mentioning the company: %q", rec.Body.String())
	}

	// The store still holds the ORIGINAL profile (nothing persisted on a failed save).
	saved, err := pstore.Load()
	if err != nil {
		t.Fatalf("expected the original profile to remain, got error: %v", err)
	}
	if saved.Company != "Acme Federal" {
		t.Errorf("store company = %q, want the unchanged Acme Federal", saved.Company)
	}
}

// TestSettingsPostCSRFMismatchRejected proves a POST with a wrong CSRF token fails closed
// (403) and persists nothing.
func TestSettingsPostCSRFMismatchRejected(t *testing.T) {
	h, pstore := settingsHandler(t, "good-token")

	form := url.Values{}
	form.Set("company", "Hacker Co")
	form.Set("naics", "541512")
	form.Set("csrf_token", "wrong-token")

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("CSRF-mismatch POST status = %d, want 403", rec.Code)
	}
	if saved, _ := pstore.Load(); saved.Company != "Acme Federal" {
		t.Errorf("store mutated on a CSRF failure: company = %q", saved.Company)
	}
}

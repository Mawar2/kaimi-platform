package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/drivetoken"
	"github.com/Mawar2/Kaimi/internal/profile"
	"github.com/Mawar2/Kaimi/internal/store"
)

// newEmptyService builds a dashboard.Service over a fresh empty JSON store so the app
// shell renders without seeding opportunities (onboarding tests care about the
// onboarding page, not the queue).
func newEmptyService(t *testing.T) *dashboard.Service {
	t.Helper()
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	return dashboard.NewService(s)
}

// memProfileStore is an in-memory profile.ProfileStore for onboarding tests: no temp
// files, deterministic ErrProfileNotFound before a Save.
type memProfileStore struct {
	p *profile.CapabilityProfile
}

func (m *memProfileStore) Load() (*profile.CapabilityProfile, error) {
	if m.p == nil {
		return nil, profile.ErrProfileNotFound
	}
	// Return a copy so callers cannot mutate the stored profile in place.
	cp := *m.p
	return &cp, nil
}

func (m *memProfileStore) Save(p *profile.CapabilityProfile) error {
	if p == nil {
		return profile.ErrProfileNotFound
	}
	cp := *p
	m.p = &cp
	return nil
}

// validProfile is a minimal profile that passes profile.Validate.
func validProfile() *profile.CapabilityProfile {
	return &profile.CapabilityProfile{
		Company:    "Acme Federal",
		UEI:        "ABC123DEF456",
		NAICSCodes: []profile.NAICSCode{{Code: "541512", Description: "Custom Programming", Tier: profile.TierPrimary}},
	}
}

// newOnboardingHandler builds a dashboard.Handler wired with the given options plus a
// real (empty) store-backed service so the shell renders.
func newOnboardingHandler(t *testing.T, opts ...dashboard.Option) *dashboard.Handler {
	t.Helper()
	return dashboard.NewHandler(newEmptyService(t), opts...)
}

// TestOnboardingGETEmpty proves GET /onboarding renders (200) with an empty profile
// form when no profile has been saved.
func TestOnboardingGETEmpty(t *testing.T) {
	h := newOnboardingHandler(t, dashboard.WithProfileStore(&memProfileStore{}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding", http.NoBody))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /onboarding status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Company profile") {
		t.Errorf("body missing the company-profile section: %q", body)
	}
	if !strings.Contains(body, `name="company"`) {
		t.Errorf("body missing the company form field")
	}
}

// TestOnboardingGETPrefilled proves GET /onboarding pre-fills the form from a saved
// profile.
func TestOnboardingGETPrefilled(t *testing.T) {
	h := newOnboardingHandler(t, dashboard.WithProfileStore(&memProfileStore{p: validProfile()}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding", http.NoBody))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Acme Federal") {
		t.Errorf("body did not pre-fill the saved company name: %q", body)
	}
	if !strings.Contains(body, "541512") {
		t.Errorf("body did not pre-fill the saved NAICS code")
	}
}

// TestOnboardingNoStore503 proves the onboarding routes degrade to 503 when no
// profile store is wired, mirroring how the JSON API degrades.
func TestOnboardingNoStore503(t *testing.T) {
	h := newOnboardingHandler(t) // no WithProfileStore

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding", http.NoBody))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /onboarding with no store = %d, want 503", rec.Code)
	}
}

// TestOnboardingPostValidPersistsAndRedirects proves a valid POST persists the
// profile and redirects (PRG, 303) to /onboarding?saved=1, and that a subsequent GET
// reflects the saved values.
func TestOnboardingPostValidPersistsAndRedirects(t *testing.T) {
	pstore := &memProfileStore{}
	h := newOnboardingHandler(t, dashboard.WithProfileStore(pstore))

	form := url.Values{}
	form.Set("company", "New Co")
	form.Set("uei", "ZZZ999")
	form.Set("naics", "541511|Web Programming|primary\n541512")
	form.Set("sa_small_business", "on")

	req := httptest.NewRequest(http.MethodPost, "/onboarding/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST status = %d, want 303 (PRG); body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "saved=1") {
		t.Errorf("Location = %q, want PRG redirect with saved=1", loc)
	}

	// Persistence happened.
	saved, err := pstore.Load()
	if err != nil {
		t.Fatalf("expected a saved profile, got error: %v", err)
	}
	if saved.Company != "New Co" {
		t.Errorf("saved company = %q, want New Co", saved.Company)
	}
	if len(saved.NAICSCodes) != 2 {
		t.Fatalf("saved NAICS count = %d, want 2", len(saved.NAICSCodes))
	}
	if !saved.SetAside.SmallBusiness {
		t.Errorf("set-aside small_business not persisted")
	}

	// A follow-up GET reflects the saved profile.
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/onboarding", http.NoBody))
	if !strings.Contains(getRec.Body.String(), "New Co") {
		t.Errorf("follow-up GET did not reflect the saved company name")
	}
}

// TestOnboardingPostInvalidReRendersAndPersistsNothing proves an invalid POST (no
// NAICS) re-renders the form with an error, returns 400, and persists nothing.
func TestOnboardingPostInvalidReRendersAndPersistsNothing(t *testing.T) {
	pstore := &memProfileStore{}
	h := newOnboardingHandler(t, dashboard.WithProfileStore(pstore))

	form := url.Values{}
	form.Set("company", "Has Name But No NAICS")
	// No naics field → fails profile.Validate.

	req := httptest.NewRequest(http.MethodPost, "/onboarding/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid POST status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "NAICS") {
		t.Errorf("re-rendered form missing a validation error mentioning NAICS: %q", body)
	}
	// The submitted company value is preserved on the re-render.
	if !strings.Contains(body, "Has Name But No NAICS") {
		t.Errorf("re-rendered form did not preserve the submitted company value")
	}
	// Nothing was persisted.
	if _, err := pstore.Load(); err == nil {
		t.Errorf("expected no profile persisted on validation failure")
	}
}

// TestOnboardingSharedValidation proves the SSR form and the shared profile.Validate
// reject the SAME input: the exact body that fails the form must also fail Validate,
// and the exact body that passes must also pass — so the two configuration surfaces
// cannot diverge.
func TestOnboardingSharedValidation(t *testing.T) {
	// The invalid form above (company, no NAICS) maps to this profile.
	invalid := &profile.CapabilityProfile{Company: "Has Name But No NAICS"}
	if err := profile.Validate(invalid); err == nil {
		t.Fatalf("profile.Validate accepted a profile the SSR form rejects (no NAICS)")
	}
	// The valid form maps to this profile.
	valid := &profile.CapabilityProfile{
		Company:    "New Co",
		NAICSCodes: []profile.NAICSCode{{Code: "541511"}, {Code: "541512"}},
	}
	if err := profile.Validate(valid); err != nil {
		t.Fatalf("profile.Validate rejected a profile the SSR form accepts: %v", err)
	}
}

// TestOnboardingDriveStatus proves the page reflects connected vs not-connected Drive
// state via the injected DriveStatusFunc.
func TestOnboardingDriveStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   dashboard.DriveStatus
		contains string
		excludes string
	}{
		{
			name:     "not connected shows connect button",
			status:   dashboard.DriveStatus{Configured: true, Connected: false},
			contains: "Connect Drive",
		},
		{
			name:     "connected shows target",
			status:   dashboard.DriveStatus{Configured: true, Connected: true, Target: "drive-xyz"},
			contains: "drive-xyz",
			excludes: "Connect Drive",
		},
		{
			name:     "not configured shows administrator note",
			status:   dashboard.DriveStatus{Configured: false},
			contains: "not enabled in this deployment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newOnboardingHandler(t,
				dashboard.WithProfileStore(&memProfileStore{}),
				dashboard.WithDriveStatus(func() dashboard.DriveStatus { return tt.status }))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding", http.NoBody))
			body := rec.Body.String()
			if !strings.Contains(body, tt.contains) {
				t.Errorf("body missing %q; got %q", tt.contains, body)
			}
			if tt.excludes != "" && strings.Contains(body, tt.excludes) {
				t.Errorf("body unexpectedly contains %q", tt.excludes)
			}
		})
	}
}

// TestDriveStatusFromStores proves the store-backed reader reports connected/target
// from the drivetoken stores.
func TestDriveStatusFromStores(t *testing.T) {
	dir := t.TempDir()
	tokens, err := drivetoken.NewJSONTokenStore(dir)
	if err != nil {
		t.Fatalf("token store: %v", err)
	}
	targets, err := drivetoken.NewJSONTargetStore(dir)
	if err != nil {
		t.Fatalf("target store: %v", err)
	}
	fn := dashboard.DriveStatusFromStores(tokens, targets)

	// Before any connect: configured, not connected, no target.
	if st := fn(); !st.Configured || st.Connected || st.Target != "" {
		t.Fatalf("pre-connect status = %+v, want configured/not-connected/no-target", st)
	}

	if err := targets.Save(drivetoken.Target{DriveID: "drive-1"}); err != nil {
		t.Fatalf("save target: %v", err)
	}
	if st := fn(); st.Target != "drive-1" {
		t.Errorf("post-target status target = %q, want drive-1", st.Target)
	}
}

// TestOnboardingShowsSignedInIdentity proves the page renders the signed-in email
// from the injected IdentityFunc.
func TestOnboardingShowsSignedInIdentity(t *testing.T) {
	h := newOnboardingHandler(t,
		dashboard.WithProfileStore(&memProfileStore{}),
		dashboard.WithIdentity(func(context.Context) (dashboard.Identity, bool) {
			return dashboard.Identity{Email: "user@example.com", CSRFToken: "tok-123"}, true
		}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding", http.NoBody))
	body := rec.Body.String()
	if !strings.Contains(body, "user@example.com") {
		t.Errorf("body did not show signed-in email: %q", body)
	}
	if !strings.Contains(body, `value="tok-123"`) {
		t.Errorf("body did not embed the CSRF token in the form")
	}
}

// TestOnboardingCSRFRequired proves that when an identity with a CSRF token is wired,
// a POST with a missing/bad token is rejected (403) and persists nothing, while the
// matching token is accepted.
func TestOnboardingCSRFRequired(t *testing.T) {
	const token = "good-token"
	identity := dashboard.WithIdentity(func(context.Context) (dashboard.Identity, bool) {
		return dashboard.Identity{Email: "u@example.com", CSRFToken: token}, true
	})

	baseForm := func() url.Values {
		f := url.Values{}
		f.Set("company", "CSRF Co")
		f.Set("naics", "541512")
		return f
	}

	// Bad token → 403, nothing persisted.
	pstore := &memProfileStore{}
	h := newOnboardingHandler(t, dashboard.WithProfileStore(pstore), identity)
	form := baseForm()
	form.Set("csrf_token", "wrong")
	req := httptest.NewRequest(http.MethodPost, "/onboarding/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bad-CSRF POST status = %d, want 403", rec.Code)
	}
	if _, err := pstore.Load(); err == nil {
		t.Errorf("bad-CSRF POST persisted a profile")
	}

	// Good token → 303, persisted.
	pstore2 := &memProfileStore{}
	h2 := newOnboardingHandler(t, dashboard.WithProfileStore(pstore2), identity)
	form2 := baseForm()
	form2.Set("csrf_token", token)
	req2 := httptest.NewRequest(http.MethodPost, "/onboarding/profile", strings.NewReader(form2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec2 := httptest.NewRecorder()
	h2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusSeeOther {
		t.Fatalf("good-CSRF POST status = %d, want 303; body=%s", rec2.Code, rec2.Body.String())
	}
	if _, err := pstore2.Load(); err != nil {
		t.Errorf("good-CSRF POST did not persist a profile: %v", err)
	}
}

// TestFirstRunLinkAppearsWhenNoProfile proves the main dashboard surfaces a
// "Complete onboarding" entry point when no company profile is configured, and hides
// it once a profile exists.
func TestFirstRunLinkAppearsWhenNoProfile(t *testing.T) {
	// No profile → first-run link present.
	h := newOnboardingHandler(t, dashboard.WithProfileStore(&memProfileStore{}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Complete onboarding") {
		t.Errorf("first-run link missing when no profile configured")
	}

	// Profile present → no first-run link.
	h2 := newOnboardingHandler(t, dashboard.WithProfileStore(&memProfileStore{p: validProfile()}))
	rec2 := httptest.NewRecorder()
	h2.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if strings.Contains(rec2.Body.String(), "Complete onboarding") {
		t.Errorf("first-run link should be hidden once a profile is configured")
	}
}

// TestOnboardingSAMKeyStatusOnly proves the SAM.gov section is status/guidance only:
// it explains the key is a deployment secret and never offers an input for it.
func TestOnboardingSAMKeyStatusOnly(t *testing.T) {
	h := newOnboardingHandler(t, dashboard.WithProfileStore(&memProfileStore{}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding", http.NoBody))
	body := rec.Body.String()
	if !strings.Contains(body, "SAM_API_KEY") {
		t.Errorf("SAM.gov section missing the deployment-secret guidance")
	}
	if strings.Contains(body, `name="sam_api_key"`) {
		t.Errorf("SAM.gov section must NOT accept the raw key via a form field")
	}
}

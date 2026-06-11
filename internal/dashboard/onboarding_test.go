package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"

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

// identityOpt wires a fixed authenticated identity + CSRF token for the POST tests.
func identityOpt(email, csrf string) dashboard.Option {
	return dashboard.WithIdentity(func(context.Context) (dashboard.Identity, bool) {
		return dashboard.Identity{Email: email, CSRFToken: csrf}, true
	})
}

// TestOnboardingPostValidPersistsAndRedirects proves a valid POST (authenticated +
// matching CSRF token) persists the profile and redirects (PRG, 303) to
// /onboarding?saved=1, and that a subsequent GET reflects the saved values.
func TestOnboardingPostValidPersistsAndRedirects(t *testing.T) {
	const token = "session-csrf"
	pstore := &memProfileStore{}
	h := newOnboardingHandler(t, dashboard.WithProfileStore(pstore), identityOpt("u@example.com", token))

	form := url.Values{}
	form.Set("company", "New Co")
	form.Set("uei", "ZZZ999")
	form.Set("naics", "541511|Web Programming|primary\n541512")
	form.Set("sa_small_business", "on")
	form.Set("csrf_token", token)

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
	const token = "session-csrf"
	pstore := &memProfileStore{}
	h := newOnboardingHandler(t, dashboard.WithProfileStore(pstore), identityOpt("u@example.com", token))

	form := url.Values{}
	form.Set("company", "Has Name But No NAICS")
	form.Set("csrf_token", token)
	// No naics field → fails profile.Validate (after passing the auth/CSRF gate).

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

// postOnboardingProfile drives a POST /onboarding/profile against a handler wired with
// the given options, supplying a valid company+NAICS body plus the given CSRF token
// value (omitted when csrf == ""). It returns the recorder and the store so callers can
// assert the status and whether anything persisted.
func postOnboardingProfile(t *testing.T, csrf string, opts ...dashboard.Option) (*httptest.ResponseRecorder, *memProfileStore) {
	t.Helper()
	pstore := &memProfileStore{}
	allOpts := append([]dashboard.Option{dashboard.WithProfileStore(pstore)}, opts...)
	h := newOnboardingHandler(t, allOpts...)

	form := url.Values{}
	form.Set("company", "CSRF Co")
	form.Set("naics", "541512")
	if csrf != "" {
		form.Set("csrf_token", csrf)
	}
	req := httptest.NewRequest(http.MethodPost, "/onboarding/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, pstore
}

// TestOnboardingCSRFValidTokenPersists proves an authenticated POST with the matching
// CSRF token is accepted (303 PRG) and persists the profile.
func TestOnboardingCSRFValidTokenPersists(t *testing.T) {
	const token = "good-token"
	rec, pstore := postOnboardingProfile(t, token, identityOpt("u@example.com", token))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("good-CSRF POST status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	if _, err := pstore.Load(); err != nil {
		t.Errorf("good-CSRF POST did not persist a profile: %v", err)
	}
}

// TestOnboardingCSRFWrongTokenRejected proves an authenticated POST with a WRONG CSRF
// token is rejected (403) and persists nothing.
func TestOnboardingCSRFWrongTokenRejected(t *testing.T) {
	rec, pstore := postOnboardingProfile(t, "wrong", identityOpt("u@example.com", "good-token"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong-CSRF POST status = %d, want 403", rec.Code)
	}
	if _, err := pstore.Load(); err == nil {
		t.Errorf("wrong-CSRF POST persisted a profile")
	}
}

// TestOnboardingCSRFMissingTokenRejected proves an authenticated POST with a
// MISSING/EMPTY CSRF token is rejected (403) and persists nothing. This guards the
// removed `&& ident.CSRFToken != ""` bypass: an authenticated session must always
// supply a token.
func TestOnboardingCSRFMissingTokenRejected(t *testing.T) {
	rec, pstore := postOnboardingProfile(t, "", identityOpt("u@example.com", "good-token"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing-CSRF POST status = %d, want 403", rec.Code)
	}
	if _, err := pstore.Load(); err == nil {
		t.Errorf("missing-CSRF POST persisted a profile")
	}
}

// TestOnboardingNoIdentityFailsClosed is the key fail-closed test: with no resolvable
// identity (ok == false) and insecureNoAuth NOT set (production default), a
// state-mutating POST is rejected (403) and persists NOTHING. The mutation must not
// fall through to the store just because the upstream session middleware was bypassed.
func TestOnboardingNoIdentityFailsClosed(t *testing.T) {
	// No WithIdentity and no WithInsecureNoAuth → resolveIdentity returns ok=false and
	// insecureNoAuth defaults false.
	rec, pstore := postOnboardingProfile(t, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no-identity POST status = %d, want 403 (fail closed); body=%s", rec.Code, rec.Body.String())
	}
	if _, err := pstore.Load(); err == nil {
		t.Errorf("no-identity POST persisted a profile (must fail closed)")
	}
}

// TestOnboardingNoIdentityInsecureDevAllowed documents the explicit dev exception:
// with no identity but WithInsecureNoAuth(true) (the operator's deliberate
// -insecure-no-auth opt-in), the POST is allowed WITHOUT a CSRF token and persists,
// relying on SameSite=Lax + same-origin. This path must NEVER be the production
// default.
func TestOnboardingNoIdentityInsecureDevAllowed(t *testing.T) {
	rec, pstore := postOnboardingProfile(t, "", dashboard.WithInsecureNoAuth(true))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("insecure-dev POST status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	if _, err := pstore.Load(); err != nil {
		t.Errorf("insecure-dev POST did not persist a profile: %v", err)
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

// driveStatusOpt wires a fixed DriveStatus for the destination-UI tests.
func driveStatusOpt(st dashboard.DriveStatus) dashboard.Option {
	return dashboard.WithDriveStatus(func() dashboard.DriveStatus { return st })
}

// TestOnboardingDriveDestinationDisplay proves the connected Drive step renders the
// CURRENT destination three ways (WS-C5b): a folder id with an Open-in-Drive link, the
// "My Drive (root)" label for the "root" sentinel, and the not-set treatment when no
// target is set.
func TestOnboardingDriveDestinationDisplay(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		contains []string
		excludes []string
	}{
		{
			name:   "folder id shows open-in-drive link",
			target: "folder-abc123",
			contains: []string{
				"folder-abc123",
				"https://drive.google.com/drive/folders/folder-abc123",
				"Open in Drive",
			},
		},
		{
			name:     "root shows my drive label",
			target:   "root",
			contains: []string{"My Drive (root)"},
			excludes: []string{"https://drive.google.com/drive/folders/"},
		},
		{
			name:     "unset shows not-set treatment",
			target:   "",
			contains: []string{"Not set yet"},
			excludes: []string{"https://drive.google.com/drive/folders/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newOnboardingHandler(t,
				dashboard.WithProfileStore(&memProfileStore{}),
				driveStatusOpt(dashboard.DriveStatus{Configured: true, Connected: true, Target: tt.target}))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding", http.NoBody))
			body := rec.Body.String()
			for _, want := range tt.contains {
				if !strings.Contains(body, want) {
					t.Errorf("body missing %q; got %q", want, body)
				}
			}
			for _, no := range tt.excludes {
				if strings.Contains(body, no) {
					t.Errorf("body unexpectedly contains %q", no)
				}
			}
		})
	}
}

// TestOnboardingDriveChangeControlGating proves the change-destination form appears
// only when a target saver is wired AND the Drive is connected. Without a saver, or
// when not connected, the control is hidden (read-only display only).
func TestOnboardingDriveChangeControlGating(t *testing.T) {
	const marker = `action="/onboarding/drive/target"`

	// Connected + saver wired → control present.
	h := newOnboardingHandler(t,
		dashboard.WithProfileStore(&memProfileStore{}),
		driveStatusOpt(dashboard.DriveStatus{Configured: true, Connected: true, Target: "root"}),
		dashboard.WithDriveTargetSaver(func(string) error { return nil }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding", http.NoBody))
	if !strings.Contains(rec.Body.String(), marker) {
		t.Errorf("change-destination form missing when connected + saver wired")
	}

	// Connected but NO saver → control hidden.
	h2 := newOnboardingHandler(t,
		dashboard.WithProfileStore(&memProfileStore{}),
		driveStatusOpt(dashboard.DriveStatus{Configured: true, Connected: true, Target: "root"}))
	rec2 := httptest.NewRecorder()
	h2.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/onboarding", http.NoBody))
	if strings.Contains(rec2.Body.String(), marker) {
		t.Errorf("change-destination form should be hidden without a saver")
	}

	// Saver wired but NOT connected → control hidden.
	h3 := newOnboardingHandler(t,
		dashboard.WithProfileStore(&memProfileStore{}),
		driveStatusOpt(dashboard.DriveStatus{Configured: true, Connected: false}),
		dashboard.WithDriveTargetSaver(func(string) error { return nil }))
	rec3 := httptest.NewRecorder()
	h3.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/onboarding", http.NoBody))
	if strings.Contains(rec3.Body.String(), marker) {
		t.Errorf("change-destination form should be hidden when not connected")
	}
}

// driveSaverFunc captures the last id saved so tests can assert what the SSR form
// persisted via the (single) drivetoken write path the JSON PUT also uses.
type driveSaverFunc struct {
	saved string
	calls int
}

func (d *driveSaverFunc) save(id string) error {
	d.saved = id
	d.calls++
	return nil
}

// postDriveTarget drives a POST /onboarding/drive/target with the given form fields
// against a handler wired with the given options. choice/folderID populate the radio
// and folder-id fields; csrf sets the token (omitted when "").
func postDriveTarget(t *testing.T, choice, folderID, csrf string, opts ...dashboard.Option) *httptest.ResponseRecorder {
	t.Helper()
	h := newOnboardingHandler(t, opts...)
	form := url.Values{}
	if choice != "" {
		form.Set("drive_choice", choice)
	}
	if folderID != "" {
		form.Set("drive_id", folderID)
	}
	if csrf != "" {
		form.Set("csrf_token", csrf)
	}
	req := httptest.NewRequest(http.MethodPost, "/onboarding/drive/target", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestOnboardingDriveTargetSaveFolder proves posting a folder id (authenticated +
// matching CSRF) persists the trimmed id via the saver and PRG-redirects.
func TestOnboardingDriveTargetSaveFolder(t *testing.T) {
	const token = "drive-csrf"
	saver := &driveSaverFunc{}
	rec := postDriveTarget(t, "folder", "  folder-xyz  ", token,
		dashboard.WithProfileStore(&memProfileStore{}),
		identityOpt("u@example.com", token),
		dashboard.WithDriveTargetSaver(saver.save))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (PRG); body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "drive_saved=1") {
		t.Errorf("Location = %q, want PRG redirect with drive_saved=1", loc)
	}
	if saver.saved != "folder-xyz" {
		t.Errorf("saved id = %q, want trimmed folder-xyz", saver.saved)
	}
}

// TestOnboardingDriveTargetSaveRoot proves choosing "My Drive root" persists the
// literal "root" sentinel.
func TestOnboardingDriveTargetSaveRoot(t *testing.T) {
	const token = "drive-csrf"
	saver := &driveSaverFunc{}
	rec := postDriveTarget(t, "root", "", token,
		dashboard.WithProfileStore(&memProfileStore{}),
		identityOpt("u@example.com", token),
		dashboard.WithDriveTargetSaver(saver.save))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	if saver.saved != "root" {
		t.Errorf("saved id = %q, want root", saver.saved)
	}
}

// TestOnboardingDriveTargetEmptyFolderRejected proves choosing "folder" with an empty
// id re-renders with an error (400) and persists nothing.
func TestOnboardingDriveTargetEmptyFolderRejected(t *testing.T) {
	const token = "drive-csrf"
	saver := &driveSaverFunc{}
	rec := postDriveTarget(t, "folder", "   ", token,
		dashboard.WithProfileStore(&memProfileStore{}),
		identityOpt("u@example.com", token),
		dashboard.WithDriveTargetSaver(saver.save))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if saver.calls != 0 {
		t.Errorf("saver called %d times on empty folder id, want 0", saver.calls)
	}
	if !strings.Contains(rec.Body.String(), "folder id") {
		t.Errorf("re-render missing the empty-folder error: %q", rec.Body.String())
	}
}

// TestOnboardingDriveTargetUnknownChoiceRejected proves a missing/unrecognized
// drive_choice is rejected (400) and persists nothing, rather than silently
// defaulting to a destination the operator did not pick.
func TestOnboardingDriveTargetUnknownChoiceRejected(t *testing.T) {
	const token = "drive-csrf"
	for _, choice := range []string{"", "bogus"} {
		saver := &driveSaverFunc{}
		rec := postDriveTarget(t, choice, "", token,
			dashboard.WithProfileStore(&memProfileStore{}),
			identityOpt("u@example.com", token),
			dashboard.WithDriveTargetSaver(saver.save))

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("choice=%q: status = %d, want 400; body=%s", choice, rec.Code, rec.Body.String())
		}
		if saver.calls != 0 {
			t.Errorf("choice=%q: saver called %d times, want 0", choice, saver.calls)
		}
	}
}

// TestOnboardingDriveTargetCSRFRejected proves the Drive-destination write fails
// closed on a wrong CSRF token (403) and persists nothing — the same gate the profile
// write uses.
func TestOnboardingDriveTargetCSRFRejected(t *testing.T) {
	saver := &driveSaverFunc{}
	rec := postDriveTarget(t, "root", "", "wrong",
		dashboard.WithProfileStore(&memProfileStore{}),
		identityOpt("u@example.com", "good-token"),
		dashboard.WithDriveTargetSaver(saver.save))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if saver.calls != 0 {
		t.Errorf("saver called %d times on bad CSRF, want 0", saver.calls)
	}
}

// TestOnboardingDriveTargetNoSaver503 proves the POST degrades to 503 when no saver is
// wired (Drive connect disabled), mirroring the JSON API's degradation.
func TestOnboardingDriveTargetNoSaver503(t *testing.T) {
	rec := postDriveTarget(t, "root", "", "tok",
		dashboard.WithProfileStore(&memProfileStore{}),
		identityOpt("u@example.com", "tok"))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

// TestOnboardingDriveTargetRoundTrip proves a saved folder id (via the real target
// store) shows as the current destination on a follow-up GET — the live-target read
// path C5a's auto-created folder also flows through.
func TestOnboardingDriveTargetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	targets, err := drivetoken.NewJSONTargetStore(dir)
	if err != nil {
		t.Fatalf("target store: %v", err)
	}
	tokens, err := drivetoken.NewJSONTokenStore(dir)
	if err != nil {
		t.Fatalf("token store: %v", err)
	}
	// A connected Drive (token present) is required for the destination block to
	// render, mirroring a real deployment after the WS-C2 connect handshake.
	if err := tokens.Save(&oauth2.Token{AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatalf("save token: %v", err)
	}
	// Simulate C5a having auto-created the "Kaimi Proposals" folder.
	if err := targets.Save(drivetoken.Target{DriveID: "kaimi-proposals-folder"}); err != nil {
		t.Fatalf("save target: %v", err)
	}

	h := newOnboardingHandler(t,
		dashboard.WithProfileStore(&memProfileStore{}),
		dashboard.WithDriveStatus(dashboard.DriveStatusFromStores(tokens, targets)),
		dashboard.WithDriveTargetSaver(func(id string) error {
			return targets.Save(drivetoken.Target{DriveID: id})
		}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboarding", http.NoBody))
	body := rec.Body.String()
	if !strings.Contains(body, "kaimi-proposals-folder") {
		t.Errorf("follow-up GET did not show the C5a auto-created folder as the destination: %q", body)
	}
	if !strings.Contains(body, "https://drive.google.com/drive/folders/kaimi-proposals-folder") {
		t.Errorf("follow-up GET missing the Open-in-Drive link for the saved folder")
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

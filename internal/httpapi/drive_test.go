package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/Mawar2/Kaimi/internal/drivetoken"
)

// fakeTokenStore is an in-memory drivetoken.TokenStore for handler tests.
type fakeTokenStore struct {
	tok *oauth2.Token
}

func (f *fakeTokenStore) Load() (*oauth2.Token, error) {
	if f.tok == nil {
		return nil, drivetoken.ErrNotConnected
	}
	return f.tok, nil
}
func (f *fakeTokenStore) Save(tok *oauth2.Token) error {
	f.tok = tok
	return nil
}

// fakeTargetStore is an in-memory drivetoken.TargetStore for handler tests.
type fakeTargetStore struct {
	target *drivetoken.Target
}

func (f *fakeTargetStore) Load() (drivetoken.Target, error) {
	if f.target == nil {
		return drivetoken.Target{}, drivetoken.ErrNotConnected
	}
	return *f.target, nil
}
func (f *fakeTargetStore) Save(t drivetoken.Target) error {
	f.target = &t
	return nil
}

// newTestDriveHandler builds a DriveHandler wired to in-memory stores and a fake
// code exchange so tests never reach Google.
func newTestDriveHandler(t *testing.T, exchange driveExchangeFunc) (*DriveHandler, *fakeTokenStore, *fakeTargetStore) {
	t.Helper()
	tokens := &fakeTokenStore{}
	targets := &fakeTargetStore{}
	h, err := newDriveHandler(DriveOAuthConfig{
		ClientID:     "drive-client-id",
		ClientSecret: "drive-client-secret",
		RedirectURL:  "https://app.example.com/api/v1/integrations/drive/callback",
	}, tokens, targets)
	if err != nil {
		t.Fatalf("newDriveHandler: %v", err)
	}
	if exchange != nil {
		h.exchange = exchange
	}
	return h, tokens, targets
}

// TestDriveConnectRedirectsWithMinimalScopesOfflineState verifies the connect
// endpoint redirects to Google consent carrying: the minimal Drive scopes
// (drive.file + documents, NOT full drive), access_type=offline, prompt=consent
// (so a refresh token is returned), a state value, and the S256 PKCE challenge.
func TestDriveConnectRedirectsWithMinimalScopesOfflineState(t *testing.T) {
	h, _, _ := newTestDriveHandler(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/drive/connect", http.NoBody)
	rec := httptest.NewRecorder()
	h.handleConnect(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("connect status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse Location %q: %v", loc, err)
	}
	q := u.Query()

	scope := q.Get("scope")
	if !strings.Contains(scope, drivetoken.ScopeDriveFile) {
		t.Errorf("consent scope %q missing drive.file", scope)
	}
	if !strings.Contains(scope, drivetoken.ScopeDocuments) {
		t.Errorf("consent scope %q missing documents", scope)
	}
	if strings.Contains(scope, "auth/drive ") || strings.HasSuffix(scope, "auth/drive") {
		t.Errorf("consent scope %q must NOT include the full-drive scope", scope)
	}
	if q.Get("access_type") != "offline" {
		t.Errorf("access_type = %q, want offline (needed for a refresh token)", q.Get("access_type"))
	}
	if q.Get("prompt") != "consent" {
		t.Errorf("prompt = %q, want consent (forces a refresh token)", q.Get("prompt"))
	}
	if q.Get("state") == "" {
		t.Error("consent URL missing state")
	}
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		t.Errorf("consent URL missing S256 PKCE challenge: challenge=%q method=%q", q.Get("code_challenge"), q.Get("code_challenge_method"))
	}

	// State + PKCE must be parked in cookies for the callback to verify.
	var sawState, sawPKCE bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == driveStateCookieName {
			sawState = true
		}
		if c.Name == drivePKCECookieName {
			sawPKCE = true
		}
	}
	if !sawState || !sawPKCE {
		t.Errorf("connect set cookies state=%v pkce=%v, want both", sawState, sawPKCE)
	}
}

// TestDriveCallbackStoresTokenOnValidState verifies a callback with a state that
// matches the cookie exchanges the code and stores the returned token (incl refresh).
func TestDriveCallbackStoresTokenOnValidState(t *testing.T) {
	exchanged := &oauth2.Token{AccessToken: "acc", RefreshToken: "ref", TokenType: "Bearer"}
	exchange := func(_ context.Context, code, _ string) (*oauth2.Token, error) {
		if code != "auth-code-123" {
			t.Errorf("exchange got code %q, want auth-code-123", code)
		}
		return exchanged, nil
	}
	h, tokens, _ := newTestDriveHandler(t, exchange)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/drive/callback?state=s1&code=auth-code-123", http.NoBody)
	req.AddCookie(&http.Cookie{Name: driveStateCookieName, Value: "s1"})
	req.AddCookie(&http.Cookie{Name: drivePKCECookieName, Value: "verifier-1"})
	rec := httptest.NewRecorder()
	h.handleCallback(rec, req)

	if rec.Code != http.StatusFound && rec.Code != http.StatusOK {
		t.Fatalf("callback status = %d, want 302 or 200", rec.Code)
	}
	if tokens.tok == nil {
		t.Fatal("callback did not store a token")
	}
	if tokens.tok.RefreshToken != "ref" {
		t.Errorf("stored refresh token = %q, want ref", tokens.tok.RefreshToken)
	}
}

// TestDriveCallbackRejectsBadState verifies a state mismatch is rejected and NO
// token is stored (CSRF defense).
func TestDriveCallbackRejectsBadState(t *testing.T) {
	exchange := func(_ context.Context, _, _ string) (*oauth2.Token, error) {
		t.Fatal("exchange must not run on a bad-state callback")
		return nil, nil
	}
	h, tokens, _ := newTestDriveHandler(t, exchange)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/drive/callback?state=WRONG&code=c", http.NoBody)
	req.AddCookie(&http.Cookie{Name: driveStateCookieName, Value: "s1"})
	req.AddCookie(&http.Cookie{Name: drivePKCECookieName, Value: "verifier-1"})
	rec := httptest.NewRecorder()
	h.handleCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want 400 on state mismatch", rec.Code)
	}
	if tokens.tok != nil {
		t.Error("callback stored a token despite a state mismatch")
	}
}

// TestDriveStatusReflectsConnection verifies status reports connected=false before
// a token exists and connected=true (with the target) after.
func TestDriveStatusReflectsConnection(t *testing.T) {
	h, tokens, targets := newTestDriveHandler(t, nil)

	// Not connected yet.
	rec := httptest.NewRecorder()
	h.handleStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/integrations/drive/status", http.NoBody))
	var before driveStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &before); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if before.Connected {
		t.Error("status reported connected before any token was stored")
	}

	// Connect + set a target.
	tokens.tok = &oauth2.Token{AccessToken: "a", RefreshToken: "r"}
	targets.target = &drivetoken.Target{DriveID: "drive-9"}

	rec = httptest.NewRecorder()
	h.handleStatus(rec, httptest.NewRequest(http.MethodGet, "/api/v1/integrations/drive/status", http.NoBody))
	var after driveStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &after); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if !after.Connected {
		t.Error("status reported not connected after a token was stored")
	}
	if after.Target != "drive-9" {
		t.Errorf("status target = %q, want drive-9", after.Target)
	}
}

// TestDriveSetTargetPersists verifies PUT target stores the provided Drive id.
func TestDriveSetTargetPersists(t *testing.T) {
	h, _, targets := newTestDriveHandler(t, nil)

	body := strings.NewReader(`{"drive_id":"0AshareddriveID"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/integrations/drive/target", body)
	rec := httptest.NewRecorder()
	h.handleSetTarget(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("set target status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if targets.target == nil || targets.target.DriveID != "0AshareddriveID" {
		t.Errorf("target not persisted: %+v", targets.target)
	}
}

// TestDriveSetTargetRejectsEmpty verifies a blank id is a 400.
func TestDriveSetTargetRejectsEmpty(t *testing.T) {
	h, _, _ := newTestDriveHandler(t, nil)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/integrations/drive/target", strings.NewReader(`{"drive_id":""}`))
	rec := httptest.NewRecorder()
	h.handleSetTarget(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("set empty target status = %d, want 400", rec.Code)
	}
}

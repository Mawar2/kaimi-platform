//go:build live

// Live end-to-end test for the Workspace OAuth sign-in path (WS-B7).
//
// This is the SECOND testing layer. The first layer — fully-mocked, deterministic
// unit tests that inject the exchange/verifyIDToken seams — already lives in
// auth_test.go and runs on every commit. This file instead exercises the pieces
// that can only be checked against REAL Google, so it is gated behind the `live`
// build tag and is EXCLUDED from the default `go test` (it never runs in CI).
//
// WHY a separate, partly-manual test:
//
//	Full 3-legged OAuth requires an interactive browser and a human at Google's
//	consent screen, which a Go test cannot drive. So WS-B7 splits the flow into
//	what is genuinely automatable live versus what must be done by hand:
//
//	AUTOMATED here (no human, no browser):
//	  - The real AuthHandler builds a real Google consent URL from real config, and
//	    we assert it carries client_id, the configured redirect_uri, a CSRF state, an
//	    S256 PKCE code_challenge, and the hd Workspace hint.
//	  - If a real Google-issued ID token is supplied out-of-band (see below), we call
//	    the REAL idtoken.Validate and assert it validates, then drive the real
//	    handleCallback and assert its enforcement: an in-domain verified token mints a
//	    session cookie; an out-of-domain token is rejected 403 with no session.
//
//	MANUAL (documented, not automated):
//	  - Obtaining the ID token requires completing the interactive Google consent in
//	    a browser with a real Workspace test account (or minting an OIDC/ID token
//	    out-of-band). The tokens are then fed to this test via the env vars below.
//
// All assertions check STRUCTURE / BEHAVIOR (a valid session is minted, or the
// request is rejected) — never exact strings, which are non-deterministic.
//
// # Required environment (mirrors httpapi.LoadOAuthConfig)
//
//	OAUTH_CLIENT_ID       Google OAuth client id (also the ID-token audience)
//	OAUTH_CLIENT_SECRET   Google OAuth client secret
//	OAUTH_REDIRECT_URL    this service's absolute /auth/callback URL
//	OAUTH_ALLOWED_DOMAIN  the Google Workspace domain ("hd") allowed to sign in
//	SESSION_SECRET        HMAC-SHA256 key used to sign session cookies
//
// If ANY required var is missing, the test t.Skip()s with a clear message, so the
// live suite is a safe no-op when unconfigured.
//
// # Optional environment (enables the real-token sub-cases)
//
//	KAIMI_TEST_ID_TOKEN          a real Google-issued ID token for an IN-DOMAIN,
//	                             email-verified account (aud == OAUTH_CLIENT_ID).
//	                             When set, the test runs idtoken.Validate against it
//	                             and asserts handleCallback mints a session.
//	KAIMI_TEST_ID_TOKEN_FOREIGN  a real Google-issued ID token for an OUT-OF-DOMAIN
//	                             account. When set, the test asserts handleCallback
//	                             rejects it 403 with no session.
//
// When a token var is absent, its sub-case t.Skip()s — never logged, never failed.
//
// # How to obtain a test ID token (manual)
//
// You need a real Google Workspace TEST account in OAUTH_ALLOWED_DOMAIN. Easiest
// path: complete the interactive flow once and capture the id_token from the
// callback exchange (e.g. via the OAuth 2.0 Playground at
// https://developers.google.com/oauthplayground, configured with this client id,
// requesting the openid+email+profile scopes), or mint an OIDC ID token whose
// audience equals OAUTH_CLIENT_ID. Copy the raw JWT into KAIMI_TEST_ID_TOKEN.
// Repeat with an account OUTSIDE the domain for KAIMI_TEST_ID_TOKEN_FOREIGN.
//
// NOTE: ID tokens are short-lived (~1 hour). Re-mint them right before running.
//
// # Run
//
//	OAUTH_CLIENT_ID=... OAUTH_CLIENT_SECRET=... OAUTH_REDIRECT_URL=... \
//	OAUTH_ALLOWED_DOMAIN=yourdomain.com SESSION_SECRET=... \
//	KAIMI_TEST_ID_TOKEN=... KAIMI_TEST_ID_TOKEN_FOREIGN=... \
//	  go test -tags live ./internal/httpapi/...
//
// This requires a real Google Workspace test account and live network access to
// Google. It is NEVER part of the default `go test ./...` and never runs in CI.
package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

// liveEnv names the env vars the live OAuth test reads. The first five are the
// required OAuth config (mirroring LoadOAuthConfig); the last two are optional and
// only enable the real-token enforcement sub-cases.
const (
	envTestIDToken        = "KAIMI_TEST_ID_TOKEN"
	envTestIDTokenForeign = "KAIMI_TEST_ID_TOKEN_FOREIGN"
)

// liveOAuthConfig loads the real OAuthConfig from the environment, or skips the
// test when any required value is missing. It deliberately reuses LoadOAuthConfig
// so the live test exercises the exact same env contract production uses.
func liveOAuthConfig(t *testing.T) OAuthConfig {
	t.Helper()
	cfg, enabled, err := LoadOAuthConfig()
	if err != nil {
		// Some OAUTH_* vars are set but not all — a misconfiguration, not an
		// "unconfigured" no-op. Report it (the message names the missing var; it
		// carries no secret value).
		t.Fatalf("LoadOAuthConfig: %v", err)
	}
	if !enabled {
		t.Skipf("OAuth env not set (need %s, %s, %s, %s, %s) — skipping live OAuth test",
			envOAuthClientID, envOAuthClientSecret, envOAuthRedirectURL,
			envOAuthAllowedDomain, envOAuthSessionSecret)
	}
	return cfg
}

// liveAuthHandler builds the REAL AuthHandler from live config: its exchange and
// verifyIDToken seams keep their production defaults (real oauth2 exchange + real
// idtoken.Validate). No fakes are injected — that is the whole point of WS-B7.
func liveAuthHandler(t *testing.T, cfg *OAuthConfig) *AuthHandler {
	t.Helper()
	ah, err := NewAuthHandler(cfg)
	if err != nil {
		t.Fatalf("NewAuthHandler: %v", err)
	}
	return ah
}

// TestLiveLoginRedirectsToRealGoogle asserts that the real AuthHandler, built from
// real OAuth config, produces a redirect to a genuine Google accounts consent URL
// carrying client_id, the configured redirect_uri, a CSRF state, an S256 PKCE
// code_challenge, and the hd Workspace hint. This needs no token and no human — it
// validates the URL the production handler would actually send a user to.
func TestLiveLoginRedirectsToRealGoogle(t *testing.T) {
	cfg := liveOAuthConfig(t)
	ah := liveAuthHandler(t, &cfg)

	req := httptest.NewRequest(http.MethodGet, "/auth/login", http.NoBody)
	rec := httptest.NewRecorder()
	ah.handleLogin(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("login status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc == "" {
		t.Fatal("login set no Location header")
	}
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}

	// Must point at a real Google accounts consent host (e.g. accounts.google.com).
	if !strings.HasSuffix(u.Host, "google.com") {
		t.Errorf("consent host = %q, want a *.google.com host", u.Host)
	}
	if u.Scheme != "https" {
		t.Errorf("consent scheme = %q, want https", u.Scheme)
	}

	q := u.Query()
	if got := q.Get("client_id"); got != cfg.ClientID {
		t.Errorf("client_id = %q, want %q", got, cfg.ClientID)
	}
	if got := q.Get("redirect_uri"); got != cfg.RedirectURL {
		t.Errorf("redirect_uri = %q, want %q", got, cfg.RedirectURL)
	}
	if q.Get("state") == "" {
		t.Error("consent URL missing CSRF state")
	}
	if q.Get("code_challenge") == "" {
		t.Error("consent URL missing PKCE code_challenge")
	}
	if got := q.Get("code_challenge_method"); got != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", got)
	}
	// AllowedDomain is lowercased by newAuthHandler; compare against that.
	if got, want := q.Get("hd"), strings.ToLower(cfg.AllowedDomain); got != want {
		t.Errorf("hd hint = %q, want %q", got, want)
	}

	t.Logf("live consent URL host=%q scheme=%q (client_id, redirect_uri, state, S256 PKCE, hd all present)", u.Host, u.Scheme)
}

// TestLiveValidateRealIDToken calls the REAL idtoken.Validate against a real
// Google-issued ID token (KAIMI_TEST_ID_TOKEN), proving the token validates with
// Google's live certs for the configured audience. It asserts the audience matches
// the client id; it never logs the token. Skips when no token is supplied.
func TestLiveValidateRealIDToken(t *testing.T) {
	cfg := liveOAuthConfig(t)

	raw := os.Getenv(envTestIDToken)
	if raw == "" {
		t.Skipf("%s not set — skipping real ID-token validation (obtain one via a manual consent flow; see file header)", envTestIDToken)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	payload, err := idtoken.Validate(ctx, raw, cfg.ClientID)
	if err != nil {
		t.Fatalf("idtoken.Validate against live Google failed: %v", err)
	}
	if payload.Audience != cfg.ClientID {
		t.Errorf("validated token audience = %q, want client id %q", payload.Audience, cfg.ClientID)
	}
	if payload.Subject == "" {
		t.Error("validated token has empty subject")
	}
	// Do not log the email or token; report only non-identifying structure.
	t.Logf("real ID token validated by live Google (aud matches client id, subject present)")
}

// TestLiveCallbackInDomainMintsSession drives the REAL handleCallback end to end
// with a real in-domain ID token. It stubs ONLY the upstream code exchange (which
// inherently needs the interactive browser leg) to return the supplied real token,
// then leaves the real idtoken.Validate seam in place — so the verification, the hd
// domain check, the email_verified check, and the session minting are all exercised
// against Google for real. Asserts a session cookie is minted. Skips without a token.
func TestLiveCallbackInDomainMintsSession(t *testing.T) {
	cfg := liveOAuthConfig(t)
	raw := os.Getenv(envTestIDToken)
	if raw == "" {
		t.Skipf("%s not set — skipping in-domain session minting (see file header for how to obtain a token)", envTestIDToken)
	}

	ah := liveAuthHandler(t, &cfg)
	// Stub ONLY the code exchange (the irreducibly-interactive leg): hand the real
	// token to the real verifyIDToken seam, which still calls live Google.
	ah.exchange = realTokenExchange(raw)

	rec := httptest.NewRecorder()
	ah.handleCallback(rec, liveCallbackRequest())

	if rec.Code != http.StatusFound {
		t.Fatalf("in-domain callback status = %d, want 302 (a session should be minted); body=%s", rec.Code, rec.Body.String())
	}
	c := liveSessionCookie(rec)
	if c == nil {
		t.Fatal("in-domain verified token minted no session cookie")
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		t.Errorf("session cookie hardening flags wrong: HttpOnly=%v Secure=%v SameSite=%v Path=%q",
			c.HttpOnly, c.Secure, c.SameSite, c.Path)
	}
	t.Logf("in-domain real token → session minted (redirect 302, hardened cookie set)")
}

// TestLiveCallbackForeignDomainRejected drives the REAL handleCallback with a real
// OUT-OF-DOMAIN ID token and asserts it is rejected 403 with NO session cookie —
// proving the live hd-domain enforcement. Like the in-domain case it stubs only the
// interactive exchange leg and keeps the real idtoken.Validate. Skips without a token.
func TestLiveCallbackForeignDomainRejected(t *testing.T) {
	cfg := liveOAuthConfig(t)
	raw := os.Getenv(envTestIDTokenForeign)
	if raw == "" {
		t.Skipf("%s not set — skipping out-of-domain rejection (supply a real token for an account OUTSIDE %s)", envTestIDTokenForeign, envOAuthAllowedDomain)
	}

	ah := liveAuthHandler(t, &cfg)
	ah.exchange = realTokenExchange(raw)

	rec := httptest.NewRecorder()
	ah.handleCallback(rec, liveCallbackRequest())

	if rec.Code != http.StatusForbidden {
		t.Fatalf("out-of-domain callback status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if c := liveSessionCookie(rec); c != nil {
		t.Error("out-of-domain token minted a session cookie, want none")
	}
	t.Logf("out-of-domain real token → rejected 403, no session (live hd enforcement holds)")
}

// realTokenExchange returns an exchange seam that skips the interactive code-for-
// token leg and yields an oauth2.Token whose id_token extra is the supplied real
// Google ID token. Everything downstream (real idtoken.Validate, hd + email checks,
// session minting) then runs unmodified against the real token.
func realTokenExchange(rawIDToken string) exchangeFunc {
	return func(ctx context.Context, code, verifier string) (*oauth2.Token, error) {
		tok := &oauth2.Token{AccessToken: "live-test-not-used"}
		return tok.WithExtra(map[string]interface{}{"id_token": rawIDToken}), nil
	}
}

// liveCallbackRequest builds a /auth/callback request carrying matching state +
// PKCE cookies so the callback's CSRF and PKCE-presence checks pass and the flow
// reaches the real ID-token verification (the part WS-B7 exercises live).
func liveCallbackRequest() *http.Request {
	const state = "live-test-state"
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+url.QueryEscape(state)+"&code=live-test-code", http.NoBody)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: state})
	req.AddCookie(&http.Cookie{Name: pkceCookieName, Value: "live-test-pkce-verifier"})
	return req
}

// liveSessionCookie returns the minted session cookie from the recorder, or nil.
func liveSessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionCookieName {
			return ck
		}
	}
	return nil
}

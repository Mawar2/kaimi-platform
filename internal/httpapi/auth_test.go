package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
)

// newTestAuth builds an AuthHandler wired with fakes so unit tests never reach
// Google: the exchange returns a token carrying a fixed raw id_token, and the
// id-token verifier returns the supplied payload. The OAuth config uses dummy
// client credentials and a fixed allowed domain.
func newTestAuth(t *testing.T, exchange exchangeFunc, verify verifyFunc) *AuthHandler {
	t.Helper()
	oc := OAuthConfig{
		ClientID:      "test-client-id.apps.googleusercontent.com",
		ClientSecret:  "test-client-secret",
		RedirectURL:   "https://app.example.com/auth/callback",
		AllowedDomain: "example.com",
		PostLoginPath: "/",
	}
	sm := newSessionManager(testSessionSecret, time.Hour)
	ah, err := newAuthHandler(&oc, sm)
	if err != nil {
		t.Fatalf("newAuthHandler: %v", err)
	}
	if exchange != nil {
		ah.exchange = exchange
	}
	if verify != nil {
		ah.verifyIDToken = verify
	}
	return ah
}

// fakePayload builds an idtoken.Payload with the given hd and email_verified claim.
func fakePayload(hd string, emailVerified bool) *idtoken.Payload {
	return &idtoken.Payload{
		Issuer:   "https://accounts.google.com",
		Audience: "test-client-id.apps.googleusercontent.com",
		Subject:  "user-sub-123",
		Expires:  time.Now().Add(time.Hour).Unix(),
		Claims: map[string]interface{}{
			"hd":             hd,
			"email":          "user@" + hd,
			"email_verified": emailVerified,
		},
	}
}

// okExchange returns a token whose Extra("id_token") yields the fixed raw token.
func okExchange(_ context.Context, _, _ string) (*oauth2.Token, error) {
	tok := &oauth2.Token{AccessToken: "fake-access"}
	return tok.WithExtra(map[string]interface{}{"id_token": "raw.fake.idtoken"}), nil
}

// TestLoginRedirectsToGoogleWithStatePKCEAndHD verifies GET /auth/login sets the
// state + PKCE cookies and redirects to a Google consent URL carrying state, the
// S256 code_challenge, and the hd domain hint.
func TestLoginRedirectsToGoogleWithStatePKCEAndHD(t *testing.T) {
	ah := newTestAuth(t, nil, nil)

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
		t.Fatalf("parse Location %q: %v", loc, err)
	}
	if !strings.Contains(u.Host, "google.com") {
		t.Errorf("login redirect host = %q, want a google.com consent host", u.Host)
	}
	q := u.Query()
	if q.Get("state") == "" {
		t.Error("consent URL missing state")
	}
	if q.Get("code_challenge") == "" {
		t.Error("consent URL missing PKCE code_challenge")
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", q.Get("code_challenge_method"))
	}
	if q.Get("hd") != "example.com" {
		t.Errorf("hd = %q, want example.com", q.Get("hd"))
	}

	// State + PKCE-verifier cookies must be set (HttpOnly+Secure), so the callback
	// can validate them.
	var stateCk, pkceCk *http.Cookie
	for _, ck := range rec.Result().Cookies() {
		switch ck.Name {
		case stateCookieName:
			stateCk = ck
		case pkceCookieName:
			pkceCk = ck
		}
	}
	if stateCk == nil || pkceCk == nil {
		t.Fatalf("login must set %q and %q cookies", stateCookieName, pkceCookieName)
	}
	for _, ck := range []*http.Cookie{stateCk, pkceCk} {
		if !ck.HttpOnly || !ck.Secure || ck.SameSite != http.SameSiteLaxMode {
			t.Errorf("temp cookie %q missing HttpOnly/Secure/SameSite=Lax", ck.Name)
		}
		// __Host- prefix requires Path=/ (and no Domain) to be honored by browsers.
		if ck.Path != "/" {
			t.Errorf("temp cookie %q Path = %q, want / (required by __Host- prefix)", ck.Name, ck.Path)
		}
		if ck.Domain != "" {
			t.Errorf("temp cookie %q Domain = %q, want empty (required by __Host- prefix)", ck.Name, ck.Domain)
		}
		if !strings.HasPrefix(ck.Name, "__Host-") {
			t.Errorf("temp cookie name %q missing __Host- prefix", ck.Name)
		}
	}
	// The state cookie value must match the state in the URL.
	if stateCk.Value != q.Get("state") {
		t.Errorf("state cookie %q != URL state %q", stateCk.Value, q.Get("state"))
	}
}

// callbackRequest builds a /auth/callback request carrying the state + PKCE cookies
// from a prior login and the given code/state query params.
func callbackRequest(state, code string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+url.QueryEscape(state)+"&code="+url.QueryEscape(code), http.NoBody)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: state})
	req.AddCookie(&http.Cookie{Name: pkceCookieName, Value: "test-pkce-verifier"})
	return req
}

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionCookieName {
			return ck
		}
	}
	return nil
}

// TestCallbackHappyPathSetsSessionAndRedirects verifies the happy path: matching
// state, successful exchange, valid id token with correct hd + verified email →
// session cookie (HttpOnly+Secure+SameSite=Lax) + redirect to "/".
func TestCallbackHappyPathSetsSessionAndRedirects(t *testing.T) {
	verify := func(_ context.Context, raw, aud string) (*idtoken.Payload, error) {
		return fakePayload("example.com", true), nil
	}
	ah := newTestAuth(t, okExchange, verify)

	rec := httptest.NewRecorder()
	ah.handleCallback(rec, callbackRequest("the-state", "the-code"))

	if rec.Code != http.StatusFound {
		t.Fatalf("callback status = %d, want 302; body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("post-login redirect = %q, want /", loc)
	}
	c := sessionCookie(rec)
	if c == nil {
		t.Fatal("happy path set no session cookie")
	}
	if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		t.Errorf("session cookie flags wrong: %+v", c)
	}
}

// TestCallbackWrongDomainForbidden verifies an id token whose hd != allowed domain
// is rejected 403 with NO session cookie set.
func TestCallbackWrongDomainForbidden(t *testing.T) {
	verify := func(_ context.Context, raw, aud string) (*idtoken.Payload, error) {
		return fakePayload("evil.com", true), nil
	}
	ah := newTestAuth(t, okExchange, verify)

	rec := httptest.NewRecorder()
	ah.handleCallback(rec, callbackRequest("the-state", "the-code"))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong-domain status = %d, want 403", rec.Code)
	}
	if c := sessionCookie(rec); c != nil {
		t.Errorf("wrong-domain set a session cookie %+v, want none", c)
	}
}

// TestCallbackUnverifiedEmailForbidden verifies email_verified=false → 403, no cookie.
func TestCallbackUnverifiedEmailForbidden(t *testing.T) {
	verify := func(_ context.Context, raw, aud string) (*idtoken.Payload, error) {
		return fakePayload("example.com", false), nil
	}
	ah := newTestAuth(t, okExchange, verify)

	rec := httptest.NewRecorder()
	ah.handleCallback(rec, callbackRequest("the-state", "the-code"))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("unverified-email status = %d, want 403", rec.Code)
	}
	if c := sessionCookie(rec); c != nil {
		t.Errorf("unverified-email set a session cookie, want none")
	}
}

// TestCallbackStateMismatchBadRequest verifies a state param not matching the state
// cookie → 400, no exchange, no cookie.
func TestCallbackStateMismatchBadRequest(t *testing.T) {
	exchanged := false
	exchange := func(ctx context.Context, code, verifier string) (*oauth2.Token, error) {
		exchanged = true
		return okExchange(ctx, code, verifier)
	}
	ah := newTestAuth(t, exchange, func(_ context.Context, _, _ string) (*idtoken.Payload, error) {
		return fakePayload("example.com", true), nil
	})

	// Cookie says "real-state" but the query param says "attacker-state".
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state=attacker-state&code=c", http.NoBody)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "real-state"})
	req.AddCookie(&http.Cookie{Name: pkceCookieName, Value: "v"})
	rec := httptest.NewRecorder()
	ah.handleCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("state-mismatch status = %d, want 400", rec.Code)
	}
	if exchanged {
		t.Error("state mismatch must short-circuit BEFORE the code exchange")
	}
	if c := sessionCookie(rec); c != nil {
		t.Error("state mismatch set a session cookie, want none")
	}
}

// TestCallbackCaseInsensitiveDomainAllowed verifies the hd/domain compare is
// case-insensitive: an hd claim of "Example.com" against AllowedDomain "example.com"
// is ALLOWED (DNS domains are case-insensitive) and must not produce a false 403.
func TestCallbackCaseInsensitiveDomainAllowed(t *testing.T) {
	verify := func(_ context.Context, raw, aud string) (*idtoken.Payload, error) {
		// Mixed-case hd claim; AllowedDomain in newTestAuth is lowercase "example.com".
		return fakePayload("Example.Com", true), nil
	}
	ah := newTestAuth(t, okExchange, verify)

	rec := httptest.NewRecorder()
	ah.handleCallback(rec, callbackRequest("the-state", "the-code"))

	if rec.Code != http.StatusFound {
		t.Fatalf("case-insensitive domain status = %d, want 302 (allowed); body=%s", rec.Code, rec.Body.String())
	}
	c := sessionCookie(rec)
	if c == nil {
		t.Fatal("case-insensitive domain set no session cookie, want one")
	}
}

// clearedTempCookie returns the most recent Set-Cookie for name that is a deletion
// (MaxAge < 0 and empty value), or nil if no such deletion was written.
func clearedTempCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	var found *http.Cookie
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == name && ck.MaxAge < 0 && ck.Value == "" {
			found = ck
		}
	}
	return found
}

// TestCallbackStateMismatchClearsTempCookies verifies that on the state-mismatch
// error path (an early return), the temp state + pkce cookies are still cleared —
// the deletion Set-Cookie headers (MaxAge<0, Path=/, __Host- names) are present.
func TestCallbackStateMismatchClearsTempCookies(t *testing.T) {
	ah := newTestAuth(t, okExchange, func(_ context.Context, _, _ string) (*idtoken.Payload, error) {
		return fakePayload("example.com", true), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state=attacker-state&code=c", http.NoBody)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "real-state"})
	req.AddCookie(&http.Cookie{Name: pkceCookieName, Value: "v"})
	rec := httptest.NewRecorder()
	ah.handleCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("state-mismatch status = %d, want 400", rec.Code)
	}
	for _, name := range []string{stateCookieName, pkceCookieName} {
		c := clearedTempCookie(rec, name)
		if c == nil {
			t.Fatalf("state-mismatch path did not clear temp cookie %q", name)
		}
		if c.Path != "/" {
			t.Errorf("cleared temp cookie %q Path = %q, want / (must match set Path)", name, c.Path)
		}
	}
}

// TestCallbackMissingStateCookieBadRequest verifies an absent state cookie → 400.
func TestCallbackMissingStateCookieBadRequest(t *testing.T) {
	ah := newTestAuth(t, okExchange, func(_ context.Context, _, _ string) (*idtoken.Payload, error) {
		return fakePayload("example.com", true), nil
	})
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state=s&code=c", http.NoBody)
	// No cookies attached.
	rec := httptest.NewRecorder()
	ah.handleCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing-state-cookie status = %d, want 400", rec.Code)
	}
	if c := sessionCookie(rec); c != nil {
		t.Error("missing state cookie set a session, want none")
	}
}

// TestCallbackExchangeFailureBadRequest verifies a failed code exchange → 400, no cookie.
func TestCallbackExchangeFailureBadRequest(t *testing.T) {
	exchange := func(_ context.Context, _, _ string) (*oauth2.Token, error) {
		return nil, context.DeadlineExceeded
	}
	ah := newTestAuth(t, exchange, nil)

	rec := httptest.NewRecorder()
	ah.handleCallback(rec, callbackRequest("the-state", "the-code"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("exchange-failure status = %d, want 400", rec.Code)
	}
	if c := sessionCookie(rec); c != nil {
		t.Error("exchange failure set a session, want none")
	}
}

// callbackRequestWithReturn is like callbackRequest but also attaches the return
// cookie a prior /auth/login would have stashed, so the callback can consume it.
func callbackRequestWithReturn(state, code, returnPath string) *http.Request {
	req := callbackRequest(state, code)
	req.AddCookie(&http.Cookie{Name: returnCookieName, Value: returnPath})
	return req
}

// loginReturnCookie returns the return cookie a /auth/login call set, or nil.
func loginReturnCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	var found *http.Cookie
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == returnCookieName && ck.MaxAge > 0 {
			found = ck
		}
	}
	return found
}

// TestLoginStashesSafeReturnCookie verifies GET /auth/login?return=/proposals stashes
// the sanitized return path in a hardened (__Host-, HttpOnly, Secure, Lax, Path=/)
// short-lived cookie so it survives the Google round-trip.
func TestLoginStashesSafeReturnCookie(t *testing.T) {
	ah := newTestAuth(t, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/login?return="+url.QueryEscape("/proposals?filter=active"), http.NoBody)
	rec := httptest.NewRecorder()
	ah.handleLogin(rec, req)

	ck := loginReturnCookie(rec)
	if ck == nil {
		t.Fatal("login did not stash a return cookie for a safe deep link")
	}
	if ck.Value != "/proposals?filter=active" {
		t.Errorf("return cookie value = %q, want %q", ck.Value, "/proposals?filter=active")
	}
	if !ck.HttpOnly || !ck.Secure || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/" {
		t.Errorf("return cookie not hardened: %+v", ck)
	}
	if !strings.HasPrefix(ck.Name, "__Host-") {
		t.Errorf("return cookie name %q missing __Host- prefix", ck.Name)
	}
	if ck.MaxAge <= 0 || ck.MaxAge > tempCookieMaxAge {
		t.Errorf("return cookie MaxAge = %d, want short-lived (0 < n <= %d)", ck.MaxAge, tempCookieMaxAge)
	}
}

// TestLoginDoesNotStashUnsafeReturn verifies an open-redirect return at login ENTRY
// (//evil.com) is NOT stashed — it collapses to "/" which is not worth a cookie.
func TestLoginDoesNotStashUnsafeReturn(t *testing.T) {
	for _, bad := range []string{"//evil.com", "https://evil.com", "/"} {
		ah := newTestAuth(t, nil, nil)
		req := httptest.NewRequest(http.MethodGet, "/auth/login?return="+url.QueryEscape(bad), http.NoBody)
		rec := httptest.NewRecorder()
		ah.handleLogin(rec, req)
		if ck := loginReturnCookie(rec); ck != nil {
			t.Errorf("return=%q stashed cookie %q, want none (unsafe/default)", bad, ck.Value)
		}
	}
}

// TestCallbackHonorsSafeReturnCookie verifies the happy path redirects to a safe
// stashed deep link (query preserved) instead of PostLoginPath.
func TestCallbackHonorsSafeReturnCookie(t *testing.T) {
	verify := func(_ context.Context, raw, aud string) (*idtoken.Payload, error) {
		return fakePayload("example.com", true), nil
	}
	ah := newTestAuth(t, okExchange, verify)

	rec := httptest.NewRecorder()
	ah.handleCallback(rec, callbackRequestWithReturn("the-state", "the-code", "/proposals?filter=active"))

	if rec.Code != http.StatusFound {
		t.Fatalf("callback status = %d, want 302; body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/proposals?filter=active" {
		t.Errorf("redirect = %q, want the stashed deep link with query preserved", loc)
	}
	if sessionCookie(rec) == nil {
		t.Fatal("happy path with return set no session cookie")
	}
	// The return cookie must be cleared on the success path.
	if clearedTempCookie(rec, returnCookieName) == nil {
		t.Error("success path did not clear the return cookie")
	}
}

// TestCallbackForgedReturnCookieRejected is the critical open-redirect-at-EXIT test:
// a forged return cookie pointing off-site must be RE-VALIDATED at redirect time and
// fall back to PostLoginPath, never sending the browser to evil.com.
func TestCallbackForgedReturnCookieRejected(t *testing.T) {
	// Note: a raw backslash cannot survive cookie serialization (Go drops it), so the
	// "/\\evil" form is exercised at the path level by TestSafeReturnPath; here we use
	// the scheme-relative and absolute forms that a forged cookie could actually carry.
	for _, evil := range []string{"//evil.com", "https://evil.com/pwn", "//evil.com/path?x=1"} {
		verify := func(_ context.Context, raw, aud string) (*idtoken.Payload, error) {
			return fakePayload("example.com", true), nil
		}
		ah := newTestAuth(t, okExchange, verify) // PostLoginPath is "/"

		rec := httptest.NewRecorder()
		ah.handleCallback(rec, callbackRequestWithReturn("the-state", "the-code", evil))

		if rec.Code != http.StatusFound {
			t.Fatalf("callback status = %d, want 302 for return=%q", rec.Code, evil)
		}
		if loc := rec.Header().Get("Location"); loc != "/" {
			t.Errorf("forged return=%q redirected to %q, want / (PostLoginPath) — open redirect!", evil, loc)
		}
	}
}

// TestLoginToCallbackOpenRedirectAtEntry exercises the full login→callback flow with
// an open-redirect return supplied at the login ENTRY. Because login never stashes an
// unsafe path, the callback has no return cookie and lands on PostLoginPath — the
// attacker's //evil.com / https://evil never wins.
func TestLoginToCallbackOpenRedirectAtEntry(t *testing.T) {
	for _, evil := range []string{"//evil.com", "https://evil"} {
		ah := newTestAuth(t, okExchange, func(_ context.Context, _, _ string) (*idtoken.Payload, error) {
			return fakePayload("example.com", true), nil
		})

		// Login with the malicious return.
		loginReq := httptest.NewRequest(http.MethodGet, "/auth/login?return="+url.QueryEscape(evil), http.NoBody)
		loginRec := httptest.NewRecorder()
		ah.handleLogin(loginRec, loginReq)

		// Carry whatever cookies login set (state, pkce, and — for an unsafe return —
		// none) into the callback, alongside the known fake state/pkce values.
		cb := callbackRequest("the-state", "the-code")
		for _, ck := range loginRec.Result().Cookies() {
			if ck.Name == returnCookieName && ck.MaxAge > 0 {
				cb.AddCookie(&http.Cookie{Name: ck.Name, Value: ck.Value})
			}
		}
		cbRec := httptest.NewRecorder()
		ah.handleCallback(cbRec, cb)

		if cbRec.Code != http.StatusFound {
			t.Fatalf("callback status = %d, want 302 for entry return=%q", cbRec.Code, evil)
		}
		if loc := cbRec.Header().Get("Location"); loc != "/" {
			t.Errorf("entry open-redirect return=%q landed on %q, want / (PostLoginPath)", evil, loc)
		}
	}
}

// TestCallbackClearsReturnCookieOnBadState verifies the return cookie is cleared even
// on an early-return error path (state mismatch), like state/pkce are.
func TestCallbackClearsReturnCookieOnBadState(t *testing.T) {
	ah := newTestAuth(t, okExchange, func(_ context.Context, _, _ string) (*idtoken.Payload, error) {
		return fakePayload("example.com", true), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state=attacker-state&code=c", http.NoBody)
	req.AddCookie(&http.Cookie{Name: stateCookieName, Value: "real-state"})
	req.AddCookie(&http.Cookie{Name: pkceCookieName, Value: "v"})
	req.AddCookie(&http.Cookie{Name: returnCookieName, Value: "/proposals"})
	rec := httptest.NewRecorder()
	ah.handleCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("state-mismatch status = %d, want 400", rec.Code)
	}
	if clearedTempCookie(rec, returnCookieName) == nil {
		t.Error("bad-state path did not clear the return cookie")
	}
}

// TestLogoutClearsSession verifies POST /auth/logout clears the session cookie.
func TestLogoutClearsSession(t *testing.T) {
	ah := newTestAuth(t, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", http.NoBody)
	rec := httptest.NewRecorder()
	ah.handleLogout(rec, req)

	c := sessionCookie(rec)
	if c == nil {
		t.Fatal("logout set no session cookie")
	}
	if c.MaxAge >= 0 || c.Value != "" {
		t.Errorf("logout cookie not cleared: MaxAge=%d Value=%q", c.MaxAge, c.Value)
	}
}

// TestAuthRoutesRegisteredUnauthenticated verifies the /auth/* routes are reachable
// on the root mux without a session (they sit outside the protected /api/v1 group).
func TestAuthRoutesRegisteredUnauthenticated(t *testing.T) {
	ah := newTestAuth(t, nil, nil)
	srv := New(Deps{Auth: ah})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/auth/login", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("GET /auth/login via Routes status = %d, want 302", rec.Code)
	}
}

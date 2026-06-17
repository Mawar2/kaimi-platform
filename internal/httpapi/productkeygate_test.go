package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/productkey"
)

// gateSecret is a fixed non-secret HMAC key for deterministic gate tests.
var gateSecret = []byte("unit-test-product-key-gate-secret-not-real")

// newTestGate builds a ProductKeyGate over a fresh in-memory registry with a fixed
// clock, returning both so tests can mint keys and advance time deterministically.
func newTestGate(t *testing.T, now time.Time) (*ProductKeyGate, *productkey.MemoryRegistry) {
	t.Helper()
	reg := productkey.NewMemoryRegistry()
	reg.SetClock(func() time.Time { return now }) // mint stamps issued/expiry from this
	g, err := NewProductKeyGate(reg, gateSecret, 12*time.Hour, "/")
	if err != nil {
		t.Fatalf("NewProductKeyGate: %v", err)
	}
	g.now = func() time.Time { return now } // validate() reads this clock
	return g, reg
}

// sessionCookieFrom extracts the minted session cookie from a response, failing the
// test if none was set.
func sessionCookieFrom(t *testing.T, res *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range res.Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatalf("no %s cookie set on response", sessionCookieName)
	return nil
}

// TestGateAccessMagicLinkGrantsSession: a valid ?key= mints a session bound to the key
// and redirects into the app.
func TestGateAccessMagicLinkGrantsSession(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	g, reg := newTestGate(t, now)
	rec, _ := reg.Mint(context.Background(), "Ey3 Technologies", 14*24*time.Hour)

	req := httptest.NewRequest(http.MethodGet, "/access?key="+url.QueryEscape(rec.Key), http.NoBody)
	w := httptest.NewRecorder()
	g.handleAccess(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/" {
		t.Errorf("redirect Location = %q, want /", loc)
	}
	cookie := sessionCookieFrom(t, res)
	sess, err := g.session.verify(cookie.Value)
	if err != nil {
		t.Fatalf("minted session does not verify: %v", err)
	}
	if sess.KeyID != rec.Key {
		t.Errorf("session KeyID = %q, want %q", sess.KeyID, rec.Key)
	}
	if sess.Email != "" || sess.Subject != "" {
		t.Errorf("product-key session must carry no Google identity, got %+v", sess)
	}
}

// TestGateAccessNoKeyShowsEntry: a bare /access (no key) renders the entry form (200).
func TestGateAccessNoKeyShowsEntry(t *testing.T) {
	g, _ := newTestGate(t, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/access", http.NoBody)
	w := httptest.NewRecorder()
	g.handleAccess(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

// TestGateAccessInvalidKeyDenied: an unknown key renders the form with 401 and mints NO
// session.
func TestGateAccessInvalidKeyDenied(t *testing.T) {
	g, _ := newTestGate(t, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/access?key=KAIMI-AAAA-BBBB-CCCC", http.NoBody)
	w := httptest.NewRecorder()
	g.handleAccess(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
	for _, c := range res.Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			t.Errorf("invalid key must not mint a session cookie")
		}
	}
}

// TestGateEntrySubmitValid: posting a valid key to /entry grants a session and redirects.
func TestGateEntrySubmitValid(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	g, reg := newTestGate(t, now)
	rec, _ := reg.Mint(context.Background(), "T", 7*24*time.Hour)

	form := url.Values{"key": {strings.ToLower(rec.Key)}} // re-typed lowercase tolerated
	req := httptest.NewRequest(http.MethodPost, "/entry", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	g.handleEntrySubmit(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.StatusCode)
	}
	if sess, err := g.session.verify(sessionCookieFrom(t, res).Value); err != nil || sess.KeyID != rec.Key {
		t.Errorf("entry submit did not mint the expected session: sess=%+v err=%v", sess, err)
	}
}

// TestGateEntrySubmitInvalid: posting a bad key re-renders the form with 401, no session.
func TestGateEntrySubmitInvalid(t *testing.T) {
	g, _ := newTestGate(t, time.Now())
	form := url.Values{"key": {"not-a-key"}}
	req := httptest.NewRequest(http.MethodPost, "/entry", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	g.handleEntrySubmit(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Result().StatusCode)
	}
}

// grantedRequest mints a key, grants a session, and returns a fresh request carrying the
// session cookie — the common setup for middleware tests.
func grantedRequest(t *testing.T, g *ProductKeyGate, reg *productkey.MemoryRegistry, ttl time.Duration, method, target string) (*http.Request, productkey.Record) {
	t.Helper()
	rec, err := reg.Mint(context.Background(), "T", ttl)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	gw := httptest.NewRecorder()
	g.grant(gw, &rec)
	cookie := sessionCookieFrom(t, gw.Result())
	req := httptest.NewRequest(method, target, http.NoBody)
	req.AddCookie(cookie)
	return req, rec
}

// okNext is a trivial downstream handler that records whether it ran and what session
// the middleware injected.
type okNext struct {
	called bool
	sess   *Session
}

func (n *okNext) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	n.called = true
	n.sess, _ = SessionFromContext(r.Context())
}

// TestRequireProductKeyPassesValidSession: a request with a valid session reaches next
// with the verified session injected into context.
func TestRequireProductKeyPassesValidSession(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	g, reg := newTestGate(t, now)
	req, rec := grantedRequest(t, g, reg, 14*24*time.Hour, http.MethodGet, "/api/v1/me")

	next := &okNext{}
	g.RequireProductKey(next).ServeHTTP(httptest.NewRecorder(), req)

	if !next.called {
		t.Fatal("valid session should reach next")
	}
	if next.sess == nil || next.sess.KeyID != rec.Key {
		t.Errorf("middleware injected session = %+v, want KeyID %q", next.sess, rec.Key)
	}
}

// TestRequireProductKeyDeniesNoCookie: a request with no session is 401 and next is not
// called.
func TestRequireProductKeyDeniesNoCookie(t *testing.T) {
	g, _ := newTestGate(t, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", http.NoBody)
	w := httptest.NewRecorder()
	next := &okNext{}
	g.RequireProductKey(next).ServeHTTP(w, req)

	if next.called {
		t.Error("missing session must NOT reach next")
	}
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Result().StatusCode)
	}
}

// TestRequireProductKeyDeniesForgedCookie: a session cookie signed with a DIFFERENT
// secret is rejected (unforgeable).
func TestRequireProductKeyDeniesForgedCookie(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	g, _ := newTestGate(t, now)

	// Forge a session with an attacker's secret but a plausible key id.
	forger := newSessionManager([]byte("attacker-different-secret"), time.Hour)
	fw := httptest.NewRecorder()
	forger.SetSession(fw, Session{KeyID: "KAIMI-ZZZZ-ZZZZ-ZZZZ"})
	forged := sessionCookieFrom(t, fw.Result())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", http.NoBody)
	req.AddCookie(forged)
	w := httptest.NewRecorder()
	next := &okNext{}
	g.RequireProductKey(next).ServeHTTP(w, req)

	if next.called || w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("forged cookie must be rejected: called=%v status=%d", next.called, w.Result().StatusCode)
	}
}

// TestRequireProductKeyEnforcesExpiry: once the clock passes the key's expiry, a
// previously-valid session is rejected (the registry re-check is the authority).
func TestRequireProductKeyEnforcesExpiry(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	g, reg := newTestGate(t, now)
	req, _ := grantedRequest(t, g, reg, 24*time.Hour, http.MethodGet, "/api/v1/me")

	// Advance the gate's clock past the key's 24h expiry.
	g.now = func() time.Time { return now.Add(48 * time.Hour) }

	w := httptest.NewRecorder()
	next := &okNext{}
	g.RequireProductKey(next).ServeHTTP(w, req)
	if next.called || w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("expired key must be rejected: called=%v status=%d", next.called, w.Result().StatusCode)
	}
}

// TestRequireProductKeyEnforcesRevocation: revoking the key locks out an existing
// session immediately (instant revoke).
func TestRequireProductKeyEnforcesRevocation(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	g, reg := newTestGate(t, now)
	req, rec := grantedRequest(t, g, reg, 14*24*time.Hour, http.MethodGet, "/api/v1/me")

	if err := reg.Revoke(context.Background(), rec.Key); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	w := httptest.NewRecorder()
	next := &okNext{}
	g.RequireProductKey(next).ServeHTTP(w, req)
	if next.called || w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("revoked key must be rejected immediately: called=%v status=%d", next.called, w.Result().StatusCode)
	}
}

// TestRequireProductKeyExemptsDriveCallback: the Drive OAuth callback passes through
// without a session (it is self-protected by its own state cookie).
func TestRequireProductKeyExemptsDriveCallback(t *testing.T) {
	g, _ := newTestGate(t, time.Now())
	req := httptest.NewRequest(http.MethodGet, driveCallbackPath+"?code=x&state=y", http.NoBody)
	w := httptest.NewRecorder()
	next := &okNext{}
	g.RequireProductKey(next).ServeHTTP(w, req)
	if !next.called {
		t.Error("drive callback must be exempt from the product-key gate")
	}
}

// TestRequireProductKeyHTMLRedirectsToEntry: an unauthenticated HTML request is
// redirected to /entry (not 401), and a valid one reaches next.
func TestRequireProductKeyHTMLRedirectsToEntry(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	g, reg := newTestGate(t, now)

	// Unauthenticated → 302 to /entry.
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w := httptest.NewRecorder()
	g.RequireProductKeyHTML(&okNext{}).ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusFound || w.Result().Header.Get("Location") != entryPath {
		t.Errorf("unauth HTML: status=%d loc=%q, want 302 -> %s", w.Result().StatusCode, w.Result().Header.Get("Location"), entryPath)
	}

	// Authenticated → next runs.
	greq, _ := grantedRequest(t, g, reg, 14*24*time.Hour, http.MethodGet, "/")
	next := &okNext{}
	g.RequireProductKeyHTML(next).ServeHTTP(httptest.NewRecorder(), greq)
	if !next.called {
		t.Error("valid session should reach the dashboard handler")
	}
}

// TestGateRoutesIntegration drives the property end-to-end through Server.Routes(): the
// probe is public, the API is gated (no bypass), the entry page is reachable without a
// session (no redirect loop), and a magic-link cookie then opens the API.
func TestGateRoutesIntegration(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	g, reg := newTestGate(t, now)
	rec, _ := reg.Mint(context.Background(), "Ey3", 14*24*time.Hour)
	h := New(Deps{ProductKey: g}).Routes()

	// /healthz is public.
	if code := gateGet(h, "/healthz", nil); code != http.StatusOK {
		t.Errorf("/healthz = %d, want 200", code)
	}
	// /entry is reachable without a session (no loop).
	if code := gateGet(h, "/entry", nil); code != http.StatusOK {
		t.Errorf("/entry = %d, want 200", code)
	}
	// /api/v1/me is gated: no session → 401 (no bypass).
	if code := gateGet(h, "/api/v1/me", nil); code != http.StatusUnauthorized {
		t.Errorf("/api/v1/me unauthenticated = %d, want 401", code)
	}

	// Magic link → 302 + session cookie.
	aw := httptest.NewRecorder()
	h.ServeHTTP(aw, httptest.NewRequest(http.MethodGet, "/access?key="+rec.Key, http.NoBody))
	if aw.Result().StatusCode != http.StatusFound {
		t.Fatalf("magic link = %d, want 302", aw.Result().StatusCode)
	}
	cookie := sessionCookieFrom(t, aw.Result())

	// With the cookie, the API opens.
	if code := gateGet(h, "/api/v1/me", cookie); code != http.StatusOK {
		t.Errorf("/api/v1/me with session = %d, want 200", code)
	}

	// After revoke, the same cookie is locked out.
	if err := reg.Revoke(context.Background(), rec.Key); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if code := gateGet(h, "/api/v1/me", cookie); code != http.StatusUnauthorized {
		t.Errorf("/api/v1/me after revoke = %d, want 401", code)
	}
}

// doGet issues a GET through the handler, optionally with a cookie, and returns status.
func gateGet(h http.Handler, target string, cookie *http.Cookie) int {
	req := httptest.NewRequest(http.MethodGet, target, http.NoBody)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Result().StatusCode
}

// TestProductKeyGateRequiresSecret: building a gate without a session secret fails closed.
func TestProductKeyGateRequiresSecret(t *testing.T) {
	if _, err := NewProductKeyGate(productkey.NewMemoryRegistry(), nil, time.Hour, "/"); err == nil {
		t.Error("NewProductKeyGate with no secret must error (fail closed)")
	}
	if _, err := NewProductKeyGate(nil, gateSecret, time.Hour, "/"); err == nil {
		t.Error("NewProductKeyGate with no registry must error")
	}
}

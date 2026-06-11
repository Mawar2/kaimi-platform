package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testSessionSecret is a non-secret fixed key for deterministic session tests.
// It is NOT a real credential — unit tests must never use a production secret.
var testSessionSecret = []byte("unit-test-session-secret-key-not-real")

// TestSessionSignVerifyRoundTrip proves a freshly minted session token verifies
// back to the same claims with the same secret.
func TestSessionSignVerifyRoundTrip(t *testing.T) {
	sm := newSessionManager(testSessionSecret, time.Hour)
	want := Session{
		Subject: "1234567890",
		Email:   "alice@example.com",
		Domain:  "example.com",
		Expiry:  time.Now().Add(time.Hour).Unix(),
	}

	token := sm.sign(want)
	got, err := sm.verify(token)
	if err != nil {
		t.Fatalf("verify fresh token: %v", err)
	}
	if got.Subject != want.Subject || got.Email != want.Email || got.Domain != want.Domain {
		t.Errorf("round-trip claims = %+v, want %+v", got, want)
	}
}

// TestSessionVerifyRejectsTamperedPayload proves a token whose payload is altered
// (but signature left in place) fails the constant-time signature check.
func TestSessionVerifyRejectsTamperedPayload(t *testing.T) {
	sm := newSessionManager(testSessionSecret, time.Hour)
	token := sm.sign(Session{Subject: "1", Email: "a@example.com", Domain: "example.com", Expiry: time.Now().Add(time.Hour).Unix()})

	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("token has no payload.sig structure: %q", token)
	}
	// Flip the FIRST character of the payload segment so the decoded bytes are
	// guaranteed to change. (Flipping the LAST base64 char is unreliable: the final
	// char of a base64 string carries "don't-care" trailing bits, so A<->B there can
	// decode to the same bytes and leave the signature still valid — a flaky test.)
	payload := parts[0]
	tampered := flipChar(payload[0]) + payload[1:] + "." + parts[1]

	if _, err := sm.verify(tampered); err == nil {
		t.Fatal("verify tampered payload: want error, got nil")
	}
}

// TestSessionVerifyRejectsTamperedSignature proves a token whose signature is
// altered fails verification.
func TestSessionVerifyRejectsTamperedSignature(t *testing.T) {
	sm := newSessionManager(testSessionSecret, time.Hour)
	token := sm.sign(Session{Subject: "1", Email: "a@example.com", Domain: "example.com", Expiry: time.Now().Add(time.Hour).Unix()})

	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("token has no payload.sig structure: %q", token)
	}
	// Flip the FIRST character of the signature so the decoded MAC bytes are
	// guaranteed to change (see the payload test: the last base64 char has don't-care
	// trailing bits and can flip to the same bytes, which made this test flaky).
	sig := parts[1]
	tampered := parts[0] + "." + flipChar(sig[0]) + sig[1:]

	if _, err := sm.verify(tampered); err == nil {
		t.Fatal("verify tampered signature: want error, got nil")
	}
}

// TestSessionVerifyRejectsExpired proves an expired token is rejected even though
// its signature is valid.
func TestSessionVerifyRejectsExpired(t *testing.T) {
	sm := newSessionManager(testSessionSecret, time.Hour)
	token := sm.sign(Session{Subject: "1", Email: "a@example.com", Domain: "example.com", Expiry: time.Now().Add(-time.Minute).Unix()})

	if _, err := sm.verify(token); err == nil {
		t.Fatal("verify expired token: want error, got nil")
	}
}

// TestSessionVerifyRejectsForeignSecret proves a token signed with a different
// secret does not verify — forgery resistance: an attacker without the server
// secret cannot mint an accepted token.
func TestSessionVerifyRejectsForeignSecret(t *testing.T) {
	attacker := newSessionManager([]byte("a-different-secret-the-attacker-guessed"), time.Hour)
	token := attacker.sign(Session{Subject: "1", Email: "a@example.com", Domain: "example.com", Expiry: time.Now().Add(time.Hour).Unix()})

	server := newSessionManager(testSessionSecret, time.Hour)
	if _, err := server.verify(token); err == nil {
		t.Fatal("verify foreign-secret token: want error, got nil")
	}
}

// TestSessionVerifyRejectsGarbage proves malformed input is rejected without panic.
func TestSessionVerifyRejectsGarbage(t *testing.T) {
	sm := newSessionManager(testSessionSecret, time.Hour)
	for _, bad := range []string{"", "no-dot", "a.b.c.d", "!!!.@@@", "."} {
		if _, err := sm.verify(bad); err == nil {
			t.Errorf("verify %q: want error, got nil", bad)
		}
	}
}

// TestSetSessionCookieFlags proves SetSession writes a cookie with the mandatory
// security flags: HttpOnly, Secure, SameSite=Lax, Path=/, and a positive Max-Age.
func TestSetSessionCookieFlags(t *testing.T) {
	sm := newSessionManager(testSessionSecret, time.Hour)
	rec := httptest.NewRecorder()

	sm.SetSession(rec, Session{Subject: "1", Email: "a@example.com", Domain: "example.com"})

	cookies := rec.Result().Cookies()
	var c *http.Cookie
	for _, ck := range cookies {
		if ck.Name == sessionCookieName {
			c = ck
		}
	}
	if c == nil {
		t.Fatalf("SetSession set no %q cookie; cookies=%v", sessionCookieName, cookies)
	}
	if !c.HttpOnly {
		t.Error("session cookie missing HttpOnly")
	}
	if !c.Secure {
		t.Error("session cookie missing Secure")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("session cookie SameSite = %v, want Lax", c.SameSite)
	}
	if c.Path != "/" {
		t.Errorf("session cookie Path = %q, want /", c.Path)
	}
	// __Host- prefix requires Path=/ and no Domain; assert the prefix + no Domain.
	if !strings.HasPrefix(c.Name, "__Host-") {
		t.Errorf("session cookie name %q missing __Host- prefix", c.Name)
	}
	if c.Domain != "" {
		t.Errorf("session cookie Domain = %q, want empty (required by __Host- prefix)", c.Domain)
	}
	if c.MaxAge <= 0 {
		t.Errorf("session cookie MaxAge = %d, want > 0", c.MaxAge)
	}
	if c.Value == "" {
		t.Error("session cookie has empty value")
	}
}

// TestParseSessionRoundTrip proves ParseSession reads back a cookie SetSession wrote.
func TestParseSessionRoundTrip(t *testing.T) {
	sm := newSessionManager(testSessionSecret, time.Hour)
	rec := httptest.NewRecorder()
	sm.SetSession(rec, Session{Subject: "42", Email: "bob@corp.com", Domain: "corp.com"})

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	for _, ck := range rec.Result().Cookies() {
		req.AddCookie(ck)
	}

	s, err := sm.ParseSession(req)
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	if s.Subject != "42" || s.Email != "bob@corp.com" || s.Domain != "corp.com" {
		t.Errorf("ParseSession = %+v, want sub=42 email=bob@corp.com domain=corp.com", s)
	}
}

// TestParseSessionMissingCookie proves ParseSession errors (not panics) with no cookie.
func TestParseSessionMissingCookie(t *testing.T) {
	sm := newSessionManager(testSessionSecret, time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	if _, err := sm.ParseSession(req); err == nil {
		t.Fatal("ParseSession with no cookie: want error, got nil")
	}
}

// TestClearSessionExpiresCookie proves ClearSession writes an expiring cookie that
// evicts the session in the browser.
func TestClearSessionExpiresCookie(t *testing.T) {
	sm := newSessionManager(testSessionSecret, time.Hour)
	rec := httptest.NewRecorder()
	sm.ClearSession(rec)

	var c *http.Cookie
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionCookieName {
			c = ck
		}
	}
	if c == nil {
		t.Fatalf("ClearSession set no %q cookie", sessionCookieName)
	}
	if c.MaxAge >= 0 {
		t.Errorf("cleared cookie MaxAge = %d, want < 0 (immediate expiry)", c.MaxAge)
	}
	if c.Value != "" {
		t.Errorf("cleared cookie Value = %q, want empty", c.Value)
	}
	if !c.HttpOnly || !c.Secure {
		t.Error("cleared cookie should keep HttpOnly+Secure flags")
	}
}

// flipChar returns a different ASCII byte than b so tamper tests reliably alter input.
func flipChar(b byte) string {
	if b == 'A' {
		return "B"
	}
	return "A"
}

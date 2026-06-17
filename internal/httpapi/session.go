package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// This file implements Kaimi's signed session token and the cookie helpers the
// auth flow (WS-B4) and the RequireSession middleware (WS-B5) use. We mint and
// verify our OWN compact token with stdlib crypto only (HMAC-SHA256) — no JWT
// library — keeping the trust surface small and the dependency set minimal.
//
// Token format (two base64url segments joined by a dot):
//
//	base64url(payloadJSON) "." base64url(HMAC-SHA256(secret, payloadSegment))
//
// The signature covers the *encoded* payload segment (the exact bytes before the
// dot), so verification re-encodes nothing it must trust: it splits on the dot,
// recomputes the MAC over the first segment, and compares in constant time. Only
// after the signature checks out is the payload decoded and the expiry enforced.

// sessionCookieName is the cookie that carries the signed session token. The
// __Host- prefix is a browser-enforced hardening: a cookie with this prefix is
// only accepted when it is Secure, has Path=/, and carries NO Domain attribute,
// which defeats subdomain cookie-tossing (a sibling/sub domain cannot overwrite
// it). SetSession/ClearSession below satisfy those constraints.
const sessionCookieName = "__Host-kaimi_session"

// ErrInvalidSession is returned by ParseSession/verify when a token is missing,
// malformed, has a bad signature, or is expired. Callers (e.g. the WS-B5
// middleware) test for it with errors.Is and respond 401 without leaking which of
// those conditions failed.
var ErrInvalidSession = errors.New("invalid or expired session")

// Session is the authenticated identity carried by the session cookie. It holds
// only what downstream authorization needs — the Google subject, the verified
// email, the Workspace domain the login was constrained to, and an absolute expiry
// — and deliberately carries no tokens or secrets.
type Session struct {
	// Subject is the Google account's stable unique id (the "sub" claim).
	Subject string `json:"sub"`
	// Email is the verified Workspace email address.
	Email string `json:"email"`
	// Domain is the Google Workspace domain ("hd") the login was restricted to.
	Domain string `json:"hd"`
	// KeyID identifies the product key that authorized this session in product-key
	// gate mode (the canonical KAIMI-XXXX-XXXX-XXXX string). It is empty for a
	// Workspace-OAuth session. The product-key gate re-validates this key against the
	// registry on each request so revocation is honored immediately. omitempty keeps
	// it out of Workspace-session payloads.
	KeyID string `json:"kid,omitempty"`
	// Expiry is the absolute expiry as a Unix timestamp (seconds). A token past
	// this instant is rejected even if its signature is valid.
	Expiry int64 `json:"exp"`
}

// sessionManager mints and verifies session tokens with a server-held HMAC key
// and applies the configured session lifetime. It is constructed once at startup
// and shared (read-only) across requests, so it is safe for concurrent use.
type sessionManager struct {
	secret []byte        // HMAC-SHA256 key; never logged
	ttl    time.Duration // session lifetime applied by SetSession
}

// newSessionManager builds a sessionManager from the HMAC secret and session TTL.
func newSessionManager(secret []byte, ttl time.Duration) *sessionManager {
	return &sessionManager{secret: secret, ttl: ttl}
}

// sign encodes the session as JSON, base64url-encodes it, and appends a
// base64url-encoded HMAC-SHA256 of that encoded payload. The returned token is
// "<payload>.<sig>".
func (m *sessionManager) sign(s Session) string {
	payloadJSON, _ := json.Marshal(s) // Session contains only strings/int64; cannot fail.
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig := base64.RawURLEncoding.EncodeToString(m.mac(payload))
	return payload + "." + sig
}

// verify checks the token's signature in constant time, then decodes the payload
// and rejects it if expired. It returns ErrInvalidSession (wrapped) for any
// failure so callers cannot distinguish a forged signature from an expired token.
func (m *sessionManager) verify(token string) (Session, error) {
	payload, sig, ok := strings.Cut(token, ".")
	if !ok || payload == "" || sig == "" {
		return Session{}, fmt.Errorf("%w: malformed token", ErrInvalidSession)
	}

	gotSig, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return Session{}, fmt.Errorf("%w: bad signature encoding", ErrInvalidSession)
	}
	// Constant-time compare resists timing attacks on signature forgery.
	if !hmac.Equal(gotSig, m.mac(payload)) {
		return Session{}, fmt.Errorf("%w: signature mismatch", ErrInvalidSession)
	}

	// Signature is authentic; now it is safe to decode the payload.
	payloadJSON, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Session{}, fmt.Errorf("%w: bad payload encoding", ErrInvalidSession)
	}
	var s Session
	if err := json.Unmarshal(payloadJSON, &s); err != nil {
		return Session{}, fmt.Errorf("%w: bad payload json", ErrInvalidSession)
	}
	if time.Now().Unix() >= s.Expiry {
		return Session{}, fmt.Errorf("%w: expired", ErrInvalidSession)
	}
	return s, nil
}

// mac computes HMAC-SHA256(secret, payload) over the encoded payload segment.
func (m *sessionManager) mac(payload string) []byte {
	h := hmac.New(sha256.New, m.secret)
	// hash.Hash.Write never returns an error.
	_, _ = h.Write([]byte(payload))
	return h.Sum(nil)
}

// csrfToken derives a stable per-session CSRF token bound to the session subject:
// base64url(HMAC-SHA256(secret, "csrf:"+subject)). It uses the SAME server HMAC key
// as session signing, so the token is unforgeable without the secret and stable for
// the life of a subject's session (a GET-rendered form token still matches on POST).
// The "csrf:" domain-separation prefix ensures this MAC can never collide with a
// session-payload MAC computed by mac(). It backs the WS-C3 onboarding form's CSRF
// defense-in-depth on top of the SameSite=Lax session cookie.
func (m *sessionManager) csrfToken(subject string) string {
	h := hmac.New(sha256.New, m.secret)
	_, _ = h.Write([]byte("csrf:" + subject))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// SetSession mints a session token for the given claims (stamping Expiry from the
// manager's TTL) and writes it as a hardened cookie: HttpOnly, Secure,
// SameSite=Lax, Path=/, with a Max-Age matching the TTL. The middleware (WS-B5)
// relies on this (via writeSessionCookie) being the single place the cookie is set.
func (m *sessionManager) SetSession(w http.ResponseWriter, s Session) {
	m.writeSessionCookie(w, s, time.Now().Add(m.ttl))
}

// SetSessionBounded mints a session whose absolute expiry is the EARLIER of the
// manager's TTL and hardCap. The product-key gate uses it so a session can never
// outlive the credential that authorized it: when a key expires in 3 days, a 12-hour
// session TTL would otherwise let a tester keep working for hours past the key's
// expiry. Capping the cookie's Max-Age and the signed Expiry to the key's expiry
// closes that window. If hardCap is already in the past, the cookie is written with a
// minimal Max-Age and an already-expired Expiry, so verify() rejects it immediately.
func (m *sessionManager) SetSessionBounded(w http.ResponseWriter, s Session, hardCap time.Time) {
	exp := time.Now().Add(m.ttl)
	if hardCap.Before(exp) {
		exp = hardCap
	}
	m.writeSessionCookie(w, s, exp)
}

// writeSessionCookie stamps the absolute expiry into the claims, signs them, and
// writes the hardened session cookie with a Max-Age derived from that expiry. It is
// the single place the session cookie is written, shared by SetSession and
// SetSessionBounded so the cookie's flags and the signed Expiry never diverge.
func (m *sessionManager) writeSessionCookie(w http.ResponseWriter, s Session, exp time.Time) {
	s.Expiry = exp.Unix()
	// Max-Age tracks the absolute expiry so the browser drops the cookie when the
	// session (or the bounding key) expires. Floor at 1s: a zero/negative Max-Age is
	// a delete directive, which would wipe a just-minted cookie; verify() still
	// rejects the already-expired payload, so the session is dead either way.
	maxAge := int(time.Until(exp).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    m.sign(s),
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSession overwrites the session cookie with an empty, immediately-expiring
// cookie (MaxAge < 0) so the browser drops it. It keeps the same security flags so
// the eviction is accepted under the same constraints the cookie was set with.
func (m *sessionManager) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ParseSession reads the session cookie from the request and returns the verified
// Session. It is the exported entry point the WS-B5 RequireSession middleware
// calls to authenticate each protected request. It returns ErrInvalidSession
// (wrapped) when the cookie is absent, malformed, forged, or expired.
func (m *sessionManager) ParseSession(r *http.Request) (*Session, error) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, fmt.Errorf("%w: no session cookie", ErrInvalidSession)
	}
	s, err := m.verify(c.Value)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

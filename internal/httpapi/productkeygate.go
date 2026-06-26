package httpapi

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/Mawar2/Kaimi/internal/productkey"
)

// This file implements the PRODUCT-KEY ACCESS GATE: the pilot access model where a
// time-limited KAIMI-XXXX-XXXX-XXXX product key (internal/productkey) — not a Google
// sign-in — is the credential that lets a tester into a deployment.
//
// SECURITY MODEL (this is the control that decides who reaches the app):
//   - A tester opens a MAGIC LINK (the key in the URL: /access?key=KAIMI-...) or types
//     the key on the /entry page. A valid key (exists && now < expires && !revoked)
//     mints a signed session cookie (the same HMAC sessionManager as Workspace OAuth),
//     bound so the session can never outlive the key (SetSessionBounded).
//   - RequireProductKey / RequireProductKeyHTML guard EVERY app and /api/v1 route and
//     FAIL CLOSED: a missing, malformed, forged, or expired session is denied. The
//     ONLY unauthenticated routes are the gate's own /entry + /access, the /healthz
//     probe (registered outside the wrap), and the Drive OAuth callback (exempted here
//     and self-protected by its own state-cookie CSRF check).
//   - The session's key id is RE-VALIDATED against the registry on EVERY request, so a
//     revocation (or expiry) takes effect immediately — not only when the 12-hour
//     session cookie lapses. This costs one registry read per request; at pilot scale
//     (a handful of testers, a server-rendered dashboard) that is cheap, and it is the
//     security-correct choice: the registry is the single source of truth for access.
//   - Google OAuth is used ONLY for connecting a customer Drive in this mode, never for
//     sign-in. The gate carries no Google identity; the session holds only the key id.

// driveCallbackPath is the one /api/v1 route the product-key gate lets through
// unauthenticated. Google redirects the customer's browser back here after the Drive
// consent screen; the callback is self-protected by its own per-flow state cookie
// (constant-time CSRF check in drive.go), so exempting it is safe and avoids losing a
// just-granted Drive token if the gate session lapsed mid-consent.
const driveCallbackPath = "/api/v1/integrations/drive/callback"

// entryPath / accessPath are the gate's own unauthenticated routes. entryPath renders
// the key-entry form; accessPath consumes a magic link. They must be reachable WITHOUT
// a session or the gate would redirect them to themselves in a loop — Routes() registers
// them outside the protected wrap and routes them to the public handler.
const (
	entryPath  = "/entry"
	accessPath = "/access"
	// loggedOutPath is where a missing/invalid session sends a browser: the public homepage
	// (the "main site"), which has the sign-in / enter-access-link call to action. It is
	// served ungated, so there is no redirect loop.
	loggedOutPath = "/home"
)

// Static, operator-controlled messages shown on the entry page. They are deliberately
// vague: the gate never reveals WHETHER a key existed (only that access was not
// granted), so a probe cannot distinguish "no such key" from "expired" or "revoked".
const (
	msgAccessDenied = "That access key is not valid, has expired, or has been revoked. Check the link in your invitation, or paste your key below."
)

// ProductKeyGate enforces product-key access. It is constructed once at startup and is
// safe for concurrent use: registry and session are concurrency-safe, and the rest of
// its fields are read-only after construction.
type ProductKeyGate struct {
	registry productkey.Registry
	session  *sessionManager

	// postAccess is the sanitized path a successful access redirects to (default "/").
	postAccess string

	// now is the clock, injectable so tests exercise key expiry deterministically.
	// It feeds Record.Valid; the session's own expiry is enforced by the manager.
	now func() time.Time
}

// NewProductKeyGate builds the gate from a key registry and the HMAC session secret.
// ttl bounds a session's lifetime (the key's own expiry caps it further). postAccess
// is where a successful access lands (sanitized; "" → "/"). The session secret is
// required — without it sessions cannot be signed, so the gate refuses to build rather
// than run open.
func NewProductKeyGate(registry productkey.Registry, sessionSecret []byte, ttl time.Duration, postAccess string) (*ProductKeyGate, error) {
	if registry == nil {
		return nil, errors.New("productkey gate requires a key registry")
	}
	if len(sessionSecret) == 0 {
		return nil, fmt.Errorf("productkey gate requires a session secret: %w", ErrMissingRequired)
	}
	dest := safeReturnPath(postAccess) // collapses "" and any off-site value to "/"
	return &ProductKeyGate{
		registry:   registry,
		session:    newSessionManager(sessionSecret, ttl),
		postAccess: dest,
		now:        time.Now,
	}, nil
}

// validate looks a raw (possibly re-typed) key up in the registry and reports whether
// it currently grants access. It returns false for an unknown key, any lookup error,
// an expired key, or a revoked key — every "no" path collapses to the same result so
// the caller cannot leak which condition failed. The registry is the source of truth,
// so this is called both at grant time AND on every protected request.
func (g *ProductKeyGate) validate(ctx context.Context, rawKey string) (productkey.Record, bool) {
	rec, err := g.registry.Lookup(ctx, rawKey)
	if err != nil {
		// ErrNotFound or any backend error → deny. Fail closed: if the registry is
		// unreachable we lock out rather than let traffic through unchecked.
		return productkey.Record{}, false
	}
	if !rec.Valid(g.now()) {
		return productkey.Record{}, false // expired or revoked
	}
	return rec, true
}

// grant mints the session cookie for a validated key, bounding the session so it can
// never outlive the key. The session carries ONLY the key id (no Google identity). rec
// is taken by pointer (it is a multi-field record) and is not retained.
func (g *ProductKeyGate) grant(w http.ResponseWriter, rec *productkey.Record) {
	g.session.SetSessionBounded(w, Session{KeyID: rec.Key}, rec.ExpiresAt)
}

// handleAccess serves GET /access — the magic-link entry point. A valid ?key= mints a
// session and redirects into the app (one click, no typing). A missing key falls back
// to the entry form; an invalid key re-renders the form with a vague denial (401) so a
// probe learns nothing about which keys exist.
func (g *ProductKeyGate) handleAccess(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		// A bare /access with no key is treated as "show me the way in".
		g.renderEntry(w, http.StatusOK, "")
		return
	}
	rec, ok := g.validate(r.Context(), key)
	if !ok {
		g.renderEntry(w, http.StatusUnauthorized, msgAccessDenied)
		return
	}
	g.grant(w, &rec)
	http.Redirect(w, r, g.postAccess, http.StatusFound)
}

// handleEntry serves GET /entry — the manual key-entry form (the fallback when a tester
// does not have the magic link handy). It always renders 200; submitting the form posts
// back to the same path (handleEntrySubmit).
func (g *ProductKeyGate) handleEntry(w http.ResponseWriter, _ *http.Request) {
	g.renderEntry(w, http.StatusOK, "")
}

// handleEntrySubmit serves POST /entry — the form submission. It validates the typed
// key exactly as the magic link does. On success it grants a session and redirects into
// the app; on failure it re-renders the form with the vague denial (401).
//
// This POST is deliberately NOT CSRF-protected: it is a login (it establishes, rather
// than mutates, a session). The only "login-CSRF" risk is forcing a victim into a
// session keyed by an attacker's product key — negligible here because a deployment is
// single-tenant and a key carries no per-user identity, data, or quota; it only opens
// the door to THIS environment. SameSite=Lax on the resulting session cookie further
// limits cross-site abuse. TODO(phase-N): add a CSRF token if keys ever map to
// per-user identities or metered quotas.
func (g *ProductKeyGate) handleEntrySubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		g.renderEntry(w, http.StatusBadRequest, msgAccessDenied)
		return
	}
	rec, ok := g.validate(r.Context(), r.PostFormValue("key"))
	if !ok {
		g.renderEntry(w, http.StatusUnauthorized, msgAccessDenied)
		return
	}
	g.grant(w, &rec)
	http.Redirect(w, r, g.postAccess, http.StatusFound)
}

// authenticate is the shared check both middlewares run: it parses the session cookie
// and RE-VALIDATES the carried key id against the registry, so expiry and revocation
// are enforced on every request, not just at grant time. It returns (nil, false) for
// any failure — absent/forged/expired cookie, a non-product-key session (empty key id),
// or a key the registry now rejects.
func (g *ProductKeyGate) authenticate(r *http.Request) (*Session, bool) {
	sess, err := g.session.ParseSession(r)
	if err != nil {
		return nil, false
	}
	if sess.KeyID == "" {
		// Not a product-key session (e.g. a stale Workspace cookie). Deny.
		return nil, false
	}
	if _, ok := g.validate(r.Context(), sess.KeyID); !ok {
		return nil, false
	}
	return sess, true
}

// RequireProductKey guards the JSON /api/v1 group: every request must carry a valid
// product-key session, except the Drive OAuth callback (self-protected, see
// driveCallbackPath). On success it injects the verified Session into the context
// (reachable via SessionFromContext) and calls next; on failure it answers 401 JSON and
// does NOT call next. It never redirects — this wraps the API, not an HTML surface.
func (g *ProductKeyGate) RequireProductKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == driveCallbackPath {
			next.ServeHTTP(w, r)
			return
		}
		sess, ok := g.authenticate(r)
		if !ok {
			// Do not leak which check failed; never log the cookie or key.
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), sessionContextKey{}, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireProductKeyHTML guards the server-rendered dashboard ("/"): same check as
// RequireProductKey, but a missing/invalid session REDIRECTS the browser (302) to the
// public homepage (the main site), which carries the sign-in / enter-access-link CTA,
// rather than answering 401 JSON. On success it injects the verified Session into the
// context and serves the page.
func (g *ProductKeyGate) RequireProductKeyHTML(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == driveCallbackPath {
			next.ServeHTTP(w, r)
			return
		}
		sess, ok := g.authenticate(r)
		if !ok {
			http.Redirect(w, r, loggedOutPath, http.StatusFound)
			return
		}
		ctx := context.WithValue(r.Context(), sessionContextKey{}, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// DashboardIdentity adapts the product-key session into the dashboard's IdentityFunc so
// the SSR onboarding form is CSRF-protected in product-key mode (mirroring
// AuthHandler.DashboardIdentity for the OAuth mode). The email is empty — a product-key
// session carries no Google identity — but the CSRF token is derived from the SAME HMAC
// key that signs the session, bound to the key id, so it is stable per session and
// unforgeable. license is the MASKED product key for the onboarding "License" step (the
// full key never leaves the session). It returns ok=false when no product-key session
// is present.
func (g *ProductKeyGate) DashboardIdentity(ctx context.Context) (email, csrfToken, license string, ok bool) {
	sess, ok := SessionFromContext(ctx)
	if !ok || sess == nil || sess.KeyID == "" {
		return "", "", "", false
	}
	return "", g.session.csrfToken(sess.KeyID), maskKey(sess.KeyID), true
}

// maskKey renders a product key for display with only its last group shown, e.g.
// "KAIMI-7F3A-9C2E-B1D4" → "KAIMI-····-····-B1D4". A key that is not the canonical
// 4-group shape falls back to showing only its last 4 characters. It NEVER returns the
// full key, so the onboarding page can confirm "your license is verified" without
// reprinting the credential.
func maskKey(key string) string {
	parts := strings.Split(key, "-")
	if len(parts) >= 3 && parts[0] == productkey.KeyPrefix { // KAIMI-XXXX-…-XXXX
		masked := []string{parts[0]}
		for i := 1; i < len(parts)-1; i++ {
			masked = append(masked, "····")
		}
		return strings.Join(append(masked, parts[len(parts)-1]), "-")
	}
	tail := key
	if len(tail) > 4 {
		tail = tail[len(tail)-4:]
	}
	return "····" + tail
}

// renderEntry writes the access/entry page with the given status and an optional error
// message. The page is fully self-contained (inline CSS, no external assets), so it
// renders even before any app data exists and in a locked-down deployment.
func (g *ProductKeyGate) renderEntry(w http.ResponseWriter, status int, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	// Ignore the write error: the status/headers are already committed and a failed
	// flush means the client is gone.
	_ = entryTemplate.Execute(w, entryPageData{
		Error:      errMsg,
		ActionPath: entryPath,
	})
}

// entryPageData is the template model for the access/entry page.
type entryPageData struct {
	Error      string // shown only when non-empty
	ActionPath string // where the form posts (the /entry path)
}

// entryTemplate is the self-contained access page. html/template auto-escapes every
// field, so the (operator-controlled) error text and the action path are injection-safe
// regardless. The key field is type=text with autofocus so a tester can paste-and-go.
var entryTemplate = template.Must(template.New("entry").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Kaimi · Access</title>
<style>
  :root { color-scheme: light dark; }
  * { box-sizing: border-box; }
  body {
    margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    background: #0b1220; color: #e8edf6;
  }
  .card {
    width: 100%; max-width: 420px; margin: 24px; padding: 32px;
    background: #121a2b; border: 1px solid #233047; border-radius: 14px;
    box-shadow: 0 12px 40px rgba(0,0,0,.35);
  }
  h1 { margin: 0 0 4px; font-size: 22px; letter-spacing: .2px; }
  p.sub { margin: 0 0 22px; color: #93a1bd; font-size: 14px; line-height: 1.45; }
  label { display: block; font-size: 13px; color: #b8c4db; margin-bottom: 8px; }
  input[type=text] {
    width: 100%; padding: 12px 14px; font-size: 16px; letter-spacing: 1px;
    border: 1px solid #2c3a55; border-radius: 9px; background: #0e1626; color: #e8edf6;
    text-transform: uppercase;
  }
  input[type=text]:focus { outline: 2px solid #3b82f6; border-color: #3b82f6; }
  button {
    margin-top: 16px; width: 100%; padding: 12px 14px; font-size: 15px; font-weight: 600;
    border: 0; border-radius: 9px; background: #3b82f6; color: #fff; cursor: pointer;
  }
  button:hover { background: #2f6fe0; }
  .err {
    margin: 0 0 18px; padding: 11px 13px; font-size: 13px; line-height: 1.4;
    background: #2a1620; border: 1px solid #5b2230; color: #f3b5c2; border-radius: 9px;
  }
  .brand { font-weight: 700; }
</style>
</head>
<body>
  <main class="card">
    <h1><span class="brand">Kaimi</span> access</h1>
    <p class="sub">Enter your product key to start your evaluation. It looks like
      <strong>KAIMI-XXXX-XXXX-XXXX</strong>.</p>
    {{if .Error}}<div class="err" role="alert">{{.Error}}</div>{{end}}
    <form method="POST" action="{{.ActionPath}}" autocomplete="off">
      <label for="key">Product key</label>
      <input id="key" name="key" type="text" placeholder="KAIMI-XXXX-XXXX-XXXX"
             autofocus spellcheck="false" autocapitalize="characters">
      <button type="submit">Enter</button>
    </form>
  </main>
</body>
</html>`))

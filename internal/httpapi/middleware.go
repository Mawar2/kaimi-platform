package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// This file implements the WS-B5 RequireSession middleware that locks down the
// protected /api/v1 group behind the signed session minted by WS-B4. It is the
// control that turns the read/action endpoints from "reachable by anyone" into
// "reachable only by an authenticated Workspace user."
//
// The middleware and the login flow (auth.go) MUST verify with the SAME session
// manager that minted the cookie — otherwise a valid login would be rejected, or
// worse, a token signed by a stale key would be honored. They are shared via
// Deps.Auth: AuthHandler owns the one sessionManager, RequireSession reads through
// it (s.deps.Auth.session). There is exactly one HMAC key in the process.

// sessionContextKey is the unexported key type under which the verified Session is
// stored in the request context. Using a dedicated unexported type (not a string)
// prevents collisions with context keys set by other packages.
type sessionContextKey struct{}

// SessionFromContext returns the verified Session the RequireSession middleware
// injected into the request context, and true, on an authenticated request.
// Handlers behind the middleware (e.g. GET /api/v1/me) use it to read the caller's
// identity. It returns (nil, false) on a context the middleware never touched, so
// callers can distinguish authenticated from anonymous requests.
func SessionFromContext(ctx context.Context) (*Session, bool) {
	s, ok := ctx.Value(sessionContextKey{}).(*Session)
	return s, ok
}

// DashboardIdentity resolves the signed-in operator's email and a per-session CSRF
// token from the verified session in ctx, for the WS-C3 SSR onboarding flow. It is
// the adapter cmd/api wraps into a dashboard.IdentityFunc so the dashboard package
// (which must not import httpapi — that would be an import cycle) gets the identity
// and a CSRF token WITHOUT reaching the private session machinery directly.
//
// The CSRF token is HMAC-SHA256(sessionSecret, "csrf:"+subject) over the SAME server
// HMAC key that signs the session cookie. It is therefore: (a) stable for the life of
// a session (so a GET-rendered form token still matches on the POST), (b) bound to
// the specific signed-in subject, and (c) unforgeable without the server secret. The
// onboarding POST compares the submitted token to this value in constant time.
//
// It returns ok=false when no session is present (e.g. insecure dev mode), in which
// case onboarding renders the signed-out treatment and relies on SameSite=Lax alone.
func (a *AuthHandler) DashboardIdentity(ctx context.Context) (email, csrfToken string, ok bool) {
	sess, ok := SessionFromContext(ctx)
	if !ok || sess == nil {
		return "", "", false
	}
	return sess.Email, a.session.csrfToken(sess.Subject), true
}

// RequireSession wraps next so every request must carry a valid session cookie. On
// success it injects the verified Session into the request context (reachable via
// SessionFromContext) and calls next. On failure — missing, malformed, forged, or
// expired cookie — it answers 401 with the API's JSON error envelope and does NOT
// call next; it never redirects, because this wraps the JSON API group, not an HTML
// surface.
//
// It is a method on *Server so it closes over the shared session manager held by
// Deps.Auth. It is applied ONLY to the /api/v1 group in Routes(); the public routes
// (/healthz, /auth/*) are registered outside the wrap and stay reachable without a
// session. RequireSession must only be called when Deps.Auth is non-nil (Routes()
// guards this and skips the wrap in offline/dev mode).
func (s *Server) RequireSession(next http.Handler) http.Handler {
	// Capture the shared session manager once at wrap time. Auth is guaranteed
	// non-nil by the Routes() guard, but verify defensively rather than panic on a
	// misuse: with no manager we cannot authenticate, so fail closed (401).
	var sm *sessionManager
	if s.deps.Auth != nil {
		sm = s.deps.Auth.session
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sm == nil {
			// Fail closed: a wrapped group with no way to authenticate must not pass
			// requests through. (Routes() prevents this in practice.)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		sess, err := sm.ParseSession(r)
		if err != nil {
			// Do not leak which check failed (absent vs. forged vs. expired) and never
			// log the cookie value — only that the request was unauthenticated.
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		ctx := context.WithValue(r.Context(), sessionContextKey{}, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// loginPath is where RequireSessionHTML sends an unauthenticated human to sign in.
// It is the same /auth/login route the WS-B4 OAuth flow registers on the root mux.
const loginPath = "/auth/login"

// RequireSessionHTML is the WS-C3a HTML variant of RequireSession: it guards the
// server-side-rendered dashboard so only authenticated Workspace users reach it.
// It uses the SAME session manager as RequireSession (Deps.Auth.session), so a
// single signed cookie authorizes both the HTML pages and the /api/v1 API.
//
// The ONLY difference from RequireSession is the failure mode. The JSON API answers
// a missing/invalid session with 401 JSON; an HTML surface instead REDIRECTS the
// browser (302) to /auth/login so a human is taken to sign in. The redirect carries
// a sanitized "return" query parameter (the original request path) so login can send
// the user back where they were headed.
//
// SECURITY: the return path is passed through safeReturnPath, which accepts only
// local, single-slash-rooted relative paths — defeating open-redirect attacks (a
// crafted //evil.com or http://evil return value collapses to "/"). The redirect
// deliberately does NOT reveal why auth failed (absent vs. forged vs. expired), and
// never logs the cookie. On a valid session it injects the verified identity into
// the request context (reachable via SessionFromContext) and serves the page.
//
// Like RequireSession, it is a method on *Server so it closes over the shared
// session manager, and it is applied ONLY to the HTML "/" mount in Routes(); the
// public routes (/healthz, /auth/*) and the JSON API keep their own handling.
func (s *Server) RequireSessionHTML(next http.Handler) http.Handler {
	// Capture the shared session manager once at wrap time (same source as the API's
	// RequireSession). Auth is guaranteed non-nil by the Routes() guard, but verify
	// defensively: with no manager we cannot authenticate, so fail closed by
	// redirecting to login rather than serving the page.
	var sm *sessionManager
	if s.deps.Auth != nil {
		sm = s.deps.Auth.session
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectToLogin := func() {
			// Preserve BOTH the path and the raw query so a deep link like
			// /dashboard?filter=active survives login intact. safeReturnPath validates
			// the path/host portion and rejects open-redirect forms while allowing a
			// normal "?key=val" query on an otherwise-local path.
			target := r.URL.Path
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			dest := loginPath + "?return=" + url.QueryEscape(safeReturnPath(target))
			http.Redirect(w, r, dest, http.StatusFound)
		}

		if sm == nil {
			// Fail closed: no way to authenticate → send the human to sign in.
			redirectToLogin()
			return
		}

		sess, err := sm.ParseSession(r)
		if err != nil {
			// Do not leak which check failed and never log the cookie value — only
			// bounce the browser to login.
			redirectToLogin()
			return
		}

		ctx := context.WithValue(r.Context(), sessionContextKey{}, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// safeReturnPath sanitizes a post-login return path so the login redirect (and the
// post-login redirect at the moment of use) can never be turned into an open
// redirect. It returns p only when p is a LOCAL, relative path rooted at a single
// "/"; anything else (empty, scheme-relative "//host", absolute "http://host",
// backslash tricks, non-rooted, control chars/whitespace) collapses to "/".
//
// A normal query string ("?key=val") on an otherwise-local path is preserved: the
// guard validates the PATH portion (everything before the first "?") for the
// dangerous host-escaping forms, then re-attaches the original query/fragment only
// if the path is safe. This lets deep links like "/dashboard?filter=active" survive
// login while "//evil.com?x=1" and "https://evil/?x=1" still collapse to "/".
//
// The guard is intentionally strict: it requires a leading "/", forbids a second
// leading "/" or "\" (which browsers may treat as a scheme-relative host), and
// rejects any value url.Parse reports as having a scheme or host.
//
// It is unexported but lives in the httpapi package so both RequireSessionHTML
// (setting the return param) and the OAuth callback (re-validating it at redirect
// time, defense-in-depth) reach it.
func safeReturnPath(p string) string {
	const safe = "/"
	if p == "" || p[0] != '/' {
		// Must be rooted at "/". Empty and relative paths are unsafe.
		return safe
	}
	// Validate the PATH portion only — split off any query/fragment first so a benign
	// "?key=val" does not cause an otherwise-local path to be rejected. The dangerous
	// open-redirect forms ("//host", "/\\host", "https://host") all live in the path
	// portion, so checking it is sufficient.
	pathOnly := p
	if i := strings.IndexAny(pathOnly, "?#"); i >= 0 {
		pathOnly = pathOnly[:i]
	}
	// After stripping the query/fragment the path must still be rooted (e.g. "?x=1"
	// alone, which would leave an empty path, is not a legitimate dashboard route).
	if pathOnly == "" || pathOnly[0] != '/' {
		return safe
	}
	// Reject scheme-relative ("//host") and the "/\\" backslash variant some browsers
	// normalize to "//"; both can escape to an external host.
	if len(pathOnly) > 1 && (pathOnly[1] == '/' || pathOnly[1] == '\\') {
		return safe
	}
	// A well-formed local path must parse with no scheme and no host. This also rejects
	// values like "/\t//evil" that slip past the prefix checks once normalized.
	u, err := url.Parse(pathOnly)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return safe
	}
	// Defense in depth: a control character or whitespace anywhere in the value is not
	// a legitimate dashboard route. Check the full p (path + query + fragment) so an
	// injected newline/space in the query cannot ride along.
	if strings.ContainsAny(p, " \t\r\n") {
		return safe
	}
	return p
}

// handleMe serves GET /api/v1/me, returning the authenticated caller's identity
// (email, Workspace domain, subject) from the session in context. It lives inside
// the RequireSession-protected group, so by the time it runs the session is
// guaranteed present; the missing-session branch only guards against the route
// being mounted without the middleware (a wiring bug) and answers 401 rather than
// dereferencing a nil session.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, MeResponse{
		Email:   sess.Email,
		Domain:  sess.Domain,
		Subject: sess.Subject,
	})
}

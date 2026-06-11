package httpapi

import (
	"context"
	"net/http"
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

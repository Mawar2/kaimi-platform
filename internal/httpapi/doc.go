// Package httpapi implements Kaimi's JSON HTTP API: a programmatic surface over
// the same opportunity store, dashboard read layer, and proposal action service
// that power the server-rendered dashboard.
//
// This package exposes the liveness check (WS-B1), the read endpoints (WS-B2),
// the select/action endpoints (WS-B3), and Workspace OAuth2/OIDC sign-in (WS-B4).
// The cross-cutting RequireSession middleware (WS-B5) plugs into the same Server
// and ServeMux without reshaping this foundation:
//
//   - auth.go + session.go implement OAuth sign-in: GET /auth/login → Google →
//     GET /auth/callback mints a signed (HMAC-SHA256) session cookie, restricted
//     to one Google Workspace domain (hd) with a verified email; POST /auth/logout
//     clears it. The /auth/* routes are unauthenticated (on the root mux, outside
//     /api/v1) and are wired only when OAuth is configured. ParseSession is the
//     exported seam WS-B5's middleware calls to authenticate /api/v1 requests.
//
//   - Server holds its dependencies (a *dashboard.Service for reads and a
//     *proposal.Service for actions) so later handlers read and act through the
//     same wiring cmd/dashboard uses.
//
//   - Routes builds a Go 1.25 http.ServeMux using method+wildcard patterns
//     (e.g. "GET /opportunities/{id}"), so new routes are one mux.HandleFunc away.
//
//   - response.go centralizes JSON success/error encoding so every handler
//     returns a consistent envelope and content type.
//
//   - config.go isolates the HTTP/server layer's settings (Host/Port today,
//     OAuth fields in WS-B4) from the app-wide internal/config.Config.
//
// The package depends only on the standard library's net/http for routing — no
// third-party router — per the project's minimal-dependency rule.
package httpapi

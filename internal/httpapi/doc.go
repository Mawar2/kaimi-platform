// Package httpapi implements Kaimi's JSON HTTP API: a programmatic surface over
// the same opportunity store, dashboard read layer, and proposal action service
// that power the server-rendered dashboard.
//
// This package is the skeleton the later API tickets build on. Today it exposes
// only an unauthenticated GET /healthz liveness check (WS-B1). The read
// endpoints (WS-B2), the select/action endpoints (WS-B3), OAuth (WS-B4), and the
// cross-cutting middleware (WS-B5) plug into the same Server and ServeMux without
// reshaping this foundation:
//
//   - Server holds its dependencies (a *dashboard.Service for reads and a
//     *proposal.Service for actions) so later handlers read and act through the
//     same wiring cmd/dashboard uses.
//   - Routes builds a Go 1.25 http.ServeMux using method+wildcard patterns
//     (e.g. "GET /opportunities/{id}"), so new routes are one mux.HandleFunc away.
//   - response.go centralizes JSON success/error encoding so every handler
//     returns a consistent envelope and content type.
//   - config.go isolates the HTTP/server layer's settings (Host/Port today,
//     OAuth fields in WS-B4) from the app-wide internal/config.Config.
//
// The package depends only on the standard library's net/http for routing — no
// third-party router — per the project's minimal-dependency rule.
package httpapi

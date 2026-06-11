package httpapi

import (
	"log"
	"net/http"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/profile"
)

// serviceName identifies this binary in health and (later) log output.
const serviceName = "kaimi-api"

// Deps are the collaborators a Server needs. Both are injected so tests can
// supply fixtures and so cmd/api can build them exactly the way cmd/dashboard
// does (a dashboard.Service for reads, a proposal.Service for actions). Both may
// be nil in the WS-B1 skeleton — /healthz uses neither — but WS-B2/WS-B3 will
// require them, so the constructor stores them as-is rather than validating now.
type Deps struct {
	// Dashboard is the store-backed read layer (stage derivation + views) that the
	// read endpoints (WS-B2) render over.
	Dashboard *dashboard.Service

	// Proposals is the Zone-2 action service the select/gate endpoints (WS-B3)
	// drive. It is the ProposalService interface (not the concrete
	// *proposal.Service) so production wiring injects the real service while tests
	// inject a fake — "accept interfaces, return structs." It may be nil for a
	// read-only API deployment, in which case the action/status endpoints answer
	// 503 Service Unavailable.
	Proposals ProposalService

	// Auth serves the unauthenticated /auth/* OAuth endpoints (WS-B4). It is nil in
	// the offline/dev deployment (OAuth not configured), in which case the /auth/*
	// routes are simply not registered. When present, its session manager also backs
	// the WS-B5 RequireSession middleware via ParseSession.
	Auth *AuthHandler

	// AllowInsecureNoAuth is an EXPLICIT, dev-only opt-in to run the /api/v1 API
	// WITHOUT authentication when no OAuth is configured (Auth == nil). It defaults
	// to false so the server FAILS CLOSED: a production deploy with a missing or
	// typo'd OAuth env var will NOT silently serve an open API — Routes() panics
	// instead. The caller (cmd/api) must deliberately set this true (via
	// -insecure-no-auth / KAIMI_INSECURE_NO_AUTH) to run open for local UI dev. It
	// has no effect when Auth is non-nil; OAuth always takes precedence.
	AllowInsecureNoAuth bool

	// ProfileStore persists the tenant's company profile (WS-C1) so onboarding can
	// configure a deployment at runtime via GET/PUT /api/v1/profile without editing
	// files. It may be nil for a deployment that does not expose runtime profile
	// configuration, in which case the /api/v1/profile routes answer 503 Service
	// Unavailable (mirroring how the action endpoints degrade when Proposals is nil).
	ProfileStore profile.ProfileStore

	// Drive serves the WS-C2 customer-Drive connect endpoints
	// (/api/v1/integrations/drive/*). It lets a deployment connect the CUSTOMER's
	// own Google Workspace so generated Docs land in their Drive. It may be nil when
	// customer-Drive connect is not configured, in which case those routes answer
	// 503 Service Unavailable (mirroring how Proposals/ProfileStore degrade when
	// absent). It is registered INSIDE the protected /api/v1 group — connecting a
	// Drive is an authenticated action — even though the consent handshake itself
	// bounces through Google.
	Drive *DriveHandler

	// AllowedOrigins is the CORS allow-list (WS-B6). It is EMPTY by default, in which
	// case CORS is a no-op and the API is same-origin only (the preferred
	// deployment). Populate it (from Config.AllowedOrigins / CORS_ALLOWED_ORIGINS)
	// only when a browser SPA is served from a different origin than the API. Each
	// entry is a full origin (e.g. "https://app.example.com"); the middleware echoes
	// the matching origin and never "*", because the API uses credentialed cookies.
	AllowedOrigins []string
}

// Server is the JSON API's HTTP application. It holds its dependencies and builds
// the route table; it does not own its own *http.Server lifecycle (listen/
// shutdown), which the cmd/api entry point manages so the binary controls graceful
// shutdown and signal handling.
type Server struct {
	deps Deps
}

// New constructs a Server from its dependencies. The dependencies are stored
// as-is so later tickets' handlers can read through Deps.Dashboard and act through
// Deps.Proposals without rewiring.
func New(deps Deps) *Server { //nolint:gocritic // Deps is an established by-value dependency struct constructed at every call site (cmd/api, tests); it crossed the size threshold only as fields accreted across WS-B/WS-C, and switching to a pointer here would churn all callers for no behavioral gain.
	return &Server{deps: deps}
}

// Routes builds and returns the API's HTTP handler. It uses Go 1.25
// http.ServeMux instances with method + wildcard patterns ("GET /path/{id}").
//
// The route table is split into two layers to give WS-B5 a clean auth seam:
//
//   - apiMux holds the PROTECTED API group (everything under /api/v1/...). The
//     read (WS-B2) and select (WS-B3) endpoints register here. WS-B5 wraps ONLY
//     this group with auth middleware — apiHandler is the single point it
//     decorates, so authentication can never accidentally cover the probe.
//   - rootMux is the public surface. It mounts the protected group under
//     "/api/v1/" and registers the UNAUTHENTICATED routes — GET /healthz today,
//     /auth/* in WS-B4 — directly on itself, OUTSIDE the wrapper.
//
// The whole rootMux is then wrapped by jsonErrorResponder so the stdlib mux's
// plain-text 404 (unknown path) and 405 (unsupported method) bodies are rewritten
// into the API's JSON error envelope. That wrapper is a generic response shim and
// is NOT the auth seam — WS-B5 wraps apiHandler (the /api/v1 group), leaving
// rootMux's own routes (/healthz, /auth/*) reachable without a session.
func (s *Server) Routes() http.Handler {
	// apiMux is the protected group. It is empty in the WS-B1 skeleton; WS-B2/B3
	// register their endpoints here (e.g. "GET /api/v1/opportunities"). WS-B5
	// wraps the handler derived from it, not rootMux.
	apiMux := http.NewServeMux()

	// WS-B2 read endpoints. Registered with their full "/api/v1/..." patterns so
	// the route strings are self-describing at the call site (StripPrefix is
	// intentionally omitted on the mount below).
	apiMux.HandleFunc("GET /api/v1/opportunities", s.handleListOpportunities)
	apiMux.HandleFunc("GET /api/v1/opportunities/{id}", s.handleGetOpportunity)
	apiMux.HandleFunc("GET /api/v1/stages/counts", s.handleStageCounts)

	// WS-B3 action + proposal-status endpoints. The select POST is the Zone-1 →
	// Zone-2 bridge; the proposal GET composes the read layer with the draft.
	apiMux.HandleFunc("POST /api/v1/opportunities/{id}/select", s.handleSelectOpportunity)
	apiMux.HandleFunc("GET /api/v1/proposals/{id}", s.handleGetProposalStatus)

	// WS-B5 identity endpoint. It reads the caller's identity from the session the
	// RequireSession middleware injects, so it is registered inside the protected
	// group and only ever runs after authentication.
	apiMux.HandleFunc("GET /api/v1/me", s.handleMe)

	// WS-C1 runtime profile configuration. Both are protected (onboarding is an
	// authenticated action) and degrade to 503 when no ProfileStore is wired.
	apiMux.HandleFunc("GET /api/v1/profile", s.handleGetProfile)
	apiMux.HandleFunc("PUT /api/v1/profile", s.handlePutProfile)

	// WS-C2 customer-Drive connect. All four endpoints are protected (connecting a
	// Drive is an authenticated action) and degrade to 503 when no DriveHandler is
	// wired. The connect/callback handshake itself bounces through Google's consent
	// screen, but the endpoints are only reachable by an authenticated operator.
	apiMux.HandleFunc("GET /api/v1/integrations/drive/connect", s.handleDriveConnect)
	apiMux.HandleFunc("GET /api/v1/integrations/drive/callback", s.handleDriveCallback)
	apiMux.HandleFunc("GET /api/v1/integrations/drive/status", s.handleDriveStatus)
	apiMux.HandleFunc("PUT /api/v1/integrations/drive/target", s.handleDriveSetTarget)

	// WS-B5 auth seam: wrap ONLY this group with RequireSession so /api/v1/* demands
	// a valid session, while rootMux's own routes (/healthz, /auth/*) stay public.
	//
	// The wrap decision FAILS CLOSED by default. There are exactly three cases:
	//
	//   1. Auth configured (Deps.Auth != nil) → always wrap with RequireSession.
	//      This is the production path; OAuth always takes precedence.
	//   2. Auth nil AND AllowInsecureNoAuth == true → skip the wrap and log a loud
	//      WARNING. This is the EXPLICIT local-dev opt-in for credential-less UI work.
	//   3. Auth nil AND AllowInsecureNoAuth == false → PANIC. We refuse to serve an
	//      open API by default, so a production deploy with a missing/typo'd OAuth
	//      env var can never silently come up unauthenticated. A panic at wiring time
	//      is the desired backstop: the insecure server never starts.
	var apiHandler http.Handler = apiMux
	switch {
	case s.deps.Auth != nil:
		apiHandler = s.RequireSession(apiHandler)
	case s.deps.AllowInsecureNoAuth:
		log.Printf("WARNING: Workspace OAuth not configured; the /api/v1 API is UNAUTHENTICATED (insecure local/dev mode opted in via AllowInsecureNoAuth). Do NOT use this configuration in production.")
	default:
		panic("httpapi: refusing to serve an unauthenticated API: configure Workspace OAuth, or set AllowInsecureNoAuth for local dev only")
	}

	rootMux := http.NewServeMux()

	// Protected API group, mounted under its prefix. StripPrefix is intentionally
	// omitted so handlers register with their full "/api/v1/..." pattern, keeping
	// the route strings self-describing at the call site.
	rootMux.Handle("/api/v1/", apiHandler)

	// Unauthenticated routes live on the root mux, outside the wrapped group.
	rootMux.HandleFunc("GET /healthz", s.handleHealth)

	// OAuth sign-in endpoints (WS-B4). They are registered on the root mux — OUTSIDE
	// the protected /api/v1 group — because a user must be able to reach login and
	// callback WITHOUT a session. They are wired only when OAuth is configured;
	// offline/dev mode (Auth nil) omits them.
	if s.deps.Auth != nil {
		rootMux.HandleFunc("GET /auth/login", s.deps.Auth.handleLogin)
		rootMux.HandleFunc("GET /auth/callback", s.deps.Auth.handleCallback)
		rootMux.HandleFunc("POST /auth/logout", s.deps.Auth.handleLogout)
	}

	// Wrap the mux so its built-in plain-text 404/405 responses come back as JSON.
	// A catch-all "/" route is intentionally NOT used: it would shadow the mux's
	// per-path 405 dispatch (a method mismatch on a known path would fall through
	// to "/" and 404 instead of 405). The response shim preserves the mux's own
	// status codes and only rewrites the body/content type when nothing was
	// written yet.
	handler := jsonErrorResponder(rootMux)

	// CORS is applied at the ROOT level — OUTSIDE jsonErrorResponder's routing — so
	// it also covers the public /auth and /healthz routes and so an allowed-origin
	// OPTIONS preflight is answered (204) BEFORE the mux's per-path method dispatch
	// would 405 it. With no origins configured (the same-origin default) CORS is a
	// no-op pass-through, so it never interferes with RequireSession: the auth wrap
	// on /api/v1 still runs untouched for every real (non-preflight) request.
	return CORS(s.deps.AllowedOrigins)(handler)
}

// handleHealth serves the unauthenticated liveness probe. It reports 200 with a
// small JSON body and touches no dependency, so it succeeds even before the store
// or agents are reachable.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok", Service: serviceName})
}

// jsonErrorResponder wraps an http.Handler (the route mux) so the stdlib
// ServeMux's plain-text 404 and 405 responses are rewritten into the API's JSON
// error envelope. Application handlers (e.g. /healthz) write their own JSON before
// the mux's fallbacks ever run, so this only takes effect for the mux's
// auto-generated 404 (unknown path) and 405 (method not allowed) responses.
//
// It works by buffering the handler's first WriteHeader: if the status is a 404
// or 405 and the handler wrote no body of its own, the shim discards the plain
// body and emits the JSON envelope instead; any other status (or a status with a
// body already started) passes through unchanged. This keeps the mux's status
// codes authoritative — including its per-path 405 dispatch — while making every
// error response JSON.
func jsonErrorResponder(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jw := &jsonErrorWriter{ResponseWriter: w}
		next.ServeHTTP(jw, r)
		jw.finish()
	})
}

// jsonErrorWriter intercepts an http.ResponseWriter to rewrite the stdlib mux's
// plain-text 404/405 responses as JSON. It defers committing the status until
// finish() so a 404/405 with no application body can be replaced wholesale.
type jsonErrorWriter struct {
	http.ResponseWriter
	status      int  // status passed to WriteHeader; 0 until set
	wroteHeader bool // whether WriteHeader has been observed
	wroteBody   bool // whether the wrapped handler wrote any body bytes
	committed   bool // whether we have flushed status+headers downstream
}

// WriteHeader records the status without forwarding it yet, so finish() can decide
// whether to replace a bare 404/405 with a JSON envelope.
func (jw *jsonErrorWriter) WriteHeader(status int) {
	if jw.wroteHeader {
		return
	}
	jw.status = status
	jw.wroteHeader = true
}

// Write forwards body bytes. For a 404/405 the body is the mux's plain-text
// message, which we suppress (and replace in finish()); for any other status the
// bytes are an application response and pass through, committing the status first.
func (jw *jsonErrorWriter) Write(b []byte) (int, error) {
	if !jw.wroteHeader {
		jw.WriteHeader(http.StatusOK)
	}
	if jw.isInterceptable() {
		// Swallow the stdlib plain-text body; finish() writes the JSON envelope.
		// Report the bytes as written so the caller sees no short-write error.
		return len(b), nil
	}
	jw.commit()
	jw.wroteBody = true
	return jw.ResponseWriter.Write(b)
}

// commit flushes the recorded status and headers downstream exactly once.
func (jw *jsonErrorWriter) commit() {
	if jw.committed {
		return
	}
	jw.committed = true
	jw.ResponseWriter.WriteHeader(jw.status)
}

// isInterceptable reports whether the recorded status is one of the mux's
// auto-generated errors we replace with JSON (404 unknown path, 405 method).
func (jw *jsonErrorWriter) isInterceptable() bool {
	return jw.status == http.StatusNotFound || jw.status == http.StatusMethodNotAllowed
}

// finish emits the JSON error envelope for an intercepted 404/405, or flushes any
// status the handler set but never wrote a body for (so an empty 200 still
// completes). It is a no-op once the response has been committed via Write.
func (jw *jsonErrorWriter) finish() {
	if jw.committed {
		return
	}
	if !jw.wroteHeader {
		// Handler wrote nothing at all; leave the default 200 to the stdlib path.
		return
	}
	if jw.isInterceptable() {
		// Replace the mux's plain-text body with the API's JSON error envelope,
		// preserving the mux's status code.
		writeError(jw.ResponseWriter, jw.status, http.StatusText(jw.status))
		jw.committed = true
		return
	}
	// Non-error status with no body (e.g. 204): forward the bare status.
	jw.commit()
}

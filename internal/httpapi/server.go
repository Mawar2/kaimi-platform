package httpapi

import (
	"net/http"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/proposal"
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
	// drive.
	Proposals *proposal.Service
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
func New(deps Deps) *Server {
	return &Server{deps: deps}
}

// Routes builds and returns the API's HTTP handler. It uses a Go 1.25
// http.ServeMux with method + wildcard patterns ("GET /path/{id}") so the read
// (WS-B2) and select (WS-B3) endpoints register here as ordinary mux entries, and
// WS-B5's middleware wraps the returned handler.
//
// Today it registers a single unauthenticated route: GET /healthz.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	return mux
}

// handleHealth serves the unauthenticated liveness probe. It reports 200 with a
// small JSON body and touches no dependency, so it succeeds even before the store
// or agents are reachable.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok", Service: serviceName})
}

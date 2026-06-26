package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/store"
)

// TestHealthzReachableThroughOuterMux is the regression guard requested by #54:
// GET /healthz (no trailing slash) must return 200 through the FULL Routes()
// composition — when DashboardHTML wires the C3a OUTER mux AND the telemetry
// streamMux wraps everything, each adding a "/" catch-all that /healthz must not
// fall through to.
//
// #54 reported /healthz 404ing on a live deploy. It does NOT reproduce on current
// main (the explicit "/healthz" registration on the outer mux + Go 1.22 ServeMux's
// most-specific-pattern precedence route it correctly); the live 404 was a deployed
// image lagging main. This test exists so that stays true: earlier tests built
// Routes() WITHOUT DashboardHTML, so the outer mux — and this path — went untested.
func TestHealthzReachableThroughOuterMux(t *testing.T) {
	st, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	// DashboardHTML wires the C3a outer mux; Monitor wires the telemetry streamMux.
	// Together they reproduce the real deploy's full handler stack. Their own
	// behavior is irrelevant — this test only hits /healthz. AllowInsecureNoAuth
	// keeps the fail-closed gate switch from panicking with no gate configured.
	srv := New(Deps{
		DashboardHTML:       dashboard.NewHandler(dashboard.NewService(st)),
		Monitor:             http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }),
		AllowInsecureNoAuth: true,
	})
	h := srv.Routes()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /healthz = %d, want 200 (the un-slashed health probe must be reachable through the outer mux)", rec.Code)
	}
}

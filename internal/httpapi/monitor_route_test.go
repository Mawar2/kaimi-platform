package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeMonitor stands in for monitor.Handler() so this smoke test does not depend
// on the telemetry core's embedded bundle.
type fakeMonitor struct{}

func (fakeMonitor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Echo the post-StripPrefix path so we can assert the mount prefix is removed.
	_, _ = w.Write([]byte("MONITOR path=" + r.URL.Path))
}

func TestMonitorMountStripsPrefixAndServes(t *testing.T) {
	srv := New(Deps{Monitor: fakeMonitor{}, AllowInsecureNoAuth: true})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/monitor/assets/app.js", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "MONITOR path=/assets/app.js" {
		t.Fatalf("body = %q, want prefix stripped to /assets/app.js", got)
	}
}

func TestMonitorGatedRedirectsUnauthenticated(t *testing.T) {
	gate, _ := newTestGate(t, time.Now().UTC())
	srv := New(Deps{Monitor: fakeMonitor{}, ProductKey: gate})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/monitor/", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 redirect to entry", rec.Code)
	}
	if strings.HasPrefix(rec.Body.String(), "MONITOR") {
		t.Fatalf("unauthenticated request reached the monitor handler")
	}
}

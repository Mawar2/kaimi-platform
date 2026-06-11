package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// corsReachHandler is the inner handler CORS wraps in these tests. It reports 200
// with a sentinel so a test can prove the request reached it (CORS did not
// short-circuit).
func corsReachHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("reached"))
	})
}

// TestCORSAllowedOriginEchoesOrigin verifies that for an allowed Origin the
// middleware echoes that SPECIFIC origin (never "*"), sets Allow-Credentials:true,
// advertises methods/headers, and still calls the inner handler for the actual
// (non-preflight) request.
func TestCORSAllowedOriginEchoesOrigin(t *testing.T) {
	const origin = "https://app.example.com"
	h := CORS([]string{origin})(corsReachHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/opportunities", http.NoBody)
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("Allow-Origin = %q, want the specific origin %q", got, origin)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Error("Allow-Origin must never be \"*\" when credentials are allowed")
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, want \"true\"", got)
	}
	if got := rec.Header().Get("Vary"); got == "" {
		t.Error("Vary header should include Origin so caches key per-origin")
	}
	if rec.Code != http.StatusOK || rec.Body.String() != "reached" {
		t.Errorf("non-preflight request should reach inner handler: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

// TestCORSPreflightReturns204 verifies an OPTIONS preflight from an allowed origin
// is answered 204 by the middleware itself (the inner handler is NOT invoked) with
// the CORS headers and the allowed methods/headers advertised.
func TestCORSPreflightReturns204(t *testing.T) {
	const origin = "https://app.example.com"
	reached := false
	inner := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })
	h := CORS([]string{origin})(inner)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/opportunities", http.NoBody)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", rec.Code)
	}
	if reached {
		t.Error("preflight must be handled by CORS middleware, not passed to inner handler")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("preflight Allow-Origin = %q, want %q", got, origin)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("preflight should advertise Access-Control-Allow-Methods")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("preflight should advertise Access-Control-Allow-Headers")
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("preflight Allow-Credentials = %q, want \"true\"", got)
	}
}

// TestCORSDisallowedOriginNoHeaders verifies a request from an origin NOT in the
// allow-list gets no CORS headers, yet still proceeds to the inner handler (so a
// same-origin or server-to-server caller is never blocked by this middleware).
func TestCORSDisallowedOriginNoHeaders(t *testing.T) {
	h := CORS([]string{"https://app.example.com"})(corsReachHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/opportunities", http.NoBody)
	req.Header.Set("Origin", "https://evil.example.org")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty for a disallowed origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Allow-Credentials = %q, want empty for a disallowed origin", got)
	}
	if rec.Code != http.StatusOK || rec.Body.String() != "reached" {
		t.Errorf("disallowed origin should still reach inner handler: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

// TestCORSDisallowedPreflightDoesNotEmitHeaders verifies an OPTIONS preflight from a
// disallowed origin gets no Allow-Origin header. The request is still passed through
// (the middleware only short-circuits preflights for ALLOWED origins); the inner
// mux then answers it.
func TestCORSDisallowedPreflightDoesNotEmitHeaders(t *testing.T) {
	reached := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	h := CORS([]string{"https://app.example.com"})(inner)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/opportunities", http.NoBody)
	req.Header.Set("Origin", "https://evil.example.org")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty for a disallowed preflight", got)
	}
	if !reached {
		t.Error("a disallowed-origin preflight should pass through to the inner handler, not be answered 204 by CORS")
	}
}

// TestCORSNoOriginsConfiguredIsNoOp verifies that when no origins are configured the
// middleware is a transparent pass-through: no CORS headers, OPTIONS is NOT
// short-circuited, and every request reaches the inner handler unchanged. This is
// the preferred same-origin deployment.
func TestCORSNoOriginsConfiguredIsNoOp(t *testing.T) {
	for _, origins := range [][]string{nil, {}, {""}} {
		reached := false
		inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached = true
			w.WriteHeader(http.StatusOK)
		})
		h := CORS(origins)(inner)

		// Even an OPTIONS with an Origin must pass straight through when CORS is off.
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/opportunities", http.NoBody)
		req.Header.Set("Origin", "https://app.example.com")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("origins=%v: Allow-Origin = %q, want empty (no-op)", origins, got)
		}
		if !reached {
			t.Errorf("origins=%v: no-op middleware must pass the request through", origins)
		}
	}
}

// TestParseCORSOrigins verifies the comma-separated env parsing trims spaces and
// drops empty entries so a trailing comma or stray space does not create a "" origin
// that could be matched against.
func TestParseCORSOrigins(t *testing.T) {
	got := parseCORSOrigins("https://a.example.com, https://b.example.com ,, ")
	want := []string{"https://a.example.com", "https://b.example.com"}
	if len(got) != len(want) {
		t.Fatalf("parseCORSOrigins len = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseCORSOrigins[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if len(parseCORSOrigins("")) != 0 {
		t.Error("parseCORSOrigins(\"\") should be empty")
	}
}

// TestLoadConfigCORSOrigins verifies LoadConfig reads CORS_ALLOWED_ORIGINS into the
// Config.AllowedOrigins slice, and leaves it empty when unset (no-op default).
func TestLoadConfigCORSOrigins(t *testing.T) {
	t.Setenv(envAPIHost, "")
	t.Setenv(envAPIPort, "")
	t.Setenv(envPort, "")

	t.Setenv(envCORSOrigins, "https://app.example.com")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != "https://app.example.com" {
		t.Errorf("AllowedOrigins = %v, want [https://app.example.com]", cfg.AllowedOrigins)
	}

	t.Setenv(envCORSOrigins, "")
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig (unset): %v", err)
	}
	if len(cfg.AllowedOrigins) != 0 {
		t.Errorf("AllowedOrigins = %v, want empty when CORS unset", cfg.AllowedOrigins)
	}
}

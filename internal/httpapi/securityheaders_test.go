package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSecurityHeadersSet verifies the OWASP baseline headers are present on responses and
// that the wrapped handler still runs.
func TestSecurityHeadersSet(t *testing.T) {
	called := false
	h := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))

	if !called {
		t.Fatal("wrapped handler was not called")
	}
	want := map[string]string{
		"Strict-Transport-Security": "max-age=63072000; includeSubDomains",
		"X-Frame-Options":           "DENY",
		"X-Content-Type-Options":    "nosniff",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Content-Security-Policy not set")
	}
	// Clickjacking protection must be present in the CSP.
	if !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP missing frame-ancestors 'none': %q", csp)
	}
	// The SSR dashboard embeds icons + web fonts as data: URIs; the CSP must allow them or
	// the browser blocks the fonts and the UI degrades to system fonts (browser-QA regression).
	if !strings.Contains(csp, "font-src 'self' data:") {
		t.Errorf("CSP missing font-src 'self' data: (embedded fonts would be blocked): %q", csp)
	}
	if !strings.Contains(csp, "img-src 'self' data:") {
		t.Errorf("CSP missing img-src 'self' data: (embedded icons would be blocked): %q", csp)
	}
}

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRoutesHealthzReturnsOK verifies the skeleton's only route: GET /healthz
// answers 200 with the expected JSON body and content type, using nil deps to
// prove the probe touches neither the store nor the agents.
func TestRoutesHealthzReturnsOK(t *testing.T) {
	// No OAuth in this skeleton test; opt in to the insecure no-auth path so
	// Routes() builds instead of failing closed (the production default).
	srv := New(Deps{AllowInsecureNoAuth: true})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}

	var got HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got.Status != "ok" {
		t.Errorf("status = %q, want %q", got.Status, "ok")
	}
	if got.Service != serviceName {
		t.Errorf("service = %q, want %q", got.Service, serviceName)
	}
}

// TestRoutesHealthzRejectsNonGET confirms the method-scoped pattern
// ("GET /healthz") does not answer POST, so the route table enforces the verb,
// and that the mux's 405 is rewritten into the API's JSON error envelope by the
// jsonErrorResponder wrapper (not stdlib's text/plain "Method Not Allowed").
func TestRoutesHealthzRejectsNonGET(t *testing.T) {
	srv := New(Deps{AllowInsecureNoAuth: true})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodPost, "/healthz", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /healthz status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}

	var got ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got.Error == "" {
		t.Errorf("error envelope = %+v, want a non-empty error message", got)
	}
}

// TestRoutesUnknownPathReturns404 confirms the catch-all "/" handler 404s an
// unregistered route with the API's JSON error envelope (not stdlib's text/plain
// "404 page not found"), so the skeleton does not silently swallow paths later
// tickets will add and clients always get a single error shape.
func TestRoutesUnknownPathReturns404(t *testing.T) {
	srv := New(Deps{AllowInsecureNoAuth: true})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown path status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}

	var got ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got.Error == "" {
		t.Errorf("error envelope = %+v, want a non-empty error message", got)
	}
}

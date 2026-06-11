package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWriteJSONSetsStatusContentTypeAndBody verifies writeJSON emits the given
// status, the JSON content type, and the encoded value.
func TestWriteJSONSetsStatusContentTypeAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusCreated, HealthResponse{Status: "ok", Service: "x"})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}

	var got HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got.Status != "ok" || got.Service != "x" {
		t.Errorf("body = %+v, want {ok x}", got)
	}
}

// TestWriteJSONUnmarshalableValueEmits500 verifies writeJSON catches a marshal
// failure BEFORE committing the status: an unmarshalable value (here a struct
// containing a chan, which encoding/json cannot encode) yields a 500 with the
// static JSON error envelope — not a 200 with a truncated body.
func TestWriteJSONUnmarshalableValueEmits500(t *testing.T) {
	rec := httptest.NewRecorder()

	// A chan cannot be marshalled by encoding/json, so Marshal fails.
	type bad struct {
		Ch chan int `json:"ch"`
	}
	writeJSON(rec, http.StatusOK, bad{Ch: make(chan int)})

	if rec.Code == http.StatusOK {
		t.Fatalf("status = %d, want a non-200 on marshal failure (must not commit the requested 200)", rec.Code)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}

	var got ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got.Error != "internal server error" {
		t.Errorf("error = %q, want %q", got.Error, "internal server error")
	}
}

// TestWriteErrorEmitsErrorEnvelope verifies writeError emits the given status and
// the {"error": "..."} envelope with the JSON content type.
func TestWriteErrorEmitsErrorEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "bad input")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", ct)
	}

	var got ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if got.Error != "bad input" {
		t.Errorf("error = %q, want %q", got.Error, "bad input")
	}
}

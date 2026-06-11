package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
)

// writeJSON marshals v as JSON and writes it with the given status code. It
// marshals into a buffer FIRST so that an encoding failure is caught before any
// status line or body is committed: on a marshal error it emits a 500 with a
// safe, static JSON error envelope instead of a truncated 200. Only on success
// does it set the Content-Type, write the status, and stream the bytes — so the
// status the caller asked for and the JSON content type are committed together
// with a complete body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		// v is not encodable. Nothing has been written yet, so we can still send a
		// correct 500 with a static body that is guaranteed to marshal.
		log.Printf("httpapi: marshal JSON response: %v", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		// Static literal — cannot itself fail to encode. Ignore the Write error:
		// the connection is already failing and there is nothing else to send.
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// Ignore the Write error: the status and headers are already committed and a
	// failed flush means the client is gone; there is nothing useful to do.
	_, _ = w.Write(b)
}

// writeError writes a JSON ErrorResponse with the given status and message. It is
// the single error path for handlers so every failure returns the same envelope
// ({"error": "..."}) and content type.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

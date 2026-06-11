package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
)

// writeJSON encodes v as JSON and writes it with the given status code. It sets
// the Content-Type header before WriteHeader (headers must be set first) so every
// JSON response carries the correct content type. An encoding failure is logged
// rather than returned: the status line and headers have already been committed
// by that point, so there is nothing useful to send the client instead.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The response is already partially written; we can only record the fault.
		log.Printf("httpapi: encode JSON response: %v", err)
	}
}

// writeError writes a JSON ErrorResponse with the given status and message. It is
// the single error path for handlers so every failure returns the same envelope
// ({"error": "..."}) and content type.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

package httpapi

// This file holds the JSON data-transfer objects the API returns. It starts with
// only the health response; WS-B2 fills it with the opportunity DTOs (list and
// detail views derived from the dashboard read layer) and WS-B3 adds the
// select/action request and response shapes.

// HealthResponse is the body returned by GET /healthz. It is intentionally small
// — a liveness probe only needs to confirm the process is up and serving JSON.
type HealthResponse struct {
	// Status is a fixed machine-readable token ("ok") that probes can assert on.
	Status string `json:"status"`

	// Service names the binary so a shared log/monitoring view can tell Kaimi's
	// API apart from its other HTTP surfaces.
	Service string `json:"service"`
}

// ErrorResponse is the envelope returned for non-2xx responses so clients can
// rely on a single error shape across every endpoint.
type ErrorResponse struct {
	// Error is a human-readable message describing what went wrong.
	Error string `json:"error"`
}

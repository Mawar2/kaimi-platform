// Package monitor serves the Monitor single-page application — the live
// telemetry event-stream UI — from a Go binary.
//
// The built front-end bundle (web/dist) is embedded with go:embed and exposed
// through Handler, so the SPA ships inside the kaimi-telemetry core with no
// runtime filesystem or npm dependency. The bundle must be committed for
// `go build ./...` to succeed.
//
// The Monitor is domain-agnostic: it renders whatever arrives over the
// telemetry event stream and contains no host-specific knowledge.
package monitor

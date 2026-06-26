// Package telemetry is the core of a privacy-first, self-hosted agent and
// product observability platform.
//
// The module is intentionally domain-agnostic: it knows about events,
// redaction, transport, storage, and rendering — and nothing about the host
// application that emits into it. It MUST NOT import any package outside its
// own module; in particular it must never import anything under
// github.com/Mawar2/Kaimi. That one-way dependency rule (the host depends on
// the core, never the reverse) is what keeps the module continuously
// extractable into its own repository and sellable as a standalone product.
// A CI guard enforces the rule on every change.
//
// The packages that make up the core are added across the T0.x tickets:
//
//   - event:      the Event envelope and per-attribute content/usage class
//   - sink:       the EventSink interface and its implementations
//   - emit:       the async, non-blocking emitter
//   - redact:     the redaction gate (usage may leave; content never does)
//   - httpstream: the framework-agnostic Server-Sent Events handler
//   - monitor:    the embedded real-time Monitor UI
//
// See README.md for the zero-domain-import contract and the procedure for
// splitting this module into its own repository.
package telemetry

// Package sink defines the EventSink interface — the destination every emitted
// event is delivered to — and the small set of implementations the core ships.
//
// An EventSink accepts events (Emit), optionally durably persists what it has
// buffered (Flush), and releases its resources (Close). After Close, Emit and
// Flush report ErrClosed rather than silently dropping or panicking, so callers
// can distinguish a shut-down sink from a transient failure.
//
// The shipped implementations are:
//
//   - Multi:      fans every call out to a slice of sinks, aggregating their
//     errors with errors.Join so one failing sink never hides another.
//   - JSONLSink:  append-only newline-delimited JSON to a file, buffered for
//     throughput and fsync'd on Flush for durability.
//   - LiveSink:   an in-memory ring buffer plus a set of subscriber channels
//     that powers the real-time Monitor. Ingestion never blocks: a subscriber
//     that cannot keep up has its send dropped rather than stalling Emit.
//
// The package is domain-agnostic — it moves event.Event values and knows
// nothing about the host application that produced them.
package sink

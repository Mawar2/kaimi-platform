// Package emit provides the async, non-blocking emitter that decouples the
// host application's hot path from the (potentially slow) destination sink.
//
// The emitter hands every event to a bounded in-memory channel that a pool of
// worker goroutines drains into the configured sink.EventSink. The defining
// guarantee is that Emit NEVER blocks the caller: if the buffer is full the
// event is dropped and an atomic counter is incremented rather than stalling
// the goroutine that produced it. Observability must never become a source of
// latency or back-pressure in the host.
//
// Lifecycle:
//
//   - New starts the worker pool and returns a ready Emitter.
//   - Emit offers an event to the buffer, returning whether it was accepted.
//   - Flush blocks until everything currently buffered has reached the sink and
//     the sink itself has been flushed.
//   - Shutdown stops accepting events, drains what is buffered, then flushes
//     and closes the sink. It is idempotent.
//
// Like the rest of the core, the package is domain-agnostic: it moves
// event.Event values and knows nothing about the host that produced them.
package emit

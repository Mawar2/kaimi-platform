// Package httpstream exposes a telemetry Source as a Server-Sent Events stream
// over a plain net/http handler.
//
// The handler is framework-agnostic — it depends only on the standard library
// and the core's event/sink packages, never on any host application — so it can
// be mounted on any router and the core stays extractable.
//
// On each request it asserts http.Flusher (failing with 500 if the
// ResponseWriter cannot stream), sets the SSE response headers, and resolves a
// resume cursor from the Last-Event-ID request header (falling back to the
// last_event_id query parameter). It then replays the events the client missed
// since that cursor and tails live events until the client disconnects, an
// optional per-connection maxStream cap fires (emitting a "retry:" reconnect
// hint), or the Source closes. An idle stream emits a periodic ": keep-alive"
// comment so intermediaries do not reap a healthy connection.
//
// *sink.LiveSink satisfies Source directly, so the live in-memory sink can be
// streamed with no adapter.
package httpstream

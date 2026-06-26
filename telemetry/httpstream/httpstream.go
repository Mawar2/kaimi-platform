package httpstream

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Mawar2/kaimi-telemetry/sink"
)

// defaultHeartbeat is how often an idle stream emits a keep-alive comment when
// no WithHeartbeat option overrides it. It keeps proxies and load balancers from
// reaping a quiet but healthy connection.
const defaultHeartbeat = 15 * time.Second

// reconnectHint is the reconnection delay (advertised via the SSE "retry:"
// field) the handler suggests to the client when it closes a stream that has hit
// maxStream. A short hint turns the graceful close into an invisible reconnect.
const reconnectHint = 1 * time.Second

// Source is the read side of an event stream the handler renders as SSE. It is
// deliberately the subset of *sink.LiveSink the handler needs, so the handler
// stays decoupled from the concrete sink and the core stays extractable.
type Source interface {
	// Replay returns every retained event with Seq greater than afterSeq,
	// oldest first, for catching a reconnecting client up on missed events.
	Replay(afterSeq uint64) []sink.Stamped
	// Subscribe returns a channel of future events with Seq greater than
	// afterSeq and a cancel func that unsubscribes and releases its resources.
	Subscribe(afterSeq uint64) (<-chan sink.Stamped, func())
}

// Handler is a framework-agnostic http.Handler that streams a Source as
// Server-Sent Events. It is safe for concurrent use: each request gets its own
// replay cursor, subscription, and timers.
type Handler struct {
	src       Source
	heartbeat time.Duration
	maxStream time.Duration
}

// Option configures a Handler at construction.
type Option func(*Handler)

// WithHeartbeat sets how often an idle stream emits a keep-alive comment. A
// non-positive duration is ignored and the default is kept.
func WithHeartbeat(d time.Duration) Option {
	return func(h *Handler) {
		if d > 0 {
			h.heartbeat = d
		}
	}
}

// WithMaxStream caps how long a single connection stays open before the handler
// emits a retry hint and closes it gracefully, letting the client reconnect. A
// non-positive duration (the default) disables the cap and the stream is
// unbounded.
func WithMaxStream(d time.Duration) Option {
	return func(h *Handler) { h.maxStream = d }
}

// NewHandler returns a Handler that streams src as SSE, applying opts over the
// defaults (15s heartbeat, no maxStream cap).
func NewHandler(src Source, opts ...Option) *Handler {
	h := &Handler{src: src, heartbeat: defaultHeartbeat}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// ServeHTTP streams the Source to the client as Server-Sent Events. It replays
// events missed since the client's Last-Event-ID cursor, then tails live events
// until the client disconnects, the maxStream cap fires, or the source closes.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		// Without flushing we cannot push frames as they arrive, so this would
		// be a buffered non-stream — reject it rather than degrade silently.
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	// Disable proxy buffering (nginx and some ingress controllers honor this),
	// so frames reach the client immediately instead of being batched.
	header.Set("X-Accel-Buffering", "no")

	cursor := parseCursor(r)

	// Replay first so the client never sees a gap between what it last received
	// and the live tail. Advance the cursor past the replayed events so the
	// subsequent Subscribe does not re-seed and duplicate them.
	var buf strings.Builder
	replayed := h.src.Replay(cursor)
	for i := range replayed {
		s := &replayed[i]
		if err := appendStamped(&buf, s); err != nil {
			continue // skip an unmarshalable event rather than abort the stream
		}
		if s.Seq > cursor {
			cursor = s.Seq
		}
	}
	if buf.Len() > 0 {
		_, _ = io.WriteString(w, buf.String())
	}
	// Flush now (even with no replay) so the response headers go out immediately
	// and the client's EventSource sees an open stream right away rather than
	// blocking until the first live event or heartbeat.
	flusher.Flush()

	events, unsubscribe := h.src.Subscribe(cursor)
	defer unsubscribe()

	heartbeat := time.NewTicker(h.heartbeat)
	defer heartbeat.Stop()

	var maxStreamC <-chan time.Time
	if h.maxStream > 0 {
		timer := time.NewTimer(h.maxStream)
		defer timer.Stop()
		maxStreamC = timer.C
	}

	ctx := r.Context()
	for {
		select {
		case s, open := <-events:
			if !open {
				// The source closed (e.g. sink shutdown); end the response.
				return
			}
			buf.Reset()
			if err := appendStamped(&buf, &s); err != nil {
				continue
			}
			_, _ = io.WriteString(w, buf.String())
			flusher.Flush()

		case <-heartbeat.C:
			// SSE comment line: keeps the connection warm without emitting an
			// event the client would mistake for data.
			_, _ = io.WriteString(w, ": keep-alive\n\n")
			flusher.Flush()

		case <-maxStreamC:
			// Hit the per-connection cap. Advertise a reconnect delay, then
			// return so the client's EventSource reconnects on its own.
			_, _ = fmt.Fprintf(w, "retry: %d\n\n", reconnectHint.Milliseconds())
			flusher.Flush()
			return

		case <-ctx.Done():
			// Client went away; unsubscribe (via defer) and stop.
			return
		}
	}
}

// parseCursor reads the resume position as a uint64 Seq from the Last-Event-ID
// request header, falling back to the last_event_id query parameter. It returns
// 0 (stream from the start of retained history) when neither is present or the
// value does not parse.
func parseCursor(r *http.Request) uint64 {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("last_event_id")
	}
	if raw == "" {
		return 0
	}
	seq, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return seq
}

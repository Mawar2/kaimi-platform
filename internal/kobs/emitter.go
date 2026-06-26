package kobs

import (
	"context"
	"sync/atomic"

	"github.com/Mawar2/kaimi-telemetry/emit"
	"github.com/Mawar2/kaimi-telemetry/event"
)

// handle is the process-wide emitter installed by Init. It is stored through an
// atomic pointer so that Init (typically called once at startup) and the many
// concurrent Emit callers on the hot path never race. The zero value (nil) means
// telemetry has not been wired, in which case Emit is a no-op.
var handle atomic.Pointer[emit.Emitter]

// Init installs em as the process-wide telemetry emitter. It is intended to be
// called once from an entrypoint after the emitter is constructed. Passing a nil
// emitter clears the handle, returning Emit to its no-op state. Init is safe for
// concurrent use, though callers normally invoke it a single time during setup.
func Init(em *emit.Emitter) {
	handle.Store(em)
}

// Emit hands ev to the installed emitter. When no emitter has been installed
// (the common case in unit tests and in binaries that have not wired
// telemetry), Emit returns immediately without doing anything, so instrumenting
// a code path is always additive and never panics.
//
// Emit inherits the core emitter's non-blocking guarantee: it never blocks the
// caller and silently drops the event if the emitter's buffer is full. The
// drop signal is intentionally discarded here because telemetry must never alter
// the host's behavior or control flow.
func Emit(ev event.Event) {
	em := handle.Load()
	if em == nil {
		return
	}
	// The core Emit takes a context for symmetry with slow sinks; the buffered
	// hand-off is non-blocking, so a background context is correct and the
	// accepted/dropped bool is deliberately ignored.
	_ = em.Emit(context.Background(), ev)
}

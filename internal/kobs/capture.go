package kobs

import (
	"context"
	"sync"

	"github.com/Mawar2/kaimi-telemetry/emit"
	"github.com/Mawar2/kaimi-telemetry/event"
)

// Capture is an in-memory EventSink that records every event delivered through
// the process-wide emitter. It exists so any Kaimi package can assert its own
// instrumentation in tests without importing the telemetry core directly — only
// kobs may do that. Install one with NewCapture, run the code under test, then
// read the recorded events with Drain.
type Capture struct {
	mu     sync.Mutex
	events []event.Event
	em     *emit.Emitter
}

// NewCapture installs a fresh Capture as the process-wide emitter and returns it
// together with a restore function. The restore function reinstates the
// previously installed emitter and shuts the capture emitter down; callers
// should defer it so a failed assertion never leaks capture state into another
// test. Read recorded events with Drain (which flushes first).
func NewCapture() (capture *Capture, restore func()) {
	capture = &Capture{}
	// A single worker keeps delivery order deterministic for assertions.
	capture.em = emit.New(capture, emit.Config{Workers: 1})
	prev := handle.Swap(capture.em)
	restore = func() {
		handle.Store(prev)
		// Shutdown drains and closes the capture emitter; the error is ignored
		// because this is teardown of a test-only emitter.
		_ = capture.em.Shutdown(context.Background())
	}
	return capture, restore
}

// Emit records ev. It satisfies sink.EventSink.
func (c *Capture) Emit(_ context.Context, ev event.Event) error { //nolint:gocritic // Event passed by value to satisfy sink.EventSink.Emit
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
	return nil
}

// Flush is a no-op: the Capture holds events in memory and never buffers. It
// satisfies sink.EventSink.
func (c *Capture) Flush(context.Context) error { return nil }

// Close is a no-op. It satisfies sink.EventSink.
func (c *Capture) Close() error { return nil }

// Drain flushes the emitter so every event handed to Emit has reached the
// Capture, then returns a copy of the recorded events in delivery order.
func (c *Capture) Drain() []event.Event {
	// Block until the async emitter has delivered everything buffered so far.
	_ = c.em.Flush(context.Background())
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]event.Event, len(c.events))
	copy(out, c.events)
	return out
}

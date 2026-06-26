package emit

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Mawar2/kaimi-telemetry/event"
)

// ev builds a distinct test event each call.
func ev(name string) event.Event {
	return event.NewEvent(event.CategorySystem, name)
}

// recordingSink records every event handed to it and counts Flush/Close calls.
// It is safe for concurrent use.
type recordingSink struct {
	mu      sync.Mutex
	events  []event.Event
	flushes int
	closes  int
}

//nolint:gocritic // hugeParam: implements the EventSink.Emit value-event contract.
func (r *recordingSink) Emit(_ context.Context, e event.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	return nil
}

func (r *recordingSink) Flush(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushes++
	return nil
}

func (r *recordingSink) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closes++
	return nil
}

func (r *recordingSink) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func (r *recordingSink) flushCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.flushes
}

func (r *recordingSink) closeCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closes
}

// blockingSink blocks every Emit until release is closed, so the emitter's
// single worker is stuck and the buffer can fill up deterministically.
type blockingSink struct {
	release chan struct{}
}

//nolint:gocritic // hugeParam: implements the EventSink.Emit value-event contract.
func (b *blockingSink) Emit(ctx context.Context, _ event.Event) error {
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *blockingSink) Flush(_ context.Context) error { return nil }
func (b *blockingSink) Close() error                  { return nil }

// TestEmitNeverBlocksWhenSinkBlocks proves Emit returns promptly even when the
// destination sink stalls every delivery, exercising the drop path.
func TestEmitNeverBlocksWhenSinkBlocks(t *testing.T) {
	dst := &blockingSink{release: make(chan struct{})}
	defer close(dst.release)

	em := New(dst, Config{BufferSize: 1, Workers: 1})
	defer func() { _ = em.Shutdown(context.Background()) }()

	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		// Far more events than the buffer can hold while the worker is stuck.
		for i := 0; i < 1000; i++ {
			em.Emit(ctx, ev("flood"))
		}
		close(done)
	}()

	select {
	case <-done:
		// Emit kept flowing despite the stuck worker.
	case <-time.After(5 * time.Second):
		t.Fatal("Emit blocked when the destination sink blocked")
	}
}

// TestDroppedIncrementsWhenBufferFull proves the dropped counter advances once
// the bounded buffer is saturated and the worker cannot drain it.
func TestDroppedIncrementsWhenBufferFull(t *testing.T) {
	dst := &blockingSink{release: make(chan struct{})}
	defer close(dst.release)

	em := New(dst, Config{BufferSize: 2, Workers: 1})
	defer func() { _ = em.Shutdown(context.Background()) }()

	ctx := context.Background()
	const n = 500
	accepted := 0
	for i := 0; i < n; i++ {
		if em.Emit(ctx, ev("x")) {
			accepted++
		}
	}

	if em.Dropped() == 0 {
		t.Fatal("Dropped() = 0, want > 0 once the buffer is full")
	}
	if accepted == n {
		t.Fatal("expected some events to be dropped, but all were accepted")
	}
	if uint64(n-accepted) != em.Dropped() {
		t.Errorf("Dropped() = %d, want %d (n - accepted)", em.Dropped(), n-accepted)
	}
}

// TestFlushDrainsEverythingToSink proves Flush moves all buffered events to the
// destination and flushes it.
func TestFlushDrainsEverythingToSink(t *testing.T) {
	dst := &recordingSink{}
	em := New(dst, Config{BufferSize: 1024, Workers: 2})
	defer func() { _ = em.Shutdown(context.Background()) }()

	ctx := context.Background()
	const n = 200
	for i := 0; i < n; i++ {
		if !em.Emit(ctx, ev("e")) {
			t.Fatalf("Emit %d was dropped with a large buffer", i)
		}
	}

	if err := em.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if got := dst.count(); got != n {
		t.Errorf("sink received %d events, want %d", got, n)
	}
	if dst.flushCount() == 0 {
		t.Error("Flush did not flush the destination sink")
	}
}

// TestShutdownIsIdempotentAndFlushes proves Shutdown drains, flushes, and
// closes exactly once even when called repeatedly.
func TestShutdownIsIdempotentAndFlushes(t *testing.T) {
	dst := &recordingSink{}
	em := New(dst, Config{BufferSize: 1024, Workers: 1})

	ctx := context.Background()
	const n = 50
	for i := 0; i < n; i++ {
		em.Emit(ctx, ev("e"))
	}

	if err := em.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	// Repeated shutdowns must be safe no-ops.
	if err := em.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
	if err := em.Shutdown(ctx); err != nil {
		t.Fatalf("third Shutdown: %v", err)
	}

	if got := dst.count(); got != n {
		t.Errorf("sink received %d events, want %d (Shutdown should drain)", got, n)
	}
	if dst.closeCount() != 1 {
		t.Errorf("sink Close called %d times, want exactly 1", dst.closeCount())
	}
	if dst.flushCount() == 0 {
		t.Error("Shutdown did not flush the destination sink")
	}

	// Emit after Shutdown must not block and must report a drop.
	if em.Emit(ctx, ev("late")) {
		t.Error("Emit after Shutdown returned true, want false (not accepting)")
	}
}

// TestDefaultsApplied proves zero-value Config fields receive defaults and the
// emitter functions without an explicit OnError.
func TestDefaultsApplied(t *testing.T) {
	dst := &recordingSink{}
	em := New(dst, Config{}) // all zero values
	defer func() { _ = em.Shutdown(context.Background()) }()

	ctx := context.Background()
	if !em.Emit(ctx, ev("e")) {
		t.Error("Emit dropped the first event under default config")
	}
	if err := em.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if dst.count() != 1 {
		t.Errorf("sink received %d events, want 1", dst.count())
	}
}

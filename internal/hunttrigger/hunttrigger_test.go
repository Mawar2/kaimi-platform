package hunttrigger

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRunner records invocations and can be made to block (release) so a test can hold a
// hunt "in flight", and signals done after each Run so a test can wait deterministically.
type fakeRunner struct {
	calls   int32
	started chan struct{} // Run sends here at start (nil = skip)
	release chan struct{} // Run blocks until this is closed/received (nil = don't block)
	done    chan struct{} // Run sends here after finishing (buffered; nil = skip)
}

func (f *fakeRunner) Run(_ context.Context) error {
	atomic.AddInt32(&f.calls, 1)
	if f.started != nil {
		f.started <- struct{}{}
	}
	if f.release != nil {
		<-f.release
	}
	if f.done != nil {
		f.done <- struct{}{}
	}
	return nil
}

type panicRunner struct{}

func (panicRunner) Run(_ context.Context) error { panic("boom") }

// fireEventually polls Fire until it launches a hunt or the timeout elapses, tolerating the
// brief window where the previous hunt's goroutine is still clearing the running flag.
func fireEventually(t *testing.T, tr *Trigger) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if tr.Fire() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return false
}

// TestTrigger_FiresAndRuns: a Fire launches the runner exactly once.
func TestTrigger_FiresAndRuns(t *testing.T) {
	fr := &fakeRunner{done: make(chan struct{}, 1)}
	tr := New(fr, 0)

	if !tr.Fire() {
		t.Fatal("Fire() = false, want true (should launch a hunt)")
	}
	select {
	case <-fr.done:
	case <-time.After(2 * time.Second):
		t.Fatal("runner was never invoked")
	}
	if got := atomic.LoadInt32(&fr.calls); got != 1 {
		t.Errorf("runner calls = %d, want 1", got)
	}
}

// TestTrigger_SuppressesConcurrent: a Fire while a hunt is in flight is a no-op, so two
// rapid key-saves can't run overlapping hunts (which would double the SAM quota spend).
func TestTrigger_SuppressesConcurrent(t *testing.T) {
	fr := &fakeRunner{started: make(chan struct{}), release: make(chan struct{}), done: make(chan struct{}, 1)}
	tr := New(fr, 0)

	if !tr.Fire() {
		t.Fatal("first Fire() = false, want true")
	}
	<-fr.started // the hunt is now in flight

	if tr.Fire() {
		t.Error("second Fire() while running = true, want false (must suppress concurrent hunts)")
	}

	close(fr.release)
	<-fr.done
	if got := atomic.LoadInt32(&fr.calls); got != 1 {
		t.Errorf("runner calls = %d, want 1 (the suppressed Fire must not have run)", got)
	}
}

// TestTrigger_DebouncesWithinInterval: after a hunt, a Fire within minInterval is suppressed;
// a Fire after the interval runs again. Uses an injected clock for determinism.
func TestTrigger_DebouncesWithinInterval(t *testing.T) {
	fr := &fakeRunner{done: make(chan struct{}, 8)}
	tr := New(fr, time.Hour)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cur := base
	tr.now = func() time.Time { return cur }

	if !tr.Fire() {
		t.Fatal("first Fire() = false, want true")
	}
	<-fr.done

	cur = base.Add(30 * time.Minute)
	if tr.Fire() {
		t.Error("Fire() 30m after last hunt = true, want false (within 1h minInterval)")
	}

	cur = base.Add(61 * time.Minute)
	if !fireEventually(t, tr) {
		t.Fatal("Fire() 61m after last hunt never launched, want a new hunt past minInterval")
	}
	<-fr.done
	if got := atomic.LoadInt32(&fr.calls); got != 2 {
		t.Errorf("runner calls = %d, want 2 (one initial, one after the interval)", got)
	}
}

// TestTrigger_RecoversFromPanic: a panicking Runner must not permanently wedge the trigger.
func TestTrigger_RecoversFromPanic(t *testing.T) {
	tr := New(panicRunner{}, 0)
	tr.Fire() // panics inside, must be recovered and clear the running flag

	if !fireEventually(t, tr) {
		t.Error("trigger stayed wedged after a Runner panic; running flag was not cleared")
	}
}

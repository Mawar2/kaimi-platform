package sink

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Mawar2/kaimi-telemetry/event"
)

// ev is a small helper that builds a distinct test event each call.
func ev(name string) event.Event {
	return event.NewEvent(event.CategorySystem, name)
}

func TestJSONLSinkWritesOneLinePerEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	s, err := NewJSONLSink(path)
	if err != nil {
		t.Fatalf("NewJSONLSink: %v", err)
	}

	ctx := context.Background()
	want := []event.Event{ev("a"), ev("b"), ev("c")}
	for _, e := range want {
		if err := s.Emit(ctx, e); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()

	var got []event.Event
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e event.Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("Unmarshal line %q: %v", sc.Text(), err)
		}
		got = append(got, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("read %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].EventID != want[i].EventID || got[i].Name != want[i].Name {
			t.Errorf("line %d = %s/%s, want %s/%s", i, got[i].EventID, got[i].Name, want[i].EventID, want[i].Name)
		}
	}
}

func TestJSONLSinkFlushPersistsBufferedData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	s, err := NewJSONLSink(path)
	if err != nil {
		t.Fatalf("NewJSONLSink: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	if err := s.Emit(ctx, ev("buffered")); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	// The write is small, so it sits in the bufio buffer and the on-disk file
	// is still empty until Flush runs.
	if info, err := os.Stat(path); err != nil {
		t.Fatalf("Stat before flush: %v", err)
	} else if info.Size() != 0 {
		t.Fatalf("file size before flush = %d, want 0 (data should be buffered)", info.Size())
	}

	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if info, err := os.Stat(path); err != nil {
		t.Fatalf("Stat after flush: %v", err)
	} else if info.Size() == 0 {
		t.Fatal("file size after flush = 0, want > 0 (Flush should persist)")
	}
}

func TestJSONLSinkEmitAfterCloseIsErrClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	s, err := NewJSONLSink(path)
	if err != nil {
		t.Fatalf("NewJSONLSink: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	ctx := context.Background()
	if err := s.Emit(ctx, ev("late")); !errors.Is(err, ErrClosed) {
		t.Errorf("Emit after Close = %v, want ErrClosed", err)
	}
	if err := s.Flush(ctx); !errors.Is(err, ErrClosed) {
		t.Errorf("Flush after Close = %v, want ErrClosed", err)
	}
}

// recordingSink records calls and optionally returns a fixed error.
type recordingSink struct {
	mu      sync.Mutex
	emits   int
	flushes int
	closes  int
	err     error
}

//nolint:gocritic // hugeParam: implements the EventSink.Emit value-event contract.
func (r *recordingSink) Emit(_ context.Context, _ event.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.emits++
	return r.err
}

func (r *recordingSink) Flush(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushes++
	return r.err
}

func (r *recordingSink) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closes++
	return r.err
}

func TestMultiFansOutAndJoinsErrors(t *testing.T) {
	errA := errors.New("sink A failed")
	a := &recordingSink{err: errA}
	b := &recordingSink{}
	m := Multi{a, b}

	ctx := context.Background()
	err := m.Emit(ctx, ev("x"))
	if !errors.Is(err, errA) {
		t.Errorf("Emit error = %v, want it to wrap %v", err, errA)
	}
	// Both sinks must be reached even though the first one errored.
	if a.emits != 1 || b.emits != 1 {
		t.Errorf("emits = a:%d b:%d, want a:1 b:1", a.emits, b.emits)
	}

	if err := m.Flush(ctx); !errors.Is(err, errA) {
		t.Errorf("Flush error = %v, want it to wrap %v", err, errA)
	}
	if err := m.Close(); !errors.Is(err, errA) {
		t.Errorf("Close error = %v, want it to wrap %v", err, errA)
	}
	if a.flushes != 1 || b.flushes != 1 || a.closes != 1 || b.closes != 1 {
		t.Errorf("fan-out incomplete: a=%+v b=%+v", a, b)
	}
}

func TestMultiAllSucceedReturnsNil(t *testing.T) {
	a := &recordingSink{}
	b := &recordingSink{}
	m := Multi{a, b}
	if err := m.Emit(context.Background(), ev("ok")); err != nil {
		t.Errorf("Emit = %v, want nil", err)
	}
}

func TestRingSinceReturnsOnlyNewerAndEvictsAtCap(t *testing.T) {
	r := newRing(3)

	var stamped []Stamped
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		stamped = append(stamped, r.Append(ev(name)))
	}

	// Seq must be monotonic starting at 1.
	for i, s := range stamped {
		if s.Seq != uint64(i+1) {
			t.Errorf("stamped[%d].Seq = %d, want %d", i, s.Seq, i+1)
		}
	}

	// Capacity 3 with 5 appends: a and b (seq 1,2) are evicted; c,d,e remain.
	since0 := r.Since(0)
	if len(since0) != 3 {
		t.Fatalf("Since(0) returned %d, want 3 (older evicted)", len(since0))
	}
	wantSeqs := []uint64{3, 4, 5}
	for i, s := range since0 {
		if s.Seq != wantSeqs[i] {
			t.Errorf("Since(0)[%d].Seq = %d, want %d", i, s.Seq, wantSeqs[i])
		}
	}

	// Since is exclusive on its argument.
	since4 := r.Since(4)
	if len(since4) != 1 || since4[0].Seq != 5 {
		t.Errorf("Since(4) = %+v, want one entry with Seq 5", since4)
	}

	if got := r.Since(5); len(got) != 0 {
		t.Errorf("Since(5) = %+v, want empty", got)
	}
	if got := r.Since(100); len(got) != 0 {
		t.Errorf("Since(100) = %+v, want empty", got)
	}
}

func TestLiveSinkReplayReturnsBufferedHistory(t *testing.T) {
	l := NewLiveSink(10, 4)
	ctx := context.Background()
	for _, name := range []string{"a", "b", "c"} {
		if err := l.Emit(ctx, ev(name)); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}

	all := l.Replay(0)
	if len(all) != 3 {
		t.Fatalf("Replay(0) = %d entries, want 3", len(all))
	}
	tail := l.Replay(2)
	if len(tail) != 1 || tail[0].Seq != 3 {
		t.Errorf("Replay(2) = %+v, want one entry with Seq 3", tail)
	}
}

func TestLiveSinkSubscriberReceivesLiveEvents(t *testing.T) {
	l := NewLiveSink(10, 8)
	ch, cancel := l.Subscribe(0)
	defer cancel()

	ctx := context.Background()
	if err := l.Emit(ctx, ev("live")); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	select {
	case s := <-ch:
		if s.Event.Name != "live" {
			t.Errorf("received Name = %q, want %q", s.Event.Name, "live")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscriber to receive event")
	}
}

func TestLiveSinkSubscribeSeedsHistory(t *testing.T) {
	l := NewLiveSink(10, 8)
	ctx := context.Background()
	for _, name := range []string{"a", "b", "c"} {
		if err := l.Emit(ctx, ev(name)); err != nil {
			t.Fatalf("Emit: %v", err)
		}
	}

	// Subscribing after seq 1 should pre-load the missed events b and c.
	ch, cancel := l.Subscribe(1)
	defer cancel()

	for _, want := range []uint64{2, 3} {
		select {
		case s := <-ch:
			if s.Seq != want {
				t.Errorf("seeded Seq = %d, want %d", s.Seq, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for seeded Seq %d", want)
		}
	}
}

func TestLiveSinkSlowSubscriberDroppedNeverBlocks(t *testing.T) {
	// Tiny subscriber buffer so a subscriber that never reads fills immediately.
	l := NewLiveSink(1000, 2)
	_, cancel := l.Subscribe(0) // intentionally never drained
	defer cancel()

	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			if err := l.Emit(ctx, ev("flood")); err != nil {
				t.Errorf("Emit: %v", err)
				return
			}
		}
		close(done)
	}()

	select {
	case <-done:
		// Emit kept flowing despite the stuck subscriber.
	case <-time.After(5 * time.Second):
		t.Fatal("Emit blocked on a slow subscriber")
	}
}

func TestLiveSinkEmitAfterCloseIsErrClosed(t *testing.T) {
	l := NewLiveSink(10, 8)
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	ctx := context.Background()
	if err := l.Emit(ctx, ev("late")); !errors.Is(err, ErrClosed) {
		t.Errorf("Emit after Close = %v, want ErrClosed", err)
	}
	if err := l.Flush(ctx); !errors.Is(err, ErrClosed) {
		t.Errorf("Flush after Close = %v, want ErrClosed", err)
	}
}

func TestLiveSinkCloseClosesSubscriberChannels(t *testing.T) {
	l := NewLiveSink(10, 8)
	ch, _ := l.Subscribe(0)
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected subscriber channel to be closed and drained")
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber channel was not closed on Close")
	}
}

func TestLiveSinkConcurrentEmitAndSubscribe(t *testing.T) {
	l := NewLiveSink(256, 16)
	ctx := context.Background()

	var wg sync.WaitGroup

	// Concurrent emitters.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = l.Emit(ctx, ev("e"))
			}
		}()
	}

	// Concurrent subscribers that come and go while draining.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				ch, cancel := l.Subscribe(0)
				go func() {
					for range ch {
					}
				}()
				cancel()
			}
		}()
	}

	wg.Wait()
}

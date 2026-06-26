package httpstream

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mawar2/kaimi-telemetry/event"
	"github.com/Mawar2/kaimi-telemetry/sink"
)

// *sink.LiveSink must satisfy Source so the handler can stream straight from
// the live sink with no adapter.
var _ Source = (*sink.LiveSink)(nil)

// emit appends a distinct system event to s and fails the test on error.
func emit(t *testing.T, s *sink.LiveSink, name string) {
	t.Helper()
	if err := s.Emit(context.Background(), event.NewEvent(event.CategorySystem, name)); err != nil {
		t.Fatalf("Emit %q: %v", name, err)
	}
}

// sseFrame is a parsed Server-Sent Events frame (the fields up to a blank line).
type sseFrame struct {
	id      string
	event   string
	data    []string
	comment string
	retry   string
}

// lineReader reads newline-delimited lines from an SSE body on a background
// goroutine so the test can wait for each line with a timeout instead of
// blocking forever on a stalled stream.
type lineReader struct {
	lines chan string
}

func newLineReader(r io.Reader) *lineReader {
	lr := &lineReader{lines: make(chan string, 256)}
	go func() {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			lr.lines <- sc.Text()
		}
		close(lr.lines)
	}()
	return lr
}

// nextLine returns the next line, ok=false if the stream closed, and fails the
// test if nothing arrives within timeout.
func (lr *lineReader) nextLine(t *testing.T, timeout time.Duration) (string, bool) {
	t.Helper()
	select {
	case l, ok := <-lr.lines:
		return l, ok
	case <-time.After(timeout):
		t.Fatalf("timed out after %s waiting for an SSE line", timeout)
		return "", false
	}
}

// nextFrame reads lines until the blank-line frame terminator and returns the
// parsed frame. ok=false means the stream closed before a full frame.
func (lr *lineReader) nextFrame(t *testing.T, timeout time.Duration) (sseFrame, bool) {
	t.Helper()
	var f sseFrame
	saw := false
	for {
		line, ok := lr.nextLine(t, timeout)
		if !ok {
			return f, saw
		}
		if line == "" {
			if !saw {
				// Leading blank line before any content; keep reading.
				continue
			}
			return f, true
		}
		saw = true
		switch {
		case strings.HasPrefix(line, "id: "):
			f.id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			f.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			f.data = append(f.data, strings.TrimPrefix(line, "data: "))
		case strings.HasPrefix(line, "retry: "):
			f.retry = strings.TrimPrefix(line, "retry: ")
		case strings.HasPrefix(line, ": "):
			f.comment = strings.TrimPrefix(line, ": ")
		}
	}
}

// newServer starts an httptest.Server for h and registers cleanup that force-
// closes any still-open streaming connections before Close. Without forcing the
// connections shut, Close would block forever waiting on a long-lived SSE
// handler that only returns when its connection drops.
func newServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(h)
	t.Cleanup(func() {
		s.CloseClientConnections()
		s.Close()
	})
	return s
}

// connect opens a streaming GET against s with the given Last-Event-ID header
// (empty header is omitted) and returns a lineReader over the response body.
func connect(t *testing.T, s *httptest.Server, lastEventID string) *lineReader {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, s.URL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	resp, err := s.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	return newLineReader(resp.Body)
}

func TestServeHTTPFramesCarryMonotonicID(t *testing.T) {
	live := sink.NewLiveSink(100, 100)
	emit(t, live, "a")
	emit(t, live, "b")
	emit(t, live, "c")

	srv := newServer(t, NewHandler(live, WithHeartbeat(time.Hour)))

	lr := connect(t, srv, "")

	var ids []uint64
	for i := 0; i < 3; i++ {
		f, ok := lr.nextFrame(t, 2*time.Second)
		if !ok {
			t.Fatalf("stream closed before frame %d", i)
		}
		n, err := strconv.ParseUint(f.id, 10, 64)
		if err != nil {
			t.Fatalf("frame %d id %q not a uint: %v", i, f.id, err)
		}
		ids = append(ids, n)
		if f.event != string(event.CategorySystem) {
			t.Fatalf("frame %d event = %q, want %q", i, f.event, event.CategorySystem)
		}
		if len(f.data) != 1 || !strings.Contains(f.data[0], "event_id") {
			t.Fatalf("frame %d data = %v, want one JSON line with event_id", i, f.data)
		}
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Fatalf("ids not monotonic: %v", ids)
		}
	}

	// A live event after the replay tail must also stream through.
	emit(t, live, "d")
	f, ok := lr.nextFrame(t, 2*time.Second)
	if !ok {
		t.Fatal("stream closed before live frame")
	}
	if f.id != "4" {
		t.Fatalf("live frame id = %q, want 4", f.id)
	}
}

func TestServeHTTPResumeReplaysThenTails(t *testing.T) {
	live := sink.NewLiveSink(100, 100)
	emit(t, live, "a") // seq 1
	emit(t, live, "b") // seq 2
	emit(t, live, "c") // seq 3

	srv := newServer(t, NewHandler(live, WithHeartbeat(time.Hour)))

	// Reconnect having last seen seq 1: replay must start at seq 2 and skip 1.
	lr := connect(t, srv, "1")

	f, ok := lr.nextFrame(t, 2*time.Second)
	if !ok || f.id != "2" {
		t.Fatalf("first replayed frame id = %q ok=%v, want 2", f.id, ok)
	}
	f, ok = lr.nextFrame(t, 2*time.Second)
	if !ok || f.id != "3" {
		t.Fatalf("second replayed frame id = %q ok=%v, want 3", f.id, ok)
	}

	// Then live events tail without duplicating the replayed ones.
	emit(t, live, "d") // seq 4
	f, ok = lr.nextFrame(t, 2*time.Second)
	if !ok || f.id != "4" {
		t.Fatalf("tailed frame id = %q ok=%v, want 4", f.id, ok)
	}
}

func TestServeHTTPEmitsHeartbeat(t *testing.T) {
	live := sink.NewLiveSink(100, 100) // empty: nothing to replay

	srv := newServer(t, NewHandler(live, WithHeartbeat(40*time.Millisecond)))

	lr := connect(t, srv, "")

	f, ok := lr.nextFrame(t, 2*time.Second)
	if !ok {
		t.Fatal("stream closed before heartbeat")
	}
	if f.comment != "keep-alive" {
		t.Fatalf("first frame comment = %q, want keep-alive", f.comment)
	}
}

func TestServeHTTPMaxStreamClosesGracefully(t *testing.T) {
	live := sink.NewLiveSink(100, 100)

	srv := newServer(t, NewHandler(live,
		WithHeartbeat(time.Hour),
		WithMaxStream(80*time.Millisecond),
	))

	lr := connect(t, srv, "")

	// The maxStream timer must emit a retry hint then close the stream.
	sawRetry := false
	for {
		f, ok := lr.nextFrame(t, 2*time.Second)
		if !ok {
			break // stream closed: graceful shutdown
		}
		if f.retry != "" {
			sawRetry = true
		}
	}
	if !sawRetry {
		t.Fatal("expected a retry hint before the stream closed")
	}
}

func TestServeHTTPMissingFlusherReturns500(t *testing.T) {
	live := sink.NewLiveSink(100, 100)
	h := NewHandler(live)

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w := &nonFlusher{header: http.Header{}}
	h.ServeHTTP(w, req)

	if w.status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.status, http.StatusInternalServerError)
	}
}

// nonFlusher is an http.ResponseWriter that deliberately does NOT implement
// http.Flusher, so the handler must reject it with 500.
type nonFlusher struct {
	header http.Header
	status int
}

func (n *nonFlusher) Header() http.Header         { return n.header }
func (n *nonFlusher) Write(b []byte) (int, error) { return len(b), nil }
func (n *nonFlusher) WriteHeader(code int)        { n.status = code }

func TestServeHTTPClientCancelUnsubscribes(t *testing.T) {
	ms := &mockSource{ch: make(chan sink.Stamped)}
	srv := newServer(t, NewHandler(ms, WithHeartbeat(time.Hour)))

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	// Wait until the handler has subscribed, then cut the client connection.
	deadline := time.Now().Add(2 * time.Second)
	for ms.subscribeCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("handler never subscribed")
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	_ = resp.Body.Close()

	// The handler must call the unsubscribe cancel exactly once on disconnect.
	deadline = time.Now().Add(2 * time.Second)
	for ms.cancelCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("handler never unsubscribed after client cancel")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if got := ms.cancelCount(); got != 1 {
		t.Fatalf("cancel called %d times, want 1", got)
	}
}

// mockSource is a Source whose Subscribe hands back a never-sending channel and
// records how many times it was subscribed to and unsubscribed from.
type mockSource struct {
	mu         sync.Mutex
	subscribed int
	canceled   int
	ch         chan sink.Stamped
}

func (m *mockSource) Replay(_ uint64) []sink.Stamped { return nil }

func (m *mockSource) Subscribe(_ uint64) (events <-chan sink.Stamped, cancel func()) {
	m.mu.Lock()
	m.subscribed++
	m.mu.Unlock()
	return m.ch, func() {
		m.mu.Lock()
		m.canceled++
		m.mu.Unlock()
	}
}

func (m *mockSource) subscribeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.subscribed
}

func (m *mockSource) cancelCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.canceled
}

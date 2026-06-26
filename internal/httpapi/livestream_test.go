package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/kaimi-telemetry/event"
	"github.com/Mawar2/kaimi-telemetry/sink"
)

// TestLiveStreamEmitsEventFrame verifies the T2.1 route: with telemetry enabled
// (a LiveSource injected) and no gate configured (insecure dev), GET
// /v1/events/stream replays a buffered event as a text/event-stream SSE frame.
//
// The event is emitted BEFORE the request so the handler's synchronous replay
// writes (and flushes) it before entering the blocking tail loop. The request
// context is pre-cancelled so that loop returns immediately afterward, making the
// assertion deterministic without sleeps or background goroutines.
func TestLiveStreamEmitsEventFrame(t *testing.T) {
	ls := sink.NewLiveSink(16, 16)
	if err := ls.Emit(context.Background(), event.NewEvent(event.CategorySystem, "boot")); err != nil {
		t.Fatalf("seed Emit: %v", err)
	}

	// Insecure dev mode (no gate) so Routes() builds open and the stream is reachable
	// without a session.
	srv := New(Deps{LiveSource: ls, AllowInsecureNoAuth: true})
	h := srv.Routes()

	// Pre-cancel: replay runs and flushes first, then the tail loop observes the done
	// context and returns, so ServeHTTP completes synchronously after the frame.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/v1/events/stream", http.NoBody).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: system") {
		t.Errorf("body missing SSE event line for the emitted event:\n%s", body)
	}
	if !strings.Contains(body, "data: ") {
		t.Errorf("body missing SSE data line:\n%s", body)
	}
	if !strings.Contains(body, `"name":"boot"`) {
		t.Errorf("body missing emitted event payload:\n%s", body)
	}
}

// TestLiveStreamUnauthorizedRejected verifies the stream is gated like the
// dashboard: with a product-key gate configured and no session on the request, the
// route does NOT stream — it redirects (302) to the entry page, just as
// RequireProductKeyHTML does for the SSR dashboard.
func TestLiveStreamUnauthorizedRejected(t *testing.T) {
	gate, _ := newTestGate(t, time.Now().UTC())
	ls := sink.NewLiveSink(16, 16)

	srv := New(Deps{LiveSource: ls, ProductKey: gate})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/v1/events/stream", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("unauthenticated stream status = %d, want %d (redirect to entry)", rec.Code, http.StatusFound)
	}
	if ct := rec.Header().Get("Content-Type"); ct == "text/event-stream" {
		t.Errorf("unauthenticated request was streamed; Content-Type = %q", ct)
	}
}

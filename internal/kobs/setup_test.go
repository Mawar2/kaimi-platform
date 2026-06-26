package kobs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/kaimi-telemetry/event"
	"github.com/Mawar2/kaimi-telemetry/sink"
)

// TestLocalGateHasNoEgress asserts the privacy wiring directly: the gate Setup
// builds writes the full event to a Local sink and has a nil Central, so there
// is no path for content to leave the deployment.
func TestLocalGateHasNoEgress(t *testing.T) {
	live := sink.NewLiveSink(8, 8)
	gate := localGate(live, live)
	if gate.Central != nil {
		t.Fatal("localGate Central is non-nil; content could egress")
	}
	if gate.Local == nil {
		t.Fatal("localGate Local is nil; events would be dropped")
	}
}

// TestSetupInstallsEmitterAndPersists proves Setup makes telemetry live end to
// end: it installs the process-wide kobs handle, creates the event log under the
// telemetry dir, an emitted event reaches the in-memory LiveSink, and Shutdown
// flushes it durably to the JSONL log — including its content-class attribute,
// which stays in the LOCAL log only (there is no central sink to forward to).
func TestSetupInstallsEmitterAndPersists(t *testing.T) {
	// Setup installs a process-wide emitter; clear it afterward so this test does
	// not leak the handle into others.
	defer Init(nil)

	dir := filepath.Join(t.TempDir(), "telemetry")
	live, em, err := Setup(dir, 0)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if live == nil || em == nil {
		t.Fatalf("Setup returned nil handles: live=%v em=%v", live, em)
	}

	logPath := filepath.Join(dir, eventLogName)
	if _, statErr := os.Stat(logPath); statErr != nil {
		t.Fatalf("expected event log at %s: %v", logPath, statErr)
	}

	const secret = "super-secret-prompt-body"
	ev := event.NewEvent(event.CategoryLLM, "llm.request.completed",
		event.Usage("total_tokens", 42),
		event.Content("prompt", secret),
	)
	Emit(ev)

	// The async emitter hands off on a worker, so poll briefly for the event to
	// reach the LiveSink rather than assuming synchronous delivery.
	if !waitForReplay(live.Replay, ev.EventID, time.Second) {
		t.Fatal("emitted event did not reach the LiveSink")
	}

	// Shutdown flushes the JSONL sink to disk, so the event is durably persisted.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := em.Shutdown(ctx); err != nil {
		t.Fatalf("emitter shutdown: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read event log: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, ev.EventID) {
		t.Fatalf("event log missing event %s; got %q", ev.EventID, got)
	}
	// The content-class attribute is retained in the LOCAL log (the gate keeps
	// content in by having no Central sink).
	if !strings.Contains(got, secret) {
		t.Fatalf("event log missing local content attribute %q; got %q", secret, got)
	}
}

// waitForReplay polls a LiveSink Replay function until it returns an event with
// the given id or the timeout elapses.
func waitForReplay(replay func(uint64) []sink.Stamped, eventID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		stamped := replay(0)
		for i := range stamped {
			if stamped[i].Event.EventID == eventID {
				return true
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/kaimi-telemetry/event"
	"github.com/Mawar2/kaimi-telemetry/sink"

	"github.com/Mawar2/Kaimi/internal/kobs"
)

func TestTelemetryEnabled(t *testing.T) {
	// Telemetry is ENABLED by default and only an explicit false-y value turns
	// it off. A malformed value stays on the safe (enabled) side, since the
	// pipeline is additive and never alters host behavior.
	cases := []struct {
		env  string
		want bool
	}{
		{"", true},
		{"true", true},
		{"1", true},
		{"false", false},
		{"0", false},
		{"FALSE", false},
		{"garbage", true},
	}
	for _, tc := range cases {
		if got := telemetryEnabled(tc.env); got != tc.want {
			t.Errorf("telemetryEnabled(%q) = %v, want %v", tc.env, got, tc.want)
		}
	}
}

func TestSetupTelemetry(t *testing.T) {
	// kobs installs a process-wide emitter; clear it afterward so this test does
	// not leak the handle into others.
	defer kobs.Init(nil)

	storePath := t.TempDir()
	live, em, err := setupTelemetry(storePath)
	if err != nil {
		t.Fatalf("setupTelemetry: %v", err)
	}
	if live == nil {
		t.Fatal("setupTelemetry returned a nil LiveSink")
	}
	if em == nil {
		t.Fatal("setupTelemetry returned a nil Emitter")
	}

	// The telemetry directory and event log are created under the store path.
	logPath := filepath.Join(storePath, "telemetry", "events.jsonl")
	if _, statErr := os.Stat(logPath); statErr != nil {
		t.Fatalf("expected event log at %s: %v", logPath, statErr)
	}

	// Emitting through the installed kobs handle reaches the LiveSink ring...
	ev := event.NewEvent(event.CategorySystem, "test.boot", event.Usage("k", "v"))
	kobs.Emit(ev)

	// ...which the LiveSink replays. The async emitter hands off on a worker, so
	// poll briefly for the event to land rather than assuming synchronous delivery.
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
	if !strings.Contains(string(data), ev.EventID) {
		t.Fatalf("event log does not contain emitted event %s; got %q", ev.EventID, string(data))
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

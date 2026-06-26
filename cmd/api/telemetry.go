package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Mawar2/kaimi-telemetry/emit"
	"github.com/Mawar2/kaimi-telemetry/redact"
	"github.com/Mawar2/kaimi-telemetry/sink"

	"github.com/Mawar2/Kaimi/internal/kobs"
)

// envTelemetryEnabled is the off-switch for the telemetry pipeline. Telemetry is
// ENABLED by default; setting this to a false-y value (false/0) turns it off.
const envTelemetryEnabled = "KAIMI_TELEMETRY_ENABLED"

// liveHistoryCap is how many recent events the in-memory LiveSink retains for
// replay to a (re)connecting live-stream client.
const liveHistoryCap = 500

// liveSubscriberBuf is the per-subscriber channel buffer for the LiveSink. It is
// intentionally small: the LiveSink drops sends to a full subscriber rather than
// blocking ingestion, and a lagging client recovers via replay on reconnect.
const liveSubscriberBuf = 64

// telemetryEnabled reports whether the telemetry pipeline should be wired, given
// the raw value of the KAIMI_TELEMETRY_ENABLED env var. It defaults to ENABLED:
// an empty value is on, and only a value strconv.ParseBool reads as false turns
// it off. A malformed value stays ENABLED, keeping the additive observability on
// the safe side of a typo.
func telemetryEnabled(env string) bool {
	if env == "" {
		return true
	}
	v, err := strconv.ParseBool(env)
	if err != nil {
		return true
	}
	return v
}

// setupTelemetry builds the local, privacy-first telemetry pipeline rooted under
// storePath and installs it as the process-wide kobs emitter.
//
// The pipeline fans every event to two local sinks: a durable append-only JSONL
// event log (storePath/telemetry/events.jsonl) and an in-memory LiveSink that
// backs the real-time stream. Both sit behind a redact.Gate whose Central is nil
// — there is no egress path yet, so content cannot leave the deployment (central
// forwarding is an opt-in for a later phase). The async emit.Emitter is the
// non-blocking front end the rest of the code reaches through kobs.Emit.
//
// It returns the LiveSink (so the HTTP layer can expose the live stream) and the
// Emitter (so the entrypoint can flush+close it on shutdown via em.Shutdown).
func setupTelemetry(storePath string) (*sink.LiveSink, *emit.Emitter, error) {
	dir := filepath.Join(storePath, "telemetry")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create telemetry dir %q: %w", dir, err)
	}

	jsonl, err := sink.NewJSONLSink(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		return nil, nil, fmt.Errorf("open telemetry event log: %w", err)
	}

	live := sink.NewLiveSink(liveHistoryCap, liveSubscriberBuf)

	// Local fan-out: durable log + live stream. The gate keeps content in by
	// having no Central sink (egress is a later-phase opt-in).
	gate := redact.Gate{Local: sink.Multi{jsonl, live}, Central: nil}

	em := emit.New(gate, emit.Config{})
	kobs.Init(em)

	return live, em, nil
}

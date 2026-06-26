package kobs

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Mawar2/kaimi-telemetry/emit"
	"github.com/Mawar2/kaimi-telemetry/redact"
	"github.com/Mawar2/kaimi-telemetry/sink"
)

// Telemetry tuning constants for the local pipeline built by Setup.
const (
	// liveHistoryCap is how many recent events the in-memory LiveSink retains for
	// replay to a (re)connecting live-stream client.
	liveHistoryCap = 500
	// liveSubscriberBuf is the per-subscriber channel buffer for the LiveSink. It
	// is intentionally small: the LiveSink drops sends to a full subscriber rather
	// than blocking ingestion, and a lagging client recovers via replay on
	// reconnect.
	liveSubscriberBuf = 64
	// eventLogName is the file, under the telemetry directory, that holds the
	// durable append-only JSONL event log.
	eventLogName = "events.jsonl"
)

// Setup builds the local, privacy-first telemetry pipeline rooted at dir and
// installs it as the process-wide kobs emitter (via Init). It is the single
// builder every Kaimi binary uses to make telemetry live, so the wiring — and
// the privacy boundary — lives in exactly one place.
//
// The pipeline fans every event to two local sinks: a durable append-only JSONL
// event log (dir/events.jsonl) and an in-memory LiveSink that backs the
// real-time stream. Both sit behind a redact.Gate whose Central is nil — there
// is NO egress path, so content-class attributes (prompts, responses) can never
// leave the deployment. Central forwarding is an opt-in for a later phase
// (TODO(phase-N): central rollup, tracked as T0.10). The async emit.Emitter is
// the non-blocking front end the rest of the code reaches through Emit.
//
// bufferSize sets the emitter's bounded-channel capacity; a value below 1 lets
// the emitter apply its own default. Setup returns the LiveSink (so an HTTP
// layer can expose the live stream — the seam T0.9's SSE Monitor consumes) and
// the Emitter (so the entrypoint can flush+close it on shutdown via Shutdown).
func Setup(dir string, bufferSize int) (*sink.LiveSink, *emit.Emitter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create telemetry dir %q: %w", dir, err)
	}

	jsonl, err := sink.NewJSONLSink(filepath.Join(dir, eventLogName))
	if err != nil {
		return nil, nil, fmt.Errorf("open telemetry event log: %w", err)
	}

	live := sink.NewLiveSink(liveHistoryCap, liveSubscriberBuf)

	em := emit.New(localGate(jsonl, live), emit.Config{BufferSize: bufferSize})
	Init(em)

	return live, em, nil
}

// localGate builds the redact.Gate that fans the full event to the durable JSONL
// log and the live stream while keeping content local: Central is nil, so the
// gate has no egress path and content-class attributes never leave the
// deployment. Factored out so the privacy wiring can be asserted directly in a
// unit test.
func localGate(jsonl, live sink.EventSink) redact.Gate {
	return redact.Gate{Local: sink.Multi{jsonl, live}, Central: nil}
}

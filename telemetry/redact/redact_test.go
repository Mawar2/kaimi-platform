package redact

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/Mawar2/kaimi-telemetry/event"
	"github.com/Mawar2/kaimi-telemetry/sink"
)

// recordingSink captures every event it receives so a test can inspect exactly
// what reached a destination. It is safe for concurrent use.
type recordingSink struct {
	mu       sync.Mutex
	received []event.Event
	flushes  int
	closes   int
}

//nolint:gocritic // hugeParam: implements the EventSink.Emit value-event contract.
func (r *recordingSink) Emit(_ context.Context, e event.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Copy the slice header's backing so later mutation of the caller's event
	// cannot retroactively change what we recorded.
	r.received = append(r.received, e)
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

// secret is a unique string that stands in for a sensitive prompt. The MOAT
// test proves this value never reaches the central sink, in any field.
const secret = "PROMPT-SECRET-9f83c2a1-do-not-leak-this-string"

// TestGateNeverLeaksContentToCentral is the product's core guarantee: content
// stays local, usage may travel, and the secret never appears centrally.
func TestGateNeverLeaksContentToCentral(t *testing.T) {
	local := &recordingSink{}
	central := &recordingSink{}
	g := Gate{Local: local, Central: central}

	in := event.NewEvent(event.CategoryLLM, "generation",
		event.Usage("input_tokens", 1234),
		event.Content("prompt", secret),
	)
	// Snapshot the input so we can prove Strip did not mutate it.
	before, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	if err := g.Emit(context.Background(), in); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	// (1) The central sink received exactly one event.
	if len(central.received) != 1 {
		t.Fatalf("central received %d events, want 1", len(central.received))
	}
	ce := central.received[0]

	// (2) The content key must be entirely absent from the central event.
	if _, ok := ce.Attributes.Get("prompt"); ok {
		t.Error("central event contains the content key \"prompt\", want it stripped")
	}

	// (3) The secret VALUE must not appear anywhere in the central event — prove
	// it by marshaling the whole event to JSON and scanning the bytes.
	centralJSON, err := json.Marshal(ce)
	if err != nil {
		t.Fatalf("marshal central event: %v", err)
	}
	if strings.Contains(string(centralJSON), secret) {
		t.Errorf("secret leaked to central sink: %s", centralJSON)
	}

	// (4) The usage attr DID arrive centrally and kept its value.
	if v, ok := ce.Attributes.Get("input_tokens"); !ok {
		t.Error("central event missing the usage attr \"input_tokens\"")
	} else if v != 1234 {
		t.Errorf("central usage value = %v, want 1234", v)
	}

	// The envelope is preserved centrally.
	if ce.EventID != in.EventID || ce.Name != in.Name || ce.Category != in.Category {
		t.Errorf("central envelope = %s/%s/%s, want %s/%s/%s",
			ce.EventID, ce.Name, ce.Category, in.EventID, in.Name, in.Category)
	}

	// (5) The Local sink received the content intact.
	if len(local.received) != 1 {
		t.Fatalf("local received %d events, want 1", len(local.received))
	}
	le := local.received[0]
	if v, ok := le.Attributes.Get("prompt"); !ok {
		t.Error("local event missing the content attr \"prompt\"")
	} else if v != secret {
		t.Errorf("local content value = %v, want the secret", v)
	}
	if v, ok := le.Attributes.Get("input_tokens"); !ok || v != 1234 {
		t.Errorf("local usage attr = %v/%v, want 1234/true", v, ok)
	}

	// (6) Strip did not mutate the input event.
	after, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("re-marshal input: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("input mutated:\n before=%s\n after =%s", before, after)
	}
	if _, ok := in.Attributes.Get("prompt"); !ok {
		t.Error("input lost its content attr — Strip mutated the caller's event")
	}
}

// TestGateNilCentralProducesZeroEgress proves that with no central sink, content
// never goes anywhere but local — there is simply no egress path.
func TestGateNilCentralProducesZeroEgress(t *testing.T) {
	local := &recordingSink{}
	g := Gate{Local: local, Central: nil}

	in := event.NewEvent(event.CategoryLLM, "generation",
		event.Content("prompt", secret),
	)
	if err := g.Emit(context.Background(), in); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if len(local.received) != 1 {
		t.Fatalf("local received %d events, want 1", len(local.received))
	}
	// Flush and Close must not panic with a nil central sink.
	if err := g.Flush(context.Background()); err != nil {
		t.Errorf("Flush: %v", err)
	}
	if err := g.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if local.flushes != 1 || local.closes != 1 {
		t.Errorf("local flushes/closes = %d/%d, want 1/1", local.flushes, local.closes)
	}
}

// TestStripRemovesAllContentPreservesUsage covers Strip directly, including the
// no-mutation guarantee and preservation of every usage attr.
func TestStripRemovesAllContentPreservesUsage(t *testing.T) {
	in := event.NewEvent(event.CategoryLLM, "gen",
		event.Usage("tokens", 10),
		event.Content("prompt", "p1"),
		event.Usage("model", "m"),
		event.Content("response", "r1"),
	)

	out := Strip(in)

	if len(out.Attributes) != 2 {
		t.Fatalf("stripped attrs = %d, want 2 usage attrs", len(out.Attributes))
	}
	for _, a := range out.Attributes {
		if a.Class == event.ClassContent {
			t.Errorf("stripped event still has content attr %q", a.Key)
		}
	}
	if _, ok := out.Attributes.Get("tokens"); !ok {
		t.Error("stripped event lost usage attr \"tokens\"")
	}
	if _, ok := out.Attributes.Get("model"); !ok {
		t.Error("stripped event lost usage attr \"model\"")
	}

	// Input untouched: still has its 4 attrs in original order.
	if len(in.Attributes) != 4 {
		t.Errorf("input attrs mutated: len = %d, want 4", len(in.Attributes))
	}

	// Mutating the stripped copy's slice must not bleed into the input.
	if len(out.Attributes) > 0 {
		out.Attributes[0].Value = "tampered"
		if v, _ := in.Attributes.Get("tokens"); v == "tampered" {
			t.Error("mutating stripped copy changed the input's backing array")
		}
	}
}

// TestStripNoContentReturnsEquivalentUsage confirms an all-usage event survives
// Strip unchanged in content.
func TestStripNoContentReturnsEquivalentUsage(t *testing.T) {
	in := event.NewEvent(event.CategorySystem, "boot", event.Usage("k", "v"))
	out := Strip(in)
	if len(out.Attributes) != 1 {
		t.Fatalf("stripped attrs = %d, want 1", len(out.Attributes))
	}
	if v, ok := out.Attributes.Get("k"); !ok || v != "v" {
		t.Errorf("usage attr = %v/%v, want v/true", v, ok)
	}
}

// Gate must satisfy the EventSink interface.
var _ sink.EventSink = Gate{}

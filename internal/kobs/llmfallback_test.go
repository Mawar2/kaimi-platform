package kobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Mawar2/kaimi-telemetry/event"
)

// freshCtx returns a unique, non-Background context so each test case keys the
// process-wide fallback tracker under its own operation, mirroring how the
// fallback loop threads one context through a single failover sequence while
// independent operations carry distinct contexts.
func freshCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}

// fallbackEvents returns only the llm.fallback.triggered events from evs.
func fallbackEvents(evs []event.Event) []event.Event {
	var out []event.Event
	// Iterate by index: event.Event is large (240 bytes), so a range value copy
	// is wasteful and trips gocritic's rangeValCopy.
	for i := range evs {
		if evs[i].Name == EventLLMFallbackTriggered {
			out = append(out, evs[i])
		}
	}
	return out
}

// TestFallbackTriggeredOnPrimaryFailThenFallbackSuccess drives a
// primary-fails-then-fallback-succeeds sequence (including a same-model retry of
// the primary, as the fallback loop does) through the tracker hooks and asserts
// exactly one llm.fallback.triggered carrying both model names and the
// triggering reason.
func TestFallbackTriggeredOnPrimaryFailThenFallbackSuccess(t *testing.T) {
	capture, restore := NewCapture()
	defer restore()

	ctx := freshCtx(t)
	primaryErr1 := errors.New("429 resource exhausted")
	primaryErr2 := errors.New("503 unavailable")

	// Option #0 (primary), attempt 1: fails.
	noteFallbackStart(ctx, "gemini-3.1-pro-preview")
	noteFallbackResult(ctx, "gemini-3.1-pro-preview", primaryErr1)
	// Option #0 (primary), attempt 2 — same-model transient retry: fails again.
	noteFallbackStart(ctx, "gemini-3.1-pro-preview")
	noteFallbackResult(ctx, "gemini-3.1-pro-preview", primaryErr2)
	// Option #1 (fallback): different model, succeeds.
	noteFallbackStart(ctx, "gemini-2.5-flash")
	noteFallbackResult(ctx, "gemini-2.5-flash", nil)

	got := fallbackEvents(capture.Drain())
	if len(got) != 1 {
		t.Fatalf("llm.fallback.triggered count = %d, want 1", len(got))
	}
	ev := got[0]
	if ev.Category != event.CategoryLLM {
		t.Errorf("Category = %q, want %q", ev.Category, event.CategoryLLM)
	}
	if v := attrValue(t, ev, AttrLLMPrimaryModel); v != "gemini-3.1-pro-preview" {
		t.Errorf("primary_model = %v, want gemini-3.1-pro-preview", v)
	}
	if v := attrValue(t, ev, AttrLLMFallbackModel); v != "gemini-2.5-flash" {
		t.Errorf("fallback_model = %v, want gemini-2.5-flash", v)
	}
	// The reason is the failure that immediately preceded the failover.
	if v := attrValue(t, ev, AttrLLMFallbackReason); v != primaryErr2.Error() {
		t.Errorf("fallback_reason = %v, want %q", v, primaryErr2.Error())
	}

	// All three failover attributes must be forwardable usage-class.
	for _, key := range []string{AttrLLMPrimaryModel, AttrLLMFallbackModel, AttrLLMFallbackReason} {
		if class := attrClass(t, ev, key); class != event.ClassUsage {
			t.Errorf("attr %q class = %d, want ClassUsage", key, class)
		}
	}
}

// TestFallbackNotTriggeredOnAllSuccess confirms a sequence with no failures
// emits no llm.fallback.triggered event.
func TestFallbackNotTriggeredOnAllSuccess(t *testing.T) {
	capture, restore := NewCapture()
	defer restore()

	ctx := freshCtx(t)
	noteFallbackStart(ctx, "gemini-3.1-pro-preview")
	noteFallbackResult(ctx, "gemini-3.1-pro-preview", nil)

	if got := fallbackEvents(capture.Drain()); len(got) != 0 {
		t.Fatalf("llm.fallback.triggered count = %d, want 0", len(got))
	}
}

// TestFallbackNotTriggeredOnSameModelRecovery confirms a transient failure that
// recovers on the SAME model (a plain retry, not a failover) emits nothing.
func TestFallbackNotTriggeredOnSameModelRecovery(t *testing.T) {
	capture, restore := NewCapture()
	defer restore()

	ctx := freshCtx(t)
	noteFallbackStart(ctx, "gemini-3.1-pro-preview")
	noteFallbackResult(ctx, "gemini-3.1-pro-preview", errors.New("503 unavailable"))
	noteFallbackStart(ctx, "gemini-3.1-pro-preview")
	noteFallbackResult(ctx, "gemini-3.1-pro-preview", nil)

	if got := fallbackEvents(capture.Drain()); len(got) != 0 {
		t.Fatalf("llm.fallback.triggered count = %d, want 0", len(got))
	}
}

// TestFallbackIsolatedAcrossContexts confirms the context key isolates
// concurrent operations: a failure on one operation never makes a different-model
// call on an UNRELATED operation look like a failover.
func TestFallbackIsolatedAcrossContexts(t *testing.T) {
	capture, restore := NewCapture()
	defer restore()

	ctxA := freshCtx(t)
	ctxB := freshCtx(t)

	// Operation A's primary fails.
	noteFallbackStart(ctxA, "gemini-3.1-pro-preview")
	noteFallbackResult(ctxA, "gemini-3.1-pro-preview", errors.New("429 quota"))
	// Operation B independently calls a different model and succeeds — not a
	// failover of A.
	noteFallbackStart(ctxB, "gemini-2.5-flash")
	noteFallbackResult(ctxB, "gemini-2.5-flash", nil)

	if got := fallbackEvents(capture.Drain()); len(got) != 0 {
		t.Fatalf("llm.fallback.triggered count = %d, want 0 (cross-context leak)", len(got))
	}
}

// TestFallbackStaleFailureIgnored confirms a leftover failure older than the
// staleness window does not correlate with a much-later different-model call.
func TestFallbackStaleFailureIgnored(t *testing.T) {
	capture, restore := NewCapture()
	defer restore()

	orig := fallbackStaleAfter
	fallbackStaleAfter = 10 * time.Millisecond
	defer func() { fallbackStaleAfter = orig }()

	ctx := freshCtx(t)
	noteFallbackStart(ctx, "gemini-3.1-pro-preview")
	noteFallbackResult(ctx, "gemini-3.1-pro-preview", errors.New("503 unavailable"))

	time.Sleep(20 * time.Millisecond) // let the recorded failure go stale

	noteFallbackStart(ctx, "gemini-2.5-flash")
	noteFallbackResult(ctx, "gemini-2.5-flash", nil)

	if got := fallbackEvents(capture.Drain()); len(got) != 0 {
		t.Fatalf("llm.fallback.triggered count = %d, want 0 (stale failure)", len(got))
	}
}

// TestBuildLLMFallbackEvent unit-tests the event builder directly: name, model
// fields, reason, and a nil-error empty reason.
func TestBuildLLMFallbackEvent(t *testing.T) {
	ev := buildLLMFallbackEvent("model-a", "model-b", errors.New("429 rate limit"))
	if ev.Name != EventLLMFallbackTriggered {
		t.Errorf("Name = %q, want %q", ev.Name, EventLLMFallbackTriggered)
	}
	if v := attrValue(t, ev, AttrLLMPrimaryModel); v != "model-a" {
		t.Errorf("primary_model = %v, want model-a", v)
	}
	if v := attrValue(t, ev, AttrLLMFallbackModel); v != "model-b" {
		t.Errorf("fallback_model = %v, want model-b", v)
	}
	if v := attrValue(t, ev, AttrLLMFallbackReason); v != "429 rate limit" {
		t.Errorf("fallback_reason = %v, want '429 rate limit'", v)
	}

	if v := attrValue(t, buildLLMFallbackEvent("a", "b", nil), AttrLLMFallbackReason); v != "" {
		t.Errorf("nil-error reason = %v, want empty", v)
	}
}

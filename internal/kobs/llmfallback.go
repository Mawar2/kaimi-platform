package kobs

import (
	"context"
	"sync"
	"time"

	"github.com/Mawar2/kaimi-telemetry/event"
)

// EventLLMFallbackTriggered is emitted when the model fallback chain fails over
// from one model backend to a different one: a generation call fails, and the
// next call in the same operation is issued against a different model. It is the
// "we lost the primary and switched backends" signal.
const EventLLMFallbackTriggered = "llm.fallback.triggered"

// fallbackStaleAfter bounds how long a recorded call failure stays eligible to
// correlate with a following different-model call. The fallback loop issues the
// next attempt almost immediately after a failure (only the inter-attempt
// backoff elapses before the next call *starts*), so a generous window safely
// covers any in-sequence gap. Its real job is hygiene: it lets the tracker drop
// the leftover entry from a fully-failed sequence — one whose chain then halts
// and never reuses its context — instead of holding it indefinitely. It is a var
// so tests can shrink it; production code never mutates it.
var fallbackStaleAfter = 2 * time.Minute

// fallbackPending records the most recent failed model call for one operation,
// so the next call in that operation can tell whether it is a failover.
type fallbackPending struct {
	model string
	err   error
	at    time.Time
}

// fallbackTracker correlates a failed model call with the following
// different-model call within the SAME operation, so it can emit exactly one
// llm.fallback.triggered per failover without any change to internal/fallback.
//
// The correlation key is the call's context.Context. The fallback loop
// (internal/fallback.run) threads the identical ctx through every option it
// tries, and each option passes that ctx straight to GenerateContent, so all
// calls in one failover sequence share one key while independent, concurrent
// proposals carry distinct contexts and never cross-contaminate. The detection
// therefore lives entirely in this wrapper layer — additive, and invisible to
// the fallback package.
type fallbackTracker struct {
	mu      sync.Mutex
	pending map[context.Context]fallbackPending
}

// fallbacks is the process-wide tracker. It is keyed by context, scoping state
// to a single in-flight operation; a nil or unused tracker simply never emits.
var fallbacks = &fallbackTracker{pending: make(map[context.Context]fallbackPending)}

// noteFallbackStart is called just before a model call is issued for model. If a
// previous call on the same context (operation) failed against a DIFFERENT
// model and that failure is still fresh, this call is the failover: it emits one
// llm.fallback.triggered carrying the failed (primary) model, this (fallback)
// model, and the triggering error, then consumes the record so the same failover
// is never reported twice. A same-model retry, a stale record, or no prior
// failure emits nothing.
func noteFallbackStart(ctx context.Context, model string) {
	if ctx == nil {
		return
	}

	fallbacks.mu.Lock()
	prev, ok := fallbacks.pending[ctx]
	switch {
	case !ok:
		// No prior failure on this operation: nothing to correlate.
	case time.Since(prev.at) > fallbackStaleAfter:
		// Leftover from an earlier, already-finished sequence: drop it.
		delete(fallbacks.pending, ctx)
		ok = false
	case prev.model == model:
		// Same-model retry, not a failover: leave the record to be refreshed or
		// cleared by this call's outcome.
		ok = false
	default:
		// Different model after a fresh failure: this is the failover. Consume
		// the record so it is reported exactly once.
		delete(fallbacks.pending, ctx)
	}
	fallbacks.mu.Unlock()

	if ok {
		// Emit outside the lock; Emit is a non-blocking no-op until an emitter is
		// installed, so this never blocks or alters control flow.
		Emit(buildLLMFallbackEvent(prev.model, model, prev.err))
	}
}

// noteFallbackResult is called just after a model call returns. A failure is
// recorded (keyed by the operation's context) so a following different-model
// call can recognize the failover; a success clears any pending record because
// the operation recovered — whether on the same model (a plain retry) or after a
// failover already reported by noteFallbackStart.
func noteFallbackResult(ctx context.Context, model string, err error) {
	if ctx == nil {
		return
	}

	fallbacks.mu.Lock()
	defer fallbacks.mu.Unlock()
	if err != nil {
		fallbacks.pending[ctx] = fallbackPending{model: model, err: err, at: time.Now()}
		fallbacks.sweepLocked()
		return
	}
	delete(fallbacks.pending, ctx)
}

// sweepLocked drops stale entries left behind by sequences that failed on every
// option (their chain halts and never reuses the context, so no later call would
// clear them). It runs opportunistically on the failure path, where volume is
// low, and must be called with the tracker mutex held.
func (t *fallbackTracker) sweepLocked() {
	for ctx, p := range t.pending {
		if time.Since(p.at) > fallbackStaleAfter {
			delete(t.pending, ctx)
		}
	}
}

// buildLLMFallbackEvent builds the llm.fallback.triggered event. The two model
// IDs and the triggering reason are usage-class: the reason is the transient
// backend error that forced the failover, not model content. A nil err yields an
// empty reason, though in practice a failover is only recorded after a real
// error.
func buildLLMFallbackEvent(primaryModel, fallbackModel string, err error) event.Event {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return LLMEvent(EventLLMFallbackTriggered, "",
		LLMPrimaryModel(primaryModel),
		LLMFallbackModel(fallbackModel),
		LLMFallbackReason(reason),
	)
}

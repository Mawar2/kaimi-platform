package redact

import (
	"context"
	"fmt"

	"github.com/Mawar2/kaimi-telemetry/event"
	"github.com/Mawar2/kaimi-telemetry/sink"
)

// Strip returns a copy of e with every ClassContent attribute removed. The
// envelope and all ClassUsage attributes are preserved. The input event is
// never mutated: the returned event's Attributes is a fresh slice, so callers
// can forward the result while keeping the original intact for local storage.
//
//nolint:gocritic // hugeParam: Strip takes a value event by design so it cannot alias or mutate the caller's event.
func Strip(e event.Event) event.Event {
	// Build a new attribute slice holding only the usage-class attributes. We
	// allocate a fresh backing array so that mutating the stripped copy can
	// never write through to the input's slice.
	kept := make(event.Attrs, 0, len(e.Attributes))
	for _, a := range e.Attributes {
		if a.Class == event.ClassContent {
			continue
		}
		kept = append(kept, a)
	}

	// Copy the envelope by value, then replace the Attributes header. An event
	// with no surviving attributes carries a nil slice, which JSON-omits cleanly.
	out := e
	if len(kept) == 0 {
		out.Attributes = nil
	} else {
		out.Attributes = kept
	}
	return out
}

// Gate is an EventSink that enforces the content/usage privacy boundary. It
// writes the full event to Local (which stays inside the deployment) and the
// stripped event to Central (which may be remote). When Central is nil there is
// no egress path, so content cannot leave.
type Gate struct {
	// Local receives the full, unredacted event. It must not be nil.
	Local sink.EventSink
	// Central receives the stripped event (usage only). If nil, nothing is
	// forwarded and the gate behaves as a local-only sink.
	Central sink.EventSink
}

// Emit writes the full event to Local, then the stripped event to Central when
// Central is set. Errors from both destinations are reported, with Local taking
// precedence in the wrapped message.
//
//nolint:gocritic // hugeParam: implements the EventSink.Emit value-event contract.
func (g Gate) Emit(ctx context.Context, e event.Event) error {
	if err := g.Local.Emit(ctx, e); err != nil {
		return fmt.Errorf("redact: emit to local sink: %w", err)
	}
	if g.Central != nil {
		if err := g.Central.Emit(ctx, Strip(e)); err != nil {
			return fmt.Errorf("redact: emit to central sink: %w", err)
		}
	}
	return nil
}

// Flush flushes Local, then Central when set. It returns the first error.
func (g Gate) Flush(ctx context.Context) error {
	if err := g.Local.Flush(ctx); err != nil {
		return fmt.Errorf("redact: flush local sink: %w", err)
	}
	if g.Central != nil {
		if err := g.Central.Flush(ctx); err != nil {
			return fmt.Errorf("redact: flush central sink: %w", err)
		}
	}
	return nil
}

// Close closes Local, then Central when set. It returns the first error.
func (g Gate) Close() error {
	if err := g.Local.Close(); err != nil {
		return fmt.Errorf("redact: close local sink: %w", err)
	}
	if g.Central != nil {
		if err := g.Central.Close(); err != nil {
			return fmt.Errorf("redact: close central sink: %w", err)
		}
	}
	return nil
}

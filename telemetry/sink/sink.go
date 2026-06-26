package sink

import (
	"context"
	"errors"

	"github.com/Mawar2/kaimi-telemetry/event"
)

// ErrClosed is returned by Emit and Flush after the sink has been closed.
var ErrClosed = errors.New("sink: closed")

// EventSink is a destination for emitted events. Implementations must be safe
// for concurrent use. After Close, Emit and Flush return ErrClosed.
type EventSink interface {
	// Emit delivers a single event to the sink.
	Emit(ctx context.Context, e event.Event) error
	// Flush durably persists whatever the sink has buffered.
	Flush(ctx context.Context) error
	// Close flushes and releases the sink's resources.
	Close() error
}

// Multi fans every operation out to a slice of EventSinks. It always reaches
// every sink and aggregates their errors with errors.Join, so one failing sink
// neither short-circuits the fan-out nor masks failures from the others.
type Multi []EventSink

// Emit delivers e to every underlying sink, joining any errors.
//
//nolint:gocritic // hugeParam: the EventSink.Emit contract mandates a value param.
func (m Multi) Emit(ctx context.Context, e event.Event) error {
	errs := make([]error, 0, len(m))
	for _, s := range m {
		if err := s.Emit(ctx, e); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Flush flushes every underlying sink, joining any errors.
func (m Multi) Flush(ctx context.Context) error {
	errs := make([]error, 0, len(m))
	for _, s := range m {
		if err := s.Flush(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Close closes every underlying sink, joining any errors.
func (m Multi) Close() error {
	errs := make([]error, 0, len(m))
	for _, s := range m {
		if err := s.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

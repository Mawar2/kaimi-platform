package sink

import (
	"context"
	"sync"

	"github.com/Mawar2/kaimi-telemetry/event"
)

// LiveSink is an in-memory sink that powers the real-time Monitor. It retains
// recent events in a ring buffer for replay and fans each new event out to a
// set of subscriber channels.
//
// Ingestion is never allowed to block: when a subscriber's channel is full
// (the consumer cannot keep up), that subscriber's send is dropped rather than
// stalling Emit. A dropped subscriber can recover the gap via Replay or by
// resubscribing with the last sequence number it saw.
type LiveSink struct {
	mu     sync.Mutex
	ring   *ring
	subs   map[int]chan Stamped
	nextID int
	subBuf int
	closed bool
}

// NewLiveSink returns a LiveSink that retains up to historyCap events for replay
// and gives each subscriber a channel buffered to subscriberBuf events before
// its sends start being dropped.
func NewLiveSink(historyCap, subscriberBuf int) *LiveSink {
	if subscriberBuf < 0 {
		subscriberBuf = 0
	}
	return &LiveSink{
		ring:   newRing(historyCap),
		subs:   make(map[int]chan Stamped),
		subBuf: subscriberBuf,
	}
}

// Emit appends e to the ring buffer and non-blockingly sends the stamped entry
// to every subscriber, dropping the send for any subscriber that is full. It
// returns ErrClosed if the sink has been closed.
//
//nolint:gocritic // hugeParam: the EventSink.Emit contract mandates a value param.
func (l *LiveSink) Emit(_ context.Context, e event.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return ErrClosed
	}

	s := l.ring.Append(e)
	for _, ch := range l.subs {
		select {
		case ch <- s:
		default:
			// Subscriber is not keeping up; drop this send rather than block
			// ingestion. The subscriber can recover via Replay/resubscribe.
		}
	}
	return nil
}

// Flush is a no-op for the in-memory LiveSink and exists to satisfy EventSink.
// It returns ErrClosed if the sink has been closed.
func (l *LiveSink) Flush(_ context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return ErrClosed
	}
	return nil
}

// Close marks the sink closed and closes every subscriber channel so that
// ranging consumers terminate. It is idempotent.
func (l *LiveSink) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	for id, ch := range l.subs {
		close(ch)
		delete(l.subs, id)
	}
	return nil
}

// Subscribe registers a new subscriber and returns a receive-only channel of
// future events along with a cancel function that unregisters it and closes the
// channel. The channel is first seeded (non-blockingly, up to its buffer) with
// any retained events whose Seq is greater than afterSeq, so a reconnecting
// consumer does not miss events that are still in the ring.
//
// If the sink is already closed, the returned channel is closed and cancel is a
// no-op.
func (l *LiveSink) Subscribe(afterSeq uint64) (events <-chan Stamped, cancel func()) {
	l.mu.Lock()
	defer l.mu.Unlock()

	ch := make(chan Stamped, l.subBuf)
	if l.closed {
		close(ch)
		return ch, func() {}
	}

	// Seed with still-buffered history newer than afterSeq. Holding l.mu keeps
	// this consistent with concurrent Emit, so there is no gap or duplicate
	// between the seeded events and the first live one.
	history := l.ring.Since(afterSeq)
	for i := range history {
		select {
		case ch <- history[i]:
		default:
		}
	}

	id := l.nextID
	l.nextID++
	l.subs[id] = ch

	var once sync.Once
	cancel = func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if existing, ok := l.subs[id]; ok {
				delete(l.subs, id)
				close(existing)
			}
		})
	}
	return ch, cancel
}

// Replay returns a copy of every retained event whose Seq is greater than
// afterSeq, oldest first.
func (l *LiveSink) Replay(afterSeq uint64) []Stamped {
	return l.ring.Since(afterSeq)
}

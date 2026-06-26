package sink

import (
	"sync"

	"github.com/Mawar2/kaimi-telemetry/event"
)

// Stamped pairs an event with the monotonic sequence number assigned to it when
// it entered the ring buffer. The sequence number lets subscribers resume after
// a gap (e.g. an SSE reconnect carrying a Last-Event-ID).
type Stamped struct {
	// Seq is the monotonic, 1-based sequence number for this entry.
	Seq uint64
	// Event is the stamped event.
	Event event.Event
}

// ring is a fixed-capacity, in-memory buffer of the most recent Stamped events.
// It assigns a monotonically increasing sequence number to every appended event
// and evicts the oldest entry once it reaches capacity. It is safe for
// concurrent use.
type ring struct {
	mu    sync.Mutex
	cap   int
	items []Stamped // ordered oldest-to-newest, len <= cap
	seq   uint64    // last assigned sequence number
}

// newRing returns a ring that retains up to capacity entries. A capacity below 1
// is treated as 1, since a zero-length ring could never serve a replay.
func newRing(capacity int) *ring {
	if capacity < 1 {
		capacity = 1
	}
	return &ring{cap: capacity, items: make([]Stamped, 0, capacity)}
}

// Append stores e with the next sequence number and returns the Stamped entry,
// evicting the oldest entry if the ring is already at capacity. The value
// event.Event parameter mirrors the EventSink.Emit contract that feeds the ring.
//
//nolint:gocritic // hugeParam: signature fixed to mirror the EventSink contract.
func (r *ring) Append(e event.Event) Stamped {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	s := Stamped{Seq: r.seq, Event: e}
	if len(r.items) == r.cap {
		// Drop the oldest entry, shifting the rest down, then append the new one.
		copy(r.items, r.items[1:])
		r.items[len(r.items)-1] = s
	} else {
		r.items = append(r.items, s)
	}
	return s
}

// Since returns a copy of every retained entry whose Seq is strictly greater
// than seq, oldest first. Entries already evicted are not returned.
func (r *ring) Since(seq uint64) []Stamped {
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []Stamped
	for i := range r.items {
		if r.items[i].Seq > seq {
			out = append(out, r.items[i])
		}
	}
	return out
}

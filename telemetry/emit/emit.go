package emit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"github.com/Mawar2/kaimi-telemetry/event"
	"github.com/Mawar2/kaimi-telemetry/sink"
)

// Defaults applied to zero-value Config fields.
const (
	// DefaultBufferSize is the channel capacity used when Config.BufferSize is
	// not positive.
	DefaultBufferSize = 1024
	// DefaultWorkers is the number of drain goroutines used when Config.Workers
	// is not positive.
	DefaultWorkers = 1
)

// Config tunes an Emitter. The zero value is valid: every field falls back to a
// sensible default, so New(dst, Config{}) yields a working emitter.
type Config struct {
	// BufferSize is the capacity of the bounded channel between Emit and the
	// workers. Values below 1 fall back to DefaultBufferSize.
	BufferSize int
	// Workers is the number of goroutines draining the buffer into the sink.
	// Values below 1 fall back to DefaultWorkers.
	Workers int
	// OnError is called for every non-fatal error a worker hits while
	// delivering to the sink. A nil OnError is replaced with a log.Printf
	// wrapper, so it is never nil at runtime.
	OnError func(error)
}

// withDefaults returns a copy of c with every unset field filled in. OnError is
// guaranteed non-nil on return.
func (c Config) withDefaults() Config {
	if c.BufferSize < 1 {
		c.BufferSize = DefaultBufferSize
	}
	if c.Workers < 1 {
		c.Workers = DefaultWorkers
	}
	if c.OnError == nil {
		c.OnError = func(err error) { log.Printf("emit: %v", err) }
	}
	return c
}

// item is what travels through the emitter's channel: either a real event or a
// flush barrier. Exactly one of the two is set.
type item struct {
	ev      event.Event
	barrier *barrier
}

// barrier is a Flush rendezvous. Flush enqueues one barrier per worker; each
// worker that reaches a barrier signals arrived and then parks on release.
// Because the channel is FIFO and workers process items synchronously, every
// worker can only park once all events queued ahead of the barriers have been
// fully delivered — so when all workers have arrived, the buffer is drained.
type barrier struct {
	arrived *sync.WaitGroup
	release chan struct{}
}

// Emitter is an async, non-blocking front end to a sink.EventSink. It is safe
// for concurrent use by multiple goroutines.
type Emitter struct {
	dst     sink.EventSink
	onError func(error)
	workers int

	ch chan item

	// ctx/cancel scope worker deliveries so Shutdown can unblock a worker that
	// is stuck inside a slow or hung sink.
	ctx    context.Context
	cancel context.CancelFunc

	wg sync.WaitGroup

	// mu guards closed and serializes channel sends against the close in
	// Shutdown, so no send ever races with the channel being closed.
	mu     sync.RWMutex
	closed bool

	flushMu sync.Mutex // serializes Flush so barriers from different calls never interleave
	once    sync.Once
	shutErr error
	dropped atomic.Uint64
}

// New starts cfg.Workers goroutines draining a bounded buffer into dst and
// returns the running Emitter. Zero-value Config fields take their defaults.
func New(dst sink.EventSink, cfg Config) *Emitter {
	cfg = cfg.withDefaults()
	ctx, cancel := context.WithCancel(context.Background())
	em := &Emitter{
		dst:     dst,
		onError: cfg.OnError,
		workers: cfg.Workers,
		ch:      make(chan item, cfg.BufferSize),
		ctx:     ctx,
		cancel:  cancel,
	}
	em.wg.Add(cfg.Workers)
	for i := 0; i < cfg.Workers; i++ {
		go em.worker()
	}
	return em
}

// worker drains the channel until it is closed, delivering events to the sink
// and observing flush barriers.
func (em *Emitter) worker() {
	defer em.wg.Done()
	for it := range em.ch {
		if it.barrier != nil {
			it.barrier.arrived.Done()
			<-it.barrier.release
			continue
		}
		if err := em.dst.Emit(em.ctx, it.ev); err != nil {
			em.onError(fmt.Errorf("deliver event to sink: %w", err))
		}
	}
}

// Emit offers e to the buffer and returns true if it was accepted. It NEVER
// blocks: when the buffer is full (or the emitter is shutting down) the event
// is dropped, the dropped counter is incremented, and false is returned.
//
//nolint:gocritic // hugeParam: mirrors the EventSink.Emit value-event contract.
func (em *Emitter) Emit(_ context.Context, e event.Event) bool {
	// RLock keeps the non-blocking send from racing with Shutdown's close of
	// the channel. The lock is held only for the duration of a select with a
	// default case, so it never blocks.
	em.mu.RLock()
	defer em.mu.RUnlock()
	if em.closed {
		em.dropped.Add(1)
		return false
	}
	select {
	case em.ch <- item{ev: e}:
		return true
	default:
		em.dropped.Add(1)
		return false
	}
}

// Dropped returns the total number of events dropped because the buffer was
// full or the emitter was no longer accepting.
func (em *Emitter) Dropped() uint64 {
	return em.dropped.Load()
}

// Flush blocks until every event buffered at the time of the call has been
// delivered to the sink, then flushes the sink. It is a no-op once the emitter
// has been shut down.
func (em *Emitter) Flush(ctx context.Context) error {
	em.flushMu.Lock()
	defer em.flushMu.Unlock()

	// Hold the read lock for the whole barrier dance so Shutdown cannot close
	// the channel while we are enqueuing barriers onto it.
	em.mu.RLock()
	defer em.mu.RUnlock()
	if em.closed {
		return nil
	}

	b := &barrier{arrived: &sync.WaitGroup{}, release: make(chan struct{})}
	b.arrived.Add(em.workers)
	// One barrier per worker guarantees every worker parks, which in turn
	// guarantees all events queued ahead of the barriers have drained.
	for i := 0; i < em.workers; i++ {
		em.ch <- item{barrier: b}
	}
	b.arrived.Wait()

	err := em.dst.Flush(ctx)
	close(b.release)
	if err != nil {
		return fmt.Errorf("flush sink: %w", err)
	}
	return nil
}

// Shutdown stops accepting new events, drains whatever is buffered, then
// flushes and closes the sink. It is idempotent: subsequent calls return the
// first call's result without acting again.
func (em *Emitter) Shutdown(ctx context.Context) error {
	em.once.Do(func() {
		// Stop accepting and signal end-of-stream to the workers.
		em.mu.Lock()
		em.closed = true
		close(em.ch)
		em.mu.Unlock()

		// Cancel worker deliveries so a worker stuck in a slow/hung sink
		// unblocks instead of stalling the drain forever.
		em.cancel()
		em.wg.Wait()

		if err := em.dst.Flush(ctx); err != nil {
			em.shutErr = fmt.Errorf("flush sink on shutdown: %w", err)
		}
		if err := em.dst.Close(); err != nil {
			em.shutErr = errors.Join(em.shutErr, fmt.Errorf("close sink on shutdown: %w", err))
		}
	})
	return em.shutErr
}

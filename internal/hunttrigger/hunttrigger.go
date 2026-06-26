// Package hunttrigger fires opportunity hunts on demand — e.g. immediately after a tenant
// connects their SAM.gov key during onboarding — debounced so rapid repeats can never
// launch concurrent or back-to-back hunts that waste the tenant's daily SAM quota.
//
// The actual hunt is performed by a Runner (production: executing the pipeline Cloud Run
// Job). hunttrigger owns only the "should we fire right now?" policy: at most one hunt in
// flight, and no new hunt within a minimum interval of the last one. This keeps the
// quota-safety decision in one tested place, independent of how the hunt is executed.
package hunttrigger

import (
	"context"
	"log"
	"sync"
	"time"
)

// Runner executes exactly one hunt and returns when it has been launched/finished. The
// production implementation runs the pipeline Cloud Run Job; tests inject a fake.
type Runner interface {
	Run(ctx context.Context) error
}

// runTimeout bounds a single hunt's execution so a hung Runner can't pin the "running"
// flag forever and block all future triggers.
const runTimeout = 15 * time.Minute

// Trigger debounces hunt requests. It is safe for concurrent use.
type Trigger struct {
	runner      Runner
	minInterval time.Duration
	// now is injected for deterministic tests; defaults to time.Now.
	now func() time.Time

	mu      sync.Mutex
	running bool
	lastRun time.Time
}

// New returns a Trigger that runs hunts via runner, allowing at most one hunt per
// minInterval. A minInterval of 0 disables the interval check (still serializes concurrent
// hunts via the running flag).
func New(runner Runner, minInterval time.Duration) *Trigger {
	return &Trigger{
		runner:      runner,
		minInterval: minInterval,
		now:         time.Now,
	}
}

// Fire requests a hunt and returns immediately. It returns true when a hunt was launched,
// false when the request was suppressed because a hunt is already running or one ran within
// minInterval. The hunt runs asynchronously on its own detached, time-bounded context, so a
// caller (e.g. an HTTP handler) never blocks on it and a request cancellation can't abort an
// in-flight hunt. Runner errors are logged, not returned (the caller has already moved on).
func (t *Trigger) Fire() bool {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return false
	}
	if t.minInterval > 0 && !t.lastRun.IsZero() && t.now().Sub(t.lastRun) < t.minInterval {
		t.mu.Unlock()
		return false
	}
	t.running = true
	t.lastRun = t.now()
	t.mu.Unlock()

	go t.runOnce()
	return true
}

// runOnce executes the hunt and clears the running flag, even on panic, so a Runner failure
// never permanently wedges the trigger.
func (t *Trigger) runOnce() {
	defer func() {
		t.mu.Lock()
		t.running = false
		t.mu.Unlock()
		if r := recover(); r != nil {
			log.Printf("hunttrigger: hunt panicked: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	if err := t.runner.Run(ctx); err != nil {
		log.Printf("hunttrigger: hunt failed: %v", err)
	}
}

package productkey

import (
	"context"
	"sync"
	"time"
)

// MemoryRegistry is an in-memory Registry for tests and offline/dev mode (no
// GCP). It is concurrency-safe. It is NOT durable — process restart loses keys —
// so production uses FirestoreRegistry.
type MemoryRegistry struct {
	mu sync.RWMutex
	m  map[string]Record // canonical key -> record

	// now is the clock, injectable so tests can exercise expiry deterministically.
	// Defaults to time.Now.
	now func() time.Time
}

// NewMemoryRegistry returns an empty in-memory registry.
func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{m: make(map[string]Record), now: time.Now}
}

// SetClock overrides the clock the registry uses to stamp IssuedAt/ExpiresAt on Mint.
// It exists so tests (including those in other packages, e.g. the access gate) can
// exercise key expiry deterministically; production uses the default time.Now.
func (r *MemoryRegistry) SetClock(now func() time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.now = now
}

// Mint generates a unique key for tester valid for ttl and stores it.
func (r *MemoryRegistry) Mint(_ context.Context, tester string, ttl time.Duration) (Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Generate until unique (collisions are astronomically unlikely, but the loop
	// makes "unique key" a guarantee rather than a probability).
	var key string
	for {
		k, err := GenerateKey()
		if err != nil {
			return Record{}, err
		}
		if _, exists := r.m[k]; !exists {
			key = k
			break
		}
	}
	now := r.now()
	rec := Record{
		Key:       key,
		Tester:    tester,
		IssuedAt:  now,
		ExpiresAt: now.Add(ttl),
		Revoked:   false,
	}
	r.m[key] = rec
	return rec, nil
}

// Lookup returns the record for key (after Normalize) or ErrNotFound.
func (r *MemoryRegistry) Lookup(_ context.Context, key string) (Record, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.m[Normalize(key)]
	if !ok {
		return Record{}, ErrNotFound
	}
	return rec, nil
}

// Revoke marks key revoked, or returns ErrNotFound.
func (r *MemoryRegistry) Revoke(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := Normalize(key)
	rec, ok := r.m[k]
	if !ok {
		return ErrNotFound
	}
	rec.Revoked = true
	r.m[k] = rec
	return nil
}

// List returns all records.
func (r *MemoryRegistry) List(_ context.Context) ([]Record, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Record, 0, len(r.m))
	for _, rec := range r.m {
		out = append(out, rec)
	}
	return out, nil
}

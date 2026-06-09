// Package store defines the Store interface for persisting opportunities.
//
// The Store is designed for forward compatibility: Phase 0 uses a simple JSON file
// implementation, but the interface is defined so it can be swapped for Firestore
// in Phase 1 without touching the Hunter or any other agent.
//
// This is "provision lazily, design eagerly" in action - we design the interface
// now so implementations can be swapped later without breaking the agents.
package store

import (
	"context"
	"errors"

	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// ErrNotFound is returned by Get and Delete when the requested opportunity does
// not exist in the store. Callers should use errors.Is(err, store.ErrNotFound)
// to distinguish a missing record from an infrastructure failure.
var ErrNotFound = errors.New("not found")

// Store defines the interface for persisting and retrieving opportunities.
//
// Implementations must be safe for concurrent access. The Hunter writes
// opportunities, the Scorer reads and updates them, and the Manager tracks
// proposal status - all potentially happening in parallel at scale.
type Store interface {
	// Save persists an opportunity to the store.
	// If an opportunity with the same ID already exists, it should be updated.
	// Returns an error if the save operation fails.
	Save(ctx context.Context, opp *opportunity.Opportunity) error

	// Get retrieves an opportunity by its ID.
	// Returns the opportunity if found, or an error if not found or retrieval fails.
	Get(ctx context.Context, id string) (*opportunity.Opportunity, error)

	// List returns all opportunities in the store, optionally filtered.
	// If filter is nil, returns all opportunities.
	// Returns an empty slice if no opportunities match, or an error if retrieval fails.
	List(ctx context.Context, filter *Filter) ([]*opportunity.Opportunity, error)

	// Delete removes an opportunity from the store by ID.
	// Returns an error if the opportunity doesn't exist or deletion fails.
	Delete(ctx context.Context, id string) error
}

// Filter defines criteria for filtering opportunities when listing.
//
// All non-zero fields are AND'ed together. Zero values for a field mean "don't filter
// on this field."
//
// TODO(phase-1): Expand filter criteria as needed (e.g., date ranges, score thresholds).
type Filter struct {
	// Selected filters to only selected (true) or unselected (false) opportunities.
	// If nil, no filtering on selection status.
	Selected *bool

	// MinScore filters opportunities with score >= this value.
	// Only applicable after Scorer has run (Phase 1).
	MinScore float64

	// MaxScore filters opportunities with score <= this value.
	MaxScore float64
}

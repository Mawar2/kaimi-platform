package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// JSONStore implements the Store interface using JSON files on disk.
//
// Each opportunity is stored as a separate JSON file in the queue directory,
// named by its ID (e.g., queue/{id}.json). This simple implementation is
// suitable for Phase 0 and can be swapped for Firestore in Phase 1 without
// touching the Hunter or any other agent.
//
// Thread-safety: All operations are protected by a RWMutex, allowing multiple
// concurrent readers or a single writer. This ensures the store is safe for
// concurrent access from multiple goroutines (important when the system scales
// to handle multiple proposals in parallel).
type JSONStore struct {
	basePath  string       // Base directory containing the queue/ subdirectory
	queuePath string       // Full path to queue/ directory where JSON files are stored
	mu        sync.RWMutex // Protects concurrent access to the file system
}

// NewJSONStore creates a new JSON file-backed Store.
//
// The store will create a queue/ subdirectory under basePath if it doesn't exist.
// Each opportunity will be saved as queue/{id}.json.
//
// Parameters:
//   - basePath: The directory where the queue/ subdirectory will be created.
//     Must be a valid directory path (not a file).
//
// Returns an error if:
//   - basePath is not a valid directory
//   - queue/ subdirectory cannot be created
func NewJSONStore(basePath string) (Store, error) {
	// Verify basePath exists and is a directory
	info, err := os.Stat(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			// If basePath doesn't exist, try to create it
			if err := os.MkdirAll(basePath, 0o755); err != nil {
				return nil, fmt.Errorf("failed to create base directory: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to stat base path: %w", err)
		}
	} else if !info.IsDir() {
		return nil, fmt.Errorf("base path %s is not a directory", basePath)
	}

	// Create queue subdirectory
	queuePath := filepath.Join(basePath, "queue")
	if err := os.MkdirAll(queuePath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create queue directory: %w", err)
	}

	return &JSONStore{
		basePath:  basePath,
		queuePath: queuePath,
	}, nil
}

// Save persists an opportunity to the store as a JSON file.
//
// If an opportunity with the same ID already exists, it will be overwritten.
// The opportunity is saved to queue/{id}.json.
//
// Thread-safety: Acquires a write lock for the duration of the operation.
func (s *JSONStore) Save(ctx context.Context, opp *opportunity.Opportunity) error {
	if opp == nil {
		return fmt.Errorf("opportunity cannot be nil")
	}
	if opp.ID == "" {
		return fmt.Errorf("opportunity ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Marshal opportunity to JSON with indentation for readability
	data, err := json.MarshalIndent(opp, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal opportunity: %w", err)
	}

	// Write to file
	filePath := s.filePath(opp.ID)
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write opportunity file: %w", err)
	}

	return nil
}

// Get retrieves an opportunity by ID from the store.
//
// Returns an error if:
//   - The opportunity doesn't exist
//   - The file cannot be read
//   - The JSON cannot be parsed
//
// Thread-safety: Acquires a read lock for the duration of the operation.
func (s *JSONStore) Get(ctx context.Context, id string) (*opportunity.Opportunity, error) {
	if id == "" {
		return nil, fmt.Errorf("opportunity ID cannot be empty")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := s.filePath(id)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("opportunity %s: %w", id, ErrNotFound)
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read opportunity file: %w", err)
	}

	// Unmarshal JSON
	var opp opportunity.Opportunity
	if err := json.Unmarshal(data, &opp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal opportunity: %w", err)
	}

	return &opp, nil
}

// List returns all opportunities in the store, optionally filtered.
//
// If filter is nil, returns all opportunities.
// Returns an empty slice if no opportunities match the filter.
//
// Filter criteria (all are AND'ed together):
//   - Selected: if non-nil, filters by selection status
//   - MinScore: if > 0, includes only opportunities with score >= MinScore
//   - MaxScore: if > 0, includes only opportunities with score <= MaxScore
//
// Thread-safety: Acquires a read lock for the duration of the operation.
func (s *JSONStore) List(ctx context.Context, filter *Filter) ([]*opportunity.Opportunity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Read all JSON files from queue directory
	entries, err := os.ReadDir(s.queuePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read queue directory: %w", err)
	}

	var opportunities []*opportunity.Opportunity

	for _, entry := range entries {
		// Skip non-JSON files and directories
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		// Read and parse the file
		filePath := filepath.Join(s.queuePath, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			// Skip files that can't be read
			continue
		}

		var opp opportunity.Opportunity
		if err := json.Unmarshal(data, &opp); err != nil {
			// Skip files with invalid JSON
			continue
		}

		// Apply filter if provided
		if filter != nil && !s.matchesFilter(&opp, filter) {
			continue
		}

		opportunities = append(opportunities, &opp)
	}

	return opportunities, nil
}

// Delete removes an opportunity from the store by ID.
//
// Returns an error if:
//   - The opportunity doesn't exist
//   - The file cannot be deleted
//
// Thread-safety: Acquires a write lock for the duration of the operation.
func (s *JSONStore) Delete(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("opportunity ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := s.filePath(id)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("opportunity %s: %w", id, ErrNotFound)
	}

	// Delete file
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to delete opportunity file: %w", err)
	}

	return nil
}

// filePath returns the full file path for an opportunity ID.
func (s *JSONStore) filePath(id string) string {
	return filepath.Join(s.queuePath, id+".json")
}

// matchesFilter checks if an opportunity matches the given filter criteria.
func (s *JSONStore) matchesFilter(opp *opportunity.Opportunity, filter *Filter) bool {
	// Filter by selection status
	if filter.Selected != nil && opp.Selected != *filter.Selected {
		return false
	}

	// Filter by minimum score (only if MinScore is specified)
	if filter.MinScore > 0 && opp.Score < filter.MinScore {
		return false
	}

	// Filter by maximum score (only if MaxScore is specified)
	if filter.MaxScore > 0 && opp.Score > filter.MaxScore {
		return false
	}

	return true
}

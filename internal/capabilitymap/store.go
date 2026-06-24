package capabilitymap

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ErrNotFound is returned by Store.Load when no capability map has been built yet.
var ErrNotFound = errors.New("capability map not found")

// Store persists the per-tenant CapabilityMap. JSONStore is the file-backed
// implementation; it lives alongside the opportunity queue and the company profile in
// the same store base path, so one deployment = one tenant's map (no cross-tenant mix).
type Store interface {
	Load() (*CapabilityMap, error)
	Save(m *CapabilityMap) error
}

// mapFileName is the single per-tenant capability-map document in the store base path.
const mapFileName = "capability_map.json"

// JSONStore reads/writes the capability map as a JSON file under a base directory
// (local path or a gcsfuse-mounted bucket, matching the rest of the store layer). It is
// concurrency-safe.
type JSONStore struct {
	mu   sync.RWMutex
	path string
}

// NewJSONStore returns a store rooted at basePath, creating the directory if needed.
func NewJSONStore(basePath string) (*JSONStore, error) {
	if basePath == "" {
		return nil, fmt.Errorf("capabilitymap: store base path is required")
	}
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("capabilitymap: create store dir: %w", err)
	}
	return &JSONStore{path: filepath.Join(basePath, mapFileName)}, nil
}

// Load returns the persisted map, or ErrNotFound if none has been built yet.
func (s *JSONStore) Load() (*CapabilityMap, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("capabilitymap: read: %w", err)
	}
	var m CapabilityMap
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("capabilitymap: decode: %w", err)
	}
	return &m, nil
}

// Save writes the map atomically (temp file + rename) so a concurrent Load never sees a
// half-written document.
func (s *JSONStore) Save(m *CapabilityMap) error {
	if m == nil {
		return fmt.Errorf("capabilitymap: cannot save a nil map")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("capabilitymap: encode: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("capabilitymap: write temp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("capabilitymap: rename: %w", err)
	}
	return nil
}

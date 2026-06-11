package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// ErrProfileNotFound is returned by ProfileStore.Load when no tenant profile has
// been saved yet. Callers should use errors.Is(err, profile.ErrProfileNotFound)
// to distinguish "deployment not yet onboarded" from an infrastructure failure —
// the GET /api/v1/profile handler maps it to a 404, and ResolveProfileWithStore
// maps it to "fall through to the configured file path".
var ErrProfileNotFound = errors.New("profile not found")

// ProfileStore persists and retrieves a single tenant's CapabilityProfile at
// runtime so onboarding can configure a deployment WITHOUT editing files baked
// into the image. It is the WS-C seam ResolveProfileWithStore consults first.
//
// It is deliberately a one-profile store (Load/Save take no key): a Kaimi
// deployment is single-tenant today, so the active company profile is global.
// Multi-tenancy (a key per tenant) is a future change behind this interface; the
// JSON-file implementation mirrors internal/store's JSON-on-disk pattern so it can
// be swapped for a GCS/Firestore implementation later without touching callers.
type ProfileStore interface { //nolint:revive // name is intentionally ProfileStore (not Store) to read clearly at call sites in other packages — profile.ProfileStore states the domain, and the WS-C ticket fixes this name (mirrors httpapi.Deps.ProfileStore).
	// Load returns the persisted tenant profile, or ErrProfileNotFound (wrapped)
	// if none has been saved yet.
	Load() (*CapabilityProfile, error)

	// Save persists the profile, overwriting any previously saved one. It rejects
	// a nil profile.
	Save(p *CapabilityProfile) error
}

// ProfileFileName is the fixed file name the JSON profile store writes under the
// store base path (e.g. <basePath>/profile.json). It is distinct from the
// opportunity queue/ directory the opportunity store uses, so the two live side by
// side under one base path without colliding.
const ProfileFileName = "profile.json"

// JSONProfileStore is the JSON-file implementation of ProfileStore. It stores the
// active tenant profile as a single JSON document at <basePath>/profile.json,
// mirroring how internal/store/json.go persists opportunities so it can be swapped
// for a GCS/Firestore implementation in the same way.
//
// Thread-safety: operations are guarded by a RWMutex so concurrent API reads and a
// PUT write are safe.
type JSONProfileStore struct {
	path string       // full path to the profile.json file
	mu   sync.RWMutex // protects concurrent access to the file
}

// NewJSONProfileStore creates a JSON file-backed ProfileStore rooted at basePath.
// It creates basePath if it does not exist (matching NewJSONStore) and writes the
// profile to <basePath>/profile.json. basePath must be the same store base path
// the opportunity store uses, so both persist under one directory.
func NewJSONProfileStore(basePath string) (*JSONProfileStore, error) {
	info, err := os.Stat(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			if mkErr := os.MkdirAll(basePath, 0o755); mkErr != nil {
				return nil, fmt.Errorf("create profile store base directory %q: %w", basePath, mkErr)
			}
		} else {
			return nil, fmt.Errorf("stat profile store base path %q: %w", basePath, err)
		}
	} else if !info.IsDir() {
		return nil, fmt.Errorf("profile store base path %q is not a directory", basePath)
	}

	return &JSONProfileStore{path: filepath.Join(basePath, ProfileFileName)}, nil
}

// Load reads the persisted profile from disk. It returns ErrProfileNotFound
// (wrapped) when the file does not exist yet, and a wrapped error for a read or
// parse failure.
func (s *JSONProfileStore) Load() (*CapabilityProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("load profile %q: %w", s.path, ErrProfileNotFound)
		}
		return nil, fmt.Errorf("read profile file %q: %w", s.path, err)
	}

	var p CapabilityProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse profile JSON %q: %w", s.path, err)
	}
	return &p, nil
}

// Save persists the profile as indented JSON, overwriting any prior profile. It
// rejects a nil profile rather than writing a null document that would later load
// as an empty profile.
func (s *JSONProfileStore) Save(p *CapabilityProfile) error {
	if p == nil {
		return fmt.Errorf("profile cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("write profile file %q: %w", s.path, err)
	}
	return nil
}

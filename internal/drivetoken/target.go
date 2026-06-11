package drivetoken

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Target identifies WHERE in the customer's Drive generated Docs should be created
// — a Shared Drive id or a folder id. The full interactive Drive picker is WS-C3;
// WS-C2 just persists a target id once it is provided so the Docs client has a
// parent to create files under.
type Target struct {
	// DriveID is the Shared Drive or folder id new Docs are created in. It maps
	// directly to googledocs.Config.SharedDriveID.
	DriveID string `json:"drive_id"`
}

// TargetStore persists and retrieves the connected deployment's Drive target.
type TargetStore interface {
	// Load returns the persisted target, or ErrNotConnected (wrapped) when none has
	// been set yet.
	Load() (Target, error)
	// Save persists the target, overwriting any prior one. It rejects an empty id.
	Save(t Target) error
}

// TargetFileName is the fixed file name the JSON target store writes under the
// store base path (e.g. <basePath>/drive_target.json).
const TargetFileName = "drive_target.json"

// JSONTargetStore is the JSON-file implementation of TargetStore. The target id is
// not a secret, so it uses the same 0o644 perms and atomic-rename pattern as the
// profile store (the token, which IS a secret, lives in the 0o600 JSONTokenStore).
type JSONTargetStore struct {
	path string
	mu   sync.RWMutex
}

// NewJSONTargetStore creates a JSON file-backed TargetStore rooted at basePath,
// creating basePath if absent and writing to <basePath>/drive_target.json.
func NewJSONTargetStore(basePath string) (*JSONTargetStore, error) {
	info, err := os.Stat(basePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if mkErr := os.MkdirAll(basePath, 0o755); mkErr != nil {
				return nil, fmt.Errorf("create drive target store base directory %q: %w", basePath, mkErr)
			}
		} else {
			return nil, fmt.Errorf("stat drive target store base path %q: %w", basePath, err)
		}
	} else if !info.IsDir() {
		return nil, fmt.Errorf("drive target store base path %q is not a directory", basePath)
	}
	return &JSONTargetStore{path: filepath.Join(basePath, TargetFileName)}, nil
}

// Load reads the persisted target. It returns ErrNotConnected (wrapped) when no
// target has been set yet, mirroring the token store's "not connected" sentinel so
// callers branch on the same error.
func (s *JSONTargetStore) Load() (Target, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Target{}, fmt.Errorf("load drive target: %w", ErrNotConnected)
		}
		return Target{}, fmt.Errorf("read drive target file %q: %w", s.path, err)
	}
	var t Target
	if err := json.Unmarshal(data, &t); err != nil {
		return Target{}, fmt.Errorf("parse drive target JSON %q: %w", s.path, err)
	}
	return t, nil
}

// Save persists the target atomically (temp file + rename), overwriting any prior
// one. It rejects a blank Drive id.
func (s *JSONTargetStore) Save(t Target) error {
	if strings.TrimSpace(t.DriveID) == "" {
		return fmt.Errorf("drive target id cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal drive target: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp drive target file %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp drive target %q to %q: %w", tmpPath, s.path, err)
	}
	return nil
}

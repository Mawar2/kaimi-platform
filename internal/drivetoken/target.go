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
//
// It uses os.CreateTemp for the same race-safety and consistency as the token store
// (a uniquely-named O_EXCL temp in the same directory, so no fixed-name pre-create
// race and no stale-leftover collision). The target id is NOT a secret, so after the
// rename the file is chmod'd to 0o644 to match the profile store (os.CreateTemp makes
// it 0o600 by default).
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

	// Create the temp file in the same directory so the rename stays on one
	// filesystem and is atomic. os.CreateTemp gives a unique, O_EXCL-created name.
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, "drive_target-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp drive target file in %q: %w", dir, err)
	}
	// Capture the name immediately so cleanup works even on a partial failure below.
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp drive target file %q: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp drive target file %q: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename temp drive target to %q: %w", s.path, err)
	}
	// The target is not a secret; match the profile store's 0o644 (os.CreateTemp
	// defaults to 0o600, which the rename carried over to the destination).
	if err := os.Chmod(s.path, 0o644); err != nil {
		return fmt.Errorf("set drive target file permissions %q: %w", s.path, err)
	}
	return nil
}

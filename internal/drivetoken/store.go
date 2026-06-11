package drivetoken

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/oauth2"
)

// ErrNotConnected is returned by TokenStore.Load when the customer's Drive has not
// been connected yet (no token persisted). Callers use errors.Is(err,
// drivetoken.ErrNotConnected) to distinguish "not connected" from an I/O failure —
// the status endpoint maps it to {connected:false}, and the proposal wiring maps it
// to "fall back to the service-account/ADC Docs client".
var ErrNotConnected = errors.New("customer Drive not connected")

// TokenStore persists and retrieves a single deployment's customer-Drive OAuth
// token. It is deliberately a one-token store (Load/Save take no key): a Kaimi
// deployment is single-tenant today, so the connected Drive account is global.
// Multi-tenancy (a key per tenant) is a future change behind this interface.
type TokenStore interface {
	// Load returns the persisted token, or ErrNotConnected (wrapped) if the Drive
	// has not been connected yet.
	Load() (*oauth2.Token, error)

	// Save persists the token (access + refresh), overwriting any prior token. It
	// rejects a nil token.
	Save(tok *oauth2.Token) error
}

// TokenFileName is the fixed file name the JSON token store writes under the store
// base path (e.g. <basePath>/drive_token.json). It is distinct from the profile
// and opportunity files so all three live side by side under one base path.
const TokenFileName = "drive_token.json"

// tokenFilePerm is the permission mode for the token file. Tokens are SECRETS, so
// this is owner-only (0o600) — stricter than the profile file's 0o644.
const tokenFilePerm fs.FileMode = 0o600

// JSONTokenStore is the JSON-file implementation of TokenStore. It stores the
// customer-Drive OAuth token as a single JSON document at
// <basePath>/drive_token.json with owner-only permissions, mirroring how
// internal/profile.JSONProfileStore persists the company profile so it can be
// swapped for a Secret Manager/GCS implementation later.
//
// Thread-safety: operations are guarded by a RWMutex so a concurrent status read
// and a callback write are safe.
type JSONTokenStore struct {
	path string
	mu   sync.RWMutex
}

// NewJSONTokenStore creates a JSON file-backed TokenStore rooted at basePath. It
// creates basePath if it does not exist (matching the profile store) and writes the
// token to <basePath>/drive_token.json. basePath must be the same store base path
// the opportunity and profile stores use, so all persist under one directory.
func NewJSONTokenStore(basePath string) (*JSONTokenStore, error) {
	info, err := os.Stat(basePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if mkErr := os.MkdirAll(basePath, 0o755); mkErr != nil {
				return nil, fmt.Errorf("create drive token store base directory %q: %w", basePath, mkErr)
			}
		} else {
			return nil, fmt.Errorf("stat drive token store base path %q: %w", basePath, err)
		}
	} else if !info.IsDir() {
		return nil, fmt.Errorf("drive token store base path %q is not a directory", basePath)
	}

	return &JSONTokenStore{path: filepath.Join(basePath, TokenFileName)}, nil
}

// Load reads the persisted token from disk. It returns ErrNotConnected (wrapped)
// when the file does not exist yet, and a wrapped error for a read or parse
// failure. It never logs the token.
func (s *JSONTokenStore) Load() (*oauth2.Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("load drive token: %w", ErrNotConnected)
		}
		// Do not include the file contents; only the path and the I/O error.
		return nil, fmt.Errorf("read drive token file %q: %w", s.path, err)
	}

	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		// Never echo the raw bytes (they contain the secret) — only that parsing failed.
		return nil, fmt.Errorf("parse drive token JSON %q: %w", s.path, err)
	}
	return &tok, nil
}

// Save persists the token as JSON with owner-only permissions, overwriting any
// prior token. It rejects a nil token rather than writing a null document that
// would later load as an empty, unusable token.
//
// The write is atomic: it writes to a temp file in the SAME directory (created with
// 0o600 so the secret is never briefly world-readable) and then os.Renames it over
// the destination. os.Rename replaces the destination atomically on both Unix and
// Windows, so a crash mid-write leaves the old token intact rather than a truncated
// one. It never logs the token.
func (s *JSONTokenStore) Save(tok *oauth2.Token) error {
	if tok == nil {
		return fmt.Errorf("drive token cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal drive token: %w", err)
	}

	// Write to a temp file in the same directory so the rename stays on the same
	// filesystem (cross-device renames are not atomic and would fail). Create it
	// with 0o600 directly so the secret is never momentarily readable by others.
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, tokenFilePerm); err != nil {
		_ = os.Remove(tmpPath) // Best-effort cleanup; the temp file may or may not exist.
		return fmt.Errorf("write temp drive token file %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath) // Best-effort cleanup of the leftover temp file.
		return fmt.Errorf("rename temp drive token %q to %q: %w", tmpPath, s.path, err)
	}
	// os.WriteFile honors the create perm only when the file does not already exist;
	// on overwrite the existing mode is kept. Enforce 0o600 explicitly so a token
	// file that predated this code (or was created with a looser umask) is tightened.
	if err := os.Chmod(s.path, tokenFilePerm); err != nil {
		return fmt.Errorf("set drive token file permissions %q: %w", s.path, err)
	}
	return nil
}

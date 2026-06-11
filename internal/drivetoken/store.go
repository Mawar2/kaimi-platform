package drivetoken

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/oauth2"
)

// chmodFile is the seam used by the atomic Save paths to set file permissions. It
// defaults to os.Chmod and is a package var only so tests can inject a failing
// chmod (to prove a permission error from the underlying filesystem does NOT abort
// the write). Production always uses os.Chmod.
var chmodFile = os.Chmod

// bestEffortChmod attempts to set perm on path and, on failure, logs a warning and
// returns WITHOUT propagating the error. POSIX permission bits are defense-in-depth
// here, not the security boundary: os.CreateTemp already opens the temp file at
// 0o600, and on object-store FUSE mounts (gcsfuse) chmod returns EPERM because the
// filesystem does not model POSIX modes — there the boundary is bucket IAM (uniform
// bucket-level access plus a service-account-only objectAdmin binding). Treating
// that EPERM as fatal would wrongly abort the write on exactly those deployments
// (the Drive connect "failed to store drive connection" failure on Cloud Run +
// gcsfuse), so we tighten perms where supported and continue where not.
func bestEffortChmod(path string, perm fs.FileMode) {
	if err := chmodFile(path, perm); err != nil {
		log.Printf("drivetoken: best-effort chmod of %q to %o failed; continuing (on gcsfuse/object-store mounts POSIX modes are unsupported and bucket IAM is the security boundary): %v", path, perm, err)
	}
}

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
// The write is atomic and race-free: it creates a UNIQUELY-named temp file in the
// SAME directory via os.CreateTemp (which opens with O_EXCL at mode 0o600), writes
// the secret into it, and then os.Renames it over the destination. Using
// os.CreateTemp instead of a fixed "<path>.tmp" name closes a CWE-377 insecure
// temp-file race: a fixed name lets an attacker pre-create a world-readable file
// that os.WriteFile would truncate-and-write into WITHOUT fixing its perms, briefly
// exposing the token. A unique O_EXCL create also can never collide with a stale
// leftover temp from a prior crash. os.Rename replaces the destination atomically on
// both Unix and Windows, so a crash mid-write leaves the old token intact rather
// than a truncated one. On any failure the temp file is removed best-effort and the
// error is wrapped WITHOUT the token bytes. It never logs the token.
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

	// Create the temp file in the same directory as the destination so the rename
	// stays on one filesystem (cross-device renames are not atomic and would fail).
	// os.CreateTemp creates a uniquely-named file with O_EXCL at mode 0o600, so the
	// secret is never momentarily readable by others and there is no pre-create race.
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, "drive_token-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp drive token file in %q: %w", dir, err)
	}
	// Capture the name immediately so cleanup works even on a partial failure below.
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName) // Best-effort cleanup of the partial temp file.
		// Do not include the data; only the path and the I/O error.
		return fmt.Errorf("write temp drive token file %q: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp drive token file %q: %w", tmpName, err)
	}
	// Defense-in-depth: re-assert 0o600 explicitly in case an unusual umask or a
	// future os.CreateTemp default ever produced a looser mode. Best-effort: a chmod
	// failure (e.g. EPERM on a gcsfuse mount) must NOT abort the write — see
	// bestEffortChmod.
	bestEffortChmod(tmpName, tokenFilePerm)
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName) // Best-effort cleanup of the leftover temp file.
		return fmt.Errorf("rename temp drive token to %q: %w", s.path, err)
	}
	// os.Rename preserves the source file's mode, so the destination is already
	// 0o600. Re-assert it anyway so a token file that predated this code (or was
	// created with a looser umask before the rename overwrote it) is tightened —
	// again best-effort, so it never blocks the write on an object-store mount.
	bestEffortChmod(s.path, tokenFilePerm)
	return nil
}

package drivetoken

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// newTestStore creates a JSONTokenStore rooted at a fresh temp dir.
func newTestStore(t *testing.T) (store *JSONTokenStore, dir string) {
	t.Helper()
	dir = t.TempDir()
	store, err := NewJSONTokenStore(dir)
	if err != nil {
		t.Fatalf("NewJSONTokenStore: %v", err)
	}
	return store, dir
}

// TestSaveLoadRoundTrip verifies a saved token loads back with its fields intact,
// including the refresh token (the field that lets the TokenSource auto-refresh).
func TestSaveLoadRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)

	want := &oauth2.Token{
		AccessToken:  "access-abc",
		RefreshToken: "refresh-xyz",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour).Truncate(time.Second),
	}
	if err := s.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.AccessToken != want.AccessToken {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, want.AccessToken)
	}
	if got.RefreshToken != want.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", got.RefreshToken, want.RefreshToken)
	}
	if got.TokenType != want.TokenType {
		t.Errorf("TokenType = %q, want %q", got.TokenType, want.TokenType)
	}
	if !got.Expiry.Equal(want.Expiry) {
		t.Errorf("Expiry = %v, want %v", got.Expiry, want.Expiry)
	}
}

// TestLoadNotConnected verifies Load returns ErrNotConnected (wrapped) before any
// token has been saved, so callers can distinguish "not connected yet" from a real
// I/O failure.
func TestLoadNotConnected(t *testing.T) {
	s, _ := newTestStore(t)

	_, err := s.Load()
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Load on empty store: got %v, want ErrNotConnected", err)
	}
}

// TestSaveRejectsNil verifies Save will not persist a nil token (which would later
// load as a useless empty token).
func TestSaveRejectsNil(t *testing.T) {
	s, _ := newTestStore(t)
	if err := s.Save(nil); err == nil {
		t.Fatal("Save(nil): expected an error, got nil")
	}
}

// TestSaveIsAtomicNoLeftoverTmp verifies the write leaves no .tmp file behind, so
// a crash mid-write cannot strand a partial token file next to the real one.
func TestSaveIsAtomicNoLeftoverTmp(t *testing.T) {
	s, dir := newTestStore(t)

	if err := s.Save(&oauth2.Token{AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover temp file after Save: %s", e.Name())
		}
	}
}

// TestSaveFilePerms0600 verifies the token file is written with owner-only perms:
// tokens are secrets, so they must be stricter than the 0644 profile file. Skipped
// on Windows, where Unix permission bits are not meaningfully enforced.
func TestSaveFilePerms0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permission bits are not enforced on Windows")
	}
	s, dir := newTestStore(t)

	if err := s.Save(&oauth2.Token{AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, TokenFileName))
	if err != nil {
		t.Fatalf("Stat token file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file perms = %o, want 0600", perm)
	}
}

// TestSaveIgnoresStaleLeftoverTmp verifies a pre-existing leftover temp file (from a
// prior crash, with a different random name) does not break Save. Because Save now
// uses os.CreateTemp's unique O_EXCL naming, it can never collide with a stale temp,
// and the final token file is written with owner-only (0600) perms regardless.
func TestSaveIgnoresStaleLeftoverTmp(t *testing.T) {
	s, dir := newTestStore(t)

	// Plant a stale leftover temp matching the Save temp pattern but with a
	// different random suffix than any new temp will get.
	stale := filepath.Join(dir, "drive_token-stale123.tmp")
	if err := os.WriteFile(stale, []byte("garbage"), 0o600); err != nil {
		t.Fatalf("plant stale temp: %v", err)
	}

	if err := s.Save(&oauth2.Token{AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatalf("Save with stale temp present: %v", err)
	}

	// The real token file must exist and round-trip.
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.AccessToken != "a" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "a")
	}

	// The final token file must be owner-only. (Skipped on Windows, where Unix
	// permission bits are not meaningfully enforced.)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, TokenFileName))
		if err != nil {
			t.Fatalf("Stat token file: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("token file perms = %o, want 0600", perm)
		}
	}

	// Save must not have removed the unrelated stale temp (it owns only its own
	// uniquely-named temp), and it must not have left its OWN temp behind.
	if _, err := os.Stat(stale); err != nil {
		t.Errorf("Save unexpectedly disturbed the unrelated stale temp: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "drive_token-stale123.tmp" && filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("Save left its own temp file behind: %s", e.Name())
		}
	}
}

// TestSaveOverwrites verifies a second Save replaces the first token rather than
// appending or failing.
func TestSaveOverwrites(t *testing.T) {
	s, _ := newTestStore(t)

	if err := s.Save(&oauth2.Token{AccessToken: "first", RefreshToken: "r1"}); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if err := s.Save(&oauth2.Token{AccessToken: "second", RefreshToken: "r2"}); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.AccessToken != "second" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "second")
	}
}

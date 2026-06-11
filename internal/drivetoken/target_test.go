package drivetoken

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// farFuture returns an expiry well in the future so a stored access token is
// treated as still valid (used by the token-source test to stay offline).
func farFuture() time.Time { return time.Now().Add(24 * time.Hour) }

// TestTargetSaveLoadRoundTrip verifies a saved target Drive id loads back intact.
func TestTargetSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONTargetStore(dir)
	if err != nil {
		t.Fatalf("NewJSONTargetStore: %v", err)
	}

	want := Target{DriveID: "0ABCDEF_shared_drive"}
	if err := s.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.DriveID != want.DriveID {
		t.Errorf("DriveID = %q, want %q", got.DriveID, want.DriveID)
	}
}

// TestTargetLoadNotSet verifies Load returns ErrNotConnected (reused as the
// "not set" sentinel) before any target has been saved.
func TestTargetLoadNotSet(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONTargetStore(dir)
	if err != nil {
		t.Fatalf("NewJSONTargetStore: %v", err)
	}

	_, err = s.Load()
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Load on empty target store: got %v, want ErrNotConnected", err)
	}
}

// TestTargetSaveRejectsEmpty verifies an empty Drive id is rejected (an empty
// target would produce no usable parent for created Docs).
func TestTargetSaveRejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONTargetStore(dir)
	if err != nil {
		t.Fatalf("NewJSONTargetStore: %v", err)
	}
	if err := s.Save(Target{DriveID: "  "}); err == nil {
		t.Fatal("Save(blank target): expected an error, got nil")
	}
}

// TestTargetSaveAtomicNoLeftoverTmp verifies the target write leaves no .tmp file.
func TestTargetSaveAtomicNoLeftoverTmp(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONTargetStore(dir)
	if err != nil {
		t.Fatalf("NewJSONTargetStore: %v", err)
	}
	if err := s.Save(Target{DriveID: "drive-1"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover temp file after target Save: %s", e.Name())
		}
	}
}

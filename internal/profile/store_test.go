package profile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// sampleProfile returns a minimal-but-valid CapabilityProfile for round-trip
// tests: a company name plus one primary NAICS code is enough to exercise
// Save/Load without depending on any fixture file.
func sampleProfile() *CapabilityProfile {
	return &CapabilityProfile{
		Company: "Round Trip Co",
		UEI:     "ABC123DEF456",
		NAICSCodes: []NAICSCode{
			{Code: "541512", Description: "Computer Systems Design", Tier: TierPrimary},
		},
		SetAside:     SetAsideStatus{SmallBusiness: true},
		Competencies: []string{"cloud migration"},
	}
}

// TestJSONProfileStore_SaveLoadRoundTrip verifies a saved profile loads back
// equal across the fields that matter, and that the file lands at the documented
// <basePath>/profile.json location.
func TestJSONProfileStore_SaveLoadRoundTrip(t *testing.T) {
	base := t.TempDir()
	ps, err := NewJSONProfileStore(base)
	if err != nil {
		t.Fatalf("NewJSONProfileStore: %v", err)
	}

	want := sampleProfile()
	if err := ps.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// The profile must persist at the known, store-derived location so it swaps to
	// GCS later the same way the opportunity store does.
	if _, err := os.Stat(filepath.Join(base, "profile.json")); err != nil {
		t.Fatalf("expected profile.json at base path: %v", err)
	}

	got, err := ps.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Company != want.Company {
		t.Errorf("Company = %q, want %q", got.Company, want.Company)
	}
	if got.UEI != want.UEI {
		t.Errorf("UEI = %q, want %q", got.UEI, want.UEI)
	}
	if len(got.NAICSCodes) != 1 || got.NAICSCodes[0].Code != "541512" {
		t.Errorf("NAICSCodes = %+v, want one 541512 entry", got.NAICSCodes)
	}
	if !got.SetAside.SmallBusiness {
		t.Error("SetAside.SmallBusiness = false, want true (round-trip lost it)")
	}
}

// TestJSONProfileStore_LoadNotFound verifies Load returns the ErrProfileNotFound
// sentinel (matchable with errors.Is) when no profile has been saved yet, so
// callers can distinguish "not configured" from an infrastructure failure.
func TestJSONProfileStore_LoadNotFound(t *testing.T) {
	ps, err := NewJSONProfileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONProfileStore: %v", err)
	}

	got, err := ps.Load()
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("Load on empty store = (%v, %v); want ErrProfileNotFound", got, err)
	}
	if got != nil {
		t.Errorf("Load returned a non-nil profile (%+v) with ErrProfileNotFound", got)
	}
}

// TestJSONProfileStore_SaveOverwrites verifies a second Save replaces the first,
// so onboarding can re-submit a corrected profile.
func TestJSONProfileStore_SaveOverwrites(t *testing.T) {
	ps, err := NewJSONProfileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONProfileStore: %v", err)
	}

	first := sampleProfile()
	if err := ps.Save(first); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	second := sampleProfile()
	second.Company = "Updated Co"
	if err := ps.Save(second); err != nil {
		t.Fatalf("Save second: %v", err)
	}

	got, err := ps.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Company != "Updated Co" {
		t.Errorf("Company = %q, want the overwritten %q", got.Company, "Updated Co")
	}
}

// TestJSONProfileStore_SaveNil rejects a nil profile rather than writing a null
// document that would later load as an empty profile.
func TestJSONProfileStore_SaveNil(t *testing.T) {
	ps, err := NewJSONProfileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONProfileStore: %v", err)
	}
	if err := ps.Save(nil); err == nil {
		t.Fatal("Save(nil) = nil error; want a rejection")
	}
}

package productkey

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"
)

var keyRE = regexp.MustCompile(`^KAIMI-[2-9A-HJ-NP-Z]{4}-[2-9A-HJ-NP-Z]{4}-[2-9A-HJ-NP-Z]{4}$`)

// TestGenerateKeyFormat: keys match KAIMI-XXXX-XXXX-XXXX over the unambiguous
// alphabet (no 0/O/1/I/L) and are distinct across calls.
func TestGenerateKeyFormat(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		k, err := GenerateKey()
		if err != nil {
			t.Fatalf("GenerateKey: %v", err)
		}
		if !keyRE.MatchString(k) {
			t.Fatalf("key %q does not match KAIMI-XXXX-XXXX-XXXX over the unambiguous alphabet", k)
		}
		if seen[k] {
			t.Fatalf("duplicate key generated: %q", k)
		}
		seen[k] = true
	}
}

// TestRecordValid: a record grants access only when un-revoked AND before expiry.
func TestRecordValid(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		expiresAt time.Time
		revoked   bool
		want      bool
	}{
		{"active", now.Add(time.Hour), false, true},
		{"expired", now.Add(-time.Hour), false, false},
		{"revoked", now.Add(time.Hour), true, false},
		{"revoked and expired", now.Add(-time.Hour), true, false},
		{"expires exactly now (boundary = not valid)", now, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Record{ExpiresAt: c.expiresAt, Revoked: c.revoked}
			if got := r.Valid(now); got != c.want {
				t.Errorf("Valid = %v, want %v", got, c.want)
			}
		})
	}
}

// TestNormalize: separators/case/spaces all canonicalize to the stored form.
func TestNormalize(t *testing.T) {
	canonical := "KAIMI-7F3A-9C2E-B1D4"
	for _, in := range []string{
		"KAIMI-7F3A-9C2E-B1D4",
		"kaimi-7f3a-9c2e-b1d4",
		"kaimi 7f3a 9c2e b1d4",
		"  KAIMI7F3A9C2EB1D4  ",
	} {
		if got := Normalize(in); got != canonical {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, canonical)
		}
	}
}

// TestMemoryRegistry_MintLookupValid: a freshly minted key looks up and is valid;
// an unknown key is ErrNotFound.
func TestMemoryRegistry_MintLookupValid(t *testing.T) {
	r := NewMemoryRegistry()
	ctx := context.Background()

	rec, err := r.Mint(ctx, "Ey3 Technologies", 14*24*time.Hour)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if !keyRE.MatchString(rec.Key) || rec.Tester != "Ey3 Technologies" {
		t.Fatalf("unexpected record: %+v", rec)
	}

	got, err := r.Lookup(ctx, rec.Key)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !got.Valid(time.Now()) {
		t.Errorf("freshly minted 14d key should be valid now")
	}

	// Lookup tolerates a re-typed key (lowercase, spaces).
	if _, err := r.Lookup(ctx, "  "+rec.Key+"  "); err != nil {
		t.Errorf("Lookup of re-typed key: %v", err)
	}

	if _, err := r.Lookup(ctx, "KAIMI-AAAA-BBBB-CCCC"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Lookup unknown key: got %v, want ErrNotFound", err)
	}
}

// TestMemoryRegistry_Revoke: revoke makes a key invalid; revoking unknown errors.
func TestMemoryRegistry_Revoke(t *testing.T) {
	r := NewMemoryRegistry()
	ctx := context.Background()
	rec, _ := r.Mint(ctx, "T", time.Hour)

	if err := r.Revoke(ctx, rec.Key); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	got, _ := r.Lookup(ctx, rec.Key)
	if got.Valid(time.Now()) {
		t.Errorf("revoked key must not be valid")
	}
	if err := r.Revoke(ctx, "KAIMI-ZZZZ-ZZZZ-ZZZZ"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Revoke unknown: got %v, want ErrNotFound", err)
	}
}

// TestMemoryRegistry_Expiry: a key past its window is invalid (clock injected).
func TestMemoryRegistry_Expiry(t *testing.T) {
	r := NewMemoryRegistry()
	base := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return base }
	ctx := context.Background()

	rec, _ := r.Mint(ctx, "T", 7*24*time.Hour) // expires base+7d

	if !rec.Valid(base.Add(6 * 24 * time.Hour)) {
		t.Errorf("key should be valid on day 6")
	}
	if rec.Valid(base.Add(8 * 24 * time.Hour)) {
		t.Errorf("key should be expired on day 8")
	}
}

// TestMemoryRegistry_List: List returns every minted key.
func TestMemoryRegistry_List(t *testing.T) {
	r := NewMemoryRegistry()
	ctx := context.Background()
	for range 3 {
		if _, err := r.Mint(ctx, "T", time.Hour); err != nil {
			t.Fatalf("Mint: %v", err)
		}
	}
	all, err := r.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("List returned %d, want 3", len(all))
	}
}

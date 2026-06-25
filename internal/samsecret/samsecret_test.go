package samsecret

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestValidateKey covers the format guard: a realistic 40-char alnum key passes;
// empty, too-short, too-long, and punctuation/space inputs are rejected as ErrInvalidKey.
func TestValidateKey(t *testing.T) {
	valid := strings.Repeat("a1B2c3D4", 5) // 40 alphanumeric chars
	cases := []struct {
		name string
		key  string
		ok   bool
	}{
		{"valid 40-char", valid, true},
		{"valid trimmed", "  " + valid + "  ", true},
		{"valid with hyphens (real SAM format)", valid[:8] + "-" + valid[9:20] + "-" + valid[21:], true},
		{"valid with underscore/dot", valid[:10] + "_" + valid[11:30] + "." + valid[31:], true},
		{"empty", "", false},
		{"whitespace only", "    ", false},
		{"too short", "abc123", false},
		{"too long", strings.Repeat("a", maxKeyLen+1), false},
		{"contains space", valid[:20] + " " + valid[21:], false},
		{"contains slash", valid[:20] + "/" + valid[21:], false},
		{"looks like a url", "https://api.sam.gov/key/abcdefghijklmno", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateKey(c.key)
			if c.ok && err != nil {
				t.Errorf("ValidateKey(%q) = %v, want nil", c.key, err)
			}
			if !c.ok {
				if err == nil {
					t.Errorf("ValidateKey(%q) = nil, want error", c.key)
				} else if !errors.Is(err, ErrInvalidKey) {
					t.Errorf("ValidateKey(%q) error = %v, want wraps ErrInvalidKey", c.key, err)
				}
			}
		})
	}
}

// TestMemoryWriterSave: a valid key is recorded and counted; an invalid key is
// rejected and nothing is recorded.
func TestMemoryWriterSave(t *testing.T) {
	w := NewMemoryWriter()
	ctx := context.Background()
	valid := strings.Repeat("Z9y8X7w6", 5)

	if err := w.Save(ctx, valid); err != nil {
		t.Fatalf("Save valid: %v", err)
	}
	if w.Versions() != 1 || w.Last() != valid {
		t.Errorf("after save: versions=%d last=%q, want 1 / %q", w.Versions(), w.Last(), valid)
	}

	if err := w.Save(ctx, "bad key!"); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("Save invalid: got %v, want ErrInvalidKey", err)
	}
	if w.Versions() != 1 {
		t.Errorf("invalid save must not record a version; versions=%d", w.Versions())
	}
}

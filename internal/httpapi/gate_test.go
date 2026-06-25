package httpapi

import (
	"errors"
	"testing"
)

// TestLoadGateMode covers the env-driven gate selection: default, explicit values,
// case/separator tolerance, and that an unrecognized value fails loudly (never falls
// open to a more permissive default).
func TestLoadGateMode(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		set     bool
		want    GateMode
		wantErr bool
	}{
		{"unset defaults to workspace-oauth", "", false, GateModeWorkspaceOAuth, false},
		{"empty defaults to workspace-oauth", "", true, GateModeWorkspaceOAuth, false},
		{"explicit workspace-oauth", "workspace-oauth", true, GateModeWorkspaceOAuth, false},
		{"explicit product-key", "product-key", true, GateModeProductKey, false},
		{"product_key underscore tolerated", "product_key", true, GateModeProductKey, false},
		{"mixed case tolerated", "Product-Key", true, GateModeProductKey, false},
		{"whitespace trimmed", "  product-key  ", true, GateModeProductKey, false},
		{"unknown value errors", "open", true, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.set {
				t.Setenv(envGateMode, c.env)
			} else {
				// Ensure the var is absent for the default case.
				t.Setenv(envGateMode, "")
			}
			got, err := LoadGateMode()
			if c.wantErr {
				if err == nil {
					t.Fatalf("LoadGateMode(%q) = %q, want error", c.env, got)
				}
				if !errors.Is(err, ErrInvalidConfig) {
					t.Errorf("error = %v, want wraps ErrInvalidConfig", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadGateMode(%q): unexpected error %v", c.env, err)
			}
			if got != c.want {
				t.Errorf("LoadGateMode(%q) = %q, want %q", c.env, got, c.want)
			}
		})
	}
}

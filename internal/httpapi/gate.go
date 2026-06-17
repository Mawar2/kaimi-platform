package httpapi

import (
	"fmt"
	"os"
	"strings"
)

// This file defines how a deployment decides WHO may reach the app: the access
// "gate mode". A single Kaimi deployment serves exactly one tenant, so it runs
// exactly one gate:
//
//   - workspace-oauth: the WS-B4 Google Workspace sign-in (any verified account in
//     the tenant's domain). The default; this is how BlueMeta's own deployment runs.
//   - product-key:     a time-limited KAIMI-XXXX-XXXX-XXXX product key (internal/
//     productkey) — the pilot access model. A tester clicks a magic link (the key in
//     the URL) or types the key, and a valid, unexpired, un-revoked key grants a
//     session for the rest of its window. Google OAuth is used ONLY for connecting a
//     customer Drive in this mode, never for sign-in.
//
// The mode is chosen by the operator at deploy time (KAIMI_GATE_MODE) so the same
// binary serves both BlueMeta and a pilot tenant without a rebuild.

// GateMode names the access-control model a deployment enforces.
type GateMode string

const (
	// GateModeWorkspaceOAuth gates on Google Workspace sign-in (the default).
	GateModeWorkspaceOAuth GateMode = "workspace-oauth"
	// GateModeProductKey gates on a time-limited product key (pilot access).
	GateModeProductKey GateMode = "product-key"
)

// envGateMode selects the access gate. Unset (or "workspace-oauth") keeps the
// established Workspace-OAuth behavior; "product-key" switches to the pilot gate.
const envGateMode = "KAIMI_GATE_MODE"

// LoadGateMode resolves the access gate mode from KAIMI_GATE_MODE, defaulting to
// workspace-oauth so an existing deployment's behavior is unchanged when the variable
// is absent. The value is matched case-insensitively and trimmed; both "product-key"
// and "product_key" are accepted so a hyphen/underscore typo does not silently fall
// back to the wrong (more permissive) mode. An unrecognized non-empty value is an
// error wrapping ErrInvalidConfig rather than a silent default — a misconfigured gate
// must fail loudly, never fail open.
func LoadGateMode() (GateMode, error) {
	raw := strings.TrimSpace(os.Getenv(envGateMode))
	switch strings.ToLower(raw) {
	case "", string(GateModeWorkspaceOAuth):
		return GateModeWorkspaceOAuth, nil
	case string(GateModeProductKey), "product_key":
		return GateModeProductKey, nil
	default:
		return "", fmt.Errorf("%s=%q is not a valid gate mode (want %q or %q): %w",
			envGateMode, raw, GateModeWorkspaceOAuth, GateModeProductKey, ErrInvalidConfig)
	}
}

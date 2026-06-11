package profile

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
)

// ExampleProfilePath is the repo-relative path to the generic, neutral company
// profile template that ships inside the product image. It carries NO real
// customer data and is used only as the explicit, logged fallback when no real
// profile is present (see ResolveProfile).
const ExampleProfilePath = "config/profile.example.yaml"

// ExampleCompanyName is the placeholder company name in the example template.
// Callers (and tests) use it to detect when the example fallback is in effect.
const ExampleCompanyName = "Example Federal Co"

// ResolveProfile resolves the active company profile at runtime so the shipped
// product image can carry no real customer data.
//
// Behavior:
//   - If a profile file exists at path, it loads and returns THAT profile and
//     reports path as the source — identical to a direct LoadProfile(path) call,
//     with no warning. This is the existing-deployment path (config/profile.json).
//   - If no file exists at path, it loads the generic ExampleProfilePath template
//     instead, reports the example path as the source, and logs an explicit
//     warning (via the standard logger used elsewhere) that the example template
//     is in use and onboarding/configuration is required.
//
// The returned source string tells the caller which file was actually used so it
// can log/surface the active profile source. Any error other than "file does not
// exist at path" (e.g. a malformed profile, or a missing example template) is
// returned wrapped.
//
// ResolveProfile resolves the active profile from the configured file path only,
// with the example-template fallback. It is the file-only entry point retained for
// callers that have no ProfileStore wired (e.g. probes). Production binaries use
// ResolveProfileWithStore so a tenant-written profile takes precedence; this is
// exactly ResolveProfileWithStore(nil, path).
func ResolveProfile(path string) (*CapabilityProfile, string, error) {
	return ResolveProfileWithStore(nil, path)
}

// ResolveProfileWithStore resolves the active company profile at runtime, fulfilling
// the WS-C onboarding seam: a tenant can configure a deployment by writing a profile
// to the ProfileStore at runtime, with NO file editing.
//
// Resolution order (highest precedence first):
//  1. The ProfileStore (the real, tenant-written profile). If ps is non-nil and a
//     stored profile exists, it is returned with source "store" and no warning.
//  2. The configured file at path (the existing-deployment path, config/profile.json).
//     Returned with source == path and no warning.
//  3. The generic ExampleProfilePath template, with an explicit logged warning that
//     onboarding/configuration is required. This is the final no-data fallback.
//
// A nil ps (or an empty store) skips step 1 entirely, making the behavior IDENTICAL
// to the original file-first ResolveProfile — so an existing file-based deployment is
// unchanged when no stored profile exists. ErrProfileNotFound from the store is the
// expected "not configured yet" signal and falls through to the file check; any OTHER
// store error fails loud (a corrupt or unreachable store must not be masked).
func ResolveProfileWithStore(ps ProfileStore, path string) (*CapabilityProfile, string, error) {
	// Step 1: consult the ProfileStore first (the WS-A6 TODO seam). A stored,
	// tenant-written profile wins over the baked-in file and the example template.
	if ps != nil {
		stored, err := ps.Load()
		switch {
		case err == nil:
			return stored, "store", nil
		case errors.Is(err, ErrProfileNotFound):
			// Not onboarded yet: fall through to the file/example resolution below.
		default:
			// A real store failure (corrupt JSON, unreachable backend). Fail loud
			// rather than silently serving a stale file or the example template.
			return nil, "", fmt.Errorf("failed to load profile from store: %w", err)
		}
	}

	// Steps 2 & 3: the original file-first resolution, unchanged.
	// Try the configured path directly and branch on the error, rather than a
	// separate os.Stat existence check. The stat-then-load pattern is both a
	// TOCTOU race (the file can change between the check and the load) and an
	// anti-idiom in Go: just attempt the operation and inspect the error.
	p, err := LoadProfile(path)
	switch {
	case err == nil:
		// A real profile exists at the configured path: return it exactly as a
		// direct LoadProfile(path) would, with no warning. This is the existing-
		// deployment path (config/profile.json present).
		return p, path, nil

	case errors.Is(err, fs.ErrNotExist):
		// No profile file at path. Fall back to the generic example template and
		// be loud about it so a fresh deployment knows it must be onboarded.
		// LoadProfile wraps os.ReadFile's error with %w, so fs.ErrNotExist
		// surfaces here only for a genuinely missing file — not for a parse or
		// permission error, which fall through to the default branch below.
		example, exErr := LoadProfile(ExampleProfilePath)
		if exErr != nil {
			return nil, "", fmt.Errorf("no profile at %q and failed to load example template %q: %w", path, ExampleProfilePath, exErr)
		}
		log.Printf("WARNING: no company profile found at %q; using the generic example template %q (company %q). "+
			"This deployment is NOT configured with real company data — complete onboarding/configure a real profile before production use.",
			path, ExampleProfilePath, example.Company)
		return example, ExampleProfilePath, nil

	default:
		// Any other error (permission denied, malformed JSON/YAML, unsupported
		// extension). Fail safe and loud rather than masking a misconfigured or
		// corrupt real profile by silently serving the example template.
		return nil, "", fmt.Errorf("failed to load company profile at %q: %w", path, err)
	}
}

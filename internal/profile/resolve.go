package profile

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
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
// TODO(WS-C): when the onboarding API + Store/GCS-backed profile lands, resolve
// the active profile from the Store first (real, tenant-written profile), then
// fall back to a local file at path, and only then to ExampleProfilePath. The
// example template remains the final no-data fallback; this function is the seam
// where the Store-backed lookup plugs in ahead of the local-file check.
func ResolveProfile(path string) (*CapabilityProfile, string, error) {
	_, statErr := os.Stat(path)
	switch {
	case statErr == nil:
		// A real profile exists at the configured path: load it exactly as before.
		p, err := LoadProfile(path)
		if err != nil {
			return nil, "", fmt.Errorf("failed to load company profile %q: %w", path, err)
		}
		return p, path, nil

	case errors.Is(statErr, fs.ErrNotExist):
		// No real profile present. Fall back to the generic example template and
		// be loud about it so a fresh deployment knows it must be onboarded.
		p, err := LoadProfile(ExampleProfilePath)
		if err != nil {
			return nil, "", fmt.Errorf("no profile at %q and failed to load example template %q: %w", path, ExampleProfilePath, err)
		}
		log.Printf("WARNING: no company profile found at %q; using the generic example template %q (company %q). "+
			"This deployment is NOT configured with real company data — complete onboarding/configure a real profile before production use.",
			path, ExampleProfilePath, p.Company)
		return p, ExampleProfilePath, nil

	default:
		// Some other stat error (e.g. permission); surface it rather than silently
		// falling back, so a misconfigured path is not masked by the example.
		return nil, "", fmt.Errorf("failed to check company profile path %q: %w", path, statErr)
	}
}

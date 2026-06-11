package profile_test

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mawar2/Kaimi/internal/profile"
	"github.com/Mawar2/Kaimi/internal/scorer"
)

// chdirRepoRoot switches the working directory to the repository root for the
// duration of the test, restoring it afterward. ResolveProfile's example-template
// fallback uses the repo-root-relative ExampleProfilePath (the same convention as
// the config/profile.json default), so the fallback tests run from the same cwd
// the real cmd/pipeline and cmd/dashboard binaries do.
func chdirRepoRoot(t *testing.T) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatalf("chdir to repo root: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

// captureLog redirects the standard logger to a buffer for the duration of fn,
// restoring the previous output and flags afterward, and returns what was logged.
// ResolveProfile logs its example-template warning via the standard log package
// (the same logger cmd/dashboard already uses), so this is how we assert on it.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prevOut := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	fn()
	return buf.String()
}

// TestResolveProfile_RealProfile verifies that when a profile file exists at the
// configured path, ResolveProfile loads and returns THAT profile, reports it as
// the source, and logs no example-template warning. This is the existing-
// deployment path (config/profile.json present) and must behave identically to a
// direct LoadProfile call.
func TestResolveProfile_RealProfile(t *testing.T) {
	realPath := "../../config/profile.json"

	var p *profile.CapabilityProfile
	var source string
	var err error
	out := captureLog(t, func() {
		p, source, err = profile.ResolveProfile(realPath)
	})
	if err != nil {
		t.Fatalf("ResolveProfile(%q) failed: %v", realPath, err)
	}
	if p == nil {
		t.Fatal("ResolveProfile returned a nil profile")
	}
	if source != realPath {
		t.Errorf("source = %q, want the real path %q", source, realPath)
	}
	// The real profile must be the one returned, not the example template.
	if p.Company == profile.ExampleCompanyName {
		t.Errorf("ResolveProfile returned the example template (%q) when a real profile exists", profile.ExampleCompanyName)
	}
	// No warning when a real profile is present.
	if strings.Contains(strings.ToLower(out), "example") {
		t.Errorf("ResolveProfile logged an example-template warning for an existing profile:\n%s", out)
	}
}

// TestResolveProfile_FallsBackToExample verifies that when no profile exists at
// the configured path, ResolveProfile loads config/profile.example.yaml, reports
// the example template as the source, and logs an explicit warning that the
// example template is in use and onboarding/config is required.
func TestResolveProfile_FallsBackToExample(t *testing.T) {
	chdirRepoRoot(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist.json")

	var p *profile.CapabilityProfile
	var source string
	var err error
	out := captureLog(t, func() {
		p, source, err = profile.ResolveProfile(missing)
	})
	if err != nil {
		t.Fatalf("ResolveProfile(%q) should fall back to the example, got error: %v", missing, err)
	}
	if p == nil {
		t.Fatal("ResolveProfile returned a nil profile on fallback")
	}
	if source != profile.ExampleProfilePath {
		t.Errorf("source = %q, want the example template path %q", source, profile.ExampleProfilePath)
	}
	if p.Company != profile.ExampleCompanyName {
		t.Errorf("fallback profile Company = %q, want example %q", p.Company, profile.ExampleCompanyName)
	}
	// An explicit warning must be logged so a fresh deployment is loud about it.
	low := strings.ToLower(out)
	if !strings.Contains(low, "example") {
		t.Errorf("expected an example-template warning, got:\n%s", out)
	}
	if !strings.Contains(low, "onboarding") && !strings.Contains(low, "configure") {
		t.Errorf("warning should tell the operator onboarding/configuration is required, got:\n%s", out)
	}
}

// TestResolveProfile_MalformedRealProfileFailsLoud verifies that a profile file
// that EXISTS at the configured path but is malformed (invalid JSON) returns an
// error and does NOT fall back to the example template. Only a genuinely missing
// file triggers the fallback; a corrupt or misconfigured real profile must fail
// safe and loud rather than be silently masked by the example.
func TestResolveProfile_MalformedRealProfileFailsLoud(t *testing.T) {
	chdirRepoRoot(t)
	bad := filepath.Join(t.TempDir(), "malformed.json")
	if err := os.WriteFile(bad, []byte("{ this is not valid json"), 0o600); err != nil {
		t.Fatalf("write malformed profile: %v", err)
	}

	var p *profile.CapabilityProfile
	var source string
	var err error
	out := captureLog(t, func() {
		p, source, err = profile.ResolveProfile(bad)
	})
	if err == nil {
		t.Fatalf("ResolveProfile(%q) on a malformed profile = nil error; want a fail-loud error", bad)
	}
	if p != nil {
		t.Errorf("ResolveProfile returned a non-nil profile (%+v) on a malformed file; want nil", p)
	}
	if source != "" {
		t.Errorf("source = %q on a malformed file; want empty (no fallback)", source)
	}
	// It must NOT have fallen back to the example template.
	if strings.Contains(strings.ToLower(out), "example") {
		t.Errorf("ResolveProfile fell back to the example template on a malformed real profile:\n%s", out)
	}
}

// TestResolveProfileWithStore_PrefersStored verifies the WS-C1 seam: when a
// tenant-written profile exists in the ProfileStore, ResolveProfileWithStore
// returns THAT profile and reports source "store", ahead of any local file or the
// example template.
func TestResolveProfileWithStore_PrefersStored(t *testing.T) {
	base := t.TempDir()
	ps, err := profile.NewJSONProfileStore(base)
	if err != nil {
		t.Fatalf("NewJSONProfileStore: %v", err)
	}
	stored := &profile.CapabilityProfile{
		Company:    "Stored Tenant Co",
		NAICSCodes: []profile.NAICSCode{{Code: "541512", Tier: profile.TierPrimary}},
	}
	if err := ps.Save(stored); err != nil {
		t.Fatalf("seed stored profile: %v", err)
	}

	// Point the file path at a real, DIFFERENT profile to prove the store wins over
	// the file when both are present.
	filePath := "../../config/profile.json"

	var p *profile.CapabilityProfile
	var source string
	out := captureLog(t, func() {
		p, source, err = profile.ResolveProfileWithStore(ps, filePath)
	})
	if err != nil {
		t.Fatalf("ResolveProfileWithStore: %v", err)
	}
	if source != "store" {
		t.Errorf("source = %q, want %q", source, "store")
	}
	if p == nil || p.Company != "Stored Tenant Co" {
		t.Errorf("returned profile = %+v, want the stored tenant profile", p)
	}
	if strings.Contains(strings.ToLower(out), "example") {
		t.Errorf("logged an example-template warning when a stored profile exists:\n%s", out)
	}
}

// TestResolveProfileWithStore_FallsBackToFile verifies that with an empty
// ProfileStore the resolver falls through to the configured file path exactly as
// the original ResolveProfile does — the existing-deployment behavior is unchanged
// when no stored profile exists.
func TestResolveProfileWithStore_FallsBackToFile(t *testing.T) {
	ps, err := profile.NewJSONProfileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONProfileStore: %v", err)
	}
	filePath := "../../config/profile.json"

	p, source, err := profile.ResolveProfileWithStore(ps, filePath)
	if err != nil {
		t.Fatalf("ResolveProfileWithStore: %v", err)
	}
	if source != filePath {
		t.Errorf("source = %q, want the file path %q", source, filePath)
	}
	if p == nil || p.Company == profile.ExampleCompanyName {
		t.Errorf("returned profile = %+v, want the real file profile (not the example)", p)
	}
}

// TestResolveProfileWithStore_FallsBackToExample verifies that with an empty
// ProfileStore AND no file at the path, the resolver still reaches the example
// template + warning — identical to the original ResolveProfile fallback.
func TestResolveProfileWithStore_FallsBackToExample(t *testing.T) {
	chdirRepoRoot(t)
	ps, err := profile.NewJSONProfileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONProfileStore: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "does-not-exist.json")

	var p *profile.CapabilityProfile
	var source string
	out := captureLog(t, func() {
		p, source, err = profile.ResolveProfileWithStore(ps, missing)
	})
	if err != nil {
		t.Fatalf("ResolveProfileWithStore: %v", err)
	}
	if source != profile.ExampleProfilePath {
		t.Errorf("source = %q, want the example path %q", source, profile.ExampleProfilePath)
	}
	if p == nil || p.Company != profile.ExampleCompanyName {
		t.Errorf("returned profile = %+v, want the example template", p)
	}
	if !strings.Contains(strings.ToLower(out), "example") {
		t.Errorf("expected an example-template warning, got:\n%s", out)
	}
}

// TestResolveProfileWithStore_NilStore verifies passing a nil ProfileStore is
// equivalent to the original file-first ResolveProfile, so callers that have no
// store wired behave unchanged.
func TestResolveProfileWithStore_NilStore(t *testing.T) {
	filePath := "../../config/profile.json"
	p, source, err := profile.ResolveProfileWithStore(nil, filePath)
	if err != nil {
		t.Fatalf("ResolveProfileWithStore(nil): %v", err)
	}
	if source != filePath {
		t.Errorf("source = %q, want the file path %q", source, filePath)
	}
	if p == nil {
		t.Fatal("returned a nil profile")
	}
}

// TestExampleProfile_LoadsAndDerivesScorer verifies the shipped example template
// is a valid CapabilityProfile that both LoadProfile and scorer.FromProfile
// accept, with a non-empty scoring block so the derived Scorer view is usable.
// This guards the requirement that the example carries a real scoring block.
func TestExampleProfile_LoadsAndDerivesScorer(t *testing.T) {
	p, err := profile.LoadProfile("../../" + profile.ExampleProfilePath)
	if err != nil {
		t.Fatalf("LoadProfile(%q) failed: %v", profile.ExampleProfilePath, err)
	}
	if p.Company != profile.ExampleCompanyName {
		t.Errorf("example Company = %q, want %q", p.Company, profile.ExampleCompanyName)
	}
	if len(p.AllNAICSCodes()) == 0 {
		t.Error("example template has no NAICS codes")
	}

	sp := scorer.FromProfile(p)
	if len(sp.PrimaryNAICS) == 0 {
		t.Error("derived scorer view has no PrimaryNAICS — example scoring block is missing/empty")
	}
	if len(sp.CompetencyTags) == 0 {
		t.Error("derived scorer view has no CompetencyTags — example scoring block is missing/empty")
	}
}

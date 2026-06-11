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

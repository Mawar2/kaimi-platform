package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setEnv sets an env var for the duration of a test and restores it afterward.
// t.Setenv would also work, but we keep an explicit helper so the precedence
// tests can clear a variable (set it to "") and assert the file/default wins.
func setEnv(t *testing.T, key, val string) {
	t.Helper()
	t.Setenv(key, val)
}

func TestLoad_Defaults(t *testing.T) {
	// With no flags, no env, and no file, Load must produce exactly the historic
	// defaults the two binaries shipped with.
	clearKaimiEnv(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"Mode", cfg.Mode, "cached"},
		{"Store.Path", cfg.Store.Path, "./queue"},
		{"Profile.EligibilityPath", cfg.Profile.EligibilityPath, "config/profile.json"},
		{"Profile.WriterPath", cfg.Profile.WriterPath, "config/profile.json"},
		{"GCP.Region", cfg.GCP.Region, "us-east4"},
		{"GCP.AgentRegion", cfg.GCP.AgentRegion, "global"},
		{"GCP.ScorerModel", cfg.GCP.ScorerModel, "gemini-2.5-pro"},
		{"GCP.WriterModel", cfg.GCP.WriterModel, "gemini-3.1-pro-preview"},
		{"GCP.OutlineModel", cfg.GCP.OutlineModel, "gemini-3.5-flash"},
		{"GCP.FinalReviewModel", cfg.GCP.FinalReviewModel, "gemini-2.5-pro"},
		{"Ingest.DocumentAILocation", cfg.Ingest.DocumentAILocation, "us"},
		{"Server.Host", cfg.Server.Host, "127.0.0.1"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("default %s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if cfg.Server.Port != 8900 {
		t.Errorf("default Server.Port = %d, want 8900", cfg.Server.Port)
	}
}

func TestLoad_EnvOverridesDefault(t *testing.T) {
	clearKaimiEnv(t)
	setEnv(t, "MODE", "live")
	setEnv(t, "STORE_PATH", "/tmp/env-store")
	setEnv(t, "GCP_PROJECT_ID", "env-project")
	setEnv(t, "GCP_REGION", "europe-west1")
	setEnv(t, "GEMINI_MODEL", "env-writer-model")
	setEnv(t, "SAM_API_KEY", "env-sam-key")
	setEnv(t, "PORT", "9999")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Mode != "live" {
		t.Errorf("Mode = %q, want live", cfg.Mode)
	}
	if cfg.Store.Path != "/tmp/env-store" {
		t.Errorf("Store.Path = %q, want /tmp/env-store", cfg.Store.Path)
	}
	if cfg.GCP.ProjectID != "env-project" {
		t.Errorf("GCP.ProjectID = %q, want env-project", cfg.GCP.ProjectID)
	}
	// GCP_REGION drives the regional Region only. AgentRegion is decoupled and
	// must NOT follow GCP_REGION (the 3.x Outline/Writer agents are served only
	// from the "global" Vertex endpoint), so it stays at its "global" default.
	if cfg.GCP.Region != "europe-west1" {
		t.Errorf("GCP.Region = %q, want europe-west1", cfg.GCP.Region)
	}
	if cfg.GCP.AgentRegion != "global" {
		t.Errorf("GCP.AgentRegion = %q, want global (decoupled from GCP_REGION)", cfg.GCP.AgentRegion)
	}
	if cfg.GCP.WriterModel != "env-writer-model" {
		t.Errorf("GCP.WriterModel = %q, want env-writer-model", cfg.GCP.WriterModel)
	}
	if cfg.SAM.APIKey != "env-sam-key" {
		t.Errorf("SAM.APIKey = %q, want env-sam-key", cfg.SAM.APIKey)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("Server.Port = %d, want 9999", cfg.Server.Port)
	}
}

// TestLoad_AgentRegionDecoupledFromGCPRegion locks in the fix for the live-deploy
// bug where GCP_REGION leaked into AgentRegion. The 3.x Outline/Writer agents are
// served only from the "global" Vertex endpoint, while the gemini-2.5-pro Scorer
// and Final Review run in the regional endpoint, so the two regions must resolve
// independently.
func TestLoad_AgentRegionDecoupledFromGCPRegion(t *testing.T) {
	t.Run("GCP_REGION set, GCP_AGENT_REGION unset: Region follows, AgentRegion stays global", func(t *testing.T) {
		// This is the deployed-API scenario: GCP_REGION=us-east4 is correct for the
		// regional Scorer/FinalReview but must NOT drag the global-only agents to
		// us-east4 (which 404s for the 3.x models).
		clearKaimiEnv(t)
		setEnv(t, "GCP_REGION", "us-east4")
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if cfg.GCP.Region != "us-east4" {
			t.Errorf("GCP.Region = %q, want us-east4", cfg.GCP.Region)
		}
		if cfg.GCP.AgentRegion != "global" {
			t.Errorf("GCP.AgentRegion = %q, want global (must not follow GCP_REGION)", cfg.GCP.AgentRegion)
		}
	})

	t.Run("GCP_AGENT_REGION override sets AgentRegion only", func(t *testing.T) {
		clearKaimiEnv(t)
		setEnv(t, "GCP_AGENT_REGION", "us-central1")
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if cfg.GCP.AgentRegion != "us-central1" {
			t.Errorf("GCP.AgentRegion = %q, want us-central1 (explicit override)", cfg.GCP.AgentRegion)
		}
		// Region keeps its own default, untouched by GCP_AGENT_REGION.
		if cfg.GCP.Region != "us-east4" {
			t.Errorf("GCP.Region = %q, want us-east4 (default, unaffected by GCP_AGENT_REGION)", cfg.GCP.Region)
		}
	})

	t.Run("neither set: Region defaults us-east4, AgentRegion defaults global", func(t *testing.T) {
		clearKaimiEnv(t)
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if cfg.GCP.Region != "us-east4" {
			t.Errorf("GCP.Region = %q, want us-east4 (default)", cfg.GCP.Region)
		}
		if cfg.GCP.AgentRegion != "global" {
			t.Errorf("GCP.AgentRegion = %q, want global (default)", cfg.GCP.AgentRegion)
		}
	})
}

// TestLoad_ProfilePathAlias documents the WS-A3 consolidation: one company
// profile feeds both the Hunter gate and the Scorer. ELIGIBILITY_PROFILE_PATH is
// the canonical env var, and the legacy PROFILE_PATH (which formerly set the now-
// removed separate scoring profile) is honored as a backward-compatible alias so
// existing deployments/scripts do not silently lose their profile path.
func TestLoad_ProfilePathAlias(t *testing.T) {
	t.Run("PROFILE_PATH alias sets the single profile", func(t *testing.T) {
		clearKaimiEnv(t)
		setEnv(t, "PROFILE_PATH", "/legacy/profile.json")
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if cfg.Profile.EligibilityPath != "/legacy/profile.json" {
			t.Errorf("EligibilityPath = %q, want /legacy/profile.json (PROFILE_PATH alias)", cfg.Profile.EligibilityPath)
		}
	})

	t.Run("ELIGIBILITY_PROFILE_PATH wins over the PROFILE_PATH alias", func(t *testing.T) {
		clearKaimiEnv(t)
		setEnv(t, "PROFILE_PATH", "/legacy/profile.json")
		setEnv(t, "ELIGIBILITY_PROFILE_PATH", "/canonical/profile.json")
		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load() unexpected error: %v", err)
		}
		if cfg.Profile.EligibilityPath != "/canonical/profile.json" {
			t.Errorf("EligibilityPath = %q, want /canonical/profile.json (canonical var wins)", cfg.Profile.EligibilityPath)
		}
	})
}

func TestLoad_FlagOverridesEnv(t *testing.T) {
	clearKaimiEnv(t)
	setEnv(t, "MODE", "live")
	setEnv(t, "STORE_PATH", "/tmp/env-store")
	setEnv(t, "GCP_REGION", "europe-west1")

	flags := &Flags{
		Mode:      strptr("cached"),
		StorePath: strptr("/tmp/flag-store"),
		Region:    strptr("asia-south1"),
	}

	cfg, err := Load(flags)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Mode != "cached" {
		t.Errorf("Mode = %q, want cached (flag beats env)", cfg.Mode)
	}
	if cfg.Store.Path != "/tmp/flag-store" {
		t.Errorf("Store.Path = %q, want /tmp/flag-store (flag beats env)", cfg.Store.Path)
	}
	if cfg.GCP.Region != "asia-south1" {
		t.Errorf("GCP.Region = %q, want asia-south1 (flag beats env)", cfg.GCP.Region)
	}
	// The -region flag sets the regional Region only; AgentRegion stays decoupled
	// at its "global" default so the flag cannot strand the global-only agents.
	if cfg.GCP.AgentRegion != "global" {
		t.Errorf("GCP.AgentRegion = %q, want global (region flag must not set it)", cfg.GCP.AgentRegion)
	}
}

func TestLoad_FileBetweenEnvAndDefault(t *testing.T) {
	clearKaimiEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "tenant.yaml")
	contents := `tenant:
  id: bluemeta
  display_name: BlueMeta Technologies
gcp:
  project_id: file-project
  region: file-region
  writer_model: file-writer
store:
  path: /file/store
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	// Env beats file: GCP_PROJECT_ID is set, so it must win over the file value.
	setEnv(t, "GCP_PROJECT_ID", "env-project")

	flags := &Flags{ConfigPath: strptr(path)}
	cfg, err := Load(flags)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Tenant.ID != "bluemeta" {
		t.Errorf("Tenant.ID = %q, want bluemeta (from file)", cfg.Tenant.ID)
	}
	if cfg.Tenant.DisplayName != "BlueMeta Technologies" {
		t.Errorf("Tenant.DisplayName = %q, want BlueMeta Technologies (from file)", cfg.Tenant.DisplayName)
	}
	// File beats default.
	if cfg.GCP.Region != "file-region" {
		t.Errorf("GCP.Region = %q, want file-region (file beats default)", cfg.GCP.Region)
	}
	if cfg.GCP.WriterModel != "file-writer" {
		t.Errorf("GCP.WriterModel = %q, want file-writer (file beats default)", cfg.GCP.WriterModel)
	}
	if cfg.Store.Path != "/file/store" {
		t.Errorf("Store.Path = %q, want /file/store (file beats default)", cfg.Store.Path)
	}
	// Env beats file.
	if cfg.GCP.ProjectID != "env-project" {
		t.Errorf("GCP.ProjectID = %q, want env-project (env beats file)", cfg.GCP.ProjectID)
	}
}

func TestLoad_RoundTripFile(t *testing.T) {
	clearKaimiEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "round.yaml")
	contents := `tenant:
  id: acme
  display_name: Acme Corp
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := Load(&Flags{ConfigPath: strptr(path)})
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.Tenant.ID != "acme" || cfg.Tenant.DisplayName != "Acme Corp" {
		t.Errorf("round-trip tenant = %+v, want {acme Acme Corp}", cfg.Tenant)
	}
}

func TestLoad_MissingConfigFileErrorsWrapped(t *testing.T) {
	clearKaimiEnv(t)
	_, err := Load(&Flags{ConfigPath: strptr("/no/such/file.yaml")})
	if err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
	if !strings.Contains(err.Error(), "/no/such/file.yaml") {
		t.Errorf("error should name the missing path, got: %v", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error should wrap os.ErrNotExist (%%w), got: %v", err)
	}
}

func TestValidateLive_MissingRequired(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantVar string
	}{
		{
			name:    "missing SAM key",
			cfg:     Config{Mode: "live", GCP: GCP{ProjectID: "p"}},
			wantVar: "SAM_API_KEY",
		},
		{
			name:    "missing project id",
			cfg:     Config{Mode: "live", SAM: SAM{APIKey: "k"}},
			wantVar: "GCP_PROJECT_ID",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.ValidateLive()
			if err == nil {
				t.Fatalf("ValidateLive() = nil, want error naming %s", tt.wantVar)
			}
			if !strings.Contains(err.Error(), tt.wantVar) {
				t.Errorf("error %q should name missing var %q", err.Error(), tt.wantVar)
			}
			if !errors.Is(err, ErrMissingRequired) {
				t.Errorf("error should wrap ErrMissingRequired (%%w), got: %v", err)
			}
		})
	}
}

func TestValidateLive_CachedModeOK(t *testing.T) {
	cfg := Config{Mode: "cached"}
	if err := cfg.ValidateLive(); err != nil {
		t.Errorf("ValidateLive() in cached mode = %v, want nil", err)
	}
}

func TestValidateMode(t *testing.T) {
	bogus := Config{Mode: "bogus"}
	if err := bogus.ValidateMode(); err == nil {
		t.Error("ValidateMode() for bogus mode = nil, want error")
	}
	for _, m := range []string{"cached", "live"} {
		cfg := Config{Mode: m}
		if err := cfg.ValidateMode(); err != nil {
			t.Errorf("ValidateMode() for %q = %v, want nil", m, err)
		}
	}
}

func strptr(s string) *string { return &s }

// clearKaimiEnv unsets every env var the config reads so a test starts from a
// known-empty environment regardless of the host shell.
func clearKaimiEnv(t *testing.T) {
	t.Helper()
	vars := []string{
		"MODE", "STORE_PATH", "PROFILE_PATH", "ELIGIBILITY_PROFILE_PATH",
		"NAICS_CODES", "SAM_API_KEY", "GCP_PROJECT_ID", "GCP_REGION",
		"GCP_AGENT_REGION",
		"GEMINI_MODEL", "OUTLINE_MODEL", "FINALREVIEW_MODEL", "PORT", "HOST",
		"GCS_SOLICITATIONS_BUCKET", "DOCUMENTAI_PROCESSOR_ID", "DOCUMENTAI_LOCATION",
		"KAIMI_TELEMETRY_ENABLED", "KAIMI_TELEMETRY_PATH", "KAIMI_TELEMETRY_BUFFER_SIZE",
	}
	for _, v := range vars {
		t.Setenv(v, "")
		if err := os.Unsetenv(v); err != nil {
			t.Fatalf("unset %s: %v", v, err)
		}
	}
}

func TestLoad_TelemetryDefaults(t *testing.T) {
	// Telemetry is ENABLED by default; Path is empty (derived from the store path
	// via TelemetryDir) and BufferSize is 0 (emitter applies its own default).
	clearKaimiEnv(t)

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if !cfg.Telemetry.Enabled {
		t.Error("default Telemetry.Enabled = false, want true")
	}
	if cfg.Telemetry.Path != "" {
		t.Errorf("default Telemetry.Path = %q, want empty", cfg.Telemetry.Path)
	}
	if cfg.Telemetry.BufferSize != 0 {
		t.Errorf("default Telemetry.BufferSize = %d, want 0", cfg.Telemetry.BufferSize)
	}
	// With Store.Path at its default, the event log lives under <store>/telemetry.
	wantDir := filepath.Join(cfg.Store.Path, "telemetry")
	if got := cfg.TelemetryDir(cfg.Store.Path); got != wantDir {
		t.Errorf("TelemetryDir = %q, want %q", got, wantDir)
	}
}

func TestLoad_TelemetryEnvOverrides(t *testing.T) {
	clearKaimiEnv(t)
	setEnv(t, "KAIMI_TELEMETRY_ENABLED", "false")
	setEnv(t, "KAIMI_TELEMETRY_PATH", "/var/telemetry")
	setEnv(t, "KAIMI_TELEMETRY_BUFFER_SIZE", "256")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.Telemetry.Enabled {
		t.Error("Telemetry.Enabled = true with KAIMI_TELEMETRY_ENABLED=false, want false")
	}
	if cfg.Telemetry.Path != "/var/telemetry" {
		t.Errorf("Telemetry.Path = %q, want /var/telemetry", cfg.Telemetry.Path)
	}
	if cfg.Telemetry.BufferSize != 256 {
		t.Errorf("Telemetry.BufferSize = %d, want 256", cfg.Telemetry.BufferSize)
	}
	// An explicit Path wins over the store-derived default.
	if got := cfg.TelemetryDir("./queue"); got != "/var/telemetry" {
		t.Errorf("TelemetryDir = %q, want /var/telemetry", got)
	}
}

func TestLoad_TelemetryMalformedEnabledStaysOn(t *testing.T) {
	// A malformed KAIMI_TELEMETRY_ENABLED must NOT silently disable telemetry: it
	// stays on the safe (enabled) side, matching the additive-observability rule.
	clearKaimiEnv(t)
	setEnv(t, "KAIMI_TELEMETRY_ENABLED", "garbage")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if !cfg.Telemetry.Enabled {
		t.Error("Telemetry.Enabled = false for malformed env value, want true (stay enabled)")
	}
}

func TestLoad_TelemetryFileDisablesEnvReenables(t *testing.T) {
	// The YAML file can disable telemetry; an env var overrides the file (env >
	// file > default), so KAIMI_TELEMETRY_ENABLED=true re-enables it.
	clearKaimiEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("telemetry:\n  enabled: false\n  buffer_size: 64\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// File alone disables it.
	cfg, err := Load(&Flags{ConfigPath: &path})
	if err != nil {
		t.Fatalf("Load(file) error: %v", err)
	}
	if cfg.Telemetry.Enabled {
		t.Error("file enabled:false did not disable telemetry")
	}
	if cfg.Telemetry.BufferSize != 64 {
		t.Errorf("file buffer_size = %d, want 64", cfg.Telemetry.BufferSize)
	}

	// Env overrides the file.
	setEnv(t, "KAIMI_TELEMETRY_ENABLED", "true")
	cfg, err = Load(&Flags{ConfigPath: &path})
	if err != nil {
		t.Fatalf("Load(file+env) error: %v", err)
	}
	if !cfg.Telemetry.Enabled {
		t.Error("env KAIMI_TELEMETRY_ENABLED=true did not override file enabled:false")
	}
}

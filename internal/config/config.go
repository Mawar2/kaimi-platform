package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrMissingRequired is the sentinel wrapped by validation errors when a
// required value is absent. Callers can test for it with errors.Is.
var ErrMissingRequired = errors.New("required configuration value is missing")

// Config is the single source of truth for one Kaimi deployment's
// configuration. It groups the inputs the Zone-1 pipeline and the dashboard
// read into logical sub-structs. Fields map one-to-one onto values the two
// binaries already read from flags/env; Tenant is included for forward
// compatibility (nothing reads it yet).
type Config struct {
	// Mode selects the pipeline backend set: "cached" (offline fixtures +
	// deterministic scorer) or "live" (SAM.gov + Gemini). Read by cmd/pipeline.
	Mode string `yaml:"mode"`

	Tenant  Tenant  `yaml:"tenant"`
	GCP     GCP     `yaml:"gcp"`
	Profile Profile `yaml:"profile"`
	Drive   Drive   `yaml:"drive"`
	SAM     SAM     `yaml:"sam"`
	Store   Store   `yaml:"store"`
	Ingest  Ingest  `yaml:"ingest"`
	Server  Server  `yaml:"server"`
}

// Tenant identifies the deployment's owning organization. Forward-compatibility
// scaffolding for multi-tenant deployments; not yet consumed by the binaries.
type Tenant struct {
	ID          string `yaml:"id"`
	DisplayName string `yaml:"display_name"`
}

// GCP holds Google Cloud project, region, and model selection. Region and
// AgentRegion both resolve from GCP_REGION but carry different defaults to
// preserve historic behavior: the dashboard's Outline/Writer agents defaulted
// to the "global" Vertex endpoint, while the pipeline scorer and Final Review
// defaulted to "us-east4".
type GCP struct {
	ProjectID string `yaml:"project_id"` // GCP_PROJECT_ID
	Region    string `yaml:"region"`     // GCP_REGION, default us-east4

	// AgentRegion is GCP_REGION with a "global" default, used by the dashboard's
	// Outline planner and Writer generator (the Gemini 3.x family is served only
	// from the global endpoint).
	AgentRegion string `yaml:"agent_region"`

	ScorerModel      string `yaml:"scorer_model"`      // GEMINI_MODEL for the pipeline scorer, default gemini-2.5-pro
	WriterModel      string `yaml:"writer_model"`      // GEMINI_MODEL for the dashboard writer, default gemini-3.1-pro-preview
	OutlineModel     string `yaml:"outline_model"`     // OUTLINE_MODEL, default gemini-3.5-flash
	FinalReviewModel string `yaml:"finalreview_model"` // FINALREVIEW_MODEL, default gemini-2.5-pro
}

// Profile holds paths to the single company profile the agents ground on. Since
// WS-A3 one profile.CapabilityProfile file feeds both the Hunter eligibility gate
// and the Scorer (the scorer view is derived via scorer.FromProfile), so there is
// no longer a separate scoring-profile path.
type Profile struct {
	// EligibilityPath is the single company-profile file. ELIGIBILITY_PROFILE_PATH
	// sets it; PROFILE_PATH is honored as a backward-compatible alias (see applyEnv).
	EligibilityPath string   `yaml:"eligibility_path"`
	WriterPath      string   `yaml:"writer_path"` // dashboard -profile flag (company profile; writer grounding derived via scorer.FromProfile)
	NAICSCodes      []string `yaml:"naics_codes"` // NAICS_CODES override (empty → eligibility profile's codes)
}

// Drive holds the Google Drive target for created proposal documents. Modeled
// for forward compatibility; the live Docs client is wired separately today.
type Drive struct {
	SharedDriveID string `yaml:"shared_drive_id"` // GOOGLE_DRIVE_SHARED_DRIVE_ID
}

// SAM holds the SAM.gov API credential. APIKey is the resolved secret value;
// APIKeyEnv records which environment variable supplied it (for diagnostics and
// for deployments that reference a secret by name rather than inline).
type SAM struct {
	APIKey    string `yaml:"-"`           // resolved value (never serialized)
	APIKeyEnv string `yaml:"api_key_env"` // env var name, default SAM_API_KEY
}

// Store holds the opportunity/proposal JSON store location.
type Store struct {
	Path string `yaml:"path"` // STORE_PATH, default ./queue
}

// Ingest holds the document-ingestion targets used by the dashboard's
// -live-ingest path.
type Ingest struct {
	GCSBucket           string `yaml:"gcs_bucket"`           // GCS_SOLICITATIONS_BUCKET
	DocumentAIProcessor string `yaml:"documentai_processor"` // DOCUMENTAI_PROCESSOR_ID
	DocumentAILocation  string `yaml:"documentai_location"`  // DOCUMENTAI_LOCATION, default us
}

// Server holds the dashboard HTTP server bind configuration.
type Server struct {
	Host string `yaml:"host"` // HOST, default 127.0.0.1
	Port int    `yaml:"port"` // PORT/-port, default 8900
}

// Flags carries values parsed from the command line. Each field is a pointer so
// Load can distinguish "flag not provided" (nil) from "flag provided with the
// zero value". A nil *Flags means no command-line overrides at all.
type Flags struct {
	ConfigPath *string // path to an optional YAML config file

	Mode      *string
	StorePath *string
	ProjectID *string
	Region    *string

	// Pipeline-specific.
	EligibilityProfilePath *string
	NAICSCodes             *string // comma-separated
	ScorerModel            *string

	// Dashboard-specific.
	WriterProfilePath *string
	Host              *string
	Port              *int
}

const (
	defaultMode             = "cached"
	defaultStorePath        = "./queue"
	defaultEligibility      = "config/profile.json"
	defaultWriterProfile    = "config/profile.json"
	defaultRegion           = "us-east4"
	defaultAgentRegion      = "global"
	defaultScorerModel      = "gemini-2.5-pro"
	defaultWriterModel      = "gemini-3.1-pro-preview"
	defaultOutlineModel     = "gemini-3.5-flash"
	defaultFinalReviewModel = "gemini-2.5-pro"
	defaultDocAILocation    = "us"
	defaultHost             = "127.0.0.1"
	defaultPort             = 8900
	defaultSAMAPIKeyEnv     = "SAM_API_KEY"
)

// Load resolves a Config using the precedence flags > env > file > default.
// A nil flags argument means no command-line overrides were supplied.
//
// It starts from a file (if one was named via flags.ConfigPath), overlays any
// non-empty environment variables, then overlays any provided flags, and fills
// in built-in defaults for anything still empty. Missing config files are
// reported as errors wrapping os.ErrNotExist; required-value validation is the
// caller's responsibility via ValidateMode / ValidateLive.
func Load(flags *Flags) (Config, error) {
	var cfg Config

	// Layer 1 (lowest): config file, if one was named.
	if flags != nil && flags.ConfigPath != nil && *flags.ConfigPath != "" {
		if err := loadFile(*flags.ConfigPath, &cfg); err != nil {
			return Config{}, err
		}
	}

	// Layer 2: environment variables override file values when non-empty.
	applyEnv(&cfg)

	// Layer 3 (highest before defaults): command-line flags.
	applyFlags(flags, &cfg)

	// Layer 4: built-in defaults fill anything still empty.
	applyDefaults(&cfg)

	return cfg, nil
}

// loadFile reads a YAML config file into cfg. A missing file is an error that
// wraps os.ErrNotExist and names the path.
func loadFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parse config file %q: %w", path, err)
	}
	return nil
}

// applyEnv overlays non-empty environment variables onto cfg. An env var that
// is unset or empty leaves the existing (file/zero) value untouched, matching
// the historic envOr/getEnv "non-empty wins" semantics.
func applyEnv(cfg *Config) {
	envInto(&cfg.Mode, "MODE")
	envInto(&cfg.Store.Path, "STORE_PATH")
	// One company profile now feeds both the Hunter gate and the Scorer (WS-A3).
	// ELIGIBILITY_PROFILE_PATH is the canonical env var; PROFILE_PATH is honored as
	// a backward-compatible alias for it (it formerly set the now-removed separate
	// scoring profile). Apply the alias first so the canonical var wins when both
	// are set.
	envInto(&cfg.Profile.EligibilityPath, "PROFILE_PATH")
	envInto(&cfg.Profile.EligibilityPath, "ELIGIBILITY_PROFILE_PATH")
	if v := os.Getenv("NAICS_CODES"); v != "" {
		cfg.Profile.NAICSCodes = splitCSV(v)
	}
	envInto(&cfg.GCP.ProjectID, "GCP_PROJECT_ID")
	// GCP_REGION drives both region values; the differing defaults are only
	// applied later when the env var is absent.
	if v := os.Getenv("GCP_REGION"); v != "" {
		cfg.GCP.Region = v
		cfg.GCP.AgentRegion = v
	}
	// GEMINI_MODEL is overloaded: the pipeline reads it as the scorer model and
	// the dashboard reads it as the writer model. Apply it to both.
	if v := os.Getenv("GEMINI_MODEL"); v != "" {
		cfg.GCP.ScorerModel = v
		cfg.GCP.WriterModel = v
	}
	envInto(&cfg.GCP.OutlineModel, "OUTLINE_MODEL")
	envInto(&cfg.GCP.FinalReviewModel, "FINALREVIEW_MODEL")
	envInto(&cfg.SAM.APIKey, defaultSAMAPIKeyEnv)
	envInto(&cfg.Drive.SharedDriveID, "GOOGLE_DRIVE_SHARED_DRIVE_ID")
	envInto(&cfg.Ingest.GCSBucket, "GCS_SOLICITATIONS_BUCKET")
	envInto(&cfg.Ingest.DocumentAIProcessor, "DOCUMENTAI_PROCESSOR_ID")
	envInto(&cfg.Ingest.DocumentAILocation, "DOCUMENTAI_LOCATION")
	envInto(&cfg.Server.Host, "HOST")
	if v := os.Getenv("PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = n
		}
	}
}

// applyFlags overlays provided (non-nil) command-line flags onto cfg.
func applyFlags(flags *Flags, cfg *Config) {
	if flags == nil {
		return
	}
	flagInto(&cfg.Mode, flags.Mode)
	flagInto(&cfg.Store.Path, flags.StorePath)
	flagInto(&cfg.GCP.ProjectID, flags.ProjectID)
	if flags.Region != nil {
		cfg.GCP.Region = *flags.Region
		cfg.GCP.AgentRegion = *flags.Region
	}
	flagInto(&cfg.Profile.EligibilityPath, flags.EligibilityProfilePath)
	if flags.NAICSCodes != nil {
		cfg.Profile.NAICSCodes = splitCSV(*flags.NAICSCodes)
	}
	flagInto(&cfg.GCP.ScorerModel, flags.ScorerModel)
	flagInto(&cfg.Profile.WriterPath, flags.WriterProfilePath)
	flagInto(&cfg.Server.Host, flags.Host)
	if flags.Port != nil {
		cfg.Server.Port = *flags.Port
	}
}

// applyDefaults fills built-in defaults for any field still at its zero value.
func applyDefaults(cfg *Config) {
	defaultInto(&cfg.Mode, defaultMode)
	defaultInto(&cfg.Store.Path, defaultStorePath)
	defaultInto(&cfg.Profile.EligibilityPath, defaultEligibility)
	defaultInto(&cfg.Profile.WriterPath, defaultWriterProfile)
	defaultInto(&cfg.GCP.Region, defaultRegion)
	defaultInto(&cfg.GCP.AgentRegion, defaultAgentRegion)
	defaultInto(&cfg.GCP.ScorerModel, defaultScorerModel)
	defaultInto(&cfg.GCP.WriterModel, defaultWriterModel)
	defaultInto(&cfg.GCP.OutlineModel, defaultOutlineModel)
	defaultInto(&cfg.GCP.FinalReviewModel, defaultFinalReviewModel)
	defaultInto(&cfg.Ingest.DocumentAILocation, defaultDocAILocation)
	defaultInto(&cfg.SAM.APIKeyEnv, defaultSAMAPIKeyEnv)
	defaultInto(&cfg.Server.Host, defaultHost)
	if cfg.Server.Port == 0 {
		cfg.Server.Port = defaultPort
	}
}

// ValidateMode checks that Mode is one of the recognized pipeline modes.
func (c *Config) ValidateMode() error {
	switch c.Mode {
	case "cached", "live":
		return nil
	default:
		return fmt.Errorf("mode must be 'cached' or 'live', got: %q", c.Mode)
	}
}

// ValidateLive verifies the inputs the live pipeline mode requires. It is a
// no-op for non-live modes. Missing values are reported as errors that name the
// missing environment variable and wrap ErrMissingRequired.
func (c *Config) ValidateLive() error {
	if c.Mode != "live" {
		return nil
	}
	if c.SAM.APIKey == "" {
		return fmt.Errorf("%s is required for live mode: %w", defaultSAMAPIKeyEnv, ErrMissingRequired)
	}
	if c.GCP.ProjectID == "" {
		return fmt.Errorf("GCP_PROJECT_ID is required for live mode: %w", ErrMissingRequired)
	}
	return nil
}

// envInto overwrites dst with the named env var when it is set and non-empty.
func envInto(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

// flagInto overwrites dst with the flag value when the flag was provided.
func flagInto(dst, flag *string) {
	if flag != nil {
		*dst = *flag
	}
}

// defaultInto sets dst to def only when dst is still empty.
func defaultInto(dst *string, def string) {
	if *dst == "" {
		*dst = def
	}
}

// splitCSV splits a comma-separated list, trimming whitespace and dropping
// empty entries. Matches the historic helper in cmd/pipeline.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

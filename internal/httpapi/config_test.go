package httpapi

import (
	"errors"
	"strings"
	"testing"
)

// TestLoadConfigDefaults verifies the env > default precedence yields the built-in
// bind address when no API_HOST/API_PORT/PORT are set.
func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv(envAPIHost, "")
	t.Setenv(envAPIPort, "")
	t.Setenv(envPort, "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Host != defaultAPIHost {
		t.Errorf("Host = %q, want %q", cfg.Host, defaultAPIHost)
	}
	if cfg.Port != defaultAPIPort {
		t.Errorf("Port = %d, want %d", cfg.Port, defaultAPIPort)
	}
}

// TestLoadConfigEnvOverrides verifies API_HOST and API_PORT override the defaults.
func TestLoadConfigEnvOverrides(t *testing.T) {
	t.Setenv(envAPIHost, "0.0.0.0")
	t.Setenv(envAPIPort, "9000")
	t.Setenv(envPort, "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want %q", cfg.Host, "0.0.0.0")
	}
	if cfg.Port != 9000 {
		t.Errorf("Port = %d, want 9000", cfg.Port)
	}
}

// TestLoadConfigPORTWinsOverAPIPORT verifies the platform-injected $PORT takes
// precedence over API_PORT, mirroring cmd/dashboard's port precedence.
func TestLoadConfigPORTWinsOverAPIPORT(t *testing.T) {
	t.Setenv(envAPIHost, "")
	t.Setenv(envAPIPort, "9000")
	t.Setenv(envPort, "8080")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080 ($PORT must win over API_PORT)", cfg.Port)
	}
}

// TestLoadOAuthConfigDisabledWhenUnset verifies that with no OAUTH_* env set, OAuth
// is reported disabled and no error is returned — the offline cmd/api dev mode must
// still construct.
func TestLoadOAuthConfigDisabledWhenUnset(t *testing.T) {
	for _, e := range []string{envOAuthClientID, envOAuthClientSecret, envOAuthRedirectURL, envOAuthAllowedDomain, envOAuthSessionSecret} {
		t.Setenv(e, "")
	}
	cfg, enabled, err := LoadOAuthConfig()
	if err != nil {
		t.Fatalf("LoadOAuthConfig (all unset): %v", err)
	}
	if enabled {
		t.Errorf("OAuth enabled = true with nothing set; want disabled. cfg=%+v", cfg)
	}
}

// TestLoadOAuthConfigEnabledFull verifies that with all OAUTH_* env present, OAuth
// is enabled and the values are read through.
func TestLoadOAuthConfigEnabledFull(t *testing.T) {
	t.Setenv(envOAuthClientID, "cid")
	t.Setenv(envOAuthClientSecret, "csecret")
	t.Setenv(envOAuthRedirectURL, "https://app/auth/callback")
	t.Setenv(envOAuthAllowedDomain, "example.com")
	t.Setenv(envOAuthSessionSecret, "a-long-enough-session-secret-1234567890")

	cfg, enabled, err := LoadOAuthConfig()
	if err != nil {
		t.Fatalf("LoadOAuthConfig (full): %v", err)
	}
	if !enabled {
		t.Fatal("OAuth enabled = false with full config; want enabled")
	}
	if cfg.ClientID != "cid" || cfg.AllowedDomain != "example.com" || cfg.RedirectURL != "https://app/auth/callback" {
		t.Errorf("cfg read-through wrong: %+v", cfg)
	}
	if cfg.SessionSecret != "a-long-enough-session-secret-1234567890" {
		t.Error("session secret not read through")
	}
}

// TestLoadOAuthConfigClientIDSetMissingRequired verifies that once the client id is
// set (the enable signal), omitting another required value errors, naming the missing
// var and wrapping ErrMissingRequired.
func TestLoadOAuthConfigClientIDSetMissingRequired(t *testing.T) {
	t.Setenv(envOAuthClientID, "cid") // enable signal
	t.Setenv(envOAuthClientSecret, "csecret")
	t.Setenv(envOAuthRedirectURL, "https://app/auth/callback")
	t.Setenv(envOAuthAllowedDomain, "") // required, missing
	t.Setenv(envOAuthSessionSecret, "a-long-enough-session-secret-1234567890")

	_, _, err := LoadOAuthConfig()
	if err == nil {
		t.Fatal("LoadOAuthConfig (client id set, domain missing): want error, got nil")
	}
	if !errors.Is(err, ErrMissingRequired) {
		t.Errorf("error = %v, want wrap of ErrMissingRequired", err)
	}
	if got := err.Error(); !strings.Contains(got, envOAuthAllowedDomain) {
		t.Errorf("error %q should name the missing var %q", got, envOAuthAllowedDomain)
	}
}

// TestLoadOAuthConfigPlaceholderSecretOnlyDisabled is the deploy-scenario regression:
// the Cloud Run substrate always mounts a non-empty placeholder for the session/client
// secret even when sign-in is unconfigured. Enablement keys on the operator-set,
// non-secret client id — so a placeholder secret with NO client id must report DISABLED
// (enabled=false, nil error), not demand the (empty) client id and crash cmd/api.
func TestLoadOAuthConfigPlaceholderSecretOnlyDisabled(t *testing.T) {
	t.Setenv(envOAuthClientID, "") // not configured by the operator
	t.Setenv(envOAuthClientSecret, "REPLACE_ME_VIA_GCLOUD")
	t.Setenv(envOAuthRedirectURL, "")
	t.Setenv(envOAuthAllowedDomain, "")
	t.Setenv(envOAuthSessionSecret, "REPLACE_ME_VIA_GCLOUD")

	cfg, enabled, err := LoadOAuthConfig()
	if err != nil {
		t.Fatalf("LoadOAuthConfig (placeholder secret only): want nil error, got %v", err)
	}
	if enabled {
		t.Errorf("OAuth enabled = true with only a placeholder secret set; want disabled. cfg=%+v", cfg)
	}
}

// TestLoadOAuthConfigAllowedDomainOnlyDisabled verifies that AllowedDomain alone (no
// client id) does NOT enable auth: the domain is not the enable signal.
func TestLoadOAuthConfigAllowedDomainOnlyDisabled(t *testing.T) {
	t.Setenv(envOAuthClientID, "")
	t.Setenv(envOAuthClientSecret, "")
	t.Setenv(envOAuthRedirectURL, "")
	t.Setenv(envOAuthAllowedDomain, "example.com")
	t.Setenv(envOAuthSessionSecret, "")

	cfg, enabled, err := LoadOAuthConfig()
	if err != nil {
		t.Fatalf("LoadOAuthConfig (allowed domain only): want nil error, got %v", err)
	}
	if enabled {
		t.Errorf("OAuth enabled = true with only AllowedDomain set; want disabled. cfg=%+v", cfg)
	}
}

// TestLoadDriveOAuthConfigDisabledWhenUnset verifies that with no DRIVE_OAUTH_* env
// set, customer-Drive connect is reported disabled with no error.
func TestLoadDriveOAuthConfigDisabledWhenUnset(t *testing.T) {
	for _, e := range []string{envDriveClientID, envDriveClientSecret, envDriveRedirectURL} {
		t.Setenv(e, "")
	}
	cfg, enabled, err := LoadDriveOAuthConfig()
	if err != nil {
		t.Fatalf("LoadDriveOAuthConfig (all unset): %v", err)
	}
	if enabled {
		t.Errorf("Drive connect enabled = true with nothing set; want disabled. cfg=%+v", cfg)
	}
}

// TestLoadDriveOAuthConfigEnabledFull verifies a full DRIVE_OAUTH_* env enables the
// feature and reads the values through.
func TestLoadDriveOAuthConfigEnabledFull(t *testing.T) {
	t.Setenv(envDriveClientID, "dcid")
	t.Setenv(envDriveClientSecret, "dsecret")
	t.Setenv(envDriveRedirectURL, "https://app/api/v1/integrations/drive/callback")

	cfg, enabled, err := LoadDriveOAuthConfig()
	if err != nil {
		t.Fatalf("LoadDriveOAuthConfig (full): %v", err)
	}
	if !enabled {
		t.Fatal("Drive connect enabled = false with full config; want enabled")
	}
	if cfg.ClientID != "dcid" || cfg.ClientSecret != "dsecret" || cfg.RedirectURL != "https://app/api/v1/integrations/drive/callback" {
		t.Errorf("cfg read-through wrong: %+v", cfg)
	}
}

// TestLoadDriveOAuthConfigClientIDSetMissingRequired verifies that once the client id
// is set (the enable signal), omitting another required value errors, naming the
// missing one and wrapping ErrMissingRequired.
func TestLoadDriveOAuthConfigClientIDSetMissingRequired(t *testing.T) {
	t.Setenv(envDriveClientID, "dcid") // enable signal
	t.Setenv(envDriveClientSecret, "") // required, missing
	t.Setenv(envDriveRedirectURL, "https://app/cb")

	_, _, err := LoadDriveOAuthConfig()
	if err == nil {
		t.Fatal("LoadDriveOAuthConfig (client id set, secret missing): want error, got nil")
	}
	if !errors.Is(err, ErrMissingRequired) {
		t.Errorf("error = %v, want wrap of ErrMissingRequired", err)
	}
	if got := err.Error(); !strings.Contains(got, envDriveClientSecret) {
		t.Errorf("error %q should name the missing var %q", got, envDriveClientSecret)
	}
}

// TestLoadDriveOAuthConfigPlaceholderSecretOnlyDisabled is the deploy-scenario
// regression for customer-Drive connect: the substrate mounts a non-empty placeholder
// client secret even when the feature is unconfigured. Enablement keys on the
// operator-set, non-secret client id, so a placeholder secret with NO client id must
// report DISABLED (enabled=false, nil error), not error on the empty client id.
func TestLoadDriveOAuthConfigPlaceholderSecretOnlyDisabled(t *testing.T) {
	t.Setenv(envDriveClientID, "") // not configured by the operator
	t.Setenv(envDriveClientSecret, "REPLACE_ME_VIA_GCLOUD")
	t.Setenv(envDriveRedirectURL, "")

	cfg, enabled, err := LoadDriveOAuthConfig()
	if err != nil {
		t.Fatalf("LoadDriveOAuthConfig (placeholder secret only): want nil error, got %v", err)
	}
	if enabled {
		t.Errorf("Drive connect enabled = true with only a placeholder secret set; want disabled. cfg=%+v", cfg)
	}
}

// TestLoadConfigPortParseError verifies a non-integer port is reported as an error
// that names the offending variable and wraps ErrInvalidConfig (the value is
// present but malformed) — NOT ErrMissingRequired, which is reserved for absent
// required values.
func TestLoadConfigPortParseError(t *testing.T) {
	t.Setenv(envAPIHost, "")
	t.Setenv(envAPIPort, "")
	t.Setenv(envPort, "not-a-number")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig: want error for non-integer PORT, got nil")
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("error = %v, want wrap of ErrInvalidConfig", err)
	}
	if errors.Is(err, ErrMissingRequired) {
		t.Errorf("error = %v, should NOT wrap ErrMissingRequired (value is present but invalid)", err)
	}
	if got := err.Error(); !strings.Contains(got, envPort) {
		t.Errorf("error %q should name the offending variable %q", got, envPort)
	}
}

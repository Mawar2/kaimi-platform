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

// TestLoadConfigPortParseError verifies a non-integer port is reported as an error
// that names the offending variable and wraps the ErrMissingRequired sentinel.
func TestLoadConfigPortParseError(t *testing.T) {
	t.Setenv(envAPIHost, "")
	t.Setenv(envAPIPort, "")
	t.Setenv(envPort, "not-a-number")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig: want error for non-integer PORT, got nil")
	}
	if !errors.Is(err, ErrMissingRequired) {
		t.Errorf("error = %v, want wrap of ErrMissingRequired", err)
	}
	if got := err.Error(); !strings.Contains(got, envPort) {
		t.Errorf("error %q should name the offending variable %q", got, envPort)
	}
}

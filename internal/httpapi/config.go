package httpapi

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// ErrMissingRequired is the sentinel wrapped by LoadConfig when a required
// configuration value is absent. Callers can test for it with errors.Is. It
// mirrors config.ErrMissingRequired (the app-wide config package) so the two
// layers report missing values the same way.
var ErrMissingRequired = errors.New("required configuration value is missing")

// Config holds the HTTP/server layer's settings for the JSON API binary. It is
// deliberately separate from the app-wide config.Config (which carries the
// tenant, GCP, store, and agent inputs): this struct owns only what the HTTP
// server itself needs to bind and, in later tickets, to authenticate requests.
//
// Today it carries just the bind address (Host/Port). It is shaped so WS-B4 can
// add OAuth fields (client ID/secret, redirect URL, allowed audience, session
// signing key) without changing the constructor's call sites — new fields get
// their own env vars in LoadConfig and their own validation below.
type Config struct {
	// Host is the interface to bind. Defaults to 127.0.0.1 for local/UI dev; set
	// to 0.0.0.0 in containers/Cloud Run via the API_HOST env var.
	Host string

	// Port is the TCP port to serve on. $PORT (injected by Cloud Run and most
	// container platforms) takes precedence over API_PORT and the built-in
	// default, mirroring cmd/dashboard's port precedence.
	Port int

	// TODO(WS-B4): OAuth fields land here (e.g. OAuthClientID, OAuthClientSecret,
	// OAuthRedirectURL, SessionSigningKey), resolved from their own env vars in
	// LoadConfig and validated against ErrMissingRequired when auth is enabled.
}

const (
	defaultAPIHost = "127.0.0.1"
	defaultAPIPort = 8901

	envAPIHost = "API_HOST"
	envAPIPort = "API_PORT"
	envPort    = "PORT"
)

// LoadConfig resolves the API server Config from the environment, applying the
// precedence env > default. The bind interface comes from API_HOST (default
// 127.0.0.1). The port comes from $PORT if set (the platform-injected variable),
// otherwise API_PORT, otherwise the built-in default; whichever variable supplies
// the port must parse as an integer, and a non-integer value is reported as an
// error naming the offending variable and wrapping ErrMissingRequired.
//
// LoadConfig never reads flags directly: cmd/api parses flags and may override
// the returned Config's fields, keeping flag wiring in the binary and env/default
// resolution here.
func LoadConfig() (Config, error) {
	cfg := Config{
		Host: defaultAPIHost,
		Port: defaultAPIPort,
	}

	if v := os.Getenv(envAPIHost); v != "" {
		cfg.Host = v
	}

	// Port precedence: $PORT (platform) > API_PORT > default. Resolve the
	// effective source so a parse failure can name the variable the operator set.
	if v := os.Getenv(envPort); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("%s must be an integer, got %q: %w", envPort, v, ErrMissingRequired)
		}
		cfg.Port = n
	} else if v := os.Getenv(envAPIPort); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("%s must be an integer, got %q: %w", envAPIPort, v, ErrMissingRequired)
		}
		cfg.Port = n
	}

	return cfg, nil
}

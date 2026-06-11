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

// ErrInvalidConfig is the sentinel wrapped by LoadConfig when a configuration
// value is present but malformed (e.g. a port that is not an integer). It is
// distinct from ErrMissingRequired: that sentinel means a required value is
// absent, whereas this one means a supplied value could not be parsed. Callers
// can test for it with errors.Is.
var ErrInvalidConfig = errors.New("invalid configuration value")

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

	// AllowedOrigins is the explicit CORS allow-list, parsed from the comma-separated
	// CORS_ALLOWED_ORIGINS env var. It is EMPTY by default: the preferred deployment
	// is same-origin (SPA and API behind one host), in which case the CORS middleware
	// is a no-op. Set it only when a browser front end is served from a DIFFERENT
	// origin than the API. Each entry must be a full scheme+host(+port) origin, e.g.
	// "https://app.example.com" — never "*", since the API uses credentialed
	// (cookie) auth and the CORS spec forbids "*" with credentials.
	AllowedOrigins []string
}

// OAuthConfig holds the Google Workspace OAuth2/OIDC settings for sign-in (WS-B4).
// It is loaded separately from Config because auth is optional to construct: the
// offline cmd/api dev mode runs without it. PRODUCTION MUST set every field — the
// secrets (ClientSecret, SessionSecret) come from Secret Manager, surfaced as env
// vars in Cloud Run.
//
// SECURITY: SessionSecret and ClientSecret are credentials and must never be
// logged. LoadOAuthConfig reads them but neither it nor any handler emits them.
type OAuthConfig struct {
	// ClientID is the Google OAuth client id. It is also the audience the callback
	// validates the ID token against.
	ClientID string
	// ClientSecret is the Google OAuth client secret (Secret Manager → env).
	ClientSecret string
	// RedirectURL is this service's absolute /auth/callback URL registered with Google.
	RedirectURL string
	// AllowedDomain is the Google Workspace domain ("hd") permitted to sign in. The
	// callback rejects any account whose verified hd claim differs.
	AllowedDomain string
	// SessionSecret is the HMAC-SHA256 key used to sign session tokens (Secret
	// Manager → env). Rotating it invalidates all existing sessions.
	SessionSecret string
	// PostLoginPath is where a successful login redirects. Defaults to "/" when empty.
	PostLoginPath string
}

const (
	envOAuthClientID      = "OAUTH_CLIENT_ID"
	envOAuthClientSecret  = "OAUTH_CLIENT_SECRET"
	envOAuthRedirectURL   = "OAUTH_REDIRECT_URL"
	envOAuthAllowedDomain = "OAUTH_ALLOWED_DOMAIN"
	envOAuthSessionSecret = "SESSION_SECRET"
	envOAuthPostLoginPath = "OAUTH_POST_LOGIN_PATH"
)

// LoadOAuthConfig resolves the Workspace OAuth settings from the environment.
//
// Auth is OPTIONAL: if NONE of the OAUTH_* / SESSION_SECRET variables are set, it
// returns (zero, false, nil) so the offline cmd/api dev mode still builds and runs
// with auth disabled. If ANY of them is set — signaling intent to enable auth —
// then EVERY required value (client id, client secret, redirect URL, allowed
// domain, session secret) must be present; a missing one returns an error naming
// the variable and wrapping ErrMissingRequired. The returned bool reports whether
// auth is enabled.
func LoadOAuthConfig() (OAuthConfig, bool, error) {
	cfg := OAuthConfig{
		ClientID:      os.Getenv(envOAuthClientID),
		ClientSecret:  os.Getenv(envOAuthClientSecret),
		RedirectURL:   os.Getenv(envOAuthRedirectURL),
		AllowedDomain: os.Getenv(envOAuthAllowedDomain),
		SessionSecret: os.Getenv(envOAuthSessionSecret),
		PostLoginPath: os.Getenv(envOAuthPostLoginPath),
	}

	// Required values keyed by their env var, so an error can name the missing one.
	required := []struct {
		env, val string
	}{
		{envOAuthClientID, cfg.ClientID},
		{envOAuthClientSecret, cfg.ClientSecret},
		{envOAuthRedirectURL, cfg.RedirectURL},
		{envOAuthAllowedDomain, cfg.AllowedDomain},
		{envOAuthSessionSecret, cfg.SessionSecret},
	}

	anySet := false
	for _, r := range required {
		if r.val != "" {
			anySet = true
		}
	}
	if !anySet {
		// Nothing configured: auth disabled. Offline/dev mode path.
		return OAuthConfig{}, false, nil
	}

	// At least one OAuth var is set, so the operator intends auth. Demand all of them.
	for _, r := range required {
		if r.val == "" {
			return OAuthConfig{}, false, fmt.Errorf("%s must be set when OAuth is enabled: %w", r.env, ErrMissingRequired)
		}
	}

	return cfg, true, nil
}

const (
	envDriveClientID        = "DRIVE_OAUTH_CLIENT_ID"
	envDriveClientSecret    = "DRIVE_OAUTH_CLIENT_SECRET"
	envDriveRedirectURL     = "DRIVE_OAUTH_REDIRECT_URL"
	envDrivePostConnectPath = "DRIVE_OAUTH_POST_CONNECT_PATH"
)

// LoadDriveOAuthConfig resolves the WS-C2 customer-Drive OAuth settings from the
// environment. It is OPTIONAL and INDEPENDENT of sign-in OAuth (a deployment may
// enable one without the other): if NONE of the DRIVE_OAUTH_* variables are set it
// returns (zero, false, nil) so the connect endpoints are simply not wired (they
// then answer 503). If ANY is set — signaling intent to enable customer-Drive
// connect — then EVERY required value (client id, client secret, redirect URL) must
// be present; a missing one returns an error naming the variable and wrapping
// ErrMissingRequired. The returned bool reports whether the feature is enabled.
//
// SECURITY: ClientSecret is a credential; this function reads it but never logs it.
func LoadDriveOAuthConfig() (DriveOAuthConfig, bool, error) {
	cfg := DriveOAuthConfig{
		ClientID:        os.Getenv(envDriveClientID),
		ClientSecret:    os.Getenv(envDriveClientSecret),
		RedirectURL:     os.Getenv(envDriveRedirectURL),
		PostConnectPath: os.Getenv(envDrivePostConnectPath),
	}

	required := []struct{ env, val string }{
		{envDriveClientID, cfg.ClientID},
		{envDriveClientSecret, cfg.ClientSecret},
		{envDriveRedirectURL, cfg.RedirectURL},
	}

	anySet := false
	for _, r := range required {
		if r.val != "" {
			anySet = true
		}
	}
	if !anySet {
		return DriveOAuthConfig{}, false, nil
	}
	for _, r := range required {
		if r.val == "" {
			return DriveOAuthConfig{}, false, fmt.Errorf("%s must be set when customer-Drive connect is enabled: %w", r.env, ErrMissingRequired)
		}
	}
	return cfg, true, nil
}

const (
	defaultAPIHost = "127.0.0.1"
	defaultAPIPort = 8901

	envAPIHost     = "API_HOST"
	envAPIPort     = "API_PORT"
	envPort        = "PORT"
	envCORSOrigins = "CORS_ALLOWED_ORIGINS"
)

// LoadConfig resolves the API server Config from the environment, applying the
// precedence env > default. The bind interface comes from API_HOST (default
// 127.0.0.1). The port comes from $PORT if set (the platform-injected variable),
// otherwise API_PORT, otherwise the built-in default; whichever variable supplies
// the port must parse as an integer, and a non-integer value is reported as an
// error naming the offending variable and wrapping ErrInvalidConfig (the value is
// present but malformed, not absent).
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
			return Config{}, fmt.Errorf("%s must be an integer, got %q: %w", envPort, v, ErrInvalidConfig)
		}
		cfg.Port = n
	} else if v := os.Getenv(envAPIPort); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("%s must be an integer, got %q: %w", envAPIPort, v, ErrInvalidConfig)
		}
		cfg.Port = n
	}

	// CORS allow-list: empty unless CORS_ALLOWED_ORIGINS is set. parseCORSOrigins
	// trims and drops blanks so a trailing comma never yields a "" origin.
	cfg.AllowedOrigins = parseCORSOrigins(os.Getenv(envCORSOrigins))

	return cfg, nil
}

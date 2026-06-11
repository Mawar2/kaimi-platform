package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"golang.org/x/oauth2"

	"github.com/Mawar2/Kaimi/internal/drivetoken"
	"github.com/Mawar2/Kaimi/internal/googledocs"
)

// defaultDriveFolderName is the name of the folder auto-created in the customer's
// Drive when they connect (WS-C5a), so generated proposal Docs have a tidy home
// without the user having to pick a destination first.
const defaultDriveFolderName = "Kaimi Proposals"

// This file implements the WS-C2 customer-Drive connect flow: the endpoints that
// let a deployment connect the CUSTOMER's own Google Workspace/Drive so generated
// proposal Docs land in THEIR Drive instead of a BlueMeta service account.
//
// SECURITY: this handler obtains and stores the customer's OAuth tokens.
//   - It requests the MINIMAL scopes only (drive.file + documents — never full
//     drive); see drivetoken.NewOAuthConfig.
//   - access_type=offline + prompt=consent so Google returns a refresh token.
//   - The connect → callback handshake is CSRF-protected with a state cookie
//     (constant-time compare) and bound with PKCE, mirroring the WS-B4 login flow.
//   - Tokens are stored ONLY server-side via the 0o600 drivetoken.TokenStore and
//     are NEVER logged. The status endpoint reports connected/target — never the
//     token.
//
// The token exchange is behind the `exchange` seam so unit tests run fully offline.

// Per-connect temporary cookies, hardened the same way the WS-B4 login cookies are
// (__Host- prefix → browser-enforced Secure + Path=/ + no Domain). They live only
// between /connect and /callback to bind the redirect to this client.
const (
	driveStateCookieName = "__Host-kaimi_drive_state"
	drivePKCECookieName  = "__Host-kaimi_drive_pkce"
)

// driveExchangeFunc exchanges an authorization code (with its PKCE verifier) for a
// token. The default delegates to oauth2.Config.Exchange; tests inject a fake. It
// mirrors auth.go's exchangeFunc seam.
type driveExchangeFunc func(ctx context.Context, code, verifier string) (*oauth2.Token, error)

// driveProvisionFunc finds-or-creates a destination folder in the customer's Drive
// and returns its id. The default delegates to googledocs.EnsureFolder; tests inject
// a fake so the auto-provision-on-connect path (WS-C5a) runs fully offline. It
// mirrors the driveExchangeFunc seam above.
type driveProvisionFunc func(ctx context.Context, ts oauth2.TokenSource, name string) (folderID string, err error)

// DriveOAuthConfig holds the Google OAuth client settings for the customer-Drive
// connect flow (WS-C2). It is loaded separately from the sign-in OAuthConfig
// (WS-B4): connecting a Drive is a different consent (different scopes, different
// callback) than signing in, and a deployment may enable one without the other.
//
// SECURITY: ClientSecret is a credential and must never be logged.
type DriveOAuthConfig struct {
	// ClientID is the Google OAuth client id used for the Drive consent.
	ClientID string
	// ClientSecret is the Google OAuth client secret (Secret Manager → env).
	ClientSecret string
	// RedirectURL is this service's absolute .../drive/callback URL registered with
	// Google.
	RedirectURL string
	// PostConnectPath is where a successful connect redirects (defaults to "/").
	PostConnectPath string
}

// DriveHandler serves the /api/v1/integrations/drive/* endpoints. It is built once
// at startup and shared across requests (its fields are read-only after
// construction except the stores, which are themselves concurrency-safe), so it is
// safe for concurrent use.
type DriveHandler struct {
	cfg     DriveOAuthConfig
	oauth   *oauth2.Config
	tokens  drivetoken.TokenStore
	targets drivetoken.TargetStore

	// exchange defaults to the real code exchange; tests override it to stay offline.
	exchange driveExchangeFunc

	// provision defaults to creating a real Drive folder via googledocs.EnsureFolder;
	// tests override it to stay offline. It backs the auto-provision-on-connect flow.
	provision driveProvisionFunc
}

// NewDriveHandler builds the customer-Drive connect handler from its OAuth config
// and the per-tenant token/target stores. It is the exported constructor cmd/api
// calls when customer-Drive connect is enabled; the result goes in Deps.Drive.
func NewDriveHandler(cfg DriveOAuthConfig, tokens drivetoken.TokenStore, targets drivetoken.TargetStore) (*DriveHandler, error) {
	return newDriveHandler(cfg, tokens, targets)
}

// newDriveHandler builds a DriveHandler, defaulting the exchange seam to the real
// Google call. It requires the stores so the flow can persist what it obtains.
func newDriveHandler(cfg DriveOAuthConfig, tokens drivetoken.TokenStore, targets drivetoken.TargetStore) (*DriveHandler, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RedirectURL == "" {
		return nil, errors.New("drive oauth config requires client id, client secret, and redirect URL")
	}
	if tokens == nil || targets == nil {
		return nil, errors.New("drive handler requires a token store and a target store")
	}
	oc := drivetoken.NewOAuthConfig(cfg.ClientID, cfg.ClientSecret, cfg.RedirectURL)
	h := &DriveHandler{
		cfg:     cfg,
		oauth:   oc,
		tokens:  tokens,
		targets: targets,
	}
	h.exchange = func(ctx context.Context, code, verifier string) (*oauth2.Token, error) {
		return oc.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	}
	// googledocs.EnsureFolder matches driveProvisionFunc exactly, so it is the seam's
	// real implementation directly; tests swap in a fake to stay offline.
	h.provision = googledocs.EnsureFolder
	return h, nil
}

// handleConnect starts the customer-Drive consent flow. It generates a fresh CSRF
// state and PKCE verifier, parks both in short-lived hardened cookies, and
// redirects to Google's consent screen requesting the MINIMAL Drive scopes with
// access_type=offline + prompt=consent so Google returns a refresh token.
func (h *DriveHandler) handleConnect(w http.ResponseWriter, r *http.Request) {
	state, err := randomToken()
	if err != nil {
		// crypto/rand failure is exceptional; do not start a flow without CSRF state.
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	verifier := oauth2.GenerateVerifier()

	setTempCookie(w, driveStateCookieName, state)
	setTempCookie(w, drivePKCECookieName, verifier)

	// AccessTypeOffline + prompt=consent are REQUIRED to obtain a refresh token:
	// without them Google returns only a short-lived access token and the
	// auto-refreshing TokenSource would stop working after ~1 hour.
	authURL := h.oauth.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("prompt", "consent"),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleCallback completes the consent flow: it verifies the CSRF state
// (constant-time) against the cookie, exchanges the code (binding the PKCE
// verifier), and stores the returned token (incl refresh token) via the token
// store. It never logs the code or token. Any state/exchange failure returns 400
// and stores nothing.
func (h *DriveHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	// The temp cookies have served their purpose; clear them on EVERY exit path.
	clearTempCookie(w, driveStateCookieName)
	clearTempCookie(w, drivePKCECookieName)

	stateCookie, err := r.Cookie(driveStateCookieName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing state")
		return
	}
	stateParam := r.URL.Query().Get("state")
	if stateParam == "" || subtle.ConstantTimeCompare([]byte(stateParam), []byte(stateCookie.Value)) != 1 {
		writeError(w, http.StatusBadRequest, "state mismatch")
		return
	}
	pkceCookie, err := r.Cookie(drivePKCECookieName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing pkce verifier")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing code")
		return
	}

	tok, err := h.exchange(r.Context(), code, pkceCookie.Value)
	if err != nil {
		// Do not log the code or token; only that the exchange failed.
		log.Printf("httpapi: drive oauth code exchange failed: %v", err)
		writeError(w, http.StatusBadRequest, "code exchange failed")
		return
	}

	// Persist the token (access + refresh) server-side. Never log it.
	if err := h.tokens.Save(tok); err != nil {
		log.Printf("httpapi: failed to persist drive token: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to store drive connection")
		return
	}

	// WS-C5a: now that the Drive is connected, ensure a default destination folder
	// exists so generated Docs have a home without the user picking one first. This
	// runs AFTER the token is already persisted, so it is strictly best-effort: a
	// provisioning or target-save failure must NOT fail the connect — the user can
	// still set a target later (WS-C5b). It is also idempotent: if a target is
	// already set, we never overwrite the user's choice or make a second folder.
	h.ensureDefaultTarget(r.Context(), tok)

	dest := h.cfg.PostConnectPath
	if dest == "" {
		dest = "/"
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// ensureDefaultTarget auto-creates the default "Kaimi Proposals" folder in the
// customer's Drive and persists it as the target, but ONLY when no target has been
// set yet (WS-C5a). It is best-effort by contract: the token is already persisted by
// the time this runs, so any error here is logged as a warning and swallowed — the
// connect still succeeds and the user can set a target later (WS-C5b). It never logs
// the token.
//
// Idempotency comes from two layers: (1) if a target is already set we return
// immediately without provisioning, so a reconnect never makes a second folder or
// clobbers a user's chosen target; (2) googledocs.EnsureFolder itself reuses an
// existing app-created folder of the same name rather than duplicating it.
func (h *DriveHandler) ensureDefaultTarget(ctx context.Context, tok *oauth2.Token) {
	// If a target already exists, do nothing — never overwrite the user's choice.
	if _, err := h.targets.Load(); err == nil {
		return
	} else if !errors.Is(err, drivetoken.ErrNotConnected) {
		// An I/O/parse error reading the target is not a clean "unset"; do not risk
		// provisioning a duplicate folder on top of an unreadable target. Log and bail.
		log.Printf("httpapi: skipping drive folder auto-create; could not read existing target: %v", err)
		return
	}

	// Build a refreshing token source from the just-obtained token and provision the
	// default folder. oauth2.Config.TokenSource auto-refreshes via the refresh token.
	ts := h.oauth.TokenSource(ctx, tok)
	folderID, err := h.provision(ctx, ts, defaultDriveFolderName)
	if err != nil {
		// Best-effort: do not fail the connect. Do not log the token, only the error.
		log.Printf("httpapi: best-effort drive folder auto-create failed; connect still succeeds, user can set a target later: %v", err)
		return
	}

	if err := h.targets.Save(drivetoken.Target{DriveID: folderID, Name: defaultDriveFolderName}); err != nil {
		// Best-effort: the folder exists but we could not persist it as the target.
		// EnsureFolder will reuse that same folder on a later attempt, so no duplicate.
		log.Printf("httpapi: drive folder auto-created but persisting it as the target failed; user can set a target later: %v", err)
		return
	}
}

// handleStatus reports whether the customer's Drive is connected and, if a target
// has been set, which Drive/folder Docs will land in. It NEVER returns the token.
func (h *DriveHandler) handleStatus(w http.ResponseWriter, _ *http.Request) {
	connected := true
	if _, err := h.tokens.Load(); err != nil {
		if errors.Is(err, drivetoken.ErrNotConnected) {
			connected = false
		} else {
			// An I/O/parse failure is not a clean "not connected"; report 500.
			writeError(w, http.StatusInternalServerError, "failed to read drive connection")
			return
		}
	}

	var target string
	if t, err := h.targets.Load(); err == nil {
		target = t.DriveID
	} else if !errors.Is(err, drivetoken.ErrNotConnected) {
		writeError(w, http.StatusInternalServerError, "failed to read drive target")
		return
	}

	writeJSON(w, http.StatusOK, driveStatusResponse{Connected: connected, Target: target})
}

// handleSetTarget persists the target Drive/folder id where Docs should be created.
// The full interactive Drive picker is WS-C3; here a provided id is simply stored.
func (h *DriveHandler) handleSetTarget(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req driveTargetRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid target JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(req.DriveID) == "" {
		writeError(w, http.StatusBadRequest, "drive_id is required")
		return
	}

	if err := h.targets.Save(drivetoken.Target{DriveID: req.DriveID}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store drive target")
		return
	}
	writeJSON(w, http.StatusOK, driveStatusResponse{Connected: h.isConnected(), Target: req.DriveID})
}

// isConnected reports whether a token has been stored, treating any non
// ErrNotConnected load error as "not connected" for this advisory field (the
// authoritative read path is handleStatus).
func (h *DriveHandler) isConnected() bool {
	_, err := h.tokens.Load()
	return err == nil
}

// The four Server methods below dispatch to the DriveHandler, degrading to 503 when
// customer-Drive connect is not configured (Deps.Drive == nil) — mirroring how the
// profile/select endpoints degrade when their dependency is absent. They are the
// route targets registered on apiMux in Routes().

// handleDriveConnect dispatches GET .../drive/connect.
func (s *Server) handleDriveConnect(w http.ResponseWriter, r *http.Request) {
	if s.deps.Drive == nil {
		writeError(w, http.StatusServiceUnavailable, "customer Drive connect is not available")
		return
	}
	s.deps.Drive.handleConnect(w, r)
}

// handleDriveCallback dispatches GET .../drive/callback.
func (s *Server) handleDriveCallback(w http.ResponseWriter, r *http.Request) {
	if s.deps.Drive == nil {
		writeError(w, http.StatusServiceUnavailable, "customer Drive connect is not available")
		return
	}
	s.deps.Drive.handleCallback(w, r)
}

// handleDriveStatus dispatches GET .../drive/status.
func (s *Server) handleDriveStatus(w http.ResponseWriter, r *http.Request) {
	if s.deps.Drive == nil {
		writeError(w, http.StatusServiceUnavailable, "customer Drive connect is not available")
		return
	}
	s.deps.Drive.handleStatus(w, r)
}

// handleDriveSetTarget dispatches PUT .../drive/target.
func (s *Server) handleDriveSetTarget(w http.ResponseWriter, r *http.Request) {
	if s.deps.Drive == nil {
		writeError(w, http.StatusServiceUnavailable, "customer Drive connect is not available")
		return
	}
	s.deps.Drive.handleSetTarget(w, r)
}

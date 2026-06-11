package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"
)

// This file implements Workspace OAuth2/OIDC sign-in (WS-B4): login → Google →
// callback → signed session cookie, restricted to a single Google Workspace
// domain. It is the LOGIN FLOW + SESSION MINTING only; guarding /api/v1 with the
// session is WS-B5, which calls sessionManager.ParseSession (see session.go).
//
// Security enforcement happens entirely in handleCallback, in this order:
//  1. The CSRF "state" param must match the state cookie (constant-time compare).
//  2. The authorization code is exchanged with the PKCE verifier from the cookie.
//  3. The returned ID token is verified by idtoken.Validate — Google's signature,
//     aud == our client id, and iss.
//  4. The verified payload's "hd" claim must equal the allowed Workspace domain
//     AND "email_verified" must be true.
// Only if ALL pass is a session cookie minted. Any failure returns 403 (domain /
// email) or 400 (state / exchange) and NEVER sets a session.
//
// Two seams keep unit tests fully offline:
//   - exchange: swaps oauth2.Config.Exchange for a fake token source.
//   - verifyIDToken: swaps idtoken.Validate for a fake payload.

// Temporary per-login cookies. They are short-lived, HttpOnly+Secure, and exist
// only between /auth/login and /auth/callback to bind the redirect to this client.
// The __Host- prefix is browser-enforced hardening (requires Secure + Path=/ + no
// Domain) that defeats subdomain cookie-tossing; setTempCookie/clearTempCookie
// satisfy those constraints. SameSite=Lax still lets these cookies ride the
// top-level GET redirect Google sends back to /auth/callback, so the flow works.
const (
	stateCookieName = "__Host-kaimi_oauth_state"
	pkceCookieName  = "__Host-kaimi_oauth_pkce"

	// tempCookieMaxAge bounds how long a login may stay in flight (seconds).
	tempCookieMaxAge = 600 // 10 minutes

	// oauthScopes requests the OpenID identity claims we need: the subject (openid),
	// the email, and (implicitly) the hd domain hint for Workspace accounts.
	scopeOpenID  = "openid"
	scopeEmail   = "email"
	scopeProfile = "profile"
)

// exchangeFunc exchanges an authorization code (with its PKCE verifier) for a
// token. The default delegates to oauth2.Config.Exchange; tests inject a fake.
type exchangeFunc func(ctx context.Context, code, verifier string) (*oauth2.Token, error)

// verifyFunc validates a raw ID token against an audience and returns its payload.
// The default delegates to idtoken.Validate (which checks Google's signature, the
// audience, and the issuer); tests inject a fake to stay offline.
type verifyFunc func(ctx context.Context, rawIDToken, audience string) (*idtoken.Payload, error)

// AuthHandler serves the /auth/* endpoints. It is constructed once at startup and
// shared across requests (its fields are read-only after construction), so it is
// safe for concurrent use.
type AuthHandler struct {
	cfg     OAuthConfig
	oauth   *oauth2.Config
	session *sessionManager

	// Seams (default to the real Google calls; tests override them).
	exchange      exchangeFunc
	verifyIDToken verifyFunc
}

// defaultSessionTTL is how long a minted session stays valid. Kept modest so a
// leaked cookie has a bounded window; the user simply signs in again at expiry.
const defaultSessionTTL = 12 * time.Hour

// NewAuthHandler builds the /auth/* handler from OAuth config, wiring a session
// manager from the config's SessionSecret. It is the exported constructor cmd/api
// calls when OAuth is enabled; the returned *AuthHandler is placed in Deps.Auth.
// The handler's session manager also backs the WS-B5 middleware (ParseSession).
func NewAuthHandler(cfg *OAuthConfig) (*AuthHandler, error) {
	if cfg.SessionSecret == "" {
		return nil, fmt.Errorf("session secret is required to build the auth handler: %w", ErrMissingRequired)
	}
	sm := newSessionManager([]byte(cfg.SessionSecret), defaultSessionTTL)
	return newAuthHandler(cfg, sm)
}

// newAuthHandler builds an AuthHandler from validated OAuth config and the session
// manager. The returned handler defaults its seams to the live Google calls; tests
// replace exchange/verifyIDToken to run offline.
func newAuthHandler(cfg *OAuthConfig, session *sessionManager) (*AuthHandler, error) {
	// Normalize the allowed domain once: DNS domains are case-insensitive, so a
	// configured "Example.com" must accept an hd claim of "example.com" (and vice
	// versa). We lowercase here and lowercase the hd claim at compare time so the
	// constant-time check never produces a false 403 over letter case.
	normalized := *cfg
	normalized.AllowedDomain = strings.ToLower(cfg.AllowedDomain)
	cfg = &normalized

	oc := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     google.Endpoint,
		Scopes:       []string{scopeOpenID, scopeEmail, scopeProfile},
	}
	ah := &AuthHandler{
		cfg:     *cfg,
		oauth:   oc,
		session: session,
	}
	// Default seam: real code exchange. PKCE verifier is supplied via VerifierOption.
	ah.exchange = func(ctx context.Context, code, verifier string) (*oauth2.Token, error) {
		return oc.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	}
	// Default seam: real ID-token verification (validates Google's signature, aud, iss).
	ah.verifyIDToken = idtoken.Validate
	return ah, nil
}

// handleLogin starts the OAuth flow: it generates a fresh CSRF state and a PKCE
// verifier, stores both in short-lived secure cookies, and redirects to Google's
// consent screen. The consent URL carries the state, the S256 PKCE challenge, and
// the hd hint constraining the account chooser to the allowed Workspace domain.
func (a *AuthHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := randomToken()
	if err != nil {
		// crypto/rand failure is exceptional; do not start a flow without CSRF state.
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	verifier := oauth2.GenerateVerifier()

	setTempCookie(w, stateCookieName, state)
	setTempCookie(w, pkceCookieName, verifier)

	// hd constrains the chooser to the Workspace domain; it is a HINT, not a
	// security control — the callback re-checks hd from the verified ID token.
	authURL := a.oauth.AuthCodeURL(
		state,
		oauth2.AccessTypeOnline,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("hd", a.cfg.AllowedDomain),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleCallback completes the OAuth flow and enforces every security check before
// minting a session. See the file header for the ordered enforcement steps.
func (a *AuthHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	// The temp state/pkce cookies have served their purpose the moment this handler
	// runs; clear them on EVERY exit path so a half-finished or rejected login never
	// leaves them lingering. We emit the deletions up front (before any validation or
	// error response is written) so they ride out with whatever status we return.
	clearTempCookie(w, stateCookieName)
	clearTempCookie(w, pkceCookieName)

	// Step 1: CSRF — the state param must match the state cookie (constant-time).
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil {
		http.Error(w, "missing state", http.StatusBadRequest)
		return
	}
	stateParam := r.URL.Query().Get("state")
	if stateParam == "" || subtle.ConstantTimeCompare([]byte(stateParam), []byte(stateCookie.Value)) != 1 {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}
	pkceCookie, err := r.Cookie(pkceCookieName)
	if err != nil {
		http.Error(w, "missing pkce verifier", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	// Step 2: exchange the code (binding the PKCE verifier).
	tok, err := a.exchange(r.Context(), code, pkceCookie.Value)
	if err != nil {
		// Do not log the code or token; only that the exchange failed.
		log.Printf("httpapi: oauth code exchange failed: %v", err)
		http.Error(w, "code exchange failed", http.StatusBadRequest)
		return
	}
	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		http.Error(w, "no id token in response", http.StatusBadRequest)
		return
	}

	// Step 3: verify the ID token (Google signature, aud == our client id, iss).
	payload, err := a.verifyIDToken(r.Context(), rawIDToken, a.cfg.ClientID)
	if err != nil {
		log.Printf("httpapi: oauth id-token verification failed: %v", err)
		http.Error(w, "id token verification failed", http.StatusBadRequest)
		return
	}

	// Step 4: enforce the Workspace domain and that the email is verified. The hd
	// claim is lowercased to match the already-lowercased AllowedDomain (DNS domains
	// are case-insensitive); the compare stays constant-time for consistency.
	hd, _ := payload.Claims["hd"].(string)
	hd = strings.ToLower(hd)
	if hd == "" || subtle.ConstantTimeCompare([]byte(hd), []byte(a.cfg.AllowedDomain)) != 1 {
		// Wrong/absent Workspace domain — outside the tenant. Never mint a session.
		http.Error(w, "account is not in the allowed Workspace domain", http.StatusForbidden)
		return
	}
	if verified, _ := payload.Claims["email_verified"].(bool); !verified {
		http.Error(w, "email is not verified", http.StatusForbidden)
		return
	}

	email, _ := payload.Claims["email"].(string)

	// All checks passed — mint the session and redirect to the post-login path.
	a.session.SetSession(w, Session{
		Subject: payload.Subject,
		Email:   email,
		Domain:  hd,
	})
	dest := a.cfg.PostLoginPath
	if dest == "" {
		dest = "/"
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// handleLogout clears the session cookie. It does not call Google's revocation
// endpoint — the local session is the trust boundary, and clearing it ends access
// (the WS-B5 middleware rejects any request without a valid session cookie).
func (a *AuthHandler) handleLogout(w http.ResponseWriter, _ *http.Request) {
	a.session.ClearSession(w)
	w.WriteHeader(http.StatusNoContent)
}

// setTempCookie writes a short-lived, hardened cookie used only between the start
// of an OAuth flow and its callback. HttpOnly+Secure+SameSite=Lax mirror the
// session cookie's flags. Path MUST be "/" (not a sub-path): the __Host- prefix the
// cookie names carry is only honored by browsers when Path=/ and no Domain is set.
// It is a package function (not a method) so both the WS-B4 sign-in flow and the
// WS-C2 Drive-connect flow share one hardened implementation.
func setTempCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   tempCookieMaxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearTempCookie expires a temp OAuth-flow cookie. Path MUST match the Path used
// when the cookie was set ("/") so the deletion targets the same __Host- cookie.
func clearTempCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// randomToken returns 32 bytes of cryptographically secure randomness as a
// base64url string, used for the CSRF state value.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

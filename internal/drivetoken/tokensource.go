package drivetoken

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

// OAuth scopes requested for the customer-Drive connection. We request ONLY
// drive.file: a minimal, non-sensitive scope that needs no Google verification.
// The app may only touch files it creates itself — it can NEVER see or modify
// the customer's existing Drive contents. Doc creation no longer uses the Docs
// API (Docs are created by uploading rendered HTML via Drive with conversion),
// so the sensitive `documents` scope is deliberately not requested.
const (
	// ScopeDriveFile lets the app create and manage ONLY files it creates itself
	// (drive.file), not the user's existing Drive contents.
	ScopeDriveFile = drive.DriveFileScope

	// scopeFullDrive is the broad full-Drive scope we deliberately do NOT request.
	// It is named only so tests can assert its absence.
	scopeFullDrive = drive.DriveScope
)

// OAuthClient holds the Google OAuth client credentials needed to refresh a
// connected customer's Drive token. It is a small, dependency-free carrier so
// callers (e.g. proposalwiring) can supply credentials without importing the
// httpapi handler config. ClientSecret is a credential and must never be logged.
type OAuthClient struct {
	ClientID     string
	ClientSecret string
	// RedirectURL is required to build an oauth2.Config; it is unused on the refresh
	// path (no new consent happens), so the connect handler's callback URL is fine.
	RedirectURL string
}

// NewOAuthConfig builds the oauth2.Config for the customer-Drive connect flow. It
// requests the minimal drive.file scope against Google's endpoint with the given
// client credentials and this service's callback URL. The same config is used both
// to build the consent URL and to exchange/refresh tokens, so the scope stays
// consistent across connect, callback, and auto-refresh.
func NewOAuthConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     google.Endpoint,
		Scopes:       []string{ScopeDriveFile},
	}
}

// TokenSourceFromStore builds an oauth2.TokenSource seeded from the token in the
// store. The returned source AUTO-REFRESHES: when the access token expires it uses
// the stored refresh token (via the oauth2.Config) to mint a new one, so callers
// never re-prompt the user. It returns ErrNotConnected (wrapped) when no token has
// been stored yet, so the caller can fall back to the service-account Docs client.
//
// Note: oauth2.Config.TokenSource refreshes in memory; a refreshed access token is
// not written back to the store here. The refresh token (the durable secret)
// remains valid, so the next process start re-derives a working source from it.
func TokenSourceFromStore(ctx context.Context, store TokenStore, oc *oauth2.Config) (oauth2.TokenSource, error) {
	tok, err := store.Load()
	if err != nil {
		// Propagate ErrNotConnected unwrapped-of-context so errors.Is still matches.
		return nil, fmt.Errorf("load stored drive token: %w", err)
	}
	return oc.TokenSource(ctx, tok), nil
}

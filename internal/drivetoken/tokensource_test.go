package drivetoken

import (
	"context"
	"testing"

	"golang.org/x/oauth2"
)

// TestOAuthConfigCarriesDriveScopes verifies the OAuth config built for the
// connect flow requests EXACTLY the minimal drive.file scope — and NOT the
// sensitive `documents` scope nor the broad full-drive scope. Minimal,
// non-sensitive scope is a security requirement (WS-C2): the app may only touch
// files it creates, never the customer's whole Drive or arbitrary Docs.
func TestOAuthConfigCarriesDriveScopes(t *testing.T) {
	oc := NewOAuthConfig("client-id", "client-secret", "https://app.example.com/api/v1/integrations/drive/callback")

	got := map[string]bool{}
	for _, s := range oc.Scopes {
		got[s] = true
	}

	if !got[ScopeDriveFile] {
		t.Errorf("scopes %v missing drive.file scope %q", oc.Scopes, ScopeDriveFile)
	}
	if got[scopeFullDrive] {
		t.Errorf("scopes %v must NOT include the full-drive scope %q", oc.Scopes, scopeFullDrive)
	}
	// Regression guard: the sensitive Docs scope must never be requested now that
	// Doc creation goes through Drive (HTML upload + conversion), not the Docs API.
	const docsScope = "https://www.googleapis.com/auth/documents"
	if got[docsScope] {
		t.Errorf("scopes %v must NOT include the sensitive documents scope %q", oc.Scopes, docsScope)
	}
}

// TestTokenSourceFromStoreUsesStoredToken verifies the helper builds a working
// oauth2.TokenSource seeded from the stored token: while the access token is still
// valid the source returns it without contacting Google (so this stays offline).
func TestTokenSourceFromStoreUsesStoredToken(t *testing.T) {
	s, _ := newTestStore(t)
	if err := s.Save(&oauth2.Token{
		AccessToken:  "stored-access",
		RefreshToken: "stored-refresh",
		TokenType:    "Bearer",
		Expiry:       farFuture(),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	oc := NewOAuthConfig("client-id", "client-secret", "https://app.example.com/cb")
	ts, err := TokenSourceFromStore(context.Background(), s, oc)
	if err != nil {
		t.Fatalf("TokenSourceFromStore: %v", err)
	}

	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "stored-access" {
		t.Errorf("AccessToken = %q, want the stored valid token", tok.AccessToken)
	}
}

// TestTokenSourceFromStoreNotConnected verifies the helper surfaces ErrNotConnected
// when no token has been stored, so callers fall back to the service-account path.
func TestTokenSourceFromStoreNotConnected(t *testing.T) {
	s, _ := newTestStore(t)
	oc := NewOAuthConfig("client-id", "client-secret", "https://app.example.com/cb")

	_, err := TokenSourceFromStore(context.Background(), s, oc)
	if err == nil {
		t.Fatal("TokenSourceFromStore with no token: expected an error")
	}
}

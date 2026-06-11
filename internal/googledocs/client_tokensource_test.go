package googledocs

import (
	"context"
	"testing"

	"golang.org/x/oauth2"
)

// fakeTokenSource is a no-network oauth2.TokenSource that records whether it was
// asked for a token. Building a live Drive/Docs service does not call Token()
// eagerly, so the assertions below check that the TokenSource branch is *selected*
// (the client builds successfully without CredentialsJSON or ADC), not that a
// token was minted — minting one would require a real OAuth round-trip.
type fakeTokenSource struct {
	called bool
}

func (f *fakeTokenSource) Token() (*oauth2.Token, error) {
	f.called = true
	return &oauth2.Token{AccessToken: "fake-access-token"}, nil
}

// TestNewLiveClientUsesTokenSourceWhenSet verifies that a Config carrying a
// TokenSource builds a live client WITHOUT requiring CredentialsJSON or UseADC —
// proving the TokenSource branch is taken. The other credential fields are left
// empty on purpose: if the TokenSource branch were not honored, newLiveClient
// would fail demanding credentials.
func TestNewLiveClientUsesTokenSourceWhenSet(t *testing.T) {
	ts := &fakeTokenSource{}
	client, err := NewClient(context.Background(), Config{
		SharedDriveID: "drive-123",
		TokenSource:   ts,
		// Deliberately NO CredentialsJSON and UseADC=false: only the TokenSource
		// can satisfy authentication, so a successful build proves it was used.
	})
	if err != nil {
		t.Fatalf("NewClient with TokenSource: unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("NewClient returned nil client")
	}
}

// TestTokenSourceTakesPrecedenceOverCredentials verifies that when both a
// TokenSource and CredentialsJSON are present, the build still succeeds via the
// TokenSource. The supplied CredentialsJSON is intentionally invalid: if the
// CredentialsJSON branch were taken instead, option.WithCredentialsJSON would
// reject the garbage and the build would fail.
func TestTokenSourceTakesPrecedenceOverCredentials(t *testing.T) {
	ts := &fakeTokenSource{}
	client, err := NewClient(context.Background(), Config{
		SharedDriveID:   "drive-123",
		TokenSource:     ts,
		CredentialsJSON: []byte("this-is-not-valid-credentials-json"),
	})
	if err != nil {
		t.Fatalf("NewClient (TokenSource should win over bad CredentialsJSON): %v", err)
	}
	if client == nil {
		t.Fatal("NewClient returned nil client")
	}
}

// TestTokenSourceRequiresSharedDriveID confirms the existing SharedDriveID
// requirement still holds on the TokenSource path (the target Drive is where the
// customer's Docs land).
func TestTokenSourceRequiresSharedDriveID(t *testing.T) {
	_, err := NewClient(context.Background(), Config{
		TokenSource: &fakeTokenSource{},
	})
	if err == nil {
		t.Fatal("NewClient without SharedDriveID: expected an error, got nil")
	}
}

// TestCachedModeIgnoresTokenSource confirms the cached path is unchanged: a
// TokenSource on a cached config has no effect and no network is touched.
func TestCachedModeIgnoresTokenSource(t *testing.T) {
	client, err := NewClient(context.Background(), Config{
		UseCached:   true,
		TokenSource: &fakeTokenSource{},
	})
	if err != nil {
		t.Fatalf("NewClient cached with TokenSource: %v", err)
	}
	if _, ok := client.(*cachedClient); !ok {
		t.Fatalf("cached config did not yield a cached client, got %T", client)
	}
}

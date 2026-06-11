package httpapi

import (
	"context"
	"testing"
)

// TestDashboardIdentity proves the WS-C3 identity adapter returns the signed-in
// email and a stable, subject-bound CSRF token from a session in context, and
// reports ok=false on a context with no session.
func TestDashboardIdentity(t *testing.T) {
	auth := newTestAuth(t, nil, nil)

	// No session in context → not signed in.
	if email, tok, ok := auth.DashboardIdentity(context.Background()); ok || email != "" || tok != "" {
		t.Fatalf("DashboardIdentity on bare context = (%q, %q, %v), want empty/false", email, tok, ok)
	}

	// With a session, the email + a non-empty CSRF token come back.
	ctx := context.WithValue(context.Background(), sessionContextKey{},
		&Session{Subject: "sub-1", Email: "a@example.com", Domain: "example.com"})
	email, tok, ok := auth.DashboardIdentity(ctx)
	if !ok {
		t.Fatal("DashboardIdentity reported not signed in for a session context")
	}
	if email != "a@example.com" {
		t.Errorf("email = %q, want a@example.com", email)
	}
	if tok == "" {
		t.Fatal("CSRF token is empty")
	}

	// The token is STABLE for the same subject (so a GET-rendered form token still
	// matches on the POST).
	_, tok2, _ := auth.DashboardIdentity(ctx)
	if tok != tok2 {
		t.Errorf("CSRF token not stable across calls: %q vs %q", tok, tok2)
	}

	// A different subject yields a different token (bound to identity).
	ctx2 := context.WithValue(context.Background(), sessionContextKey{},
		&Session{Subject: "sub-2", Email: "b@example.com", Domain: "example.com"})
	_, tokOther, _ := auth.DashboardIdentity(ctx2)
	if tokOther == tok {
		t.Errorf("CSRF token must differ per subject")
	}
}

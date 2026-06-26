package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/dashboard"
)

func TestTeamInvite(t *testing.T) {
	const token = "team-csrf"
	fakeMinter := func(_ context.Context, _ string) (string, time.Time, error) {
		return "KAIMI-AAAA-BBBB-CCCC", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), nil
	}
	mk := func(opts ...dashboard.Option) *dashboard.Handler {
		// A saved profile is required to reach any dashboard page (the onboarding gate); the
		// Team page is one, so seed a profile here.
		base := []dashboard.Option{dashboard.WithProfileStore(&memProfileStore{p: validProfile()}), identityOpt("owner@ey3.com", token)}
		return newOnboardingHandler(t, append(base, opts...)...)
	}

	t.Run("GET /team shows the invite form when enabled", func(t *testing.T) {
		h := mk(dashboard.WithInviteMinter(fakeMinter))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/team", http.NoBody))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		body := rec.Body.String()
		for _, want := range []string{"Invite a teammate", `action="/team/invite"`, `name="email"`, "Team"} {
			if !strings.Contains(body, want) {
				t.Errorf("/team missing %q", want)
			}
		}
	})

	t.Run("POST mints a key and shows the magic link inline", func(t *testing.T) {
		h := mk(dashboard.WithInviteMinter(fakeMinter))
		form := url.Values{"email": {"sarah@ey3.com"}, "csrf_token": {token}}
		req := httptest.NewRequest(http.MethodPost, "/team/invite", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Host = "pilot.example.com"
		req.Header.Set("X-Forwarded-Proto", "https")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "sarah@ey3.com") {
			t.Error("response missing the invited email")
		}
		if !strings.Contains(body, "https://pilot.example.com/access?key=KAIMI-AAAA-BBBB-CCCC") {
			t.Error("response missing the absolute magic link with the minted key")
		}
	})

	t.Run("invalid email errors without minting", func(t *testing.T) {
		minted := false
		h := mk(dashboard.WithInviteMinter(func(_ context.Context, _ string) (string, time.Time, error) {
			minted = true
			return "x", time.Now(), nil
		}))
		form := url.Values{"email": {"not-an-email"}, "csrf_token": {token}}
		req := httptest.NewRequest(http.MethodPost, "/team/invite", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
		if minted {
			t.Error("minted a key for an invalid email")
		}
	})

	t.Run("CSRF mismatch is rejected", func(t *testing.T) {
		h := mk(dashboard.WithInviteMinter(fakeMinter))
		form := url.Values{"email": {"sarah@ey3.com"}, "csrf_token": {"wrong"}}
		req := httptest.NewRequest(http.MethodPost, "/team/invite", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403 (CSRF)", rec.Code)
		}
	})

	t.Run("no minter wired -> feature unavailable", func(t *testing.T) {
		h := mk() // no WithInviteMinter
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/team", http.NoBody))
		if !strings.Contains(rec.Body.String(), "aren't enabled") {
			t.Error("expected the feature-unavailable message when no minter is wired")
		}
	})
}

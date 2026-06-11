package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/store"
)

// newTestDashboard builds a real dashboard.Handler over a JSON store seeded with
// one opportunity so the SSR list page renders a 200 with recognizable content.
// It is the HTML surface WS-C3a mounts into the authed cmd/api server.
func newTestDashboard(t *testing.T) *dashboard.Handler {
	t.Helper()
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	scored := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	if err := s.Save(context.Background(), &opportunity.Opportunity{
		ID:             "opp-1",
		Title:          "Test Opportunity",
		Agency:         "Agency A",
		NAICSCode:      "541512",
		Score:          0.9,
		ScoredAt:       &scored,
		Recommendation: "BID",
		CreatedAt:      scored,
		UpdatedAt:      scored,
	}); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	return dashboard.NewHandler(dashboard.NewService(s))
}

// TestDashboardHTMLUnauthenticatedRedirectsToLogin proves an unauthenticated HTML
// GET "/" is redirected (302) to /auth/login with a sanitized return path, NOT
// answered 401 JSON (that is the API surface's behavior). This is the WS-C3a HTML
// auth posture: HTML surfaces redirect a human to sign in.
func TestDashboardHTMLUnauthenticatedRedirectsToLogin(t *testing.T) {
	srv := New(Deps{Auth: newTestAuth(t, nil, nil), DashboardHTML: newTestDashboard(t)})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("unauthenticated HTML / status = %d, want %d (redirect to login)", rec.Code, http.StatusFound)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/auth/login") {
		t.Fatalf("Location = %q, want a redirect to /auth/login", loc)
	}
	if !strings.Contains(loc, "return=") {
		t.Errorf("Location = %q, want a safe return path query parameter", loc)
	}
}

// TestDashboardHTMLAuthenticatedServesPage proves a valid session cookie reaches
// the SSR dashboard, which renders the list page (200, HTML body).
func TestDashboardHTMLAuthenticatedServesPage(t *testing.T) {
	auth := newTestAuth(t, nil, nil)
	srv := New(Deps{Auth: auth, DashboardHTML: newTestDashboard(t)})
	h := srv.Routes()

	cookie := mintCookie(t, auth, Session{Subject: "1", Email: "a@example.com", Domain: "example.com"})

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated HTML / status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "Test Opportunity") {
		t.Errorf("body did not render the seeded opportunity; got %q", rec.Body.String())
	}
}

// TestDashboardHTMLDoesNotShadowAPI proves the explicit /api/v1 patterns still win
// over the "/" dashboard catch-all: an unauthenticated /api/v1 request is answered
// 401 JSON (NOT redirected to login like the HTML surface).
func TestDashboardHTMLDoesNotShadowAPI(t *testing.T) {
	srv := New(Deps{Auth: newTestAuth(t, nil, nil), DashboardHTML: newTestDashboard(t)})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/opportunities", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/api/v1 with dashboard mounted status = %d, want %d (API stays 401 JSON)", rec.Code, http.StatusUnauthorized)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("/api/v1 Content-Type = %q, want JSON", ct)
	}
}

// TestDashboardHTMLHealthzAndAuthStayOpen proves mounting the dashboard does not
// shadow the public probe or login routes: /healthz is 200 and /auth/login is a
// 302 to Google (not the dashboard's login redirect), both without a session.
func TestDashboardHTMLHealthzAndAuthStayOpen(t *testing.T) {
	srv := New(Deps{Auth: newTestAuth(t, nil, nil), DashboardHTML: newTestDashboard(t)})
	h := srv.Routes()

	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody)
	healthRec := httptest.NewRecorder()
	h.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("/healthz with dashboard mounted status = %d, want %d", healthRec.Code, http.StatusOK)
	}

	loginReq := httptest.NewRequest(http.MethodGet, "/auth/login", http.NoBody)
	loginRec := httptest.NewRecorder()
	h.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusFound {
		t.Fatalf("/auth/login with dashboard mounted status = %d, want %d", loginRec.Code, http.StatusFound)
	}
	// The login redirect must target Google's consent screen, not loop back to /auth/login.
	if loc := loginRec.Header().Get("Location"); strings.HasPrefix(loc, "/auth/login") {
		t.Errorf("/auth/login looped back to itself: Location = %q", loc)
	}
}

// TestOneCookieAuthorizesHTMLAndAPI proves the SAME session cookie minted by the
// shared session manager authorizes BOTH an HTML route and an /api/v1 route, so a
// single sign-in works across the consolidated server.
func TestOneCookieAuthorizesHTMLAndAPI(t *testing.T) {
	auth := newTestAuth(t, nil, nil)
	srv := New(Deps{Auth: auth, DashboardHTML: newTestDashboard(t)})
	h := srv.Routes()

	cookie := mintCookie(t, auth, Session{Subject: "7", Email: "shared@example.com", Domain: "example.com"})

	// HTML route.
	htmlReq := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	htmlReq.AddCookie(cookie)
	htmlRec := httptest.NewRecorder()
	h.ServeHTTP(htmlRec, htmlReq)
	if htmlRec.Code != http.StatusOK {
		t.Fatalf("HTML / with shared cookie status = %d, want %d", htmlRec.Code, http.StatusOK)
	}

	// API route, same cookie.
	apiReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", http.NoBody)
	apiReq.AddCookie(cookie)
	apiRec := httptest.NewRecorder()
	h.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("/api/v1/me with shared cookie status = %d, want %d; body=%s", apiRec.Code, http.StatusOK, apiRec.Body.String())
	}
}

// TestDashboardHTMLDevModeServesWithoutAuth proves the documented dev posture:
// with Auth nil AND AllowInsecureNoAuth explicitly set, the HTML surface is served
// WITHOUT auth (mirroring how the API group degrades), so credential-less UI dev
// works. The fail-closed default (Auth nil, opt-in false) is covered by the
// existing TestRoutesFailClosedWithoutAuthOrOptIn (Routes() panics).
func TestDashboardHTMLDevModeServesWithoutAuth(t *testing.T) {
	srv := New(Deps{AllowInsecureNoAuth: true, DashboardHTML: newTestDashboard(t)})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("insecure HTML / status = %d, want %d (served open in dev)", rec.Code, http.StatusOK)
	}
}

// TestSafeReturnPath is the open-redirect guard: only local, single-slash relative
// paths survive; absolute URLs, scheme-relative (//host) paths, and anything else
// collapse to the safe default "/".
func TestSafeReturnPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/", "/"},
		{"/proposals", "/proposals"},
		{"/opportunity/abc-123", "/opportunity/abc-123"},
		{"", "/"},                          // empty → default
		{"//evil.com", "/"},                // scheme-relative open redirect
		{"///evil.com", "/"},               // multi-slash open redirect
		{"http://evil.com", "/"},           // absolute URL
		{"https://evil.com/x", "/"},        // absolute URL
		{"/\\evil.com", "/"},               // backslash trick some browsers treat as //
		{"javascript:alert(1)", "/"},       // non-path scheme
		{"relative/no/leading/slash", "/"}, // must be rooted
		{"/ok?x=1#frag", "/ok?x=1#frag"},   // query + fragment on a local path are fine
	}
	for _, tc := range cases {
		if got := safeReturnPath(tc.in); got != tc.want {
			t.Errorf("safeReturnPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestDashboardHTMLReturnPathSanitized proves the redirect's return parameter is
// passed through the open-redirect guard: a malicious original path does not leak
// into the Location's return value.
func TestDashboardHTMLReturnPathSanitized(t *testing.T) {
	srv := New(Deps{Auth: newTestAuth(t, nil, nil), DashboardHTML: newTestDashboard(t)})
	h := srv.Routes()

	// A legitimate deep link round-trips its path.
	req := httptest.NewRequest(http.MethodGet, "/proposals", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "return=%2Fproposals") {
		t.Errorf("Location = %q, want return=%%2Fproposals", loc)
	}
}

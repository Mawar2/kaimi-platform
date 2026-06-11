package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mintCookie signs a session with the auth handler's session manager and returns
// it as a request cookie, so tests authenticate exactly the way a real login does.
func mintCookie(t *testing.T, auth *AuthHandler, s Session) *http.Cookie {
	t.Helper()
	if s.Expiry == 0 {
		s.Expiry = time.Now().Add(time.Hour).Unix()
	}
	return &http.Cookie{Name: sessionCookieName, Value: auth.session.sign(s)}
}

// okHandler is a trivial next-handler that records whether it ran and (optionally)
// asserts the session the middleware injected into the request context.
func okHandler(ran *bool, gotSession **Session) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*ran = true
		if s, ok := SessionFromContext(r.Context()); ok && gotSession != nil {
			*gotSession = s
		}
		w.WriteHeader(http.StatusOK)
	})
}

// TestRequireSessionNoCookieReturns401 proves a request to a protected route with
// no session cookie is rejected 401 with the JSON error envelope and the next
// handler never runs.
func TestRequireSessionNoCookieReturns401(t *testing.T) {
	srv := New(Deps{Auth: newTestAuth(t, nil, nil)})

	ran := false
	h := srv.RequireSession(okHandler(&ran, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/opportunities", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	var env ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if env.Error == "" {
		t.Errorf("error envelope = %+v, want non-empty message", env)
	}
	if ran {
		t.Error("next handler ran despite missing session")
	}
}

// TestRequireSessionTamperedCookieReturns401 proves a forged cookie (valid format,
// wrong signature) is rejected 401 and next never runs.
func TestRequireSessionTamperedCookieReturns401(t *testing.T) {
	auth := newTestAuth(t, nil, nil)
	srv := New(Deps{Auth: auth})

	ran := false
	h := srv.RequireSession(okHandler(&ran, nil))

	good := mintCookie(t, auth, Session{Subject: "1", Email: "a@example.com", Domain: "example.com"})
	// Flip the FIRST character of the cookie (the payload's first base64 char) so the
	// signed bytes are guaranteed to change and the MAC no longer matches. Flipping the
	// LAST char is unreliable: a base64 string's final char carries "don't-care"
	// trailing bits, so A<->B there can decode to the same bytes — which made this test
	// flaky (occasionally a 200 instead of 401).
	v := good.Value
	tampered := flipChar(v[0]) + v[1:]

	req := httptest.NewRequest(http.MethodGet, "/api/v1/opportunities", http.NoBody)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tampered})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tampered cookie status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if ran {
		t.Error("next handler ran despite tampered session")
	}
}

// TestRequireSessionExpiredCookieReturns401 proves a validly-signed but expired
// session is rejected 401.
func TestRequireSessionExpiredCookieReturns401(t *testing.T) {
	auth := newTestAuth(t, nil, nil)
	srv := New(Deps{Auth: auth})

	ran := false
	h := srv.RequireSession(okHandler(&ran, nil))

	expired := mintCookie(t, auth, Session{
		Subject: "1", Email: "a@example.com", Domain: "example.com",
		Expiry: time.Now().Add(-time.Minute).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/opportunities", http.NoBody)
	req.AddCookie(expired)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired cookie status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if ran {
		t.Error("next handler ran despite expired session")
	}
}

// TestRequireSessionValidCookiePassesAndInjectsIdentity proves a valid session
// reaches next with the identity available via SessionFromContext.
func TestRequireSessionValidCookiePassesAndInjectsIdentity(t *testing.T) {
	auth := newTestAuth(t, nil, nil)
	srv := New(Deps{Auth: auth})

	ran := false
	var got *Session
	h := srv.RequireSession(okHandler(&ran, &got))

	cookie := mintCookie(t, auth, Session{Subject: "42", Email: "bob@example.com", Domain: "example.com"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/opportunities", http.NoBody)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("valid cookie status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !ran {
		t.Fatal("next handler did not run for valid session")
	}
	if got == nil {
		t.Fatal("SessionFromContext returned no session in next handler")
	}
	if got.Subject != "42" || got.Email != "bob@example.com" || got.Domain != "example.com" {
		t.Errorf("context session = %+v, want sub=42 email=bob@example.com domain=example.com", got)
	}
}

// TestSessionFromContextAbsent proves the accessor reports ok=false on a context
// the middleware never touched, so handlers can distinguish authed from not.
func TestSessionFromContextAbsent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	if s, ok := SessionFromContext(req.Context()); ok || s != nil {
		t.Errorf("SessionFromContext on bare context = (%v, %v), want (nil, false)", s, ok)
	}
}

// --- Full-routing integration: confirm the wrap is scoped to /api/v1 only. ---

// TestRoutesProtectsAPIGroup proves that, with Auth configured, an /api/v1 route
// requires a session: no cookie → 401.
func TestRoutesProtectsAPIGroup(t *testing.T) {
	srv := New(Deps{Auth: newTestAuth(t, nil, nil)})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/opportunities", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/api/v1 without session status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// TestRoutesHealthzExemptFromAuth proves /healthz is reachable WITHOUT a session
// even when auth is configured — the wrap covers only the /api/v1 group.
func TestRoutesHealthzExemptFromAuth(t *testing.T) {
	srv := New(Deps{Auth: newTestAuth(t, nil, nil)})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/healthz", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/healthz with auth configured status = %d, want %d (must stay public)", rec.Code, http.StatusOK)
	}
}

// TestRoutesAuthLoginExemptFromSession proves /auth/login is reachable WITHOUT a
// session — a user must reach login before they can have one. It asserts the route
// is not 401'd (it redirects to Google with 302).
func TestRoutesAuthLoginExemptFromSession(t *testing.T) {
	srv := New(Deps{Auth: newTestAuth(t, nil, nil)})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/auth/login", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("/auth/login returned 401; login must be reachable without a session")
	}
	if rec.Code != http.StatusFound {
		t.Errorf("/auth/login status = %d, want %d (redirect to Google)", rec.Code, http.StatusFound)
	}
}

// TestRoutesOfflineLeavesAPIOpen proves the documented offline/dev behavior: when
// Auth is nil (OAuth unconfigured) AND AllowInsecureNoAuth is EXPLICITLY set, the
// API group is NOT wrapped, so requests reach the mux without a session (local UI
// dev can use the API credential-less). Production must configure OAuth (see
// cmd/api). The insecure opt-in is required because the default now fails closed
// (see TestRoutesFailClosedWithoutAuthOrOptIn).
//
// We probe an unregistered /api/v1 subpath so the request reaches the mux without
// touching any nil dependency: in this insecure-opt-in mode that yields the mux's
// 404 (the request was NOT blocked by auth), whereas with auth configured the same
// request is 401'd by the middleware before the mux ever sees it (asserted by the
// companion test below). The single, unambiguous assertion is "did NOT 401".
func TestRoutesOfflineLeavesAPIOpen(t *testing.T) {
	// Auth nil + explicit insecure opt-in → no RequireSession wrap, API left open.
	srv := New(Deps{AllowInsecureNoAuth: true})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/unregistered-probe", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("insecure opt-in 401'd /api/v1; with AllowInsecureNoAuth the API must stay open for local dev")
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("insecure /api/v1 probe status = %d, want %d (reached the mux unblocked)", rec.Code, http.StatusNotFound)
	}
}

// TestRoutesFailClosedWithoutAuthOrOptIn proves the fail-closed backstop: when Auth
// is nil AND AllowInsecureNoAuth is false (the default), Routes() PANICS rather than
// serving an unauthenticated API. This guarantees a production deploy with a missing
// or typo'd OAuth env var can never silently come up open — the insecure server
// never starts.
func TestRoutesFailClosedWithoutAuthOrOptIn(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Routes() did not panic with Auth nil and AllowInsecureNoAuth false; the unauthenticated API must fail closed")
		}
	}()

	srv := New(Deps{}) // Auth nil, AllowInsecureNoAuth false (default) → must panic.
	_ = srv.Routes()
}

// TestRoutesConfiguredBlocksUnknownAPIPathBeforeMux is the companion to the offline
// test: with auth configured, even an unregistered /api/v1 subpath is 401'd by the
// middleware (the request never reaches the mux's 404). This confirms the wrap sits
// in front of the whole group, not just the registered routes.
func TestRoutesConfiguredBlocksUnknownAPIPathBeforeMux(t *testing.T) {
	srv := New(Deps{Auth: newTestAuth(t, nil, nil)})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/unregistered-probe", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("configured /api/v1 unknown path status = %d, want %d (auth blocks before mux)", rec.Code, http.StatusUnauthorized)
	}
}

// TestMeReturnsSessionIdentity proves GET /api/v1/me returns the authenticated
// identity from the session in context.
func TestMeReturnsSessionIdentity(t *testing.T) {
	auth := newTestAuth(t, nil, nil)
	srv := New(Deps{Auth: auth})
	h := srv.Routes()

	cookie := mintCookie(t, auth, Session{Subject: "99", Email: "carol@example.com", Domain: "example.com"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", http.NoBody)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/api/v1/me status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var me MeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode /me body %q: %v", rec.Body.String(), err)
	}
	if me.Email != "carol@example.com" || me.Domain != "example.com" || me.Subject != "99" {
		t.Errorf("/me = %+v, want email=carol@example.com domain=example.com sub=99", me)
	}
}

// TestMeWithoutSessionReturns401 proves /api/v1/me is itself protected: with auth
// configured and no cookie, the middleware rejects it before the handler runs.
func TestMeWithoutSessionReturns401(t *testing.T) {
	srv := New(Deps{Auth: newTestAuth(t, nil, nil)})
	h := srv.Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/api/v1/me without session status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "error") {
		t.Errorf("/me 401 body = %q, want JSON error envelope", rec.Body.String())
	}
}

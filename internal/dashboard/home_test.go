package dashboard_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHomePage proves GET /home renders the public landing page with no session/profile
// (Google's OAuth verification reviewers must reach the homepage without auth) and that it
// describes the app and links the privacy policy — both homepage requirements.
func TestHomePage(t *testing.T) {
	h := newOnboardingHandler(t) // no profile store, no saver — the homepage must still render
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/home", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("/home status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Kaimi",
		"SAM.gov",         // describes the app's function
		"never",           // the human-in-the-loop / never-auto-submit promise
		`href="/privacy"`, // homepage must link the privacy policy
		"BlueMeta",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/home missing %q", want)
		}
	}
}

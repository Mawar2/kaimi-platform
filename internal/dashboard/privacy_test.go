package dashboard_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPrivacyPage proves GET /privacy renders the privacy policy with no session/profile
// (Google's OAuth verification requires a publicly reachable policy) and that the mandatory
// Google Limited Use affirmation + its policy link are present — the load-bearing line that
// must never be dropped, or OAuth verification fails.
func TestPrivacyPage(t *testing.T) {
	h := newOnboardingHandler(t) // no profile store, no saver — privacy must still render
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/privacy", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("/privacy status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Privacy Policy",
		"drive.file",
		"Limited Use",
		"https://developers.google.com/terms/api-services-user-data-policy",
		"not used to develop, train, or improve generalized", // the AI-training disclosure
		"malik@bluemetatech.com",                             // contact
		"Last updated:",                                      // effective date
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/privacy missing %q", want)
		}
	}
}

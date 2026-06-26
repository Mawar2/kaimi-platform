package dashboard_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHelpPage proves GET /help renders the setup guide with the SAM.gov API-key
// instructions, and needs no profile/session (it's the public guide).
func TestHelpPage(t *testing.T) {
	h := newOnboardingHandler(t) // no profile store, no saver — help must still render
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/help", http.NoBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("/help status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"SAM.gov API key", "sam.gov", "NAICS", "Help"} {
		if !strings.Contains(body, want) {
			t.Errorf("/help missing %q", want)
		}
	}
}

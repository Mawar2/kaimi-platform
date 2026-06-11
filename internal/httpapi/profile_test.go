package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Mawar2/Kaimi/internal/profile"
)

// fakeProfileStore is a hand-written in-memory double for profile.ProfileStore so
// the handler tests stay fast and deterministic (no disk, no GCP). A nil stored
// value yields the not-found path.
type fakeProfileStore struct {
	stored  *profile.CapabilityProfile
	saveErr error
	loadErr error // overrides the not-found sentinel when set
	saved   []*profile.CapabilityProfile
}

func (f *fakeProfileStore) Load() (*profile.CapabilityProfile, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	if f.stored == nil {
		return nil, profile.ErrProfileNotFound
	}
	return f.stored, nil
}

func (f *fakeProfileStore) Save(p *profile.CapabilityProfile) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, p)
	f.stored = p
	return nil
}

// profileServer builds a Server wired with the given ProfileStore on the insecure
// no-auth path so Routes() builds without OAuth (auth coverage is verified
// generically elsewhere; here we exercise the handler logic).
func profileServer(ps profile.ProfileStore) http.Handler {
	srv := New(Deps{ProfileStore: ps, AllowInsecureNoAuth: true})
	return srv.Routes()
}

// validProfileJSON returns a minimal valid profile request body: a company name
// plus one NAICS code.
func validProfileJSON() []byte {
	p := profile.CapabilityProfile{
		Company:    "Acme Federal",
		NAICSCodes: []profile.NAICSCode{{Code: "541512", Tier: profile.TierPrimary}},
	}
	b, _ := json.Marshal(p)
	return b
}

// TestGetProfile_NotConfiguredReturns404 confirms GET returns 404 JSON when no
// profile has been set, so the UI knows onboarding is required.
func TestGetProfile_NotConfiguredReturns404(t *testing.T) {
	h := profileServer(&fakeProfileStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/profile", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
	var got ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	if got.Error == "" {
		t.Error("expected a non-empty error message on 404")
	}
}

// TestGetProfile_ReturnsStored confirms GET returns the stored profile as JSON.
func TestGetProfile_ReturnsStored(t *testing.T) {
	stored := &profile.CapabilityProfile{
		Company:    "Stored Co",
		NAICSCodes: []profile.NAICSCode{{Code: "541330", Tier: profile.TierPrimary}},
	}
	h := profileServer(&fakeProfileStore{stored: stored})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/profile", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var got profile.CapabilityProfile
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode profile body %q: %v", rec.Body.String(), err)
	}
	if got.Company != "Stored Co" {
		t.Errorf("Company = %q, want %q", got.Company, "Stored Co")
	}
}

// TestPutProfile_PersistsAndGetReflects confirms a valid PUT persists the profile
// and a subsequent GET returns it.
func TestPutProfile_PersistsAndGetReflects(t *testing.T) {
	fps := &fakeProfileStore{}
	h := profileServer(fps)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/profile", bytes.NewReader(validProfileJSON()))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 200/204; body = %s", rec.Code, rec.Body.String())
	}
	if len(fps.saved) != 1 {
		t.Fatalf("Save calls = %d, want 1", len(fps.saved))
	}
	if fps.saved[0].Company != "Acme Federal" {
		t.Errorf("persisted Company = %q, want %q", fps.saved[0].Company, "Acme Federal")
	}

	// GET must now reflect the persisted profile.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/profile", http.NoBody)
	getRec := httptest.NewRecorder()
	h.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET-after-PUT status = %d, want 200; body = %s", getRec.Code, getRec.Body.String())
	}
}

// TestPutProfile_InvalidReturns400 confirms validation rejects malformed/missing
// required fields with a 400 and does NOT persist anything.
func TestPutProfile_InvalidReturns400(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{
		{name: "not json", body: []byte("{not valid json")},
		{name: "missing company", body: mustJSON(profile.CapabilityProfile{
			NAICSCodes: []profile.NAICSCode{{Code: "541512"}},
		})},
		{name: "no naics", body: mustJSON(profile.CapabilityProfile{Company: "No Codes Co"})},
		{name: "blank naics code", body: mustJSON(profile.CapabilityProfile{
			Company:    "Blank Code Co",
			NAICSCodes: []profile.NAICSCode{{Code: "   "}},
		})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fps := &fakeProfileStore{}
			h := profileServer(fps)

			req := httptest.NewRequest(http.MethodPut, "/api/v1/profile", bytes.NewReader(tc.body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
			}
			if len(fps.saved) != 0 {
				t.Errorf("Save called %d times on invalid input; want 0", len(fps.saved))
			}
		})
	}
}

// TestProfileEndpoints_ServiceUnavailableWhenUnwired confirms that when no
// ProfileStore is wired the routes answer 503 rather than panicking, mirroring how
// the action endpoints degrade when their service is nil.
func TestProfileEndpoints_ServiceUnavailableWhenUnwired(t *testing.T) {
	h := profileServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/profile", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET status = %d, want 503; body = %s", rec.Code, rec.Body.String())
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

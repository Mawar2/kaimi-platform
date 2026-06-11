package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Mawar2/Kaimi/internal/profile"
)

// This file holds the WS-C1 runtime profile endpoints. They let a tenant read and
// set their company profile through the API so onboarding can configure a
// deployment WITHOUT editing files baked into the image. Persistence goes through
// the ProfileStore abstraction (internal/profile) so it swaps from JSON-on-disk to
// GCS/Firestore later without touching these handlers.
//
// Both endpoints live on the PROTECTED apiMux (configuring the active profile is an
// authenticated action) and degrade to 503 when no ProfileStore is wired, mirroring
// how the select/status endpoints degrade when the proposal service is absent.

// handleGetProfile serves GET /api/v1/profile. It returns the persisted tenant
// profile as JSON, or a 404 when none has been configured yet so the UI knows to
// start onboarding. It returns 503 when no ProfileStore is wired.
func (s *Server) handleGetProfile(w http.ResponseWriter, _ *http.Request) {
	if s.deps.ProfileStore == nil {
		writeError(w, http.StatusServiceUnavailable, "profile configuration is not available")
		return
	}

	p, err := s.deps.ProfileStore.Load()
	if err != nil {
		if errors.Is(err, profile.ErrProfileNotFound) {
			// Not onboarded yet. A 404 tells the client this deployment has no
			// configured profile and should run onboarding.
			writeError(w, http.StatusNotFound, "no company profile configured")
			return
		}
		// Keep store/internal detail server-side; clients get a generic 500.
		writeError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}

	writeJSON(w, http.StatusOK, p)
}

// handlePutProfile serves PUT /api/v1/profile. It decodes the submitted profile,
// validates the minimal required fields, and persists it via the ProfileStore. It
// returns 400 on a malformed body or failed validation, 503 when no ProfileStore
// is wired, and 200 with the stored profile on success.
func (s *Server) handlePutProfile(w http.ResponseWriter, r *http.Request) {
	if s.deps.ProfileStore == nil {
		writeError(w, http.StatusServiceUnavailable, "profile configuration is not available")
		return
	}

	// Reject unknown fields so a typo'd key surfaces as a 400 instead of silently
	// dropping data the caller believed they set.
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var p profile.CapabilityProfile
	if err := dec.Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile JSON: "+err.Error())
		return
	}

	// Validation is shared with the WS-C3 SSR onboarding form via profile.Validate,
	// so the JSON API and the HTML form can never diverge on what counts as valid.
	if err := profile.Validate(&p); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.deps.ProfileStore.Save(&p); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save profile")
		return
	}

	// Echo the stored profile so the client confirms exactly what was persisted.
	writeJSON(w, http.StatusOK, &p)
}

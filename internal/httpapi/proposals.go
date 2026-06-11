package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/proposal"
	"github.com/Mawar2/Kaimi/internal/store"
)

// This file holds the WS-B3 action + proposal-status endpoints. The SELECT
// endpoint is the Zone-1 → Zone-2 bridge (the human chooses to pursue an
// opportunity); the proposal-status endpoint composes the opportunity's derived
// stage/status (from the dashboard read layer) with the drafted document (from
// the proposal service) into one JSON view. Both wrap internal/proposal.Service
// through the small proposalService seam below so tests inject a fake and a
// read-only deployment (no proposal service) degrades to 503 rather than panics.
//
// Auth is intentionally NOT here: these register on the protected /api/v1 group,
// which WS-B5 wraps with auth middleware as a whole.

// proposalService is the narrow slice of *proposal.Service these handlers need:
// the SELECT action and the document read. Defining it here (rather than taking
// the concrete *proposal.Service) lets the unit tests inject a fake while the
// real service satisfies it unchanged — see TestRealServiceSatisfiesInterface.
type proposalService interface {
	// Select is the bridge event from Zone 1: pursue the opportunity. It returns
	// proposal.ErrAlreadySelected / proposal.ErrStageRunning for the conflict
	// cases, wrapped, so callers map them with errors.Is.
	Select(ctx context.Context, oppID string) error
	// Document returns the current proposal document for the opportunity, or an
	// error when no draft exists yet.
	Document(oppID string) (*document.Document, error)
}

// proposals resolves the proposal service for the request: the test override
// takes precedence (and is nil in production), otherwise the real Deps.Proposals.
// A nil result means the API was deployed read-only and action/status endpoints
// must answer 503.
func (s *Server) proposals() proposalService {
	if s.deps.proposalsOverride != nil {
		return s.deps.proposalsOverride
	}
	// A nil *proposal.Service must read back as a nil interface so callers can do
	// a plain == nil check; return the typed nil only when it is actually set.
	if s.deps.Proposals != nil {
		return s.deps.Proposals
	}
	return nil
}

// handleSelectOpportunity serves POST /api/v1/opportunities/{id}/select — the
// Zone-1 → Zone-2 bridge. Status mapping:
//
//   - 503 when no proposal service is wired (read-only deployment),
//   - 400 on a malformed id,
//   - 404 when the opportunity does not exist,
//   - 409 when it is already selected or a stage is already running,
//   - 202 Accepted on success, with the resulting proposal status in the body.
func (s *Server) handleSelectOpportunity(w http.ResponseWriter, r *http.Request) {
	proposals := s.proposals()
	if proposals == nil {
		writeError(w, http.StatusServiceUnavailable, "proposal actions are not enabled on this server")
		return
	}

	id := r.PathValue("id")
	if !dashboard.ValidOpportunityID(id) {
		writeError(w, http.StatusBadRequest, "invalid opportunity id")
		return
	}

	// Confirm the opportunity exists before acting so an unknown id is a clean 404
	// rather than a service error surfaced as 409/500.
	if _, err := s.deps.Dashboard.Get(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "opportunity not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load opportunity")
		return
	}

	if err := proposals.Select(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, proposal.ErrAlreadySelected):
			writeError(w, http.StatusConflict, "opportunity is already in your proposals")
		case errors.Is(err, proposal.ErrStageRunning):
			writeError(w, http.StatusConflict, "opportunity already has a stage running")
		case errors.Is(err, store.ErrNotFound):
			// Lost a race: the opportunity vanished between the existence check and
			// the select. Report it as not found rather than a generic 500.
			writeError(w, http.StatusNotFound, "opportunity not found")
		default:
			writeError(w, http.StatusInternalServerError, "failed to select opportunity")
		}
		return
	}

	// Read back the persisted status so the caller learns the resulting state
	// (e.g. "outline:in_progress") without a second request. A read-back error is
	// non-fatal: the select succeeded, so still answer 202 with the id alone.
	resp := SelectResponse{ID: id, Selected: true}
	if opp, err := s.deps.Dashboard.Get(r.Context(), id); err == nil {
		resp.Status = opp.ProposalStatus
	}
	writeJSON(w, http.StatusAccepted, resp)
}

// handleGetProposalStatus serves GET /api/v1/proposals/{id}. It composes the
// opportunity's derived stage + persisted ProposalStatus (the read layer) with
// the drafted document (the proposal service) into a single status view. Mapping:
// 503 when proposals are unwired, 400 on a malformed id, 404 for an unknown
// opportunity, 200 otherwise (with the document omitted when no draft exists yet).
func (s *Server) handleGetProposalStatus(w http.ResponseWriter, r *http.Request) {
	proposals := s.proposals()
	if proposals == nil {
		writeError(w, http.StatusServiceUnavailable, "proposal actions are not enabled on this server")
		return
	}

	id := r.PathValue("id")
	if !dashboard.ValidOpportunityID(id) {
		writeError(w, http.StatusBadRequest, "invalid opportunity id")
		return
	}

	opp, err := s.deps.Dashboard.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "opportunity not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load opportunity")
		return
	}

	resp := ProposalStatusDTO{
		Stage:  string(dashboard.DeriveStage(opp)),
		Status: opp.ProposalStatus,
		State:  proposal.DisplayState(opp.ProposalStatus),
	}
	// The drafted document is optional: a selected-but-not-yet-drafted (or never
	// selected) opportunity has none. A document error degrades to "no document"
	// rather than failing the status read — the stage/status above is always valid.
	if doc, err := proposals.Document(id); err == nil {
		resp.Document = doc
	}
	writeJSON(w, http.StatusOK, resp)
}

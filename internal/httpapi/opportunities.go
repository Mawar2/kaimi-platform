package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/store"
)

// This file holds the WS-B2 read endpoints. They wrap the existing
// dashboard.Service (the store-backed read layer the HTML dashboard renders
// over) and translate its view-models into the JSON DTOs in dto.go. They are
// read-only: no select, no write, no auth — those are WS-B3/WS-B4/WS-B5.
//
// Query-param parsing semantics (param names, accepted values, sort defaults)
// are kept consistent with the HTML dashboard's list handler
// (internal/dashboard/handler.go handleList) so the web and desktop surfaces
// behave identically over the same store.

// handleListOpportunities serves GET /api/v1/opportunities. It applies the same
// stage / minScore / rec / sort filters the HTML dashboard exposes and returns
// the rows plus a count.
func (s *Server) handleListOpportunities(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	opts := parseListOptions(r, now)

	rows, err := s.deps.Dashboard.List(r.Context(), opts)
	if err != nil {
		// Keep store/internal detail server-side; clients get a generic 500.
		writeError(w, http.StatusInternalServerError, "failed to load opportunities")
		return
	}

	dtos := make([]OpportunityDTO, 0, len(rows))
	for i := range rows {
		dtos = append(dtos, toOpportunityDTO(&rows[i]))
	}
	writeJSON(w, http.StatusOK, listResponse{Rows: dtos, Count: len(dtos)})
}

// parseListOptions builds dashboard.ListOptions from the request query, mirroring
// the param names and accepted values of the HTML dashboard's handleList:
//
//   - stage=<Stage>     filter to a derived pipeline stage (e.g. "Scored")
//   - minScore=<float>  keep rows with Score >= the value; ignored if unparseable
//   - rec=<BID|REVIEW|NO_BID> filter to a scorer recommendation
//   - sort=<deadline|score> sort field; defaults to deadline ascending
//
// Unlike the HTML list (which self-cleans by excluding pursued opportunities),
// the API returns the full set so clients can present every stage; pursuit
// filtering, if needed, is a future query option rather than a baked-in default.
func parseListOptions(r *http.Request, now time.Time) dashboard.ListOptions {
	q := r.URL.Query()
	opts := dashboard.ListOptions{Now: now}

	if stage := q.Get("stage"); stage != "" {
		st := dashboard.Stage(stage)
		opts.Stage = &st
	}
	if ms := q.Get("minScore"); ms != "" {
		// Ignore an unparseable value rather than erroring, matching the HTML
		// dashboard: a malformed filter falls back to "no score filter".
		if f, err := strconv.ParseFloat(ms, 64); err == nil {
			opts.MinScore = f
		}
	}
	if rec := q.Get("rec"); rec == "BID" || rec == "REVIEW" || rec == "NO_BID" {
		opts.Recommendation = rec
	}
	// Default to deadline ascending (ListOptions zero value); switch to score
	// descending only on an explicit sort=score.
	if q.Get("sort") == "score" {
		opts.SortBy = dashboard.SortByScore
	} else {
		opts.SortBy = dashboard.SortByDeadline
	}
	return opts
}

// handleGetOpportunity serves GET /api/v1/opportunities/{id}. It validates the id
// with the shared dashboard validator (so the API rejects path traversal and
// malformed keys exactly as the HTML surface does), returns a JSON 404 for an
// absent record, and otherwise returns the full opportunity plus derived fields.
func (s *Server) handleGetOpportunity(w http.ResponseWriter, r *http.Request) {
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

	writeJSON(w, http.StatusOK, OpportunityDetailDTO{
		Opportunity:  opp,
		DerivedStage: string(dashboard.DeriveStage(opp)),
		ScorePct:     scorePct(opp.Score),
	})
}

// handleStageCounts serves GET /api/v1/stages/counts. It returns the per-stage
// tally over every stored opportunity as a stage-string → count map.
func (s *Server) handleStageCounts(w http.ResponseWriter, r *http.Request) {
	counts, err := s.deps.Dashboard.CountStages(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count stages")
		return
	}

	out := make(map[string]int, len(counts))
	for stage, n := range counts {
		out[string(stage)] = n
	}
	writeJSON(w, http.StatusOK, stageCountsResponse{Counts: out})
}

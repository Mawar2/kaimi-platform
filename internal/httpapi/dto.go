package httpapi

import (
	"math"
	"time"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// This file holds the JSON data-transfer objects the API returns. It starts with
// the health response and the opportunity read shapes (WS-B2); WS-B3 will add the
// select/action request and response shapes.
//
// The DTOs are deliberately distinct from the dashboard view-models
// (dashboard.OpportunityRow) and the internal opportunity.Opportunity schema:
// the wire contract is owned here so the HTML dashboard's view-models can change
// without silently reshaping the JSON API, and so derived fields (percentages,
// stage strings) are computed once at the edge rather than by every client.

// OpportunityDTO is the flattened list-row shape returned by
// GET /api/v1/opportunities. It mirrors the fields a dashboard.OpportunityRow
// carries, with the stage rendered as its string and the score pre-computed as a
// 0–100 percentage so clients render identically without re-deriving it.
type OpportunityDTO struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Agency           string    `json:"agency"`
	NAICSCode        string    `json:"naics"`
	SolicitationNum  string    `json:"solicitation_num,omitempty"`
	Score            float64   `json:"score"`     // raw 0.0–1.0 fit score
	ScorePct         int       `json:"score_pct"` // score rounded to 0–100
	Recommendation   string    `json:"recommendation,omitempty"`
	Stage            string    `json:"stage"` // derived pipeline stage as a string
	ResponseDeadline time.Time `json:"response_deadline,omitempty"`
	PostedDate       time.Time `json:"posted_date,omitempty"`
	DaysSincePosted  int       `json:"days_since_posted"` // whole days from PostedDate to now (0 when unknown)
	DeadlineSoon     bool      `json:"deadline_soon"`     // deadline within 7 days of now
	EstimatedValue   float64   `json:"estimated_value,omitempty"`
	// LowValueHighEffort flags an opportunity whose estimated value does not
	// justify the response effort — the "value vs effort" signal small BD teams
	// asked for. It is false until the scorer/manager populates the underlying
	// signal; the field is part of the wire contract now so clients can render it
	// without a later breaking change.
	LowValueHighEffort bool `json:"low_value_high_effort"`
}

// OpportunityDetailDTO is the full-detail shape returned by
// GET /api/v1/opportunities/{id}. It embeds the complete opportunity.Opportunity
// (which already carries its own json tags) and adds the two derived fields the
// detail view needs but the schema does not store: the derived pipeline stage as
// a string and the score as a 0–100 percentage.
type OpportunityDetailDTO struct {
	*opportunity.Opportunity
	DerivedStage string `json:"derived_stage"`
	ScorePct     int    `json:"score_pct"`
}

// listResponse is the envelope for the list endpoint: the rows plus an explicit
// count so clients need not infer it from the array length.
type listResponse struct {
	Rows  []OpportunityDTO `json:"rows"`
	Count int              `json:"count"`
}

// stageCountsResponse is the envelope for GET /api/v1/stages/counts: a stage
// (string) → count map under a single "counts" key.
type stageCountsResponse struct {
	Counts map[string]int `json:"counts"`
}

// toOpportunityDTO flattens a dashboard.OpportunityRow into the wire DTO,
// computing the derived percentage and days-since-posted at the edge. The row
// does not carry PostedDate or EstimatedValue, so those derive to their zero
// values for list responses; the detail endpoint exposes the full record when a
// client needs them.
func toOpportunityDTO(row *dashboard.OpportunityRow, now time.Time) OpportunityDTO {
	return OpportunityDTO{
		ID:               row.ID,
		Title:            row.Title,
		Agency:           row.Agency,
		NAICSCode:        row.NAICSCode,
		SolicitationNum:  row.SolicitationNum,
		Score:            row.Score,
		ScorePct:         scorePct(row.Score),
		Recommendation:   row.Recommendation,
		Stage:            string(row.Stage),
		ResponseDeadline: row.ResponseDeadline,
		DaysSincePosted:  daysSince(row.CreatedAt, now),
		DeadlineSoon:     row.DeadlineSoon,
	}
}

// scorePct converts a 0.0–1.0 fit score to a rounded 0–100 percentage.
func scorePct(score float64) int {
	return int(math.Round(score * 100))
}

// daysSince returns the whole days elapsed from t to now, or 0 when t is the zero
// value or in the future, so the field never goes negative on the wire.
func daysSince(t, now time.Time) int {
	if t.IsZero() || now.Before(t) {
		return 0
	}
	return int(now.Sub(t).Hours() / 24)
}

// HealthResponse is the body returned by GET /healthz. It is intentionally small
// — a liveness probe only needs to confirm the process is up and serving JSON.
type HealthResponse struct {
	// Status is a fixed machine-readable token ("ok") that probes can assert on.
	Status string `json:"status"`

	// Service names the binary so a shared log/monitoring view can tell Kaimi's
	// API apart from its other HTTP surfaces.
	Service string `json:"service"`
}

// ErrorResponse is the envelope returned for non-2xx responses so clients can
// rely on a single error shape across every endpoint.
type ErrorResponse struct {
	// Error is a human-readable message describing what went wrong.
	Error string `json:"error"`
}

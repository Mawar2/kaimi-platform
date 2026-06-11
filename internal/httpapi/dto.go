package httpapi

import (
	"math"
	"time"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// This file holds the JSON data-transfer objects the API returns. It starts with
// the health response and the opportunity read shapes (WS-B2); WS-B3 will add the
// select/action request and response shapes.
//
// The list and detail endpoints own their wire shapes differently, on purpose:
//
//   - OpportunityDTO (the LIST row) is the API's owned, flattened wire contract.
//     It is deliberately decoupled from the dashboard view-model
//     (dashboard.OpportunityRow) so that view-model can change freely to serve the
//     HTML dashboard without silently reshaping the JSON list. It carries only
//     fields actually populated from real OpportunityRow data, plus derived fields
//     (percentages, stage strings) computed once at the edge. New fields are added
//     here only once OpportunityRow backs them with real data — emitting zero/false
//     placeholders for unbacked fields would send clients false signals, and adding
//     a field later is a non-breaking additive change.
//   - OpportunityDetailDTO (the DETAIL response) intentionally returns the
//     canonical opportunity.Opportunity schema (see OpportunityDetailDTO below).

// OpportunityDTO is the flattened list-row shape returned by
// GET /api/v1/opportunities. It carries the subset of dashboard.OpportunityRow
// fields backed by real data, with the stage rendered as its string and the score
// pre-computed as a 0–100 percentage so clients render identically without
// re-deriving it. Fields the row does not yet supply (e.g. estimated value,
// value-vs-effort signal, posted date) are intentionally omitted rather than
// emitted as zero/false placeholders; the detail endpoint exposes the full record.
type OpportunityDTO struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Agency           string    `json:"agency"`
	NAICSCode        string    `json:"naics"`
	SolicitationNum  string    `json:"solicitation_num,omitempty"`
	Score            float64   `json:"score"`     // raw 0.0–1.0 fit score
	ScorePct         int       `json:"score_pct"` // score rounded to 0–100
	Recommendation   string    `json:"recommendation,omitempty"`
	Stage            string    `json:"stage"`                      // derived pipeline stage as a string
	ResponseDeadline time.Time `json:"response_deadline,omitzero"` // omitted (not "0001-01-01...") when unset
	DeadlineSoon     bool      `json:"deadline_soon"`              // deadline within 7 days of now
}

// OpportunityDetailDTO is the full-detail shape returned by
// GET /api/v1/opportunities/{id}. Unlike the list DTO (which owns a flattened wire
// shape decoupled from any view-model), the detail endpoint intentionally returns
// the canonical opportunity.Opportunity schema by embedding it: that schema is the
// system's deliberately stable, forward-compatible contract (see ARCHITECTURE.md),
// so returning it directly — rather than re-declaring ~30 fields here — is the
// correct, low-drift choice for the detail view. On top of the embedded schema it
// adds the two derived fields the detail view needs but the schema does not store:
// the derived pipeline stage as a string and the score as a 0–100 percentage.
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

// SelectResponse is the body returned by POST /api/v1/opportunities/{id}/select.
// It echoes the id and reports the resulting proposal status so the caller learns
// the new pipeline state (e.g. "outline:in_progress") without a follow-up read.
type SelectResponse struct {
	// ID is the selected opportunity's id.
	ID string `json:"id"`
	// Selected is always true on a 202 response (the select succeeded).
	Selected bool `json:"selected"`
	// Status is the resulting ProposalStatus. It may be empty if the post-select
	// read-back failed; the 202 status code is the authoritative success signal.
	Status string `json:"status,omitempty"`
}

// ProposalStatusDTO is the body returned by GET /api/v1/proposals/{id}. It
// composes the opportunity's read-layer view (derived pipeline stage + persisted
// status + coarse display state) with the drafted document, if any. The document
// is omitted entirely for an opportunity with no draft yet.
type ProposalStatusDTO struct {
	// Stage is the derived pipeline stage string (dashboard.DeriveStage).
	Stage string `json:"stage"`
	// State is the coarse display state (proposal.DisplayState): progress, human,
	// done, submitted, or failed.
	State string `json:"state"`
	// Status is the raw persisted ProposalStatus ("{stage}:{status}" vocabulary).
	// Empty when the opportunity has not been selected yet.
	Status string `json:"status,omitempty"`
	// Document is the drafted proposal document, omitted when none exists yet.
	Document *document.Document `json:"document,omitempty"`
}

// toOpportunityDTO flattens a dashboard.OpportunityRow into the wire DTO,
// computing the derived score percentage at the edge. It maps only the fields the
// row actually carries; unbacked fields (estimated value, value-vs-effort signal,
// posted date / days-since-posted) are deliberately not on the list DTO, so the
// API never emits zero/false placeholders for data the row does not supply. The
// detail endpoint exposes the full record when a client needs those fields.
func toOpportunityDTO(row *dashboard.OpportunityRow) OpportunityDTO {
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
		DeadlineSoon:     row.DeadlineSoon,
	}
}

// scorePct converts a 0.0–1.0 fit score to a rounded 0–100 percentage.
func scorePct(score float64) int {
	return int(math.Round(score * 100))
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

// MeResponse is the body returned by GET /api/v1/me. It echoes the authenticated
// caller's identity from the session in context so a client can show who is signed
// in and which Workspace domain they belong to. It carries no tokens or secrets —
// only the identity claims the session already holds.
type MeResponse struct {
	// Email is the verified Workspace email of the signed-in user.
	Email string `json:"email"`
	// Domain is the Google Workspace domain ("hd") the login was restricted to.
	Domain string `json:"domain"`
	// Subject is the Google account's stable unique id ("sub"). Useful as a stable
	// per-user key for the client.
	Subject string `json:"sub"`
}

// ErrorResponse is the envelope returned for non-2xx responses so clients can
// rely on a single error shape across every endpoint.
type ErrorResponse struct {
	// Error is a human-readable message describing what went wrong.
	Error string `json:"error"`
}

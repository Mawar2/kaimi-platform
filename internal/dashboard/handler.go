package dashboard

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/store"
)

// validID accepts only characters that appear in SAM.gov opportunity IDs.
// Rejects path separators and other characters that could enable traversal or injection.
var validID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,200}$`)

// Handler wraps the dashboard Service and manages HTTP routing using stdlib net/http.
type Handler struct {
	svc          *Service
	mux          *http.ServeMux
	listTmpl     *template.Template
	detTmpl      *template.Template
	notFoundTmpl *template.Template
}

// DetailData is the template data contract for /opportunity/{id}.
// All time and score fields are pre-formatted as strings to keep templates free of logic.
type DetailData struct {
	PageTitle           string
	Opp                 opportunity.Opportunity
	DerivedStage        string
	ScoreDisplay        string
	DeadlineSoon        bool
	DeadlineStr         string
	ScoredAtStr         string
	SelectedAtStr       string
	PostedDateStr       string
	CreatedAtStr        string
	UpdatedAtStr        string
	RecommendationClass string
}

// OverviewData is the template data contract for /.
type OverviewData struct {
	PageTitle      string
	Cards          []StageCard
	Rows           []OpportunityRow
	ActiveStage    string
	ActiveMinScore string
	ActiveSort     string
}

// StageCard is one summary card in the pipeline overview header.
type StageCard struct {
	Label string
	Stage string
	Count int
	Alert bool
}

// NewHandler creates a Handler with GET / and GET /opportunity/{id} registered on a
// new stdlib ServeMux.
func NewHandler(svc *Service) *Handler {
	h := &Handler{
		svc: svc,
		mux: http.NewServeMux(),
	}
	h.setupTemplates()
	h.mux.HandleFunc("GET /", h.handleList)
	h.mux.HandleFunc("GET /opportunity/{id}", h.handleDetail)
	return h
}

// ServeHTTP implements http.Handler by delegating to the internal ServeMux.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// handleList renders the pipeline overview page at /.
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	opts := ListOptions{Now: time.Now()}

	if s := query.Get("stage"); s != "" {
		st := Stage(s)
		opts.Stage = &st
	}
	// min_score is expressed as 0–100 integer in the query param; MinScore is 0–1 internally.
	if ms := query.Get("min_score"); ms != "" {
		if f, err := strconv.ParseFloat(ms, 64); err == nil {
			if f < 0 {
				f = 0
			} else if f > 100 {
				f = 100
			}
			opts.MinScore = f / 100
		}
	}
	if query.Get("sort") == string(SortByScore) {
		opts.SortBy = SortByScore
	}

	rows, err := h.svc.List(ctx, opts)
	if err != nil {
		http.Error(w, "failed to load opportunities", http.StatusInternalServerError)
		return
	}

	counts, err := h.svc.CountStages(ctx)
	if err != nil {
		http.Error(w, "failed to load stage counts", http.StatusInternalServerError)
		return
	}

	activeSort := query.Get("sort")
	if activeSort == "" {
		activeSort = string(SortByDeadline)
	}

	data := OverviewData{
		PageTitle: "Pipeline Overview",
		Cards: []StageCard{
			{Label: "Hunted", Stage: string(StageHunted), Count: counts[StageHunted]},
			{Label: "Scored", Stage: string(StageScored), Count: counts[StageScored]},
			{Label: "Selected", Stage: string(StageSelected), Count: counts[StageSelected]},
			{Label: "In Proposal", Stage: string(StageInProposal), Count: counts[StageInProposal]},
			{
				Label: "Awaiting Review", Stage: string(StageAwaitingHumanReview),
				Count: counts[StageAwaitingHumanReview],
				Alert: counts[StageAwaitingHumanReview] > 0,
			},
			{Label: "Finalized", Stage: string(StageFinalized), Count: counts[StageFinalized]},
		},
		Rows:           rows,
		ActiveStage:    query.Get("stage"),
		ActiveMinScore: query.Get("min_score"),
		ActiveSort:     activeSort,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.listTmpl.Execute(w, data); err != nil {
		fmt.Printf("list template error: %v\n", err)
	}
}

// handleDetail renders the full opportunity record at /opportunity/{id}.
// Returns 404 for unknown IDs or IDs that fail the allowlist validation.
func (h *Handler) handleDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Validate against allowlist: only alphanumeric, hyphens, underscores.
	if !validID.MatchString(id) {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()
	opp, err := h.svc.Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			h.renderNotFound(w, id)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	data := DetailData{
		PageTitle:           opp.Title,
		Opp:                 *opp,
		DerivedStage:        string(DeriveStage(opp)),
		ScoreDisplay:        fmtScore(opp.Score),
		DeadlineSoon:        isDeadlineSoon(opp.ResponseDeadline, now),
		DeadlineStr:         fmtDate(opp.ResponseDeadline),
		ScoredAtStr:         fmtPtrTime(opp.ScoredAt),
		SelectedAtStr:       fmtPtrTime(opp.SelectedAt),
		PostedDateStr:       fmtDate(opp.PostedDate),
		CreatedAtStr:        fmtTimeVal(opp.CreatedAt),
		UpdatedAtStr:        fmtTimeVal(opp.UpdatedAt),
		RecommendationClass: recClass(opp.Recommendation),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.detTmpl.Execute(w, data); err != nil {
		fmt.Printf("detail template error: %v\n", err)
	}
}

// renderNotFound writes a 404 HTML response for a valid-format but unknown ID.
func (h *Handler) renderNotFound(w http.ResponseWriter, id string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if err := h.notFoundTmpl.Execute(w, id); err != nil {
		_, _ = fmt.Fprintf(w, "404 not found")
	}
}

// fmtDate formats a time.Time as "2006-01-02", or "—" for zero values.
func fmtDate(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02")
}

// fmtTimeVal formats a time.Time as "2006-01-02 15:04", or "—" for zero values.
func fmtTimeVal(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04")
}

// fmtPtrTime formats a *time.Time as "2006-01-02 15:04", or "—" for nil.
func fmtPtrTime(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.Format("2006-01-02 15:04")
}

// fmtScore formats a score float as "87.3%", or "—" when zero (unscored).
func fmtScore(s float64) string {
	if s == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", s*100)
}

// recClass returns the CSS class for a recommendation string.
func recClass(rec string) string {
	switch rec {
	case "BID":
		return "rec-bid"
	case "NO_BID":
		return "rec-nobid"
	}
	return ""
}

// setupTemplates builds the list, detail, and 404 template sets.
func (h *Handler) setupTemplates() {
	funcMap := template.FuncMap{
		"multiply": func(a, b float64) float64 { return a * b },
		"truncate": func(s string, n int) string {
			runes := []rune(s)
			if len(runes) <= n {
				return s
			}
			return string(runes[:n]) + "…"
		},
	}
	// Each template set is: base layout (outer HTML) + a "content" block definition.
	// The base template contains {{template "content" .}} which is filled per page.
	h.listTmpl = template.Must(template.New("list").Funcs(funcMap).Parse(tmplBase + tmplListContent))
	h.detTmpl = template.Must(template.New("det").Funcs(funcMap).Parse(tmplBase + tmplDetailContent))
	h.notFoundTmpl = template.Must(template.New("notfound").Parse(tmplNotFound))
}

// tmplBase is the shared outer layout. It does not use {{define}} so it becomes the
// body of the named template returned by New("..."). Each page provides a {{define "content"}}
// block that this template calls via {{template "content" .}}.
const tmplBase = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta http-equiv="refresh" content="30">
  <title>Kaimi — {{.PageTitle}}</title>
  <style>
    body { font-family: sans-serif; margin: 1rem 2rem; color: #222; }
    table { border-collapse: collapse; width: 100%; }
    th, td { border: 1px solid #ddd; padding: 0.4rem 0.6rem; text-align: left; }
    th { background: #f5f5f5; }
    tr:nth-child(even) { background: #fafafa; }
    .stage-cards { display: flex; gap: 1rem; flex-wrap: wrap; margin-bottom: 1.5rem; }
    .stage-card { border: 1px solid #ccc; border-radius: 4px; padding: 0.75rem 1rem; min-width: 120px; }
    .stage-card .count { font-size: 2rem; font-weight: bold; }
    .stage-card-alert { background: #fffbe6; border-color: #f0c040; }
    .deadline-soon { background: #fff0f0; color: #c00; font-weight: bold; }
    .rec-bid { color: #080; font-weight: bold; }
    .rec-nobid { color: #c00; font-weight: bold; }
    .filter-bar { margin-bottom: 0.75rem; font-size: 0.9rem; color: #555; }
    a { color: #0057b8; }
  </style>
</head>
<body>
  <h1>Kaimi Pipeline</h1>
  {{template "content" .}}
</body>
</html>`

const tmplListContent = `{{define "content"}}
<div class="stage-cards">
  {{range .Cards}}
  <div class="stage-card {{if .Alert}}stage-card-alert{{end}}">
    <div class="label">{{.Label}}</div>
    <div class="count">{{.Count}}</div>
  </div>
  {{end}}
</div>

<div class="filter-bar">
  <form method="GET" action="/">
    <label>Stage:
      <select name="stage">
        <option value="">All</option>
        <option value="Hunted" {{if eq .ActiveStage "Hunted"}}selected{{end}}>Hunted</option>
        <option value="Scored" {{if eq .ActiveStage "Scored"}}selected{{end}}>Scored</option>
        <option value="Selected" {{if eq .ActiveStage "Selected"}}selected{{end}}>Selected</option>
        <option value="In Proposal" {{if eq .ActiveStage "In Proposal"}}selected{{end}}>In Proposal</option>
        <option value="Awaiting Human Review" {{if eq .ActiveStage "Awaiting Human Review"}}selected{{end}}>Awaiting Human Review</option>
        <option value="Finalized" {{if eq .ActiveStage "Finalized"}}selected{{end}}>Finalized</option>
      </select>
    </label>
    <label>Min Score (%):
      <input type="number" name="min_score" min="0" max="100" step="1" value="{{.ActiveMinScore}}">
    </label>
    <label>Sort:
      <select name="sort">
        <option value="deadline" {{if eq .ActiveSort "deadline"}}selected{{end}}>Deadline</option>
        <option value="score" {{if eq .ActiveSort "score"}}selected{{end}}>Score</option>
      </select>
    </label>
    <button type="submit">Apply</button>
    <a href="/">Clear</a>
  </form>
</div>

<table>
  <thead>
    <tr>
      <th>ID</th>
      <th>Title</th>
      <th>Agency</th>
      <th>NAICS</th>
      <th>Score</th>
      <th>Stage</th>
      <th>Deadline</th>
      <th>Last Updated</th>
    </tr>
  </thead>
  <tbody>
    {{range .Rows}}
    <tr>
      <td title="{{.ID}}">{{truncate .ID 12}}</td>
      <td><a href="/opportunity/{{.ID}}">{{.Title}}</a></td>
      <td>{{.Agency}}</td>
      <td>{{.NAICSCode}}</td>
      <td>
        {{if gt .Score 0.0}}
          {{printf "%.1f%%" (multiply .Score 100)}}
          {{if .ReasoningSnippet}}<br><small title="{{.ReasoningSnippet}}">{{truncate .ReasoningSnippet 80}}</small>{{end}}
        {{else}}—{{end}}
      </td>
      <td>{{.Stage}}</td>
      <td {{if .DeadlineSoon}}style="background:#fff0f0;color:#c00;font-weight:bold"{{end}}>
        {{if .ResponseDeadline.IsZero}}—{{else}}{{.ResponseDeadline.Format "2006-01-02"}}{{if .DeadlineSoon}} ⚠{{end}}{{end}}
      </td>
      <td>{{.LastUpdated.Format "2006-01-02 15:04"}}</td>
    </tr>
    {{else}}
    <tr><td colspan="8">No opportunities found.</td></tr>
    {{end}}
  </tbody>
</table>
{{end}}`

const tmplDetailContent = `{{define "content"}}
<p><a href="/">← Back to pipeline</a></p>

<h2>Identification</h2>
<table>
  <tr><th>ID</th><td>{{.Opp.ID}}</td></tr>
  <tr><th>Title</th><td>{{.Opp.Title}}</td></tr>
  <tr><th>Solicitation #</th><td>{{or .Opp.SolicitationNum "—"}}</td></tr>
  <tr><th>Agency</th><td>{{or .Opp.Agency "—"}}</td></tr>
  <tr><th>Office</th><td>{{or .Opp.Office "—"}}</td></tr>
  <tr><th>Type</th><td>{{or .Opp.Type "—"}}</td></tr>
  <tr><th>Contract Type</th><td>{{or .Opp.ContractType "—"}}</td></tr>
  <tr><th>Set-Aside</th><td>{{or .Opp.SetAsideCode "—"}}</td></tr>
  <tr><th>Place of Performance</th><td>{{or .Opp.PlaceOfPerformance "—"}}</td></tr>
  <tr><th>SAM.gov Link</th>
    <td>{{if .Opp.URL}}<a href="{{.Opp.URL}}">View on SAM.gov</a>{{else}}—{{end}}</td>
  </tr>
</table>

<h2>Dates</h2>
<table>
  <tr><th>Posted</th><td>{{.PostedDateStr}}</td></tr>
  <tr><th>Response Deadline</th>
    <td{{if .DeadlineSoon}} class="deadline-soon"{{end}}>
      {{.DeadlineStr}}{{if .DeadlineSoon}} ⚠{{end}}
    </td>
  </tr>
  <tr><th>Created (local record)</th><td>{{.CreatedAtStr}}</td></tr>
  <tr><th>Last Updated</th><td>{{.UpdatedAtStr}}</td></tr>
</table>

<h2>Classification</h2>
<table>
  <tr><th>NAICS Code</th><td>{{or .Opp.NAICSCode "—"}}</td></tr>
  <tr><th>NAICS Description</th><td>{{or .Opp.NAICSDescription "—"}}</td></tr>
</table>

<h2>Description</h2>
{{if .Opp.Description}}<pre style="white-space:pre-wrap">{{.Opp.Description}}</pre>{{else}}—{{end}}

<h2>Scoring</h2>
<table>
  <tr><th>Score</th><td>{{.ScoreDisplay}}</td></tr>
  <tr><th>Recommendation</th>
    <td>{{if .Opp.Recommendation}}<span class="{{.RecommendationClass}}">{{.Opp.Recommendation}}</span>{{else}}—{{end}}</td>
  </tr>
  <tr><th>Scored At</th><td>{{.ScoredAtStr}}</td></tr>
  <tr><th>Requirements</th>
    <td>{{if .Opp.Requirements}}<ul>{{range .Opp.Requirements}}<li>{{.}}</li>{{end}}</ul>{{else}}—{{end}}</td>
  </tr>
  <tr><th>Full Reasoning</th>
    <td>{{if .Opp.ScoreReasoning}}<pre style="white-space:pre-wrap">{{.Opp.ScoreReasoning}}</pre>{{else}}—{{end}}</td>
  </tr>
</table>

<h2>Eligibility</h2>
<div id="eligibility-placeholder">Eligibility check: not yet implemented (Phase 1+)</div>

<h2>Proposal Status</h2>
<table>
  <tr><th>Current Stage</th><td>{{.DerivedStage}}</td></tr>
  <tr><th>Selected</th><td>{{if .Opp.Selected}}Yes{{else}}No{{end}}</td></tr>
  <tr><th>Selected At</th><td>{{.SelectedAtStr}}</td></tr>
  <tr><th>Proposal Status</th><td>{{or .Opp.ProposalStatus "—"}}</td></tr>
</table>
{{end}}`

// tmplNotFound is the standalone 404 page. It intentionally omits the auto-refresh
// meta tag since there is no live state to poll on an error page.
const tmplNotFound = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Kaimi — Not Found</title>
  <style>body{font-family:sans-serif;margin:1rem 2rem;color:#222;}a{color:#0057b8;}</style>
</head>
<body>
  <h1>Kaimi Pipeline</h1>
  <p>Opportunity not found: {{.}}</p>
  <p><a href="/">← Back to pipeline</a></p>
</body>
</html>`

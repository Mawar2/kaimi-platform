package dashboard

import (
	"fmt"
	"net/http"
	"strings"
)

// This file implements the Submitted archive screen (the new Kaimi App.html
// "Submitted" route, app-submitted.jsx): every proposal that has gone out the
// door, what it's worth, its award outcome, and the reference documents. It is
// server-rendered and no-JS — search and the status filter are plain GET forms,
// and rows expand via native <details> disclosure.

// SubmittedData is the view-model for GET /submitted.
type SubmittedData struct {
	shellData
	PendingValue string // formatted sum of pending-award value
	PendingCount int
	WonValue     string // formatted sum of won value
	WonCount     int
	TotalCount   int
	Query        string // current search text
	Filter       string // all | pending | won | lost
	Rows         []SubmittedRowVM
}

// SubmittedRowVM is one archive row.
type SubmittedRowVM struct {
	ID           string
	Title        string
	Agency       string
	SolNum       string
	ScorePct     int
	HasScore     bool
	Value        string // formatted, "—" when unknown
	SubmittedStr string // "Jan 2, 2006"
	StatusLabel  string
	StatusClass  string // kbadge--pending | kbadge--done | kbadge--muted
	Outcome      string // raw award outcome: "" (pending) | won | lost
	AwardNote    string
	URL          string // SAM.gov solicitation link
	Docs         []SubmittedDocVM
}

// SubmittedDocVM is one reference document chip in the expanded row.
type SubmittedDocVM struct {
	Name string
	Meta string
}

// awardStatus maps the Opportunity.AwardOutcome vocabulary ("", "won", "lost")
// to the badge label/class and a short note. Empty means pending award.
func awardStatus(outcome string) (label, class, note string) {
	switch outcome {
	case "won":
		return "Won", "kbadge--done", "Awarded · congratulations"
	case "lost":
		return "Not awarded", "kbadge--muted", "Not awarded · debrief on file"
	default:
		return "Pending award", "kbadge--pending", "Awaiting the award decision · Kaimi watches SAM.gov for notices"
	}
}

// formatMoney renders a US-dollar amount the way the BD archive reads: "$3.2M",
// "$640K", or "—" when the value is unknown (zero).
func formatMoney(dollars float64) string {
	switch {
	case dollars <= 0:
		return "—"
	case dollars >= 1_000_000:
		v := dollars / 1_000_000
		if v == float64(int64(v)) {
			return fmt.Sprintf("$%dM", int64(v))
		}
		return fmt.Sprintf("$%.1fM", v)
	case dollars >= 1_000:
		return fmt.Sprintf("$%dK", int64(dollars/1_000+0.5))
	default:
		return fmt.Sprintf("$%d", int64(dollars+0.5))
	}
}

// handleSubmitted renders the Submitted archive. It lists submitted
// opportunities (StageSubmitted), applies the search + status filter, computes
// the headline value stats, and loads each row's full record for its documents.
func (h *Handler) handleSubmitted(w http.ResponseWriter, r *http.Request) {
	now := h.Now()
	rows, err := h.svc.List(r.Context(), ListOptions{Now: now})
	if err != nil {
		fmt.Printf("submitted list failed: %v\n", err)
		http.Error(w, "failed to load the archive", http.StatusInternalServerError)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	filter := r.URL.Query().Get("status")
	switch filter {
	case "pending", "won", "lost":
	default:
		filter = "all"
	}

	data := SubmittedData{
		shellData: shellData{PageTitle: "Submitted", ActiveNav: "submitted"},
		Query:     q,
		Filter:    filter,
	}
	data.QueueCount = len(rows)
	ql := strings.ToLower(q)
	var pendingVal, wonVal float64

	for i := range rows {
		row := &rows[i]
		switch row.Stage {
		case StageAwaitingHumanReview:
			data.NeedsCount++
			data.ActiveCount++
		case StageSelected, StageInProposal, StageFinalized:
			data.ActiveCount++
		}
		if row.Stage != StageSubmitted {
			continue
		}
		data.SubmittedCount++
		data.TotalCount++

		// The list row lacks documents/value/outcome — read the full record.
		opp, err := h.svc.Get(r.Context(), row.ID)
		if err != nil {
			continue
		}
		label, class, note := awardStatus(opp.AwardOutcome)
		switch opp.AwardOutcome {
		case "won":
			wonVal += opp.EstimatedValue
		case "lost":
		default:
			pendingVal += opp.EstimatedValue
		}

		// Status filter (applied after the stat tallies so totals stay whole).
		if filter != "all" {
			cur := opp.AwardOutcome
			if cur == "" {
				cur = "pending"
			}
			if cur != filter {
				continue
			}
		}
		// Search across title / agency / solicitation.
		if ql != "" && !strings.Contains(strings.ToLower(row.Title+" "+row.Agency+" "+row.SolicitationNum), ql) {
			continue
		}

		when := opp.UpdatedAt
		if opp.SubmittedAt != nil {
			when = *opp.SubmittedAt
		}
		vm := SubmittedRowVM{
			ID:           opp.ID,
			Title:        opp.Title,
			Agency:       opp.Agency,
			SolNum:       opp.SolicitationNum,
			ScorePct:     int(opp.Score*100 + 0.5),
			HasScore:     opp.Score > 0,
			Value:        formatMoney(opp.EstimatedValue),
			SubmittedStr: when.Format("Jan 2, 2006"),
			StatusLabel:  label,
			StatusClass:  class,
			Outcome:      opp.AwardOutcome,
			AwardNote:    note,
			URL:          opp.URL,
		}
		for di := range opp.Documents {
			d := &opp.Documents[di]
			vm.Docs = append(vm.Docs, SubmittedDocVM{Name: d.Filename, Meta: docSizeMeta(d.Bytes)})
		}
		data.Rows = append(data.Rows, vm)
	}

	data.PendingValue = formatMoney(pendingVal)
	data.WonValue = formatMoney(wonVal)
	// Counts for the stat strip reflect the whole archive, not the filtered view.
	for i := range rows {
		if rows[i].Stage != StageSubmitted {
			continue
		}
		if opp, err := h.svc.Get(r.Context(), rows[i].ID); err == nil {
			switch opp.AwardOutcome {
			case "won":
				data.WonCount++
			case "lost":
			default:
				data.PendingCount++
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.submittedTmpl.Execute(w, data); err != nil {
		fmt.Printf("submitted template failed: %v\n", err)
	}
}

// handleOutcome records a win/loss award decision from the Submitted archive's
// per-row outcome control, then redirects back to the archive. The write goes
// through the proposal service (the dashboard Service is read-only by design).
func (h *Handler) handleOutcome(w http.ResponseWriter, r *http.Request) {
	if h.proposals == nil {
		http.Error(w, "recording outcomes is not enabled on this server", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if !opportunityIDPattern.MatchString(id) {
		h.renderNotFound(w, id)
		return
	}
	outcome := r.FormValue("outcome")
	if outcome == "pending" {
		outcome = ""
	}
	if err := h.proposals.RecordOutcome(r.Context(), id, outcome); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/submitted", http.StatusSeeOther)
}

// docSizeMeta renders a human document size for the reference-document chips.
func docSizeMeta(bytes int64) string {
	switch {
	case bytes <= 0:
		return "SAM.gov"
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%d KB", bytes/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// submittedContentTmpl is the Submitted archive screen (app-submitted.jsx),
// server-rendered: page head + value stats, a search/filter toolbar (GET forms),
// and the archive list where each row expands via native <details> disclosure.
const submittedContentTmpl = `{{define "content"}}
<div class="page">
  <div class="page-head">
    <div class="eyebrow">Pipeline</div>
    <h1>Submitted</h1>
    <p class="lead">Every proposal that's gone out the door: what it's worth, and everything the team produced along the way.</p>
    <div class="stats">
      <div class="stat"><div class="v">{{.PendingValue}}<small> awaiting award</small></div><div class="k">{{.PendingCount}} proposals pending decision</div></div>
      <div class="stat"><div class="v">{{.WonValue}}<small> won</small></div><div class="k">{{.WonCount}} awards on record</div></div>
      <div class="stat"><div class="v">{{.TotalCount}}<small> submitted</small></div><div class="k">All time, via Kaimi</div></div>
    </div>
  </div>

  <div class="toolbar">
    <form class="searchbox" method="GET" action="/submitted">
      ` + iconSearch + `
      <input type="search" name="q" placeholder="Search by title, agency, or solicitation…" value="{{.Query}}">
      {{if ne .Filter "all"}}<input type="hidden" name="status" value="{{.Filter}}">{{end}}
    </form>
    <div class="grow"></div>
    <div class="seg">
      <form method="GET" action="/submitted">
        {{if .Query}}<input type="hidden" name="q" value="{{.Query}}">{{end}}
        <button{{if eq .Filter "all"}} class="on"{{end}} name="status" value="all">All</button>
        <button{{if eq .Filter "pending"}} class="on"{{end}} name="status" value="pending">Pending award</button>
        <button{{if eq .Filter "won"}} class="on"{{end}} name="status" value="won">Won</button>
        <button{{if eq .Filter "lost"}} class="on"{{end}} name="status" value="lost">Not awarded</button>
      </form>
    </div>
    <a class="sortbtn" href="/submitted/export.csv{{if ne .Filter "all"}}?status={{.Filter}}{{end}}">
      <svg width="15" height="15" viewBox="0 0 24 24" fill="none"><path d="M12 4v11m0 0l-4-4m4 4l4-4M5 20h14" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>
      Export report
    </a>
  </div>

  <div class="sub-list">
    {{range .Rows}}
    <details class="srow">
      <summary class="srow-head">
        {{if .HasScore}}{{fitRing .ScorePct 42}}{{end}}
        <div class="s-body">
          <div class="sttl">{{.Title}}</div>
          <div class="smeta">
            <span>{{.Agency}}</span><span class="sep"></span>
            <span class="mono">SOL# {{orDash .SolNum}}</span><span class="sep"></span>
            <span>Submitted {{.SubmittedStr}}</span>
          </div>
        </div>
        <div class="s-right">
          <span class="sval">{{.Value}}</span>
          <span class="kbadge {{.StatusClass}}"><span class="dot"></span>{{.StatusLabel}}</span>
          <span class="schev"><svg viewBox="0 0 24 24" fill="none"><path d="M6 9l6 6 6-6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg></span>
        </div>
      </summary>
      <div class="srow-docs">
        <div class="sd-note">{{.AwardNote}}</div>
        <div class="sd-grid">
          <div>
            <div class="sd-h">Reference documents</div>
            <div class="art-row" style="margin-top:8px">
              {{range .Docs}}<span class="artifact2">` + iconDoc + `{{.Name}}<span style="color:var(--ink-4);font-family:var(--font-mono);font-size:11px">{{.Meta}}</span></span>{{end}}
              {{if .URL}}<a class="artifact2" href="{{.URL}}">` + iconLink + `View on SAM.gov</a>{{end}}
            </div>
          </div>
          <div class="sd-outcome">
            <div class="sd-h">Outcome</div>
            <div class="seg" style="margin-top:8px">
              <form method="POST" action="/submitted/{{.ID}}/outcome">
                <button{{if eq .Outcome ""}} class="on"{{end}} name="outcome" value="pending">Pending</button>
                <button{{if eq .Outcome "won"}} class="on"{{end}} name="outcome" value="won">Won</button>
                <button{{if eq .Outcome "lost"}} class="on"{{end}} name="outcome" value="lost">Not awarded</button>
              </form>
            </div>
            <div class="sd-hint">Award decisions update your pipeline stats. Kaimi also watches SAM.gov for award notices.</div>
          </div>
        </div>
      </div>
    </details>
    {{else}}
    <div class="empty2">
      <div class="g">` + iconSearch + `</div>
      <h3>{{if .Query}}No matches{{else}}Nothing submitted yet{{end}}</h3>
      <p>{{if .Query}}Nothing in the archive matches "{{.Query}}".{{else}}Submit a proposal from the workspace and it lands in this archive.{{end}}</p>
    </div>
    {{end}}
  </div>
</div>
<style>
  /* native <details> disclosure mapped to the design's .srow open state (no JS) */
  details.srow > summary{ list-style:none; cursor:pointer; }
  details.srow > summary::-webkit-details-marker{ display:none; }
  details.srow[open] .schev{ transform:rotate(180deg); }
</style>
{{end}}
`

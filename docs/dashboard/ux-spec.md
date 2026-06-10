# Dashboard UX Specification

**Last updated:** 2026-06-10
**Status:** Approved for implementation (Wave 3; visual treatment updated for #150)

This document is the authoritative contract for the Kaimi dashboard's three views. Handler and template work must implement exactly what is described here — no more, no less. Any deviation from field names, query-param names, or layout requires updating this document first.

**Visual treatment (issue #150):** the dashboard now renders the designed app surface from the design handoff. The **canonical visual source** is `design-handoff/Kaimi-handoff/kaimi/project/design_handoff_kaimi/README.md` (sections "App Structure", "1. Sidebar", "2. Opportunities", "3. Opportunity Drawer") plus the handoff stylesheets (`kaimi/tokens.css`, `kaimi/ui.css`, `kaimi/app.css`), which are embedded verbatim in `internal/dashboard`. This document remains authoritative for the **data contracts**: stage derivation, the deadline flagging rule, the query-param table, the detail-page field list, 404 behavior, and auto-refresh. Where this document and the handoff disagree on pixels, the handoff wins; where they disagree on data or behavior, this document wins.

---

## Technology Constraints

- **Renderer:** Go stdlib `net/http` + `html/template`
- **Styling:** No external CSS files, no JS framework, no external assets. The handoff stylesheets (`tokens.css`, `ui.css`, `app.css`) are embedded **verbatim** as Go string constants in `internal/dashboard` and emitted by `StyleTag()` (tokens + components + app styles) — embedded counts as inline, not external. Inline SVG and `data:` URIs (the brand mark and favicon from `internal/dashboard/brand.go`) also count as inline. Page templates keep only page-specific rules in their own inline `<style>`
- **JavaScript:** None. All interactivity is handled by GET links, HTML form submissions, and server-side rendering
- **Auto-refresh:** `<meta http-equiv="refresh">` tag (see §Auto-Refresh below)
- **Auth:** None; localhost-only, trusted network assumed

---

## Auto-Refresh

Every page that shows live pipeline state includes a meta-refresh header:

```html
<meta http-equiv="refresh" content="30">
```

- **Interval:** 30 seconds
- **Scope:** Triage (`/`) and opportunity detail (`/opportunity/{id}`) both carry this tag
- **Why 30s:** Fast enough to see worker progress without overwhelming the server during active pipeline runs; slow enough that a human reading the detail page is not interrupted mid-sentence

---

## View 1 — Triage (`/`)

The root handler (`GET /`) renders the designed **Triage screen** inside the app shell (see §Shared Layout): page head, stat strip, toolbar (segmented filter + sort), and the day-grouped opportunity card list. Visual values (sizes, colors, spacing) come from the handoff (README §2 "Opportunities" and `kaimi/app.css`); the data rules below remain authoritative.

### 1.1 Page Head and Stat Strip

- **Page head:** eyebrow `TRIAGE` (uppercase, cyan), H1 `Opportunities`, lead paragraph (one sentence describing the queue)
- **Stat strip** (three stats):

| Stat | Source |
|---|---|
| **N in queue** | Total count of stored opportunities |
| **N new today** | Count where `CreatedAt` falls on the server's current date (local time) |
| **top fit score** | Highest `Score × 100` across the queue; `—` if nothing is scored yet |

The former stage cards are **replaced** by this stat strip. Stage counts are no longer rendered on the overview; the §Stage Derivation logic remains authoritative and still backs the `stage` query-param filter. The "Awaiting Human Review" amber signal moves to the sidebar's Proposals needs-badge (see §Shared Layout) — amber is still reserved app-wide for "a human is needed."

### 1.2 Toolbar — Segmented Filter and Sort

- **Segmented filter** (pill container, three segments, rendered as plain GET links — no JS):
  - **All** → `rec` absent/empty
  - **To pursue** → `?rec=BID`
  - **Needs review** → `?rec=REVIEW`
- **Sort button** (right-aligned, GET link): toggles between `Sort: Fit score` (`?sort=score`) and `Sort: Deadline` (`?sort=deadline`)
- Both controls preserve the other active query params when toggled

### 1.3 Day Groups

Cards are split into two groups with uppercase headers and hairline rules:

- **NEW TODAY** — `CreatedAt` falls on the server's current date (local time); these rows also carry the cyan new-dot
- **EARLIER** — everything else

A group renders only when non-empty. The grouping is derived server-side from `CreatedAt` vs. the server's current time.

### 1.4 Opportunity Cards (`.orow`)

The plain-table column contract is **REPLACED** by the card list. Each opportunity renders as one `.orow` white card (the whole card links to `/opportunity/{id}`):

- New-dot (hidden, not removed, when not new — keeps alignment)
- **FitRing, 46px** — `Score × 100` rendered via the shared `FitRing` renderer, band color by score
- **Title** — `Opportunity.Title`
- Meta line — `Opportunity.Agency` · `Opportunity.NAICSCode` (mono)
- Right cluster — recommendation word (`BID` green / `REVIEW` amber / `NO BID` rose, uppercase), `DeadlinePill`, chevron

**Column-to-card mapping** (so data coverage from the old table is preserved):

| Old table column | Where it lives now |
|---|---|
| ID | The card's link target `/opportunity/{id}` (no longer displayed) |
| Title | Card title |
| Agency | Card meta line |
| NAICS | Card meta line (mono) |
| Score | FitRing (46px) |
| Stage | Implicit — via the `stage`/`rec` filters and day-group sections; not displayed on the card |
| Deadline | DeadlinePill |
| Reasoning | Detail page only now (§View 2 header bullets + Full Reasoning section) |
| Last Updated | Detail page only now (§View 2 Dates section) |

### 1.5 Empty State (`.empty2`)

When the filtered list is empty: centered glyph tile, short heading, and the caption "next hunt runs tonight at 02:00" (per handoff `.empty2`).

### 1.6 Stage Derivation (data rule — unchanged)

The "stage" value is derived from `Opportunity` fields using this priority order:

```
if ProposalStatus == "finalized"                     → "finalized"
else if ProposalStatus == "review"                   → "awaiting_review"
else if ProposalStatus in {"outline","draft"}        → "in_proposal"
else if Selected == true                             → "selected"
else if Recommendation == "BID"                      → "scored_bid"
else if Recommendation == "NO_BID"                   → "scored_nobid"
else                                                 → "hunted"
```

This logic must be implemented in a single Go function (e.g. `deriveStage(o Opportunity) string`) shared between the `stage` filter and any renderer that needs a stage label (e.g. the detail page).

### 1.7 Deadline Flagging (data rule — unchanged)

The flagging **rule is unchanged and remains authoritative**: if `ResponseDeadline` is within 7 calendar days from the current server time (inclusive of today), the deadline is flagged as critical. The **rendering** is now the shared `DeadlinePill` renderer (`UrgencyFor` escalation: >30d slate · 14–30d blue · 7–14d amber · <7d solid red `#DC2626`); the within-7-days rule maps to the critical/red band.

If `ResponseDeadline` is zero (not set), display `—` with no pill.

---

## View 1 — Filters and Sort (Query Parameters)

All filters and sort controls on `/` are implemented as plain GET requests (links or `<form method="GET">` submissions — no JS). The query-param contract is:

| Param name | Accepted values | Default (when absent) | Description |
|---|---|---|---|
| `stage` | `hunted`, `scored_bid`, `scored_nobid`, `selected`, `in_proposal`, `awaiting_review`, `finalized`, or empty | _(show all)_ | Filter list to one stage only |
| `rec` | `BID`, `REVIEW`, or empty | _(show all)_ | Recommendation segment filter: `BID` = "To pursue", `REVIEW` = "Needs review", empty = "All" |
| `min_score` | Integer `0`–`100` (inclusive) | `0` | Hide rows where `Score × 100 < min_score` |
| `sort` | `deadline`, `score` | `deadline` | Sort order for the list |
| `order` | `asc`, `desc` | `asc` for `deadline`; `desc` for `score` | Secondary sort direction override |

**Rules:**
- Unknown param values are ignored (treated as default)
- `min_score` values outside `0`–`100` are clamped to the nearest bound
- Sort by `deadline` puts zero deadlines last
- Sort by `score` puts unscored opportunities (score `0`) last
- Multiple filters are combined with AND logic

**Active filter display:** The segmented control and sort button show their own active state visually. When a filter with no dedicated control is active (`stage`, `min_score`), show a one-line summary above the list:

```
Showing: stage=scored_bid, min_score=60 | [Clear filters]
```

The "Clear filters" link points to `/` with no query params.

---

## View 2 — Opportunity Detail (`/opportunity/{id}`)

The detail handler (`GET /opportunity/{id}`) renders the full record for one opportunity.

### URL Pattern

- Path param: `{id}` corresponds to `Opportunity.ID`
- If `{id}` does not match any stored opportunity: render a plain 404 page with message `Opportunity not found: {id}`

### Drawer-Style Header (new for #150)

The page opens with the handoff's Opportunity Drawer top block (README §3), rendered inline at the top of the page (not as an overlay — there is still no JS):

- **FitRing, 92px** with the `FIT` sublabel, band color by score
- **Title** — `Opportunity.Title`
- Sub-line — `Opportunity.Agency` (· contract value when available)
- **MetaTag chips** — `NAICS {NAICSCode}` and `SOL# {SolicitationNum}` (mono, via the shared `MetaTag` renderer)
- **WHY KAIMI SCORED THIS** — bullet list derived from `Opportunity.ScoreReasoning` (cyan dot markers); omitted if not yet scored
- **MUST-HAVE REQUIREMENTS** — checklist rows derived from `Opportunity.Requirements`; omitted if empty
- **View solicitation** link to `Opportunity.URL`

The full-record field sections below are unchanged and remain the authoritative field list.

### Sections and Fields

The detail page is divided into labeled sections using `<h2>` headings.

#### Section: Identification

| Label | Field |
|---|---|
| ID | `Opportunity.ID` |
| Title | `Opportunity.Title` |
| Solicitation # | `Opportunity.SolicitationNum` |
| Agency | `Opportunity.Agency` |
| Office | `Opportunity.Office` |
| Type | `Opportunity.Type` |
| Contract Type | `Opportunity.ContractType` |
| Set-Aside | `Opportunity.SetAsideCode` (empty → `—`) |
| Place of Performance | `Opportunity.PlaceOfPerformance` |
| SAM.gov Link | `Opportunity.URL` rendered as `<a href="...">View on SAM.gov</a>` |

#### Section: Dates

| Label | Field | Format |
|---|---|---|
| Posted | `Opportunity.PostedDate` | `2006-01-02` |
| Response Deadline | `Opportunity.ResponseDeadline` | `2006-01-02`; apply deadline flag (§Deadline Flagging) if within 7 days |
| Created (local record) | `Opportunity.CreatedAt` | `2006-01-02 15:04` |
| Last Updated | `Opportunity.UpdatedAt` | `2006-01-02 15:04` |

#### Section: Classification

| Label | Field |
|---|---|
| NAICS Code | `Opportunity.NAICSCode` |
| NAICS Description | `Opportunity.NAICSDescription` |

#### Section: Description

Full `Opportunity.Description` rendered inside a `<pre style="white-space:pre-wrap">` block to preserve line breaks. If empty: `—`.

#### Section: Scoring

| Label | Field | Format |
|---|---|---|
| Score | `Opportunity.Score` | `87.3%` (one decimal, `—` if zero) |
| Recommendation | `Opportunity.Recommendation` | `BID`, `NO_BID`, `REVIEW`, or `—`; `BID` shown in green, `NO_BID` in red |
| Scored At | `Opportunity.ScoredAt` | `2006-01-02 15:04` or `—` if nil |
| Requirements | `Opportunity.Requirements` | Unordered list `<ul>`; `—` if empty |
| Full Reasoning | `Opportunity.ScoreReasoning` | Rendered inside `<pre style="white-space:pre-wrap">`; `—` if empty |

#### Section: Eligibility

Eligibility results are derived from the set-aside code and the company's capability profile. In Phase 0 / Wave 3 this section is rendered as a placeholder:

```
Eligibility check: not yet implemented (Phase 1+)
```

The HTML element `<div id="eligibility-placeholder">` must be present so Wave 3 handler tests can assert its existence.

#### Section: Proposal Status

| Label | Field | Notes |
|---|---|---|
| Current Stage | Derived stage string (§Stage Derivation) | Human-readable |
| Selected | `Opportunity.Selected` | `Yes` / `No` |
| Selected At | `Opportunity.SelectedAt` | `2006-01-02 15:04` or `—` if nil |
| Proposal Status | `Opportunity.ProposalStatus` | Raw value or `—` if empty |

#### Navigation

A `← Back to pipeline` link at the top of the page pointing to `/` (no query params preserved, per simplicity principle).

---

## Shared Layout

Both views render inside the designed **`.app` shell** (sidebar + main) instead of the bare body, per the handoff's "App Structure" and "1. Sidebar" sections. `FaviconLink()` is unchanged; `StyleTag()` is unchanged in usage but now emits three sheets (tokens + components + app styles):

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta http-equiv="refresh" content="30">
  <title>Kaimi — {{.PageTitle}}</title>
  {{.FaviconLink}}   <!-- dashboard.FaviconLink(): inline data-URI brand favicon (unchanged) -->
  {{.StyleTag}}      <!-- dashboard.StyleTag(): tokens.css + ui.css + app.css, embedded verbatim -->
  <style>/* page-specific rules only */</style>
</head>
<body>
  <div class="app">
    {{template "sidebar" .}}   <!-- see Sidebar below -->
    <main>
      {{template "content" .}}
    </main>
  </div>
</body>
</html>
```

**Sidebar** (server-rendered links, no JS; handoff README §1):
- Logo block: 34px rounded-square navy-gradient tile containing the Kai wave mark (from `internal/dashboard/brand.go`, locked per `Kaimi Brand.html`), wordmark "Kaimi" + sub-label "THE SEEKER" — this replaces the old `HeaderLockup()` header
- "PIPELINE" nav section: **Opportunities** (active on `/`, trailing mono count of queue size) and **Proposals** (trailing **amber needs-badge** with count when any opportunity is in `awaiting_review`, otherwise plain count) — amber remains reserved for "a human is needed"
- User block at the bottom: initials avatar `BM`, name "BlueMeta BD", role "Captures team"

The shell is a CSS grid (`248px 1fr`); under 860px `app.css` collapses it to a single column with the sidebar as a horizontal bar (built into the embedded stylesheet — no extra work in templates).

The `<meta http-equiv="refresh" content="30">` tag is omitted on the 404 error page since there is nothing new to fetch.

**Design system (canonical source):** the full Kaimi design system — tokens (`kaimi/tokens.css` from the handoff: status vocabulary, fit bands, urgency escalation, type/spacing/radii/elevation/motion, light Triage + dark Focus themes), component classes (`kaimi/ui.css`: `kbadge`, `krec`, `kdead`, `kfit`, `kbtn`, `kchip`, `ktag`), and app-shell/screen styles (`kaimi/app.css`: `.app`, `.side`, `.orow`, `.empty2`, drawer block, issue #150) — is defined once in `internal/dashboard`, embedded verbatim from the handoff, and emitted by `StyleTag()` (issues #132/#150). The canonical visual reference for layout and screens is the handoff `README.md` (`design-handoff/Kaimi-handoff/kaimi/project/design_handoff_kaimi/README.md`) together with those stylesheets. Layouts emit `StyleTag()` in `<head>` and keep only page-specific rules in their own inline `<style>`. For status, recommendation, deadline, fit-score, and meta-tag display, handlers use the reusable renderers (`StatusBadge`, `RecommendationPill`, `DeadlinePill`/`UrgencyFor`, `FitRing`/`FitBandFor`, `MetaTag`) instead of hand-rolled markup, so every view stays on one vocabulary.

**Brand color mapping** (semantic — color always means the same thing):
- Ink/text: navy `#0A1B3D`; secondary text `#5A6B86`; links and "agent working" blue `#2563EB`
- "A human is needed" amber `#E8870E` on `#FFF3E0` — used ONLY for Awaiting Human Review
- Bid / done green `#15A06B`; No-Bid rose `#C2354A`; failed / critical deadline red `#DC2626`
- Backgrounds are navy-tinted neutrals (`#FBFCFE` page, `#FAFCFF` panels); borders `rgba(16,30,60,0.12)`
- Fonts are **self-hosted** so the designed faces render on every machine and the deployed app, not just where they happen to be installed. `StyleTag()` embeds Figtree (sans) and Geist Mono (mono) as inline base64 `@font-face` data-URIs (`internal/dashboard/fonts.go`); this is self-hosting, not an external fetch, so it still honors the no-external-assets constraint (no Google Fonts, no network request). Both are SIL OFL variable fonts (`internal/dashboard/fonts/`), chosen as the variable build because the type tokens use non-standard weights (420/430/550/650) that static cuts cannot supply. The token order (`--font-mono: "Geist Mono", "IBM Plex Mono", …`) makes **Geist Mono** the design-system primary; the older handoff screenshots show IBM Plex Mono because the comps only ever loaded that fallback, so mono text now matches the token intent rather than those PNGs.

---

## Template Data Contracts

The handler must pass these structs to the templates. These are the minimum required fields; handlers may embed additional unexported fields.

### OverviewData (passed to `/` template)

```go
type OverviewData struct {
    PageTitle    string
    Rows         []TableRow // feeds the .orow card list; split into NEW TODAY / EARLIER via IsNew
    ActiveStage  string  // current "stage" filter value, empty if none
    ActiveRec    string  // current "rec" segment: "", "BID", or "REVIEW"
    ActiveMinScore int   // current min_score value, 0 if none
    ActiveSort   string  // "deadline" or "score"
    ActiveOrder  string  // "asc" or "desc"
    // plus stat-strip counts (in queue / new today / top fit score) — exact field
    // names are an implementation choice
}

type TableRow struct { // one .orow card (see §1.4 column-to-card mapping)
    ID             string // card link target /opportunity/{id}
    Title          string
    Agency         string
    NAICSCode      string
    ScoreDisplay   string // rendered inside the 46px FitRing; "—" if unscored
    Recommendation string // "BID", "REVIEW", "NO_BID", or "" — card rec word + rec filter
    Stage          string // human-readable label; used for stage filtering, not shown on card
    DeadlineStr    string // "2026-06-14" or "—"
    DeadlineSoon   bool   // true if within 7 days (DeadlinePill critical band)
    IsNew          bool   // CreatedAt is today (server local): NEW TODAY group + new-dot
}
```

The former `StageCard` struct is retired with the stage cards (§1.1). `ReasoningSnip` and `UpdatedStr` are no longer required on the overview — reasoning and last-updated now appear on the detail page only (§1.4 mapping).

### DetailData (passed to `/opportunity/{id}` template)

```go
type DetailData struct {
    PageTitle        string
    Opp              opportunity.Opportunity
    DerivedStage     string
    ScoreDisplay     string   // "87.3%" or "—"
    DeadlineSoon     bool
    DeadlineStr      string
    ScoredAtStr      string   // "2026-01-02 15:04" or "—"
    SelectedAtStr    string
    PostedDateStr    string
    CreatedAtStr     string
    UpdatedAtStr     string
    RecommendationClass string // "rec-bid", "rec-nobid", or ""
}
```

---

## Non-Goals (explicitly out of scope for Wave 3)

- Pagination (the list shows all matching cards; defer to Wave 4 if needed)
- Write operations via the dashboard (no "Select to pursue" button, no status changes — the handoff drawer's CTA is out of scope)
- Authentication or session management
- WebSocket live-push (meta-refresh is sufficient for Wave 3)
- External CSS files, icon libraries, or *fetched* fonts (the handoff stylesheets are embedded verbatim; the Figtree/Geist Mono faces are embedded as inline base64 `@font-face`, never fetched over the network — see Brand color mapping above)
- Mobile-responsive layout as dedicated work — **superseded**: the embedded `app.css` ships a built-in 860px single-column collapse (sidebar becomes a horizontal bar), which we get for free; no responsive work beyond that is in scope

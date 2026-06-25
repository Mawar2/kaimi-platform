package dashboard

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Mawar2/Kaimi/internal/capabilitymap"
	"github.com/Mawar2/Kaimi/internal/contextdoc"
	"github.com/Mawar2/Kaimi/internal/finalreview"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/profile"
	"github.com/Mawar2/Kaimi/internal/proposal"
	"github.com/Mawar2/Kaimi/internal/store"
)

// Handler wraps the dashboard service and manages HTTP routing. Pages render
// the designed app surface from the design handoff (Kaimi App.html /
// kaimi/app.css, GitHub issue #150): an app shell with sidebar, the Triage
// opportunities screen, and the drawer-style opportunity detail.
type Handler struct {
	svc            *Service
	proposals      *proposal.Service // nil = read-only deployment
	mux            *http.ServeMux
	listTmpl       *template.Template
	detailTmpl     *template.Template
	proposalsTmpl  *template.Template
	workspaceTmpl  *template.Template
	submittedTmpl  *template.Template
	editorTmpl     *template.Template
	onboardingTmpl *template.Template
	capMapTmpl     *template.Template
	notFoundTmpl   *template.Template
	Now            func() time.Time

	// WS-C3 onboarding collaborators. All optional: onboarding routes answer 503
	// when profileStore is nil; identity/driveStatus degrade to signed-out / "not
	// configured" treatments when nil. They are injected via the WithProfileStore /
	// WithIdentity / WithDriveStatus options so the dashboard package never imports
	// internal/httpapi (which would be an import cycle).
	profileStore profile.ProfileStore
	identity     IdentityFunc
	driveStatus  DriveStatusFunc

	// driveTargetSaver persists a new Drive destination chosen on the onboarding page
	// (WS-C5b). It is the SAME write path the JSON PUT
	// /api/v1/integrations/drive/target uses — cmd/api wires it to the drivetoken
	// TargetStore.Save so the SSR form and the JSON API never diverge into two stores.
	// nil = the change-destination control is hidden (no parallel write path is
	// invented when Drive connect is not wired). See WithDriveTargetSaver.
	driveTargetSaver DriveTargetSaver

	// samKeySaver persists the tenant's SAM.gov API key from the onboarding "Connect"
	// step to Secret Manager (see SAMKeySaver). nil = the key field is hidden and the
	// page shows the deployment-secret note. cmd/api wires it via WithSAMKeySaver.
	samKeySaver SAMKeySaver

	// contextDocs stores the context documents (capability statements, CPARS, past
	// proposals) a tester uploads on the onboarding "Connect" step; their extracted
	// text feeds the capability map. nil = the upload control is hidden. cmd/api wires
	// it via WithContextDocs.
	contextDocs contextdoc.Store

	// rebuildMap (re)builds the tenant's capability map from the saved profile + uploaded
	// docs. The onboarding profile-save and doc-upload handlers call it best-effort after
	// a successful write — a build failure must never fail the save/upload. nil = no map
	// is built (the capability-map view shows "not built yet"). cmd/api wires it via
	// WithCapabilityMapRebuild.
	rebuildMap func(ctx context.Context) error

	// capMap reads the tenant's capability map for the "Your capability map" view. nil =
	// the view reports the feature is unavailable. cmd/api wires it via WithCapabilityMap.
	capMap capabilitymap.Store

	// asyncRun dispatches background work (the capability-map rebuild) off the request
	// path so onboarding saves return immediately. Defaults to `go fn()`; never nil.
	asyncRun func(fn func())

	// rebuildState serializes + coalesces capability-map rebuilds. Concurrent triggers
	// (e.g. a profile save immediately followed by a doc upload) must not race — the
	// last-finishing rebuild would otherwise clobber a fresher one. At most one rebuild
	// runs at a time; a trigger arriving mid-run sets pending so a final rebuild runs
	// afterward against the latest profile + docs.
	rebuildState struct {
		mu      sync.Mutex
		running bool
		pending bool
	}

	// insecureNoAuth records whether running WITHOUT authentication is an explicit
	// operator opt-in (the same -insecure-no-auth / KAIMI_INSECURE_NO_AUTH signal
	// cmd/api uses to gate the whole API). It defaults to false so production fails
	// closed: a state-mutating onboarding POST with no resolvable session is rejected
	// rather than silently allowed. It is set true ONLY in explicit local dev, where
	// the onboarding profile write is permitted without a CSRF token (relying on the
	// SameSite=Lax cookie + same-origin server). See handleOnboardingProfile.
	insecureNoAuth bool

	// tenantName is the configured customer display name shown in the sidebar
	// account block (e.g. "Example Federal Co"). It is the only customer-identity
	// string in the dashboard chrome; the Kaimi product brand (colors, mark,
	// wordmark) is fixed. Empty falls back to the neutral product label so the
	// sidebar never renders blank — see tenantLabel.
	tenantName string
}

// defaultTenantLabel is the neutral product label rendered in the sidebar when
// no tenant display name is configured. The dashboard never renders a blank
// account name.
const defaultTenantLabel = "Kaimi"

// Option configures optional Handler capabilities.
type Option func(*Handler)

// WithProposals enables the Zone 2 surfaces (select, proposals view,
// workspace, gate actions) backed by the shared proposal lifecycle service.
func WithProposals(svc *proposal.Service) Option {
	return func(h *Handler) { h.proposals = svc }
}

// WithTenantName sets the customer display name shown in the sidebar account
// block. Pass config.Tenant.DisplayName here. An empty name falls back to the
// neutral product label ("Kaimi"); the rest of the brand chrome is unaffected.
func WithTenantName(name string) Option {
	return func(h *Handler) { h.tenantName = name }
}

// WithInsecureNoAuth records whether unauthenticated, state-mutating onboarding
// POSTs are an EXPLICIT operator opt-in (local dev only). cmd/api passes the same
// allowInsecure value it uses to gate the API, so production (OAuth on, allowInsecure
// false) fails closed: an onboarding profile write with no resolvable session is
// rejected. When true (dev), the write is permitted without a CSRF token, relying on
// the SameSite=Lax session cookie + same-origin server.
func WithInsecureNoAuth(allow bool) Option {
	return func(h *Handler) { h.insecureNoAuth = allow }
}

// NewHandler initializes a new dashboard handler.
func NewHandler(svc *Service, opts ...Option) *Handler {
	h := &Handler{
		svc: svc,
		mux: http.NewServeMux(),
		Now: time.Now,
		// Default dispatcher runs background work in a goroutine so the capability-map
		// rebuild doesn't block onboarding saves (the Gemini build takes ~15s).
		asyncRun: func(fn func()) { go fn() },
	}
	for _, opt := range opts {
		opt(h)
	}
	h.setupRoutes()
	h.setupTemplates()
	return h
}

func (h *Handler) setupRoutes() {
	h.mux.HandleFunc("/", h.handleList)
	h.mux.HandleFunc("GET /opportunity/{id}", h.handleDetail)
	// Zone 2 surfaces (issue #156): the bridge event, the command view,
	// the workspace, and the gate actions. Action routes are registered
	// method-agnostic with an explicit guard so a stray GET gets a 405
	// instead of falling through to the catch-all list route.
	h.mux.HandleFunc("/opportunity/{id}/select", postOnly(h.handleSelect))
	h.mux.HandleFunc("GET /proposals", h.handleProposals)
	h.mux.HandleFunc("GET /submitted", h.handleSubmitted)
	h.mux.HandleFunc("GET /submitted/export.csv", h.handleSubmittedExport)
	h.mux.HandleFunc("/submitted/{id}/outcome", postOnly(h.handleOutcome))
	h.mux.HandleFunc("GET /workspace/{id}", h.handleWorkspace)
	h.mux.HandleFunc("GET /editor/{id}", h.handleEditor)
	// WS-C3 onboarding: the in-product setup flow. GET renders the checklist; the
	// profile POST is method-guarded so a stray GET 405s instead of falling through.
	h.mux.HandleFunc("GET /onboarding", h.handleOnboarding)
	h.mux.HandleFunc("/onboarding/profile", postOnly(h.handleOnboardingProfile))
	// WS-C5b: change the Drive destination from the onboarding/settings page without
	// editing files. POST-only and CSRF-gated like the profile write; it persists via
	// the SAME drivetoken target store the JSON PUT endpoint uses (no parallel store).
	h.mux.HandleFunc("/onboarding/drive/target", postOnly(h.handleOnboardingDriveTarget))
	// SAM.gov API key entry (onboarding "Connect" step). POST-only and CSRF-gated like
	// the profile write; it persists to Secret Manager via the injected SAMKeySaver so
	// each tenant supplies their own key (per-tenant SAM quota isolation).
	h.mux.HandleFunc("/onboarding/samgov", postOnly(h.handleOnboardingSAMKey))
	// Context-document upload (onboarding "Connect" step). POST-only, multipart,
	// CSRF-gated; persists via the contextdoc store whose text feeds the capability map.
	h.mux.HandleFunc("/onboarding/docs", postOnly(h.handleOnboardingDocUpload))
	// "Your capability map" view — how Kaimi understands the tenant's business.
	h.mux.HandleFunc("GET /capability-map", h.handleCapabilityMap)
	// #246 B3: the working draft is downloadable as Markdown from the workspace.
	h.mux.HandleFunc("GET /workspace/{id}/draft.md", h.handleDraftDownload)
	h.mux.HandleFunc("/workspace/{id}/section/{sid}", postOnly(h.handleSectionSave))
	h.mux.HandleFunc("/workspace/{id}/approve", postOnly(h.handleAction("approve")))
	h.mux.HandleFunc("/workspace/{id}/changes", postOnly(h.handleAction("changes")))
	h.mux.HandleFunc("/workspace/{id}/submit", postOnly(h.handleAction("submit")))
}

// postOnly rejects every method but POST on human-action routes.
func postOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

// Inline SVG icons from the design handoff (app-screens.jsx /
// lifecycle-components.jsx). All stroke-based, currentColor.
const (
	iconQueue   = `<svg viewBox="0 0 24 24" fill="none"><path d="M4 6h16M4 12h16M4 18h10" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`
	iconProps   = `<svg viewBox="0 0 24 24" fill="none"><rect x="3" y="4" width="18" height="6" rx="2" stroke="currentColor" stroke-width="2"/><rect x="3" y="14" width="18" height="6" rx="2" stroke="currentColor" stroke-width="2"/></svg>`
	iconArchive = `<svg viewBox="0 0 24 24" fill="none"><rect x="3" y="4" width="18" height="5" rx="1.5" stroke="currentColor" stroke-width="2"/><path d="M5 9v9a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V9" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><path d="M10 13h4" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`
	iconSearch  = `<svg viewBox="0 0 24 24" fill="none"><circle cx="11" cy="11" r="7" stroke="currentColor" stroke-width="2"/><path d="M16 16l4 4" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`
	iconSliders = `<svg viewBox="0 0 24 24" fill="none"><path d="M4 8h10M18 8h2M4 16h2M10 16h10" stroke="currentColor" stroke-width="2" stroke-linecap="round"/><circle cx="16" cy="8" r="2.4" stroke="currentColor" stroke-width="2"/><circle cx="8" cy="16" r="2.4" stroke="currentColor" stroke-width="2"/></svg>`
	iconChev    = `<svg viewBox="0 0 24 24" fill="none"><path d="M9 6l6 6-6 6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`
	iconBack    = `<svg viewBox="0 0 24 24" fill="none"><path d="M19 12H5M11 18l-6-6 6-6" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/></svg>`
	iconCheck   = `<svg viewBox="0 0 24 24" fill="none"><path d="M5 13l4 4L19 7" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"/></svg>`
	iconWarn    = `<svg viewBox="0 0 24 24" fill="none"><path d="M12 4l9 16H3z" stroke="currentColor" stroke-width="1.9" stroke-linejoin="round"/><path d="M12 10v4M12 17v.01" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>`
	iconLink    = `<svg viewBox="0 0 24 24" fill="none"><path d="M10 14a4 4 0 0 0 5.7 0l2.3-2.3a4 4 0 1 0-5.7-5.7L11 7.3M14 10a4 4 0 0 0-5.7 0L6 12.3a4 4 0 1 0 5.7 5.7l1.3-1.3" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>`
)

// sidebarMarkSVG is the 22px Kai wave mark inside the sidebar's navy gradient
// logo tile, verbatim from the design handoff sidebar.
const sidebarMarkSVG = `<svg width="22" height="22" viewBox="0 0 64 64" fill="none"><circle cx="45" cy="19" r="7" fill="#22D3EE"/><path d="M9 38C17 28 24 28 31 38C38 48 45 48 53 38" stroke="#67E0F4" stroke-width="5.4" stroke-linecap="round"/><path d="M9 48C17 38 24 38 31 48C38 58 45 58 53 48" stroke="#fff" stroke-width="5.4" stroke-linecap="round" opacity="0.9"/></svg>`

// shellTmpl is the shared app shell (design handoff "App Structure"): CSS
// grid with the fixed sidebar and the main column. Page content is supplied
// by a per-page "content" template. The small inline style block holds only
// link resets the prototype did not need (it used buttons + JS routing; we
// use real anchors and GET forms).
const shellTmpl = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta http-equiv="refresh" content="30">
  <title>Kaimi — {{.PageTitle}}</title>
  {{faviconLink}}
  {{styleTag}}
  <style>
    a.nav-item, a.orow, a.pcard, a.artifact2, a.sortbtn { text-decoration: none; color: inherit; }
    a.orow { display: flex; }
    .seg form { display: contents; }
  </style>
</head>
<body>
  <div class="app">
    <aside class="side">
      <div class="logo">
        <div class="mk">` + sidebarMarkSVG + `</div>
        <div class="nm">Kaimi<small>the seeker</small></div>
      </div>
      <div class="nav-h">Pipeline</div>
      <a class="nav-item{{if eq .ActiveNav "opps"}} on{{end}}" href="/">
        ` + iconQueue + `
        <span>Opportunities</span>
        <span class="count">{{.QueueCount}}</span>
      </a>
      <a class="nav-item{{if eq .ActiveNav "proposals"}} on{{end}}" href="/proposals">
        ` + iconProps + `
        <span>Proposals</span>
        {{if gt .NeedsCount 0}}<span class="needs">{{.NeedsCount}}</span>{{else}}<span class="count">{{.ActiveCount}}</span>{{end}}
      </a>
      <a class="nav-item{{if eq .ActiveNav "submitted"}} on{{end}}" href="/submitted">
        ` + iconArchive + `
        <span>Submitted</span>
        <span class="count">{{.SubmittedCount}}</span>
      </a>
      <div class="spacer"></div>
      <div class="me">
        <div class="av">{{tenantInitials}}</div>
        <div class="who"><b>{{tenantName}}</b><span>Captures team</span></div>
      </div>
    </aside>
    <main class="main">
      {{template "content" .}}
    </main>
  </div>
</body>
</html>
`

// listContentTmpl is the Triage screen (design handoff "2. Opportunities"):
// page head with stat strip, segmented recommendation filter + sort toggle,
// day-grouped opportunity row cards, and the designed empty state.
const listContentTmpl = `{{define "content"}}
<div class="page">
  {{if .FirstRun}}
  <a class="firstrun" href="/onboarding">
    ` + iconWarn + `
    <span><b>Finish setting up Kaimi.</b> Configure your company profile so hunting and scoring match your business.</span>
    <span class="firstrun-cta">Complete onboarding ` + iconChev + `</span>
  </a>
  <style>
    .firstrun { display:flex; align-items:center; gap:12px; padding:14px 16px; margin-bottom:18px; border:1px solid var(--primary,#0b5fff); border-radius:10px; background:var(--surface-2); color:inherit; text-decoration:none; }
    .firstrun svg { width:20px; height:20px; flex:0 0 auto; }
    .firstrun-cta { margin-left:auto; display:inline-flex; align-items:center; gap:4px; color:var(--primary,#0b5fff); font-weight:700; white-space:nowrap; }
    .firstrun-cta svg { width:16px; height:16px; }
  </style>
  {{end}}
  <div class="page-head">
    <div class="eyebrow">Triage</div>
    <h1>Opportunities</h1>
    <p class="lead">Live federal opportunities Kaimi hunted and scored against your capabilities. Pick what to pursue.</p>
    <div class="stats">
      <div class="stat">
        <div class="v">{{.QueueCount}}<small> in queue</small></div>
        <div class="k">From last night&#39;s SAM.gov run</div>
      </div>
      <div class="stat">
        <div class="v">{{.NewCount}}<small> new</small></div>
        <div class="k">Added today</div>
      </div>
      <div class="stat">
        <div class="v">{{.TopFit}}</div>
        <div class="k">Top fit score</div>
      </div>
    </div>
  </div>

  <div class="toolbar">
    <div class="seg">
      <form method="GET" action="/"><input type="hidden" name="sort" value="{{.ActiveSort}}">
        <button{{if eq .ActiveRec ""}} class="on"{{end}} name="rec" value="">All</button>
        <button{{if eq .ActiveRec "BID"}} class="on"{{end}} name="rec" value="BID">To pursue</button>
        <button{{if eq .ActiveRec "REVIEW"}} class="on"{{end}} name="rec" value="REVIEW">Needs review</button>
      </form>
    </div>
    <div class="grow"></div>
    <form method="GET" action="/"><input type="hidden" name="rec" value="{{.ActiveRec}}">
      <button class="sortbtn" name="sort" value="{{.SortToggle}}">` + iconSliders + `Sort: {{.SortLabel}}</button>
    </form>
  </div>

  <div class="opp-list">
    {{if .TodayRows}}
    <div class="day"><span>New today</span><span class="ln"></span></div>
    {{range .TodayRows}}{{template "orow" .}}{{end}}
    {{end}}
    {{if .EarlierRows}}
    <div class="day"><span>Earlier</span><span class="ln"></span></div>
    {{range .EarlierRows}}{{template "orow" .}}{{end}}
    {{end}}
    {{if and .Empty (not .FirstRun)}}
    {{if .Filtered}}
    <div class="empty2">
      <div class="g">` + iconSearch + `</div>
      <h3>Nothing here right now</h3>
      <p>No opportunities match this filter. The next hunt runs tonight at 02:00.</p>
    </div>
    {{else}}
    <div class="empty2">
      <div class="g">` + iconSearch + `</div>
      <h3>No opportunities yet</h3>
      <p>The pipeline runs on a schedule and the next hunt lands here automatically — the first results show up after tonight&#39;s 02:00 SAM.gov run.</p>
    </div>
    {{end}}
    {{end}}
  </div>
</div>
{{end}}

{{define "orow"}}
<a class="orow{{if .IsNew}} new{{end}}" href="/opportunity/{{.ID}}">
  <span class="newdot" title="New today"></span>
  {{if .ScorePct}}{{fitRing .ScorePct 46}}{{end}}
  <div class="body">
    <div class="ttl">{{.Title}}</div>
    <div class="meta">
      <span>{{.Agency}}</span><span class="sep"></span>
      <span class="naics">NAICS {{.NAICSCode}}</span>
    </div>
  </div>
  <div class="right">
    {{if .RecClass}}<span class="rec-min rec-min--{{.RecClass}}">{{.RecWord}}</span>{{end}}
    {{if .DeadlineLabel}}{{deadlinePill .DeadlineLabel .DeadlineDays}}{{end}}
    <span class="chev">` + iconChev + `</span>
  </div>
</a>
{{end}}
`

// detailContentTmpl is the opportunity detail: the handoff drawer's content
// blocks (dr-top, tags, reasons, must-have checklist, solicitation link)
// followed by the full-record sections required by ux-spec View 2.
const detailContentTmpl = `{{define "content"}}
<div class="ws">
  <a class="back" href="/">` + iconBack + `All opportunities</a>

  <div class="dr-top" style="margin-top:14px">
    {{if .ScorePct}}{{fitRing .ScorePct 92}}{{end}}
    <div>
      <h2>{{.Opp.Title}}</h2>
      <div class="dr-sub">{{.Opp.Agency}}</div>
      <div class="dr-tags">
        {{if .Opp.NAICSCode}}{{metaTag (printf "NAICS %s" .Opp.NAICSCode)}}{{end}}
        {{if .Opp.SolicitationNum}}{{metaTag (printf "SOL# %s" .Opp.SolicitationNum)}}{{end}}
        {{recPill .Opp.Recommendation}}
        {{if .DeadlineLabel}}{{deadlinePill .DeadlineLabel .DeadlineDays}}{{end}}
      </div>
    </div>
  </div>

  {{if .Reasons}}
  <div class="dr-sec-h">Why Kaimi scored this {{.ScorePct}}</div>
  <ul class="reasons">
    {{range .Reasons}}<li><span class="rd"></span>{{.}}</li>{{end}}
  </ul>
  {{end}}

  {{if .Opp.Requirements}}
  <div class="dr-sec-h">Must-have requirements</div>
  <div class="musts">
    {{range .Opp.Requirements}}
    <div class="must {{$.MustClass}}"><span class="mc">{{if eq $.MustClass "ok"}}` + iconCheck + `{{else}}` + iconWarn + `{{end}}</span>{{.}}</div>
    {{end}}
  </div>
  {{end}}

  <div class="art-row" style="margin-top:22px">
    {{if .Opp.Selected}}
    <a class="kbtn kbtn--secondary" href="/workspace/{{.Opp.ID}}" style="text-decoration:none">` + iconCheck + `In your proposals</a>
    {{else if .CanSelect}}
    <form method="POST" action="/opportunity/{{.Opp.ID}}/select" style="margin:0">
      <button class="kbtn kbtn--select kbtn--lg">` + iconArrow + `Select to pursue</button>
    </form>
    {{end}}
  </div>
  {{if .Opp.URL}}
  <div class="art-row" style="margin-top:10px">
    <a class="artifact2" href="{{.Opp.URL}}" target="_blank" rel="noopener noreferrer">` + iconLink + `View solicitation</a>
  </div>
  {{end}}

  {{if .HasCapMatch}}
  <div class="dr-sec-h">Why this fits your capabilities</div>
  {{if .CapMatch.Coverage}}
  <p class="cap-lead">Matched against your capability map:</p>
  <div class="cap-chips">
    {{range .CapMatch.Competencies}}<span class="cap-chip cap-chip--strong">{{.}}</span>{{end}}
    {{range .CapMatch.Domains}}<span class="cap-chip cap-chip--dom">{{.}}</span>{{end}}
    {{range .CapMatch.Keywords}}<span class="cap-chip">{{.}}</span>{{end}}
  </div>
  {{else}}
  <p class="cap-lead">No direct capability matches in the listing summary. Kaimi will assess fit more deeply against the full solicitation text.</p>
  {{end}}
  {{end}}

  <div class="dr-sec-h">Identification</div>
  <table class="kv">
    <tr><td>ID</td><td>{{.Opp.ID}}</td></tr>
    <tr><td>Solicitation #</td><td>{{orDash .Opp.SolicitationNum}}</td></tr>
    <tr><td>Office</td><td>{{orDash .Opp.Office}}</td></tr>
    <tr><td>Type</td><td>{{orDash .Opp.Type}}</td></tr>
    <tr><td>Contract Type</td><td>{{orDash .Opp.ContractType}}</td></tr>
    <tr><td>Set-Aside</td><td>{{orDash .Opp.SetAsideCode}}</td></tr>
    <tr><td>Place of Performance</td><td>{{orDash .Opp.PlaceOfPerformance}}</td></tr>
  </table>

  <div class="dr-sec-h">Dates</div>
  <table class="kv">
    <tr><td>Posted</td><td>{{orDash .PostedDateStr}}</td></tr>
    <tr><td>Response Deadline</td><td{{if .DeadlineSoon}} class="deadline-soon"{{end}}>{{orDash .DeadlineStr}}{{if .DeadlineSoon}} ⚠{{end}}</td></tr>
    <tr><td>Created (local record)</td><td>{{orDash .CreatedAtStr}}</td></tr>
    <tr><td>Last Updated</td><td>{{orDash .UpdatedAtStr}}</td></tr>
  </table>

  <div class="dr-sec-h">Classification</div>
  <table class="kv">
    <tr><td>NAICS Code</td><td>{{orDash .Opp.NAICSCode}}</td></tr>
    <tr><td>NAICS Description</td><td>{{orDash .Opp.NAICSDescription}}</td></tr>
  </table>

  <div class="dr-sec-h">Description</div>
  {{if isHTTPURL .Opp.Description}}<p><a href="{{.Opp.Description}}" target="_blank" rel="noopener noreferrer">View the full solicitation description on SAM.gov ↗</a></p>
  {{else if .Opp.Description}}<pre class="detail-pre">{{.Opp.Description}}</pre>
  {{else}}<p>&mdash;</p>{{end}}

  {{if .Opp.Attachments}}
  <div class="dr-sec-h">Solicitation documents</div>
  <ul class="dr-attach">
    {{range .Opp.Attachments}}<li><a href="{{.}}" target="_blank" rel="noopener noreferrer">{{.}}</a></li>{{end}}
  </ul>
  {{end}}

  <div class="dr-sec-h">Scoring</div>
  <table class="kv">
    <tr><td>Score</td><td>{{.ScoreDisplay}}</td></tr>
    <tr><td>Recommendation</td><td>{{if .Opp.Recommendation}}{{recPill .Opp.Recommendation}}{{else}}&mdash;{{end}}</td></tr>
    <tr><td>Scored At</td><td>{{orDash .ScoredAtStr}}</td></tr>
    <tr><td>Full Reasoning</td><td>{{if .Opp.ScoreReasoning}}<pre class="detail-pre">{{.Opp.ScoreReasoning}}</pre>{{else}}&mdash;{{end}}</td></tr>
  </table>

  <div class="dr-sec-h">Eligibility</div>
  <!-- Every opportunity in the store passed the Zone-1 gate by construction:
       internal/pipeline drops ineligible ones before saving (issue #256). -->
  <div id="eligibility-note">Passed Zone-1 eligibility screening (NAICS + set-aside gate against the capability profile) before scoring.</div>

  <div class="dr-sec-h">Proposal Status</div>
  <table class="kv">
    <tr><td>Current Stage</td><td>{{.DerivedStage}}</td></tr>
    <tr><td>Selected</td><td>{{if .Opp.Selected}}Yes{{else}}No{{end}}</td></tr>
    <tr><td>Selected At</td><td>{{orDash .SelectedAtStr}}</td></tr>
    <tr><td>Proposal Status</td><td>{{orDash .Opp.ProposalStatus}}</td></tr>
  </table>
</div>

<style>
  .kv { border-collapse: collapse; width: 100%; background: var(--surface); }
  .kv td { border: 1px solid var(--border); padding: var(--s-2) var(--s-3); vertical-align: top; font: var(--t-small); }
  .kv td:first-child { color: var(--ink-3); width: 200px; background: var(--surface-2); }
  .detail-pre { white-space: pre-wrap; background: var(--surface-2); border: 1px solid var(--border); border-radius: var(--r-sm); padding: var(--s-3); font: var(--t-small); margin: 0; }
  .deadline-soon { background: var(--st-failed-bg); color: var(--st-failed); font-weight: bold; }
  .cap-lead { font: var(--t-small); color: var(--ink-3); margin: 0 0 var(--s-2); }
  .cap-chips { display: flex; flex-wrap: wrap; gap: var(--s-2); }
  .cap-chip { font: var(--t-small); background: var(--surface-2); border: 1px solid var(--border); border-radius: var(--r-pill, 999px); padding: 3px 10px; color: var(--ink-2); }
  .cap-chip--strong { background: var(--st-ok-bg, #e7f7ee); color: var(--st-ok, #1a7f4b); border-color: transparent; font-weight: 600; }
  .cap-chip--dom { background: var(--st-progress-bg, #e7eeff); color: var(--st-progress, #2563eb); border-color: transparent; }
</style>
{{end}}
`

// notFoundTmplStr is the plain 404 page; it deliberately omits the
// auto-refresh meta tag (ux-spec: nothing new to fetch).
const notFoundTmplStr = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Kaimi — Not Found</title>
  {{faviconLink}}
  {{styleTag}}
  <style>body { margin: 1rem 2rem; } a { color: var(--primary); }</style>
</head>
<body>
  {{headerLockup}}
  <p>Opportunity not found: {{.ID}}</p>
  <p><a href="/">← Back to pipeline</a></p>
</body>
</html>
`

func (h *Handler) setupTemplates() {
	funcMap := template.FuncMap{
		// Brand and design-system assets (issues #126/#132/#141/#150).
		"faviconLink":  FaviconLink,
		"styleTag":     StyleTag,
		"headerLockup": HeaderLockup,
		// Customer-identity strings for the sidebar account block. Closed over
		// the handler so the configured tenant name (WS-A1) renders without
		// threading it through every page view-model; empty falls back to the
		// neutral product label.
		"tenantName":     h.tenantLabel,
		"tenantInitials": h.tenantInitials,
		"fitRing":        FitRing,
		"recPill":        RecommendationPill,
		"deadlinePill":   DeadlinePill,
		"metaTag":        MetaTag,
		"miniPipe":       miniPipe,
		"wPipe":          wPipe,
		"propChip":       propChip,
		// Unresolved Writer gaps (issue #269): per-body gap texts for the
		// section editors, inline <mark> highlighting for read-only views.
		"gapTexts":      finalreview.GapTexts,
		"highlightGaps": highlightGaps,
		"orDash": func(s string) string {
			if s == "" {
				return "—"
			}
			return s
		},
		// isHTTPURL reports whether a string is an http(s) URL — used to render the
		// SAM.gov description (which the API returns as a noticedesc URL, not text) as
		// a link rather than dumping the raw URL as prose.
		"isHTTPURL": func(s string) bool {
			return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
		},
	}
	h.listTmpl = template.Must(template.Must(
		template.New("list").Funcs(funcMap).Parse(shellTmpl)).Parse(listContentTmpl))
	h.detailTmpl = template.Must(template.Must(
		template.New("detail").Funcs(funcMap).Parse(shellTmpl)).Parse(detailContentTmpl))
	h.proposalsTmpl = template.Must(template.Must(
		template.New("proposals").Funcs(funcMap).Parse(shellTmpl)).Parse(proposalsContentTmpl))
	h.workspaceTmpl = template.Must(template.Must(
		template.New("workspace").Funcs(funcMap).Parse(shellTmpl)).Parse(workspaceContentTmpl))
	h.submittedTmpl = template.Must(template.Must(
		template.New("submitted").Funcs(funcMap).Parse(shellTmpl)).Parse(submittedContentTmpl))
	// The editor is a standalone full-page surface — no app shell.
	h.editorTmpl = template.Must(template.New("editor").Funcs(funcMap).Parse(editorPageTmpl))
	h.onboardingTmpl = onboardingTemplate(funcMap)
	h.capMapTmpl = capabilityMapTemplate(funcMap)
	h.notFoundTmpl = template.Must(template.New("notfound").Funcs(funcMap).Parse(notFoundTmplStr))
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// tenantLabel returns the configured customer display name, or the neutral
// product label when none is set. The sidebar account block never renders blank.
func (h *Handler) tenantLabel() string {
	if strings.TrimSpace(h.tenantName) == "" {
		return defaultTenantLabel
	}
	return h.tenantName
}

// tenantInitials derives the two-letter avatar monogram from the tenant label
// (first letters of the first two words, or the first two letters of a single
// word), upper-cased. It always returns at least one character.
//
// Slicing is rune-based, not byte-based: display names with multibyte
// characters (accents, CJK) must produce valid UTF-8 initials rather than a
// split byte that renders as the U+FFFD replacement character in the browser.
func (h *Handler) tenantInitials() string {
	words := strings.Fields(h.tenantLabel())
	var initials string
	switch {
	case len(words) >= 2:
		r0, r1 := []rune(words[0]), []rune(words[1])
		initials = string(r0[0]) + string(r1[0])
	case len(words) == 1:
		r := []rune(words[0])
		if len(r) >= 2 {
			initials = string(r[:2])
		} else {
			initials = string(r)
		}
	default:
		initials = defaultTenantLabel[:1] // ASCII constant, byte-slice safe
	}
	return strings.ToUpper(initials)
}

// shellData carries what the app shell (sidebar) needs on every page.
type shellData struct {
	PageTitle      string
	ActiveNav      string // "opps" highlights the Opportunities nav item
	QueueCount     int    // un-pursued opportunities in the queue (Hunted/Scored)
	NeedsCount     int    // opportunities awaiting human review (amber badge)
	ActiveCount    int    // opportunities selected into proposal work
	SubmittedCount int    // opportunities submitted to SAM.gov (the archive)
}

// fillShellCounts populates the sidebar pipeline counts on sd from the unfiltered
// queue, using the SAME stage grouping as the Opportunities (overview) screen so the
// bar reads identically on every page — including onboarding and the workspace, which
// would otherwise show zeros because they never list the store (issue #246 B1). The
// sidebar is advisory, so any store error leaves the counts at zero rather than
// failing the page.
//
// QueueCount counts only un-pursued opportunities (Hunted/Scored), matching the
// visible Opportunities list; pursued ones flow into ActiveCount/SubmittedCount.
func (h *Handler) fillShellCounts(ctx context.Context, sd *shellData) {
	counts, err := h.svc.CountStages(ctx)
	if err != nil {
		return
	}
	sd.QueueCount = counts[StageHunted] + counts[StageScored]
	sd.NeedsCount = counts[StageAwaitingHumanReview]
	sd.ActiveCount = counts[StageAwaitingHumanReview] + counts[StageSelected] + counts[StageInProposal] + counts[StageFinalized]
	sd.SubmittedCount = counts[StageSubmitted]
}

// OverviewData is the view-model for the Triage screen.
type OverviewData struct {
	shellData
	NewCount    int // added today (CreatedAt on the server's current day)
	TopFit      int // max fit score over the unfiltered queue
	ActiveRec   string
	ActiveSort  string
	SortToggle  string // the sort the button switches to
	SortLabel   string
	TodayRows   []TriageRow
	EarlierRows []TriageRow
	Empty       bool
	// Filtered is true when an active recommendation filter is in effect. It lets
	// the empty state distinguish a genuinely-empty queue ("No opportunities yet")
	// from a non-empty queue that simply has no rows under the current filter
	// ("Nothing here right now") — WS-C4.
	Filtered bool
	// FirstRun is true when no company profile has been configured yet (WS-C3): the
	// Triage screen then surfaces a prominent "Complete onboarding" entry point.
	// When FirstRun is true the empty-state panel is suppressed so a brand-new
	// deployment shows the single onboarding message rather than two conflicting
	// empty states (WS-C4).
	FirstRun bool
}

// TriageRow is the view-model for one opportunity row card.
type TriageRow struct {
	ID            string
	Title         string
	Agency        string
	NAICSCode     string
	ScorePct      int
	RecClass      string // "bid" | "review" | "nobid" | ""
	RecWord       string // "Bid" | "Review" | "No bid"
	DeadlineLabel string
	DeadlineDays  int
	IsNew         bool
}

// recDisplay maps the scorer vocabulary to row display values.
var recDisplay = map[string]struct{ class, word string }{
	"BID":    {"bid", "Bid"},
	"REVIEW": {"review", "Review"},
	"NO_BID": {"nobid", "No bid"},
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	query := r.URL.Query()
	now := h.Now()

	opts := ListOptions{Now: now}
	if s := query.Get("stage"); s != "" {
		st := Stage(s)
		opts.Stage = &st
	}
	if ms := query.Get("minScore"); ms != "" {
		if f, err := strconv.ParseFloat(ms, 64); err == nil {
			opts.MinScore = f
		}
	}
	if rec := query.Get("rec"); rec == "BID" || rec == "REVIEW" || rec == "NO_BID" {
		opts.Recommendation = rec
	}
	opts.SortBy = SortByScore
	if query.Get("sort") == "deadline" {
		opts.SortBy = SortByDeadline
	}
	// Self-cleaning queue (issue #224): a pursued opportunity leaves the
	// Opportunities tab the moment it is pursued. Pursued opps still feed the
	// pipeline counts below (computed from the unfiltered `all`).
	opts.ExcludeSelected = true
	// Drop solicitations whose response deadline has already passed — a tester can't
	// bid them, so they're noise on the Opportunities board.
	opts.ExcludeExpired = true

	rows, err := h.svc.List(ctx, opts)
	if err != nil {
		// Generic message to the client; details stay server-side (issue #145).
		fmt.Printf("dashboard list failed: %v\n", err)
		http.Error(w, "failed to load opportunities", http.StatusInternalServerError)
		return
	}

	// Shell counts and stats come from the unfiltered queue.
	all, err := h.svc.List(ctx, ListOptions{Now: now})
	if err != nil {
		fmt.Printf("dashboard list failed: %v\n", err)
		http.Error(w, "failed to load opportunities", http.StatusInternalServerError)
		return
	}

	data := OverviewData{
		shellData: shellData{
			PageTitle: "Opportunities",
			ActiveNav: "opps",
		},
		ActiveRec:  opts.Recommendation,
		ActiveSort: "score",
		SortToggle: "deadline",
		SortLabel:  "Fit score",
	}
	if opts.SortBy == SortByDeadline {
		data.ActiveSort, data.SortToggle, data.SortLabel = "deadline", "score", "Deadline"
	}
	for i := range all {
		switch all[i].Stage {
		case StageHunted, StageScored:
			// Un-pursued queue: drives the Opportunities nav count and the
			// Triage header stats (in-queue / new today / top fit). Pursued
			// opps are excluded here so the numbers match the visible list.
			data.QueueCount++
			if sameDay(all[i].CreatedAt, now) {
				data.NewCount++
			}
			if pct := int(math.Round(all[i].Score * 100)); pct > data.TopFit {
				data.TopFit = pct
			}
		case StageAwaitingHumanReview:
			data.NeedsCount++
			data.ActiveCount++
		case StageSelected, StageInProposal, StageFinalized:
			data.ActiveCount++
		case StageSubmitted:
			data.SubmittedCount++
		}
	}
	for i := range rows {
		tr := toTriageRow(&rows[i], now)
		if tr.IsNew {
			data.TodayRows = append(data.TodayRows, tr)
		} else {
			data.EarlierRows = append(data.EarlierRows, tr)
		}
	}
	data.Empty = len(rows) == 0
	// Filtered drives which empty-state copy the Triage shows: when a
	// recommendation filter is active the empty state reads "Nothing here right
	// now" (the queue has rows, none match); otherwise it reads the friendly
	// first-run "No opportunities yet" (WS-C4).
	data.Filtered = opts.Recommendation != ""
	// WS-C3 first-run entry point: if no company profile is configured, surface a
	// prominent link to onboarding so a brand-new deployment is not a dead end.
	// The template suppresses the "no opportunities" empty state in this case so
	// the operator sees one message, not two conflicting ones (WS-C4).
	data.FirstRun = h.firstRunRedirect()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.listTmpl.Execute(w, data); err != nil {
		fmt.Printf("list template execution failed: %v\n", err)
	}
}

// toTriageRow converts a service row to the Triage card view-model.
func toTriageRow(row *OpportunityRow, now time.Time) TriageRow {
	tr := TriageRow{
		ID:        row.ID,
		Title:     row.Title,
		Agency:    row.Agency,
		NAICSCode: row.NAICSCode,
		ScorePct:  int(math.Round(row.Score * 100)),
		IsNew:     sameDay(row.CreatedAt, now),
	}
	if rd, ok := recDisplay[row.Recommendation]; ok {
		tr.RecClass, tr.RecWord = rd.class, rd.word
	}
	if !row.ResponseDeadline.IsZero() {
		tr.DeadlineLabel, tr.DeadlineDays = deadlineDisplay(row.ResponseDeadline, now)
	}
	return tr
}

// deadlineDisplay derives the pill label and days-left: close dates count
// down in days ("9 days"); calm dates (>30d) show the date itself, per the
// handoff's deadline escalation examples.
func deadlineDisplay(deadline, now time.Time) (label string, daysLeft int) {
	days := int(math.Ceil(deadline.Sub(now).Hours() / 24))
	if days > 30 {
		return deadline.Format("Jan 2"), days
	}
	if days == 1 {
		return "1 day", days
	}
	return fmt.Sprintf("%d days", days), days
}

// sameDay reports whether both times fall on the same calendar day in UTC.
func sameDay(a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return false
	}
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}

// opportunityIDPattern is the conservative shape of a SAM.gov notice ID as
// used for store keys: alphanumeric start, then alphanumerics, dots, dashes,
// or underscores. Anything else (path traversal, spaces, markup) is rejected
// before the store is consulted.
var opportunityIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// ValidOpportunityID reports whether id is a well-formed opportunity (store key)
// identifier: an alphanumeric first character followed by up to 127 more
// alphanumerics, dots, dashes, or underscores. It is the single source of truth
// for ID validation shared by the HTML dashboard and the JSON API (internal/
// httpapi), so both surfaces reject path traversal, spaces, and markup the same
// way before the store is consulted.
func ValidOpportunityID(id string) bool {
	return opportunityIDPattern.MatchString(id)
}

// DetailData is the view-model for the /opportunity/{id} page.
type DetailData struct {
	shellData
	Opp                                                                   *opportunity.Opportunity
	DerivedStage                                                          Stage
	ScorePct                                                              int
	ScoreDisplay                                                          string // "82.0%" or "—"
	Reasons                                                               []string
	MustClass                                                             string // "ok" when recommended BID, "no" otherwise
	CanSelect                                                             bool   // proposal service wired and the opp is unselected
	DeadlineStr                                                           string // "2026-06-18" or "" when unset
	DeadlineLabel                                                         string // "9 days" pill label
	DeadlineDays                                                          int
	DeadlineSoon                                                          bool
	PostedDateStr, CreatedAtStr, UpdatedAtStr, ScoredAtStr, SelectedAtStr string

	// HasCapMatch is true when a capability map is wired and was read; CapMatch holds
	// which of the tenant's competencies/keywords/domains appear in this solicitation —
	// the "why this fits your capabilities" rationale (additive; does not change the score).
	HasCapMatch bool
	CapMatch    capabilitymap.Match
}

func (h *Handler) handleDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !ValidOpportunityID(id) {
		h.renderNotFound(w, id)
		return
	}

	opp, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			h.renderNotFound(w, id)
			return
		}
		http.Error(w, "failed to load opportunity", http.StatusInternalServerError)
		return
	}

	now := h.Now()
	data := DetailData{
		shellData: shellData{
			PageTitle: opp.Title,
			ActiveNav: "opps",
		},
		Opp:           opp,
		DerivedStage:  DeriveStage(opp),
		ScoreDisplay:  "—",
		MustClass:     "no",
		CanSelect:     h.proposals != nil && !opp.Selected,
		DeadlineSoon:  isDeadlineSoon(opp.ResponseDeadline, now),
		PostedDateStr: fmtDate(opp.PostedDate),
		CreatedAtStr:  fmtDateTime(opp.CreatedAt),
		UpdatedAtStr:  fmtDateTime(opp.UpdatedAt),
		ScoredAtStr:   fmtDateTimePtr(opp.ScoredAt),
		SelectedAtStr: fmtDateTimePtr(opp.SelectedAt),
	}
	data.CanSelect = h.proposals != nil && !opp.Selected
	if opp.Score > 0 {
		data.ScorePct = int(math.Round(opp.Score * 100))
		data.ScoreDisplay = fmt.Sprintf("%.1f%%", opp.Score*100)
	}
	// The checklist reads as satisfied only when the scorer recommends
	// pursuit; for REVIEW/NO_BID the warn treatment flags human judgment.
	if opp.Recommendation == "BID" {
		data.MustClass = "ok"
	}
	data.Reasons = splitReasons(opp.ScoreReasoning)
	if !opp.ResponseDeadline.IsZero() {
		data.DeadlineStr = opp.ResponseDeadline.Format("2006-01-02")
		data.DeadlineLabel, data.DeadlineDays = deadlineDisplay(opp.ResponseDeadline, now)
	}

	// Capability-aware qualification (additive, does not change the score): match the
	// tenant's capability map against this solicitation's text and show the rationale.
	// Best-effort — a missing/unbuilt map simply omits the section.
	if h.capMap != nil {
		if cm, mErr := h.capMap.Load(); mErr == nil && cm != nil {
			text := opp.Title + " " + opp.Agency + " " + opp.NAICSDescription
			// opp.Description is often a SAM noticedesc URL (not text); only include it
			// when it's real prose (resolving the URL to text is the scoring phase).
			if !strings.HasPrefix(opp.Description, "http") {
				text += " " + opp.Description
			}
			data.CapMatch = cm.Match(text)
			data.HasCapMatch = true
		}
	}

	// Shell counts for the sidebar — same grouping as every other screen.
	h.fillShellCounts(r.Context(), &data.shellData)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.detailTmpl.Execute(w, data); err != nil {
		fmt.Printf("detail template execution failed: %v\n", err)
	}
}

// splitReasons turns the scorer's prose reasoning into the drawer's bullet
// list: one bullet per sentence, capped at four.
func splitReasons(reasoning string) []string {
	if reasoning == "" {
		return nil
	}
	var out []string
	rest := reasoning
	for len(out) < 4 && rest != "" {
		i := indexSentenceEnd(rest)
		if i < 0 {
			if r := strings.TrimSpace(rest); r != "" {
				out = append(out, r)
			}
			break
		}
		if r := strings.TrimSpace(rest[:i+1]); r != "" {
			out = append(out, r)
		}
		rest = rest[i+1:]
	}
	return out
}

// indexSentenceEnd finds the next sentence boundary: a period followed by a
// space, newline, or end of string.
func indexSentenceEnd(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' && (i == len(s)-1 || s[i+1] == ' ' || s[i+1] == '\n') {
			return i
		}
	}
	return -1
}

// renderNotFound writes the 404 page with the (auto-escaped) id echoed back.
func (h *Handler) renderNotFound(w http.ResponseWriter, id string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if err := h.notFoundTmpl.Execute(w, struct{ ID string }{ID: id}); err != nil {
		fmt.Printf("not-found template execution failed: %v\n", err)
	}
}

// fmtDate formats a date-only field, returning "" for the zero value.
func fmtDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

// fmtDateTime formats a timestamp field, returning "" for the zero value.
func fmtDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

// fmtDateTimePtr formats an optional timestamp, returning "" for nil.
func fmtDateTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return fmtDateTime(*t)
}

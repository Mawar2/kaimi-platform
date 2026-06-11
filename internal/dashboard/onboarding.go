package dashboard

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/Mawar2/Kaimi/internal/drivetoken"
	"github.com/Mawar2/Kaimi/internal/profile"
)

// This file implements the WS-C3 in-product onboarding flow: server-rendered pages
// (html/template, in the dashboard's existing brand) that let a brand-new business
// configure its deployment WITHOUT editing files baked into the image. It reuses the
// WS-C1 profile store and the WS-C2 Drive connect; it deliberately is NOT a React SPA.
//
// Routes (registered in setupRoutes, served behind the C3a HTML session auth):
//   - GET  /onboarding          render the checklist (company profile + sign-in +
//                               Drive + SAM key status + first-run CTA)
//   - POST /onboarding/profile  parse + validate (shared profile.Validate) + persist;
//                               PRG: redirect back to /onboarding?saved=1 on success,
//                               re-render with errors (no persistence) on invalid.
//
// The dashboard package must NOT import internal/httpapi (httpapi imports dashboard).
// The signed-in identity and the CSRF token are therefore injected from cmd/api via
// the IdentityFunc option, and the Drive status is read straight from the drivetoken
// stores (no cycle) via the DriveStatusFunc option.

// Identity is the signed-in operator's identity plus a per-session CSRF token, both
// derived from the WS-B4 session by cmd/api and injected via WithIdentity. The
// dashboard does not (and cannot, without an import cycle) read the httpapi session
// directly, so it depends on this small value type instead.
type Identity struct {
	// Email is the verified Workspace email of the signed-in operator.
	Email string
	// CSRFToken is a stable-per-session token cmd/api derives from the session. The
	// onboarding form embeds it and the POST handler compares it (constant-time) to
	// the value the same IdentityFunc returns for the request, as CSRF defense in
	// depth on top of the SameSite=Lax session cookie.
	CSRFToken string
}

// IdentityFunc resolves the signed-in operator's identity (and CSRF token) from the
// request context. cmd/api supplies an adapter over httpapi.SessionFromContext. It
// returns ok=false when no session is present (e.g. insecure dev mode), in which
// case onboarding renders the signed-out treatment and skips CSRF enforcement.
type IdentityFunc func(ctx context.Context) (Identity, bool)

// DriveStatus is the WS-C2 customer-Drive connection state shown on the onboarding
// page. It never carries the OAuth token — only whether a Drive is connected and the
// configured target id.
type DriveStatus struct {
	// Configured reports whether customer-Drive connect is wired at all. When false
	// the page shows the "not available in this deployment" treatment instead of a
	// connect button.
	Configured bool
	// Connected reports whether a customer Drive OAuth token has been stored.
	Connected bool
	// Target is the configured target Drive/folder id, or "" when none is set.
	Target string
}

// DriveStatusFunc reports the current customer-Drive connection state. cmd/api
// supplies an implementation backed by the drivetoken token/target stores; a
// deployment without Drive connect leaves it nil (Configured renders false).
type DriveStatusFunc func() DriveStatus

// DriveTargetSaver persists a new Drive destination chosen on the onboarding page
// (WS-C5b). cmd/api backs it with the SAME drivetoken TargetStore.Save the JSON
// PUT /api/v1/integrations/drive/target uses, so the SSR form and the JSON API
// write to one store and can never diverge. The dashboard package takes this
// function (not the drivetoken store type) to avoid importing internal/drivetoken
// for a single Save call. An empty id is rejected by the store, mirroring the PUT
// endpoint; the literal "root" sentinel (driveTargetRoot) is a valid value meaning
// "My Drive root".
type DriveTargetSaver func(driveID string) error

// driveTargetRoot is the sentinel target value meaning "create Docs at the root of
// My Drive" rather than inside a specific folder. It is stored verbatim as the
// drivetoken Target.DriveID and flows straight through to
// googledocs.Config.DestinationID, which already treats "root" as Drive's reserved
// root alias. Keeping it a literal (not "") lets an operator deliberately choose the
// root and have that choice persist, distinct from "no target set yet".
const driveTargetRoot = "root"

// driveFolderURLPrefix is the Drive web URL a folder id is appended to for the
// "Open in Drive" link shown next to the current destination.
const driveFolderURLPrefix = "https://drive.google.com/drive/folders/"

// WithProfileStore wires the WS-C1 runtime profile store so onboarding can pre-fill
// the company-profile form from a saved profile and persist edits. Without it the
// onboarding routes answer 503 (mirroring how the JSON API degrades when the store
// is absent).
func WithProfileStore(ps profile.ProfileStore) Option {
	return func(h *Handler) { h.profileStore = ps }
}

// WithIdentity wires the signed-in identity + CSRF reader (see IdentityFunc).
func WithIdentity(fn IdentityFunc) Option {
	return func(h *Handler) { h.identity = fn }
}

// WithDriveStatus wires the WS-C2 Drive status reader (see DriveStatusFunc).
func WithDriveStatus(fn DriveStatusFunc) Option {
	return func(h *Handler) { h.driveStatus = fn }
}

// WithDriveTargetSaver wires the WS-C5b Drive destination write path (see
// DriveTargetSaver). cmd/api passes an adapter over the SAME drivetoken
// TargetStore.Save the JSON PUT endpoint uses. Without it the onboarding page shows
// the current destination read-only (no change control) — it never invents a second
// write path.
func WithDriveTargetSaver(fn DriveTargetSaver) Option {
	return func(h *Handler) { h.driveTargetSaver = fn }
}

// DriveStatusFromStores builds a DriveStatusFunc over the drivetoken stores. It is a
// constructor cmd/api calls so the wiring of the WS-C2 stores into the onboarding
// page lives next to their types. tokens/targets must both be non-nil. Any store I/O
// error is treated as "not connected"/"no target" for this advisory display — the
// authoritative read path remains the JSON /api/v1/integrations/drive/status.
func DriveStatusFromStores(tokens drivetoken.TokenStore, targets drivetoken.TargetStore) DriveStatusFunc {
	return func() DriveStatus {
		st := DriveStatus{Configured: true}
		if _, err := tokens.Load(); err == nil {
			st.Connected = true
		}
		if t, err := targets.Load(); err == nil {
			st.Target = t.DriveID
		}
		return st
	}
}

// onboardingPath is where the onboarding flow lives and where the PRG success
// redirect lands.
const onboardingPath = "/onboarding"

// driveConnectPath is the WS-C2 connect endpoint the onboarding "Connect Drive"
// button links to. It is on the JSON API surface (served by the same authed,
// same-origin server), so a plain link works.
const driveConnectPath = "/api/v1/integrations/drive/connect"

// OnboardingData is the view-model for the onboarding page.
type OnboardingData struct {
	shellData

	// Identity / sign-in.
	SignedIn bool
	Email    string

	// CSRFToken is embedded in the form (empty when no session/CSRF is active).
	CSRFToken string

	// Company-profile form values (pre-filled from the saved profile, or the
	// submitted values when re-rendering after a validation error).
	HasProfile      bool
	Company         string
	UEI             string
	CAGE            string
	NAICS           string // newline/comma-separated "code|description|tier" lines
	Competencies    string // newline-separated
	PastPerformance string // newline-separated "client|scope|value" lines
	SetAside        profile.SetAsideStatus
	// Scoring hints (the curated Scorer signals).
	PrimaryNAICS   string // comma/newline-separated
	SecondaryNAICS string
	CompetencyTags string
	ScoringPP      string // scoring past-performance sentences, one per line

	// Drive.
	Drive DriveStatus
	// CanEditDrive reports whether the "change destination" control is shown — true
	// only when a DriveTargetSaver is wired AND the Drive is connected (no point
	// choosing a destination before connecting). When false the destination is
	// displayed read-only.
	CanEditDrive bool
	// DriveDest is the human-readable view of the CURRENT destination (WS-C5b),
	// derived from Drive.Target: a folder with an Open-in-Drive link, "My Drive
	// (root)", or "not set yet". It is computed once in newOnboardingData so the
	// template stays declarative.
	DriveDest driveDestView

	// State flags.
	Saved      bool   // PRG success banner (company profile)
	DriveSaved bool   // PRG success banner (Drive destination)
	FormErr    string // validation error to re-render
}

// driveDestView is the rendered form of the current Drive destination shown on the
// onboarding page (WS-C5b). Exactly one of IsFolder / IsRoot is true, or both are
// false meaning "not set yet". When IsFolder is true, FolderID holds the id and
// OpenURL the Drive web link.
type driveDestView struct {
	IsFolder bool
	IsRoot   bool
	FolderID string
	OpenURL  string
}

// driveDestination maps a stored target id to its rendered view. An empty id means
// no destination has been set yet (both flags false); the literal "root" sentinel
// renders as My Drive root; any other value is treated as a folder id with an
// Open-in-Drive link. The id is used only to build a URL via url-safe concatenation
// and is auto-escaped by html/template at render time.
func driveDestination(target string) driveDestView {
	switch strings.TrimSpace(target) {
	case "":
		return driveDestView{}
	case driveTargetRoot:
		return driveDestView{IsRoot: true}
	default:
		return driveDestView{IsFolder: true, FolderID: target, OpenURL: driveFolderURLPrefix + target}
	}
}

// onboardingContentTmpl is the onboarding checklist page. All dynamic values render
// through html/template's contextual auto-escaping; none use template.HTML, so a
// crafted company name or NAICS description cannot inject markup.
const onboardingContentTmpl = `{{define "content"}}
<div class="page">
  <div class="page-head">
    <div class="eyebrow">Setup</div>
    <h1>Onboarding</h1>
    <p class="lead">Configure this Kaimi deployment for your business. Everything here is stored for your workspace only — no files to edit.</p>
  </div>

  {{if .Saved}}
  <div class="ob-banner ob-banner--ok">` + iconCheck + `<span>Company profile saved. Kaimi will use it on the next hunt.</span></div>
  {{end}}
  {{if .DriveSaved}}
  <div class="ob-banner ob-banner--ok">` + iconCheck + `<span>Drive destination updated. New proposal Docs will land there.</span></div>
  {{end}}
  {{if .FormErr}}
  <div class="ob-banner ob-banner--err">` + iconWarn + `<span>{{.FormErr}}</span></div>
  {{end}}

  <ol class="ob-list">
    <li class="ob-step">
      <div class="ob-step-h">
        <span class="ob-dot {{if .SignedIn}}ob-dot--ok{{end}}">{{if .SignedIn}}` + iconCheck + `{{else}}1{{end}}</span>
        <h3>Sign-in &amp; workspace</h3>
      </div>
      {{if .SignedIn}}
      <p class="ob-note">Signed in as <b>{{.Email}}</b>.</p>
      {{else}}
      <p class="ob-note">You are not signed in. <a href="/auth/login">Sign in</a> to configure this deployment.</p>
      {{end}}
    </li>

    <li class="ob-step">
      <div class="ob-step-h">
        <span class="ob-dot {{if .HasProfile}}ob-dot--ok{{end}}">{{if .HasProfile}}` + iconCheck + `{{else}}2{{end}}</span>
        <h3>Company profile</h3>
      </div>
      <p class="ob-note">Kaimi grounds its hunting, scoring, and drafting in these facts. Required: company name and at least one NAICS code.</p>
      <form class="ob-form" method="POST" action="/onboarding/profile">
        {{if .CSRFToken}}<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">{{end}}
        <label>Company name<input name="company" value="{{.Company}}" required></label>
        <div class="ob-row">
          <label>UEI<input name="uei" value="{{.UEI}}"></label>
          <label>CAGE<input name="cage" value="{{.CAGE}}"></label>
        </div>
        <label>NAICS codes<textarea name="naics" rows="3" placeholder="One per line: code|description|tier (primary|secondary|tertiary)">{{.NAICS}}</textarea></label>
        <fieldset class="ob-fs">
          <legend>Set-aside eligibility</legend>
          <label class="ob-chk"><input type="checkbox" name="sa_small_business"{{if .SetAside.SmallBusiness}} checked{{end}}> Small business</label>
          <label class="ob-chk"><input type="checkbox" name="sa_sdb"{{if .SetAside.SDB}} checked{{end}}> SDB</label>
          <label class="ob-chk"><input type="checkbox" name="sa_minority_owned"{{if .SetAside.MinorityOwned}} checked{{end}}> Minority-owned</label>
          <label class="ob-chk"><input type="checkbox" name="sa_eight_a"{{if .SetAside.EightA}} checked{{end}}> 8(a)</label>
          <label class="ob-chk"><input type="checkbox" name="sa_sdvosb"{{if .SetAside.SDVOSB}} checked{{end}}> SDVOSB</label>
          <label class="ob-chk"><input type="checkbox" name="sa_wosb"{{if .SetAside.WOSB}} checked{{end}}> WOSB</label>
          <label class="ob-chk"><input type="checkbox" name="sa_hubzone"{{if .SetAside.HUBZone}} checked{{end}}> HUBZone</label>
        </fieldset>
        <label>Core competencies<textarea name="competencies" rows="3" placeholder="One per line">{{.Competencies}}</textarea></label>
        <label>Past performance<textarea name="past_performance" rows="3" placeholder="One per line: client|scope|value">{{.PastPerformance}}</textarea></label>
        <fieldset class="ob-fs">
          <legend>Scoring hints (optional)</legend>
          <label>Primary NAICS<input name="primary_naics" value="{{.PrimaryNAICS}}" placeholder="comma-separated"></label>
          <label>Secondary NAICS<input name="secondary_naics" value="{{.SecondaryNAICS}}" placeholder="comma-separated"></label>
          <label>Competency tags<textarea name="competency_tags" rows="2" placeholder="One per line (lowercase keywords)">{{.CompetencyTags}}</textarea></label>
          <label>Scoring past-performance<textarea name="scoring_pp" rows="2" placeholder="One sentence per line">{{.ScoringPP}}</textarea></label>
        </fieldset>
        <div class="ob-actions">
          <button class="kbtn kbtn--select" type="submit">` + iconCheck + `Save company profile</button>
        </div>
      </form>
    </li>

    <li class="ob-step">
      <div class="ob-step-h">
        <span class="ob-dot {{if .Drive.Connected}}ob-dot--ok{{end}}">{{if .Drive.Connected}}` + iconCheck + `{{else}}3{{end}}</span>
        <h3>Google Drive</h3>
      </div>
      {{if not .Drive.Configured}}
      <p class="ob-note">Customer-Drive connect is not enabled in this deployment. Generated proposal Docs use the default Drive. Ask your administrator to set the Drive OAuth configuration to land Docs in your own Workspace.</p>
      {{else if .Drive.Connected}}
      <p class="ob-note">Connected. Proposal Docs are created in this destination:</p>
      <div class="ob-dest">
        {{if .DriveDest.IsFolder}}
        <span class="ob-dest-label">Folder <code>{{.DriveDest.FolderID}}</code></span>
        <a class="ob-dest-open" href="{{.DriveDest.OpenURL}}" target="_blank" rel="noopener noreferrer">` + iconLink + `Open in Drive</a>
        {{else if .DriveDest.IsRoot}}
        <span class="ob-dest-label">My Drive (root)</span>
        {{else}}
        <span class="ob-dest-label ob-dest-unset">Not set yet — Docs land in your connected Drive&#39;s default location.</span>
        {{end}}
      </div>
      {{if .CanEditDrive}}
      <form class="ob-form ob-drive-form" method="POST" action="/onboarding/drive/target">
        {{if .CSRFToken}}<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">{{end}}
        <fieldset class="ob-fs">
          <legend>Change destination</legend>
          <label class="ob-chk"><input type="radio" name="drive_choice" value="folder"{{if not .DriveDest.IsRoot}} checked{{end}}> Specific folder</label>
          <label>Folder id<input name="drive_id" value="{{.DriveDest.FolderID}}" placeholder="Paste a Google Drive folder id"></label>
          <label class="ob-chk"><input type="radio" name="drive_choice" value="root"{{if .DriveDest.IsRoot}} checked{{end}}> My Drive root</label>
        </fieldset>
        <div class="ob-actions">
          <button class="kbtn kbtn--select" type="submit">` + iconCheck + `Save destination</button>
        </div>
      </form>
      {{end}}
      <p class="ob-note"><a href="` + driveConnectPath + `">Reconnect</a></p>
      {{else}}
      <p class="ob-note">Connect your Google Workspace so generated proposal Docs land in your own Drive.</p>
      <div class="ob-actions"><a class="kbtn kbtn--secondary" href="` + driveConnectPath + `">` + iconLink + `Connect Drive</a></div>
      {{end}}
    </li>

    <li class="ob-step">
      <div class="ob-step-h">
        <span class="ob-dot">4</span>
        <h3>SAM.gov API key</h3>
      </div>
      <p class="ob-note">Kaimi reads opportunities from SAM.gov using a server-side API key. For security the key is a deployment secret (Secret Manager) — it is <b>not</b> entered here and Kaimi never stores or logs it. Your administrator configures <code>SAM_API_KEY</code> in the deployment environment.</p>
    </li>

    <li class="ob-step">
      <div class="ob-step-h">
        <span class="ob-dot {{if .HasProfile}}ob-dot--ok{{end}}">5</span>
        <h3>You&#39;re set</h3>
      </div>
      {{if .HasProfile}}
      <p class="ob-note">Your profile is configured. Kaimi runs the next hunt automatically; jump into the queue to triage opportunities.</p>
      <div class="ob-actions"><a class="kbtn kbtn--select kbtn--lg" href="/">` + iconArrow + `Go to the dashboard</a></div>
      {{else}}
      <p class="ob-note">Save your company profile above to finish onboarding.</p>
      {{end}}
    </li>
  </ol>
</div>

<style>
  .ob-banner { display:flex; align-items:center; gap:var(--s-2); padding:var(--s-3); border-radius:var(--r-sm); margin-bottom:var(--s-4); font:var(--t-small); }
  .ob-banner svg { width:18px; height:18px; flex:0 0 auto; }
  .ob-banner--ok { background:var(--st-ok-bg,#e7f7ee); color:var(--st-ok,#1a7f4b); }
  .ob-banner--err { background:var(--st-failed-bg,#fde8e8); color:var(--st-failed,#b42318); }
  .ob-list { list-style:none; margin:0; padding:0; display:flex; flex-direction:column; gap:var(--s-4); }
  .ob-step { background:var(--surface); border:1px solid var(--border); border-radius:var(--r-md,10px); padding:var(--s-4); }
  .ob-step-h { display:flex; align-items:center; gap:var(--s-2); margin-bottom:var(--s-2); }
  .ob-step-h h3 { margin:0; }
  .ob-dot { display:inline-flex; align-items:center; justify-content:center; width:26px; height:26px; border-radius:50%; background:var(--surface-2); color:var(--ink-3); font-weight:700; flex:0 0 auto; }
  .ob-dot svg { width:16px; height:16px; }
  .ob-dot--ok { background:var(--st-ok,#1a7f4b); color:#fff; }
  .ob-note { color:var(--ink-3); font:var(--t-small); margin:var(--s-1) 0; }
  .ob-form { display:flex; flex-direction:column; gap:var(--s-3); margin-top:var(--s-3); max-width:680px; }
  .ob-form label { display:flex; flex-direction:column; gap:4px; font:var(--t-small); font-weight:600; }
  .ob-form input, .ob-form textarea { font:var(--t-body); padding:var(--s-2); border:1px solid var(--border); border-radius:var(--r-sm); background:var(--surface-2); }
  .ob-row { display:flex; gap:var(--s-3); }
  .ob-row label { flex:1; }
  .ob-fs { border:1px solid var(--border); border-radius:var(--r-sm); padding:var(--s-3); display:flex; flex-direction:column; gap:var(--s-2); }
  .ob-fs legend { font:var(--t-small); font-weight:700; padding:0 6px; }
  .ob-chk { flex-direction:row !important; align-items:center; gap:8px; font-weight:500 !important; }
  .ob-actions { margin-top:var(--s-2); }
  .ob-dest { display:flex; align-items:center; flex-wrap:wrap; gap:var(--s-2); margin:var(--s-2) 0; }
  .ob-dest-label { font:var(--t-small); }
  .ob-dest-label code { background:var(--surface-2); border:1px solid var(--border); border-radius:var(--r-sm); padding:1px 6px; }
  .ob-dest-unset { color:var(--ink-3); }
  .ob-dest-open { display:inline-flex; align-items:center; gap:4px; font:var(--t-small); font-weight:600; color:var(--primary,#0b5fff); text-decoration:none; }
  .ob-dest-open svg { width:15px; height:15px; }
  .ob-drive-form { margin-top:var(--s-2); }
</style>
{{end}}
`

// handleOnboarding serves GET /onboarding. It pre-fills the company-profile form
// from the saved profile (empty when none has been saved), shows the signed-in
// identity, the Drive status, the SAM-key guidance, and the first-run CTA.
func (h *Handler) handleOnboarding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.profileStore == nil {
		http.Error(w, "onboarding is not available in this deployment", http.StatusServiceUnavailable)
		return
	}

	data := h.newOnboardingData(r)
	data.Saved = r.URL.Query().Get("saved") == "1"
	data.DriveSaved = r.URL.Query().Get("drive_saved") == "1"

	// Pre-fill from the saved profile when one exists.
	if p, err := h.profileStore.Load(); err == nil {
		data.HasProfile = true
		fillFormFromProfile(&data, p)
	} else if !errors.Is(err, profile.ErrProfileNotFound) {
		// A real I/O/parse error (not "not onboarded yet"): keep detail server-side.
		fmt.Printf("onboarding profile load failed: %v\n", err)
		http.Error(w, "failed to load profile", http.StatusInternalServerError)
		return
	}

	h.renderOnboarding(w, &data)
}

// handleOnboardingProfile serves POST /onboarding/profile. It FAILS CLOSED on auth:
// this is a state-mutating endpoint, so it does NOT trust upstream middleware alone —
// it re-checks identity and CSRF here before mutating. It then parses the form into a
// CapabilityProfile, validates it with the SHARED profile.Validate, and persists it
// via the ProfileStore. On a validation failure it re-renders the form with the error
// and persists NOTHING; on success it follows the PRG pattern, redirecting to
// /onboarding?saved=1.
func (h *Handler) handleOnboardingProfile(w http.ResponseWriter, r *http.Request) {
	if h.profileStore == nil {
		http.Error(w, "onboarding is not available in this deployment", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	// Fail-closed auth + CSRF gate (defense in depth on top of the upstream HTML
	// session middleware). NOTHING is mutated until this gate passes.
	if !h.authorizeMutation(w, r) {
		return
	}

	p := parseProfileForm(r)

	if err := profile.Validate(p); err != nil {
		// Re-render the form with the submitted values and the error. Persist nothing.
		data := h.newOnboardingData(r)
		fillFormFromProfile(&data, p)
		data.FormErr = err.Error()
		w.WriteHeader(http.StatusBadRequest)
		h.renderOnboarding(w, &data)
		return
	}

	if err := h.profileStore.Save(p); err != nil {
		fmt.Printf("onboarding profile save failed: %v\n", err)
		http.Error(w, "failed to save profile", http.StatusInternalServerError)
		return
	}

	// PRG: redirect so a refresh does not re-POST the form.
	http.Redirect(w, r, onboardingPath+"?saved=1", http.StatusSeeOther)
}

// handleOnboardingDriveTarget serves POST /onboarding/drive/target (WS-C5b): the
// SSR form that lets an operator change the Drive destination — choose "My Drive
// root" or paste a folder id — without editing files. It deliberately reuses the
// EXISTING write path: cmd/api backs h.driveTargetSaver with the SAME drivetoken
// TargetStore.Save the JSON PUT /api/v1/integrations/drive/target uses, so the two
// surfaces never write to different stores.
//
// It is an SSR form post (not a JS fetch to the JSON PUT) so it matches the rest of
// the onboarding page exactly: form-encoded body, the shared fail-closed auth + CSRF
// gate (authorizeMutation), and the post/redirect/get pattern. Like the profile
// write it FAILS CLOSED on auth and mutates NOTHING until the gate passes.
//
// The form sends two fields:
//   - drive_choice = "root" → persist the literal driveTargetRoot sentinel.
//   - drive_choice = "folder" → persist the trimmed drive_id; an empty id is rejected
//     (re-render with an error), mirroring the PUT endpoint's "drive_id is required".
func (h *Handler) handleOnboardingDriveTarget(w http.ResponseWriter, r *http.Request) {
	if h.driveTargetSaver == nil {
		// No write path wired (Drive connect disabled): the change control is not
		// shown, so a POST here is unsupported. Mirror the JSON API's 503 degradation.
		http.Error(w, "drive destination changes are not available in this deployment", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	// Fail-closed auth + CSRF gate (the same one the profile write uses). NOTHING is
	// mutated until this passes.
	if !h.authorizeMutation(w, r) {
		return
	}

	// Resolve the chosen destination from the radio choice. The choice must be
	// EXACTLY "folder" or "root"; a missing/unrecognized value is rejected rather
	// than silently defaulting to a destination the operator did not pick.
	var target string
	switch r.PostFormValue("drive_choice") {
	case "root":
		target = driveTargetRoot
	case "folder":
		target = strings.TrimSpace(r.PostFormValue("drive_id"))
		if target == "" {
			// Re-render the page with the error; persist nothing. Mirrors the profile
			// write's invalid-input re-render (400 + form error banner).
			data := h.newOnboardingData(r)
			data.FormErr = "Enter a Google Drive folder id, or choose My Drive root."
			w.WriteHeader(http.StatusBadRequest)
			h.renderOnboarding(w, &data)
			return
		}
	default:
		// Unrecognized or missing choice: reject; persist nothing.
		data := h.newOnboardingData(r)
		data.FormErr = "Choose a destination: a specific folder or My Drive root."
		w.WriteHeader(http.StatusBadRequest)
		h.renderOnboarding(w, &data)
		return
	}

	if err := h.driveTargetSaver(target); err != nil {
		log.Printf("dashboard: onboarding drive target save failed: %v", err)
		http.Error(w, "failed to save drive destination", http.StatusInternalServerError)
		return
	}

	// PRG: redirect so a refresh does not re-POST.
	http.Redirect(w, r, onboardingPath+"?drive_saved=1", http.StatusSeeOther)
}

// authorizeMutation is the fail-closed auth + CSRF gate every state-mutating
// onboarding POST must pass before it touches the store. It writes the rejection
// response itself and returns false on denial; callers must NOT mutate when it
// returns false. The endpoint is served behind the C3a HTML session middleware, but
// a mutation never relies on upstream middleware alone — it re-resolves identity here.
//
// The policy:
//   - Authenticated session present (ok == true): a valid CSRF token is REQUIRED. The
//     submitted form token must constant-time-equal ident.CSRFToken, and neither may
//     be empty. An empty submitted token, an empty session token, or a mismatch is a
//     403 and no mutation. (There is deliberately no "empty session token bypass".)
//   - No identity (ok == false): FAIL CLOSED by default — reject 403, no mutation.
//     The ONLY exception is explicit insecure dev mode (h.insecureNoAuth, set from the
//     same -insecure-no-auth/KAIMI_INSECURE_NO_AUTH opt-in cmd/api uses to gate the
//     whole API). In that dev-only path there is no session and thus no CSRF token, so
//     the write is allowed relying on the SameSite=Lax cookie + same-origin server.
//
// The CSRF token/secret is never logged.
func (h *Handler) authorizeMutation(w http.ResponseWriter, r *http.Request) bool {
	ident, ok := h.resolveIdentity(r)
	if ok {
		// Authenticated: require a valid, non-empty CSRF token (constant-time compare).
		submitted := r.PostFormValue("csrf_token")
		if ident.CSRFToken == "" || submitted == "" ||
			subtle.ConstantTimeCompare([]byte(submitted), []byte(ident.CSRFToken)) != 1 {
			http.Error(w, "invalid CSRF token", http.StatusForbidden)
			return false
		}
		return true
	}
	// No identity. Allow ONLY when the operator has explicitly opted into insecure dev
	// mode; otherwise fail closed.
	if h.insecureNoAuth {
		return true
	}
	http.Error(w, "authentication required", http.StatusForbidden)
	return false
}

// newOnboardingData builds the base view-model with the shell + identity populated.
func (h *Handler) newOnboardingData(r *http.Request) OnboardingData {
	data := OnboardingData{
		shellData: shellData{
			PageTitle: "Onboarding",
			ActiveNav: "onboarding",
		},
	}
	if ident, ok := h.resolveIdentity(r); ok {
		data.SignedIn = true
		data.Email = ident.Email
		data.CSRFToken = ident.CSRFToken
	}
	if h.driveStatus != nil {
		data.Drive = h.driveStatus()
	}
	data.DriveDest = driveDestination(data.Drive.Target)
	// The change-destination control needs both a write path (saver wired) and a
	// connected Drive — there is nothing to target before connecting.
	data.CanEditDrive = h.driveTargetSaver != nil && data.Drive.Connected
	return data
}

// resolveIdentity reads the signed-in identity via the injected IdentityFunc,
// reporting ok=false when no reader is wired or no session is present.
func (h *Handler) resolveIdentity(r *http.Request) (Identity, bool) {
	if h.identity == nil {
		return Identity{}, false
	}
	return h.identity(r.Context())
}

// renderOnboarding executes the onboarding template, defaulting Content-Type. It
// takes the view-model by pointer (it is a heavy struct) but never mutates it.
func (h *Handler) renderOnboarding(w http.ResponseWriter, data *OnboardingData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.onboardingTmpl.Execute(w, data); err != nil {
		fmt.Printf("onboarding template execution failed: %v\n", err)
	}
}

// parseProfileForm builds a CapabilityProfile from the submitted onboarding form.
// It trims whitespace and parses the multi-line/CSV fields into their structured
// shapes. It does NOT validate — that is profile.Validate's job, shared with the API.
func parseProfileForm(r *http.Request) *profile.CapabilityProfile {
	get := func(k string) string { return strings.TrimSpace(r.PostFormValue(k)) }
	checked := func(k string) bool { return r.PostFormValue(k) != "" }

	p := &profile.CapabilityProfile{
		Company:      get("company"),
		UEI:          get("uei"),
		CAGE:         get("cage"),
		NAICSCodes:   parseNAICSLines(r.PostFormValue("naics")),
		Competencies: splitLines(r.PostFormValue("competencies")),
		SetAside: profile.SetAsideStatus{
			SmallBusiness: checked("sa_small_business"),
			SDB:           checked("sa_sdb"),
			MinorityOwned: checked("sa_minority_owned"),
			EightA:        checked("sa_eight_a"),
			SDVOSB:        checked("sa_sdvosb"),
			WOSB:          checked("sa_wosb"),
			HUBZone:       checked("sa_hubzone"),
		},
		PastPerformance: parsePastPerformanceLines(r.PostFormValue("past_performance")),
		Scoring: profile.ScoringHints{
			PrimaryNAICS:    splitCSVOrLines(r.PostFormValue("primary_naics")),
			SecondaryNAICS:  splitCSVOrLines(r.PostFormValue("secondary_naics")),
			CompetencyTags:  splitLines(r.PostFormValue("competency_tags")),
			PastPerformance: splitLines(r.PostFormValue("scoring_pp")),
		},
	}
	return p
}

// parseNAICSLines parses one NAICS code per line in "code|description|tier" form.
// Description and tier are optional; an empty code line is dropped (so a stray blank
// line does not produce a code that fails validation).
func parseNAICSLines(raw string) []profile.NAICSCode {
	var out []profile.NAICSCode
	for _, line := range splitLines(raw) {
		parts := strings.Split(line, "|")
		code := strings.TrimSpace(parts[0])
		if code == "" {
			continue
		}
		nc := profile.NAICSCode{Code: code}
		if len(parts) > 1 {
			nc.Description = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			nc.Tier = profile.NAICSTier(strings.TrimSpace(parts[2]))
		}
		out = append(out, nc)
	}
	return out
}

// parsePastPerformanceLines parses one record per line in "client|scope|value" form.
func parsePastPerformanceLines(raw string) []profile.PastPerformance {
	var out []profile.PastPerformance
	for _, line := range splitLines(raw) {
		parts := strings.Split(line, "|")
		pp := profile.PastPerformance{Client: strings.TrimSpace(parts[0])}
		if pp.Client == "" {
			continue
		}
		if len(parts) > 1 {
			pp.Scope = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			pp.Value = strings.TrimSpace(parts[2])
		}
		out = append(out, pp)
	}
	return out
}

// splitLines splits on newlines and trims, dropping empty lines.
func splitLines(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		if s := strings.TrimSpace(strings.TrimRight(line, "\r")); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// splitCSVOrLines splits on commas AND newlines (so the operator can use either),
// trimming and dropping empties.
func splitCSVOrLines(raw string) []string {
	var out []string
	for _, field := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	}) {
		if s := strings.TrimSpace(field); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// fillFormFromProfile copies a CapabilityProfile back into the view-model's flat
// form fields so the GET pre-fill and the validation-error re-render both show the
// stored/submitted values.
func fillFormFromProfile(data *OnboardingData, p *profile.CapabilityProfile) {
	data.Company = p.Company
	data.UEI = p.UEI
	data.CAGE = p.CAGE
	data.SetAside = p.SetAside
	data.NAICS = formatNAICSLines(p.NAICSCodes)
	data.Competencies = strings.Join(p.Competencies, "\n")
	data.PastPerformance = formatPastPerformanceLines(p.PastPerformance)
	data.PrimaryNAICS = strings.Join(p.Scoring.PrimaryNAICS, ", ")
	data.SecondaryNAICS = strings.Join(p.Scoring.SecondaryNAICS, ", ")
	data.CompetencyTags = strings.Join(p.Scoring.CompetencyTags, "\n")
	data.ScoringPP = strings.Join(p.Scoring.PastPerformance, "\n")
}

// formatNAICSLines renders NAICS codes as "code|description|tier" lines for the
// textarea round-trip.
func formatNAICSLines(codes []profile.NAICSCode) string {
	lines := make([]string, 0, len(codes))
	for _, nc := range codes {
		lines = append(lines, strings.Join([]string{nc.Code, nc.Description, string(nc.Tier)}, "|"))
	}
	return strings.Join(lines, "\n")
}

// formatPastPerformanceLines renders past-performance records as "client|scope|value"
// lines for the textarea round-trip.
func formatPastPerformanceLines(pp []profile.PastPerformance) string {
	lines := make([]string, 0, len(pp))
	for _, r := range pp {
		lines = append(lines, strings.Join([]string{r.Client, r.Scope, r.Value}, "|"))
	}
	return strings.Join(lines, "\n")
}

// firstRunRedirect reports whether the dashboard should steer the operator to
// onboarding because no company profile has been configured yet. It returns true
// only when a profile store is wired and Load reports ErrProfileNotFound. Any other
// error (or no store) returns false so a transient I/O error never traps the
// operator in a redirect loop.
func (h *Handler) firstRunRedirect() bool {
	if h.profileStore == nil {
		return false
	}
	_, err := h.profileStore.Load()
	return errors.Is(err, profile.ErrProfileNotFound)
}

// onboardingTemplate compiles the onboarding page over the shared shell. It is a
// package function so setupTemplates can build it alongside the other page templates.
func onboardingTemplate(funcMap template.FuncMap) *template.Template {
	return template.Must(template.Must(
		template.New("onboarding").Funcs(funcMap).Parse(shellTmpl)).Parse(onboardingContentTmpl))
}

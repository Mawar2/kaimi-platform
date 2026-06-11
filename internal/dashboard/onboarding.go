package dashboard

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"html/template"
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

	// State flags.
	Saved   bool   // PRG success banner
	FormErr string // validation error to re-render
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
      <p class="ob-note">Connected. Proposal Docs will be created in {{if .Drive.Target}}Drive <code>{{.Drive.Target}}</code>{{else}}your connected Drive (no target folder set yet){{end}}.</p>
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

// handleOnboardingProfile serves POST /onboarding/profile. It enforces CSRF (when a
// session is present), parses the form into a CapabilityProfile, validates it with
// the SHARED profile.Validate, and persists it via the ProfileStore. On a validation
// failure it re-renders the form with the error and persists NOTHING; on success it
// follows the PRG pattern, redirecting to /onboarding?saved=1.
func (h *Handler) handleOnboardingProfile(w http.ResponseWriter, r *http.Request) {
	if h.profileStore == nil {
		http.Error(w, "onboarding is not available in this deployment", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	// CSRF defense in depth (on top of the SameSite=Lax session cookie): when a
	// session-derived token is available, the submitted token must match it
	// (constant-time). When no session/CSRF is active (insecure dev mode), we rely on
	// SameSite=Lax + same-origin only.
	if ident, ok := h.resolveIdentity(r); ok && ident.CSRFToken != "" {
		submitted := r.PostFormValue("csrf_token")
		if subtle.ConstantTimeCompare([]byte(submitted), []byte(ident.CSRFToken)) != 1 {
			http.Error(w, "invalid CSRF token", http.StatusForbidden)
			return
		}
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

package dashboard

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/Mawar2/Kaimi/internal/contextdoc"
	"github.com/Mawar2/Kaimi/internal/drivetoken"
	"github.com/Mawar2/Kaimi/internal/profile"
	"github.com/Mawar2/Kaimi/internal/samsecret"
)

// Onboarding wizard step ids. The flow is a full-screen, client-stepped wizard
// (Welcome → License → Profile → Connect → Done); the server uses these to resume at
// the right step after a form POST/redirect (PRG), and the page's inline JS reads the
// active step to show the matching panel.
const (
	stepWelcome = "welcome"
	stepLicense = "license"
	stepProfile = "profile"
	stepConnect = "connect"
	stepDone    = "done"
)

// validStep reports whether s is a known wizard step, so a crafted ?step= value
// cannot push the wizard into an undefined state (it falls back to welcome).
func validStep(s string) bool {
	switch s {
	case stepWelcome, stepLicense, stepProfile, stepConnect, stepDone:
		return true
	}
	return false
}

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
	// Email is the verified Workspace email of the signed-in operator. Empty in
	// product-key gate mode (a product-key session carries no Google identity).
	Email string
	// CSRFToken is a stable-per-session token cmd/api derives from the session. The
	// onboarding form embeds it and the POST handler compares it (constant-time) to
	// the value the same IdentityFunc returns for the request, as CSRF defense in
	// depth on top of the SameSite=Lax session cookie.
	CSRFToken string
	// LicenseKey is a MASKED product key (e.g. "KAIMI-····-····-CBFQ") shown on the
	// onboarding "License" step in product-key gate mode, so a tester sees their
	// access is verified. It is empty in Workspace-OAuth mode. cmd/api masks the
	// session's key id before setting it — the full key is never sent to the template.
	LicenseKey string
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
	// TargetName is the human-readable destination label (e.g. the folder name
	// "Kaimi Proposals"), or "" when unknown — in which case the UI falls back to
	// showing the id. It is populated where the name is known (the WS-C5a
	// auto-created folder); a manually-pasted id has no resolvable name.
	TargetName string
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

// SAMKeySaver persists the tenant's SAM.gov API key entered on the onboarding
// "Connect" step. cmd/api backs it with internal/samsecret over Secret Manager, so
// the key is written as a new version of the deployment's SAM secret (never to the
// store, never logged) and the pipeline picks it up on the next hunt. The dashboard
// takes this function (not the samsecret type) to avoid a hard dependency on the
// Secret Manager client. nil = the onboarding page shows the "managed by your
// administrator" note instead of a key field. It returns samsecret.ErrInvalidKey
// (wrapped) for a malformed key so the handler can re-render with a 400.
type SAMKeySaver func(ctx context.Context, apiKey string) error

// WithSAMKeySaver wires the SAM.gov key write path (see SAMKeySaver). Without it the
// onboarding "Connect" step shows the deployment-secret note rather than a key field.
func WithSAMKeySaver(fn SAMKeySaver) Option {
	return func(h *Handler) { h.samKeySaver = fn }
}

// WithContextDocs wires the context-document store so the onboarding "Connect" step can
// accept uploads (capability statements, CPARS, past proposals) whose text feeds the
// capability map. Without it the upload control is hidden.
func WithContextDocs(store contextdoc.Store) Option {
	return func(h *Handler) { h.contextDocs = store }
}

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
			st.TargetName = t.Name
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

	// License (product-key gate mode): a masked product key shown on the License step.
	// Empty in Workspace-OAuth mode (the License step then shows the signed-in account).
	LicenseKey string

	// SAM.gov key entry. SAMKeyConfigured is true when a write path is wired
	// (WithSAMKeySaver); when false the Connect step shows the deployment-secret note
	// instead of a key field. SAMKeySaved drives the success treatment after a save.
	SAMKeyConfigured bool
	SAMKeySaved      bool

	// Context-document upload (Connect step). ContextDocsEnabled is true when a store is
	// wired (WithContextDocs); ContextDocs lists what's been uploaded; DocsSaved drives
	// the success banner after an upload.
	ContextDocsEnabled bool
	ContextDocs        []contextdoc.Doc
	DocsSaved          bool

	// Step is the wizard step to open on load ("welcome".."done"). The server sets it
	// to resume after a PRG redirect; the page's JS shows the matching panel.
	Step string

	// State flags.
	Saved      bool   // PRG success banner (company profile)
	DriveSaved bool   // PRG success banner (Drive destination)
	FormErr    string // validation error to re-render
}

// driveDestView is the rendered form of the current Drive destination shown on the
// onboarding page (WS-C5b/C5d). Exactly one of IsFolder / IsRoot is true, or both
// are false meaning "not set yet". When IsFolder is true, FolderID holds the id,
// OpenURL the Drive web link, and Label the text to show — the folder name when
// known, otherwise the id (so the user sees "Kaimi Proposals", not an opaque id).
type driveDestView struct {
	IsFolder bool
	IsRoot   bool
	FolderID string
	OpenURL  string
	Label    string
}

// driveDestination maps a stored target id and (optional) name to its rendered view.
// An empty id means no destination has been set yet (both flags false); the literal
// "root" sentinel renders as My Drive root; any other value is a folder id with an
// Open-in-Drive link. Label is the folder name when known, else the id itself, so a
// user-facing surface never shows a bare file id when a friendly name exists. The id
// is used only to build a URL via url-safe concatenation and both id and name are
// auto-escaped by html/template at render time.
func driveDestination(target, name string) driveDestView {
	id := strings.TrimSpace(target)
	switch id {
	case "":
		return driveDestView{}
	case driveTargetRoot:
		return driveDestView{IsRoot: true}
	default:
		label := strings.TrimSpace(name)
		if label == "" {
			label = id
		}
		return driveDestView{IsFolder: true, FolderID: id, OpenURL: driveFolderURLPrefix + id, Label: label}
	}
}

// onboardingContentTmpl is the full-screen onboarding WIZARD: a guided, multi-step
// setup (Welcome → License → Profile → Connect → Done) modeled on the design handoff
// and adapted to the web product and the product-key gate. It is a STANDALONE page
// (not the dashboard shell) so it fills the screen like the designed flow. Steps are
// shown one at a time by the inline script; forms POST to the existing handlers and
// the server resumes the wizard at the right step via PRG (?step=). All dynamic values
// render through html/template's contextual auto-escaping (no template.HTML), so a
// crafted company name, NAICS line, or masked key cannot inject markup or script.
const onboardingContentTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Kaimi — Setup</title>
<style>
  :root{--bg:#0b1220;--panel:#121a2b;--panel2:#0e1626;--border:#233047;--ink:#e8edf6;--ink3:#93a1bd;--accent:#3b82f6;--ok:#1a7f4b;--okbg:#e7f7ee;--errbg:#2a1620;--errbd:#5b2230;--errfg:#f3b5c2;}
  *{box-sizing:border-box;}
  body{margin:0;min-height:100vh;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;background:var(--bg);color:var(--ink);}
  .wz{display:flex;min-height:100vh;}
  .wz-hero{width:38%;max-width:520px;padding:40px;background:linear-gradient(160deg,#0d1730,#0b1220);border-right:1px solid var(--border);display:flex;flex-direction:column;justify-content:space-between;}
  .wz-brand{display:flex;align-items:center;gap:12px;}
  .wz-mark{width:34px;height:34px;border-radius:9px;background:var(--accent);display:flex;align-items:center;justify-content:center;color:#fff;font-weight:800;}
  .wz-brand h1{margin:0;font-size:18px;letter-spacing:.3px;}
  .wz-brand .tag{font-size:11px;color:var(--ink3);letter-spacing:1.5px;text-transform:uppercase;}
  .wz-hero-copy h2{font-size:30px;line-height:1.15;margin:0 0 12px;}
  .wz-hero-copy h2 .hl{color:#5aa2ff;}
  .wz-hero-copy p{color:var(--ink3);font-size:14px;line-height:1.5;max-width:340px;}
  .wz-foot{color:var(--ink3);font-size:12px;border-top:1px solid var(--border);padding-top:14px;}
  .wz-main{flex:1;padding:40px 56px;display:flex;flex-direction:column;max-width:760px;}
  .wz-top{display:flex;align-items:center;justify-content:space-between;margin-bottom:8px;}
  .wz-dots{display:flex;gap:8px;}
  .wz-dots i{width:46px;height:4px;border-radius:2px;background:#2a3650;display:block;transition:background .2s;}
  .wz-dots i.on{background:var(--accent);}
  .wz-count{color:var(--ink3);font-size:13px;}
  .wz-banner{display:flex;align-items:center;gap:8px;padding:11px 13px;border-radius:9px;font-size:13px;margin:14px 0;}
  .wz-banner svg{width:16px;height:16px;flex:0 0 auto;}
  .wz-banner--ok{background:var(--okbg);color:var(--ok);}
  .wz-banner--err{background:var(--errbg);border:1px solid var(--errbd);color:var(--errfg);}
  .wz-step{display:none;animation:fade .2s ease;}
  .wz-step.on{display:block;}
  @keyframes fade{from{opacity:0;transform:translateY(4px);}to{opacity:1;transform:none;}}
  .wz-step h2{font-size:26px;margin:18px 0 6px;}
  .wz-step .sub{color:var(--ink3);font-size:14px;line-height:1.5;margin:0 0 22px;max-width:540px;}
  .card{background:var(--panel);border:1px solid var(--border);border-radius:12px;padding:16px;margin-bottom:12px;display:flex;gap:14px;align-items:flex-start;}
  .card .ic{width:34px;height:34px;border-radius:9px;flex:0 0 auto;display:flex;align-items:center;justify-content:center;font-weight:700;color:#fff;}
  .card h3{margin:0 0 3px;font-size:15px;}
  .card p{margin:0;color:var(--ink3);font-size:13px;line-height:1.45;}
  .verified{background:#10241a;border:1px solid #1f5138;}
  .verified .ic{background:var(--ok);}
  .key{font-family:ui-monospace,Menlo,Consolas,monospace;background:var(--panel2);border:1px solid var(--border);border-radius:6px;padding:2px 8px;letter-spacing:1px;}
  form.wz-form{display:flex;flex-direction:column;gap:14px;max-width:560px;}
  label{display:flex;flex-direction:column;gap:5px;font-size:13px;font-weight:600;}
  input[type=text],textarea{font-size:14px;padding:10px 12px;border:1px solid var(--border);border-radius:8px;background:var(--panel2);color:var(--ink);font-family:inherit;}
  input:focus,textarea:focus{outline:2px solid var(--accent);border-color:var(--accent);}
  .row{display:flex;gap:14px;}.row label{flex:1;}
  .hint{font-weight:400;color:var(--ink3);font-size:12px;}
  input.mono{font-family:ui-monospace,Menlo,Consolas,monospace;letter-spacing:.4px;}
  input.mono:not(:placeholder-shown):invalid{border-color:#7a3b46;}
  /* Google connect — design-system treatment (dark Focus surface + G glyph),
     not Google's white brand button, to match the handoff's onboarding buttons. */
  .btn.gbtn{background:#16284c;color:#eaf1ff;border:1px solid rgba(150,180,230,.18);}
  .btn.gbtn:hover{background:#1c3362;border-color:rgba(150,180,230,.34);}
  .btn.gbtn:active{background:#142a52;}
  .gglyph{display:inline-flex;align-items:center;justify-content:center;width:20px;height:20px;border-radius:50%;background:#fff;color:#4285F4;font-weight:800;font-size:14px;line-height:1;flex:0 0 auto;}
  .drive-row{display:flex;align-items:center;gap:12px;flex-wrap:wrap;}
  .drive-row .muted{color:var(--ink3);font-size:12px;}
  input[type=file]{font-size:13px;color:var(--ink3);}
  .doc-list{margin:10px 0 0;padding-left:18px;}
  .doc-list li{font-size:13px;margin:3px 0;}
  fieldset{border:1px solid var(--border);border-radius:8px;padding:12px 14px;}
  legend{font-size:12px;font-weight:700;color:var(--ink3);padding:0 6px;}
  .chips{display:flex;flex-wrap:wrap;gap:8px;}
  .chk{display:inline-flex;align-items:center;gap:7px;font-weight:500;font-size:13px;}
  .wz-nav{display:flex;align-items:center;justify-content:space-between;margin-top:26px;gap:12px;}
  .btn{border:0;border-radius:9px;padding:11px 18px;font-size:14px;font-weight:600;cursor:pointer;display:inline-flex;align-items:center;gap:8px;text-decoration:none;}
  .btn svg{width:16px;height:16px;}
  .btn-primary{background:var(--accent);color:#fff;}.btn-primary:hover{background:#2f6fe0;}
  .btn-ghost{background:transparent;color:var(--ink3);border:1px solid var(--border);}
  .sum{list-style:none;margin:0;padding:0;display:flex;flex-direction:column;gap:10px;}
  .sum li{display:flex;align-items:center;gap:12px;background:var(--panel);border:1px solid var(--border);border-radius:10px;padding:13px 15px;}
  .sum .ck{width:24px;height:24px;border-radius:50%;background:var(--ok);color:#fff;display:flex;align-items:center;justify-content:center;flex:0 0 auto;}
  .sum .ck svg{width:14px;height:14px;}
  .sum .muted .ck{background:#2a3650;}
  .sum b{font-size:14px;}.sum span{color:var(--ink3);font-size:13px;}
  @media(max-width:860px){.wz-hero{display:none;}.wz-main{padding:28px 22px;}}
</style>
</head>
<body>
<div class="wz">
  <aside class="wz-hero">
    <div class="wz-brand"><span class="wz-mark">≈</span><div><h1>Kaimi</h1><div class="tag">The Seeker · by BlueMeta</div></div></div>
    <div class="wz-hero-copy">
      <h2>The agents hunt.<br><span class="hl">You</span> make the calls.</h2>
      <p>Kaimi finds and scores federal opportunities, drafts the proposals worth pursuing, and pauses for your review before anything ships.</p>
    </div>
    <div class="wz-foot">One key, your whole BD pipeline.</div>
  </aside>

  <main class="wz-main">
    <div class="wz-top">
      <div class="wz-dots"><i data-d="welcome"></i><i data-d="license"></i><i data-d="profile"></i><i data-d="connect"></i><i data-d="done"></i></div>
      <div class="wz-count"><span id="wzCur">1</span> / 5</div>
    </div>

    {{if .FormErr}}<div class="wz-banner wz-banner--err">` + iconWarn + `<span>{{.FormErr}}</span></div>{{end}}
    {{if .Saved}}<div class="wz-banner wz-banner--ok">` + iconCheck + `<span>Company profile saved.</span></div>{{end}}
    {{if .SAMKeySaved}}<div class="wz-banner wz-banner--ok">` + iconCheck + `<span>SAM.gov key saved — your next hunt will use it.</span></div>{{end}}
    {{if .DocsSaved}}<div class="wz-banner wz-banner--ok">` + iconCheck + `<span>Documents uploaded — Kaimi will use them to understand your business.</span></div>{{end}}
    {{if .DriveSaved}}<div class="wz-banner wz-banner--ok">` + iconCheck + `<span>Drive destination updated.</span></div>{{end}}

    <!-- 1. Welcome -->
    <section class="wz-step" data-step="welcome">
      <h2>Welcome to Kaimi</h2>
      <p class="sub">Setup takes about three minutes: confirm your license, tell Kaimi what your company does, and connect SAM.gov — so your next hunt is already yours.</p>
      <div class="card"><span class="ic" style="background:#3b82f6">N</span><div><h3>It hunts for you</h3><p>Kaimi pulls live SAM.gov opportunities and scores each against your capabilities.</p></div></div>
      <div class="card"><span class="ic" style="background:#22b8cf">T</span><div><h3>It drafts the ones you pick</h3><p>Select an opportunity and a team of agents outlines, writes, and checks the proposal.</p></div></div>
      <div class="card"><span class="ic" style="background:#f59e0b">✋</span><div><h3>You stay in command</h3><p>Nothing ships without you. Every proposal pauses at one human review gate — yours.</p></div></div>
      <div class="wz-nav"><span></span><button class="btn btn-primary" data-go="license">` + iconArrow + `Get started</button></div>
    </section>

    <!-- 2. License -->
    <section class="wz-step" data-step="license">
      <h2>Link your Kaimi license</h2>
      <p class="sub">Your access key connects this workspace to your evaluation and the agent runtime.</p>
      {{if .LicenseKey}}
      <div class="card verified"><span class="ic">` + iconCheck + `</span><div><h3>License verified</h3><p>Evaluation access · key <span class="key">{{.LicenseKey}}</span></p></div></div>
      {{else if .SignedIn}}
      <div class="card verified"><span class="ic">` + iconCheck + `</span><div><h3>Signed in</h3><p>{{.Email}}</p></div></div>
      {{else}}
      <div class="card"><span class="ic" style="background:#2a3650">!</span><div><h3>Not verified</h3><p>Open the access link from your invitation to verify your license.</p></div></div>
      {{end}}
      <div class="wz-nav"><button class="btn btn-ghost" data-back="welcome">Back</button><button class="btn btn-primary" data-go="profile">` + iconArrow + `Continue</button></div>
    </section>

    <!-- 3. Profile -->
    <section class="wz-step" data-step="profile">
      <h2>What does your company do?</h2>
      <p class="sub">Kaimi scores every opportunity against this profile. The better it knows you, the sharper the fit scores. Required: company name and at least one NAICS code.</p>
      <form class="wz-form" method="POST" action="/onboarding/profile">
        {{if .CSRFToken}}<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">{{end}}
        <label>Company name<input type="text" name="company" value="{{.Company}}" required></label>
        <div class="row">
          <label>UEI <span class="hint">(optional)</span><input type="text" name="uei" value="{{.UEI}}"></label>
          <label>CAGE <span class="hint">(optional)</span><input type="text" name="cage" value="{{.CAGE}}"></label>
        </div>
        <label>NAICS codes <span class="hint">— one per line: code|description|tier (primary|secondary|tertiary)</span>
          <textarea name="naics" rows="4" placeholder="541512|Computer Systems Design|primary">{{.NAICS}}</textarea></label>
        <fieldset><legend>Set-aside eligibility</legend><div class="chips">
          <label class="chk"><input type="checkbox" name="sa_small_business"{{if .SetAside.SmallBusiness}} checked{{end}}> Small business</label>
          <label class="chk"><input type="checkbox" name="sa_sdb"{{if .SetAside.SDB}} checked{{end}}> SDB</label>
          <label class="chk"><input type="checkbox" name="sa_eight_a"{{if .SetAside.EightA}} checked{{end}}> 8(a)</label>
          <label class="chk"><input type="checkbox" name="sa_sdvosb"{{if .SetAside.SDVOSB}} checked{{end}}> SDVOSB</label>
          <label class="chk"><input type="checkbox" name="sa_wosb"{{if .SetAside.WOSB}} checked{{end}}> WOSB</label>
          <label class="chk"><input type="checkbox" name="sa_hubzone"{{if .SetAside.HUBZone}} checked{{end}}> HUBZone</label>
        </div></fieldset>
        <label>Capabilities statement <span class="hint">— one competency per line</span>
          <textarea name="competencies" rows="3" placeholder="Cloud migration &amp; DevSecOps">{{.Competencies}}</textarea></label>
        <div class="wz-nav"><button class="btn btn-ghost" type="button" data-back="license">Back</button><button class="btn btn-primary" type="submit">` + iconCheck + `Save &amp; continue</button></div>
      </form>
    </section>

    <!-- 4. Connect -->
    <section class="wz-step" data-step="connect">
      <h2>Connect your data sources</h2>
      <p class="sub">Kaimi reads opportunities from SAM.gov with your own API key, and (optionally) saves finished proposals to your Google Drive.</p>
      {{if .SAMKeyConfigured}}
      <form class="wz-form" method="POST" action="/onboarding/samgov">
        {{if .CSRFToken}}<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">{{end}}
        <label>SAM.gov API key <span class="hint">— from your SAM.gov account → Account Details → API Key (about 40 characters)</span>
          <input type="text" name="sam_api_key" class="mono" maxlength="64" pattern="[A-Za-z0-9._-]{30,64}" inputmode="latin" autocomplete="off" spellcheck="false" autocapitalize="off" required title="Paste your SAM.gov API key — letters, digits, and - _ . (about 40 characters)." placeholder="e.g. AbCd1234-EfGh5678-IjKl9012-MnOp3456-Qr78"></label>
        <p class="sub" style="margin:0">Your key is stored encrypted in Secret Manager — never shown, logged, or shared. It is yours alone, so your daily quota is never shared with another tester.</p>
        <div><button class="btn btn-primary" type="submit">` + iconCheck + `Save SAM.gov key</button></div>
      </form>
      {{else}}
      <div class="card"><span class="ic" style="background:#2a3650">i</span><div><h3>Managed by your administrator</h3><p>This deployment supplies the SAM.gov key as a server secret; you don't need to enter one.</p></div></div>
      {{end}}
      {{if .ContextDocsEnabled}}
      <div class="card" style="margin-top:14px"><span class="ic" style="background:#8b5cf6">+</span><div style="flex:1">
        <h3>Business context documents <span class="hint">(optional, recommended)</span></h3>
        <p>Upload your capability statement, CPARS, or recent proposals. Kaimi reads them to understand your business and sharpen how it qualifies and scores opportunities. Tip: keep a Google Drive folder of BD context your team maintains over time — you can point Kaimi at it later.</p>
        <form class="wz-form" method="POST" action="/onboarding/docs" enctype="multipart/form-data">
          {{if .CSRFToken}}<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">{{end}}
          <input type="file" name="docs" multiple accept=".pdf,.docx,.doc,.txt,.md">
          <div><button class="btn btn-primary" type="submit">` + iconCheck + `Upload documents</button></div>
        </form>
        {{if .ContextDocs}}
        <ul class="doc-list">
          {{range .ContextDocs}}<li><strong>{{.Name}}</strong> <span class="hint">{{if .Text}}· text extracted{{else}}· stored{{end}}</span></li>{{end}}
        </ul>
        {{end}}
      </div></div>
      {{end}}
      <div class="card" style="margin-top:14px"><span class="ic" style="background:#22b8cf">` + iconLink + `</span><div style="flex:1">
        <h3>Google Drive <span class="hint">(optional)</span></h3>
        {{if not .Drive.Configured}}
        <p>Customer-Drive connect is not enabled in this deployment. Generated proposal Docs use the default Drive — drafts stay in Kaimi.</p>
        {{else if .Drive.Connected}}
        <p>Connected. Proposal Docs are created in this destination:</p>
        <p>
          {{if .DriveDest.IsFolder}}Folder <strong>{{.DriveDest.Label}}</strong> — <a href="{{.DriveDest.OpenURL}}" target="_blank" rel="noopener noreferrer" style="color:#5aa2ff">Open in Drive</a>
          {{else if .DriveDest.IsRoot}}My Drive (root)
          {{else}}Not set yet — Docs land in your connected Drive's default location.{{end}}
        </p>
        {{if .CanEditDrive}}
        <form class="wz-form" method="POST" action="/onboarding/drive/target" style="margin-top:10px">
          {{if .CSRFToken}}<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">{{end}}
          <fieldset><legend>Change destination</legend>
            <label class="chk"><input type="radio" name="drive_choice" value="folder"{{if not .DriveDest.IsRoot}} checked{{end}}> Specific folder</label>
            <label>Folder id<input type="text" name="drive_id" value="{{.DriveDest.FolderID}}" placeholder="Paste a Google Drive folder id"></label>
            <label class="chk"><input type="radio" name="drive_choice" value="root"{{if .DriveDest.IsRoot}} checked{{end}}> My Drive root</label>
          </fieldset>
          <div><button class="btn btn-primary" type="submit">` + iconCheck + `Save destination</button></div>
        </form>
        {{end}}
        <p><a href="` + driveConnectPath + `" style="color:#5aa2ff">Reconnect</a></p>
        {{else}}
        <p>Link your Google Drive so finished proposal Docs save straight to your Workspace. Kaimi requests the minimal scope — only files it creates.</p>
        <div class="drive-row">
          <a class="btn gbtn" href="` + driveConnectPath + `"><span class="gglyph" aria-hidden="true">G</span>Connect Google Drive</a>
          <span class="muted">Opens Google's secure consent screen · you can skip and connect later.</span>
        </div>
        {{end}}
      </div></div>
      <div class="wz-nav"><button class="btn btn-ghost" type="button" data-back="profile">Back</button><button class="btn btn-primary" type="button" data-go="done">` + iconArrow + `Continue</button></div>
    </section>

    <!-- 5. Done -->
    <section class="wz-step" data-step="done">
      <h2>You're set.</h2>
      <p class="sub">Kaimi runs your next hunt automatically and scores every opportunity against your profile. Jump in and triage your queue.</p>
      <ul class="sum">
        <li{{if not .LicenseKey}} class="muted"{{end}}><span class="ck">` + iconCheck + `</span><div><b>License linked</b></div></li>
        <li{{if not .HasProfile}} class="muted"{{end}}><span class="ck">` + iconCheck + `</span><div><b>Company profile {{if .HasProfile}}saved{{else}}pending{{end}}</b> <span>— grounds hunting, scoring &amp; drafting</span></div></li>
        <li{{if not .SAMKeySaved}} class="muted"{{end}}><span class="ck">` + iconCheck + `</span><div><b>SAM.gov {{if .SAMKeySaved}}connected{{else}}pending{{end}}</b> <span>— your own key &amp; quota</span></div></li>
      </ul>
      <div class="wz-nav"><button class="btn btn-ghost" type="button" data-back="connect">Back</button><a class="btn btn-primary" href="/">` + iconArrow + `Enter Kaimi</a></div>
    </section>
  </main>
</div>

<script>
(function(){
  var steps=["welcome","license","profile","connect","done"];
  var initial="{{.Step}}";
  function show(name){
    var idx=steps.indexOf(name); if(idx<0){idx=0;name=steps[0];}
    var secs=document.querySelectorAll(".wz-step");
    for(var i=0;i<secs.length;i++){secs[i].classList.toggle("on",secs[i].getAttribute("data-step")===name);}
    var dots=document.querySelectorAll(".wz-dots i");
    for(var j=0;j<dots.length;j++){dots[j].classList.toggle("on",j<=idx);}
    document.getElementById("wzCur").textContent=(idx+1);
    try{history.replaceState(null,"","?step="+name);}catch(e){}
    window.scrollTo(0,0);
  }
  document.addEventListener("click",function(ev){
    var go=ev.target.closest&&ev.target.closest("[data-go]");
    if(go){ev.preventDefault();show(go.getAttribute("data-go"));return;}
    var back=ev.target.closest&&ev.target.closest("[data-back]");
    if(back){ev.preventDefault();show(back.getAttribute("data-back"));}
  });
  show(initial);
})();
</script>
</body>
</html>`

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
	data.SAMKeySaved = r.URL.Query().Get("sam_saved") == "1"
	data.DocsSaved = r.URL.Query().Get("docs_saved") == "1"
	// Resume at the requested step after a PRG redirect; ignore an unknown value.
	if s := r.URL.Query().Get("step"); validStep(s) {
		data.Step = s
	}

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
		data.HasProfile = true // keep the License step marked done on re-render
		data.Step = stepProfile
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

	// PRG: redirect so a refresh does not re-POST the form; advance to the Connect step.
	http.Redirect(w, r, onboardingPath+"?saved=1&step="+stepConnect, http.StatusSeeOther)
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

// handleOnboardingSAMKey serves POST /onboarding/samgov: the onboarding "Connect"
// step's SAM.gov API key form. Like the profile and Drive writes it FAILS CLOSED on
// auth + CSRF (authorizeMutation) and mutates nothing until the gate passes. The key
// is handed to the injected samKeySaver (Secret Manager); a malformed key
// (samsecret.ErrInvalidKey) re-renders the page with a 400 and persists nothing, while
// a backend failure is a 500. On success it follows PRG, advancing the wizard to the
// final step. The key is never logged.
func (h *Handler) handleOnboardingSAMKey(w http.ResponseWriter, r *http.Request) {
	if h.samKeySaver == nil {
		http.Error(w, "SAM.gov key entry is not available in this deployment", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if !h.authorizeMutation(w, r) {
		return
	}

	key := strings.TrimSpace(r.PostFormValue("sam_api_key"))
	if err := h.samKeySaver(r.Context(), key); err != nil {
		if errors.Is(err, samsecret.ErrInvalidKey) {
			// Malformed key: re-render the wizard at the Connect step with the reason.
			// Never echo the key back into the page.
			data := h.newOnboardingData(r)
			data.Step = stepConnect
			data.FormErr = "That doesn't look like a SAM.gov API key. Paste the 40-character key from your SAM.gov account."
			w.WriteHeader(http.StatusBadRequest)
			h.renderOnboarding(w, &data)
			return
		}
		// Backend failure (Secret Manager). Keep detail server-side; never log the key.
		log.Printf("dashboard: onboarding SAM key save failed: %v", err)
		http.Error(w, "failed to save the SAM.gov key", http.StatusInternalServerError)
		return
	}

	// PRG: advance to the final step with a success flag.
	http.Redirect(w, r, onboardingPath+"?sam_saved=1&step="+stepDone, http.StatusSeeOther)
}

// maxUploadBytes bounds an onboarding document upload request. Capability statements /
// CPARS / past proposals are small; 25 MiB is generous headroom while preventing a
// memory-exhaustion upload.
const maxUploadBytes = 25 << 20

// handleOnboardingDocUpload serves POST /onboarding/docs: the Connect step's context-
// document upload (multipart). Like the other onboarding writes it FAILS CLOSED on auth
// + CSRF (authorizeMutation) before storing anything. Each uploaded file is persisted +
// text-extracted via the contextdoc store; on success it PRG-redirects back to the
// Connect step. Files are never logged.
func (h *Handler) handleOnboardingDocUpload(w http.ResponseWriter, r *http.Request) {
	if h.contextDocs == nil {
		http.Error(w, "document upload is not available in this deployment", http.StatusServiceUnavailable)
		return
	}
	// Cap the TOTAL request body BEFORE parsing: ParseMultipartForm only bounds the
	// in-memory portion, not the request size, so without this a large body would be
	// read to disk before the auth/CSRF gate runs. MaxBytesReader also bounds the
	// cumulative bytes across all files in one request. The small extra headroom covers
	// multipart framing overhead around the file payloads.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		http.Error(w, "upload too large or malformed", http.StatusBadRequest)
		return
	}
	if !h.authorizeMutation(w, r) {
		return
	}

	var files []*multipart.FileHeader
	if r.MultipartForm != nil {
		files = r.MultipartForm.File["docs"]
	}
	if len(files) == 0 {
		data := h.newOnboardingData(r)
		data.Step = stepConnect
		data.FormErr = "Choose at least one document to upload."
		w.WriteHeader(http.StatusBadRequest)
		h.renderOnboarding(w, &data)
		return
	}

	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			http.Error(w, "could not read the uploaded file", http.StatusBadRequest)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(f, maxUploadBytes))
		_ = f.Close()
		if err != nil {
			http.Error(w, "could not read the uploaded file", http.StatusBadRequest)
			return
		}
		if _, err := h.contextDocs.Save(r.Context(), fh.Filename, fh.Header.Get("Content-Type"), raw); err != nil {
			// Keep detail server-side; never log the file contents.
			log.Printf("dashboard: onboarding doc upload save failed: %v", err)
			http.Error(w, "failed to save the uploaded document", http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, onboardingPath+"?docs_saved=1&step="+stepConnect, http.StatusSeeOther)
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
	// Populate the sidebar pipeline counts so the bar matches every other screen
	// (without this the onboarding page showed all zeros — it never lists the store).
	h.fillShellCounts(r.Context(), &data.shellData)
	if ident, ok := h.resolveIdentity(r); ok {
		data.SignedIn = true
		data.Email = ident.Email
		data.CSRFToken = ident.CSRFToken
		data.LicenseKey = ident.LicenseKey
	}
	data.SAMKeyConfigured = h.samKeySaver != nil
	if h.contextDocs != nil {
		data.ContextDocsEnabled = true
		// Best-effort: an I/O error listing uploads must not break the page (the rest
		// of onboarding still works); just show none.
		if docs, err := h.contextDocs.List(); err == nil {
			data.ContextDocs = docs
		}
	}
	data.Step = stepWelcome
	if h.driveStatus != nil {
		data.Drive = h.driveStatus()
	}
	data.DriveDest = driveDestination(data.Drive.Target, data.Drive.TargetName)
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

// onboardingTemplate compiles the standalone onboarding WIZARD page. Unlike the other
// dashboard pages it does NOT wrap the shared shell (sidebar/chrome): the wizard is a
// full-screen, multi-step setup experience, so onboardingContentTmpl is a complete HTML
// document parsed on its own. It is a package function so setupTemplates can build it
// alongside the other page templates.
func onboardingTemplate(funcMap template.FuncMap) *template.Template {
	return template.Must(template.New("onboarding").Funcs(funcMap).Parse(onboardingContentTmpl))
}

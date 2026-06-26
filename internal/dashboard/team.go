package dashboard

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

// InviteMinter mints a new product key for a teammate (label = who it's for) and returns the
// raw key + its expiry. cmd/api implements it over the product-key registry. The handler
// turns the key into a magic link from the request host, so the link is always correct for
// the deployment's public URL.
type InviteMinter func(ctx context.Context, label string) (key string, expiresAt time.Time, err error)

// WithInviteMinter enables the self-serve Team invite page. Without it, /team explains the
// feature isn't available in this deployment.
func WithInviteMinter(fn InviteMinter) Option {
	return func(h *Handler) { h.inviteMinter = fn }
}

// TeamData backs the Team page.
type TeamData struct {
	shellData
	CSRFToken string
	Enabled   bool // an invite minter is wired

	// Inline result of a just-created invite. The link is a secret-bearing magic link, so it
	// is rendered on the POST response only — never placed in a redirect URL or logged.
	InvitedEmail string
	InviteLink   string
	InviteExpiry string
	FormErr      string
}

func (h *Handler) newTeamData(r *http.Request) TeamData {
	d := TeamData{shellData: shellData{PageTitle: "Team", ActiveNav: "team"}, Enabled: h.inviteMinter != nil}
	if ident, ok := h.resolveIdentity(r); ok {
		d.CSRFToken = ident.CSRFToken
	}
	h.fillShellCounts(r.Context(), &d.shellData)
	return d
}

// handleTeam serves GET /team: the invite-a-teammate page.
func (h *Handler) handleTeam(w http.ResponseWriter, r *http.Request) {
	d := h.newTeamData(r)
	h.renderTeam(w, &d)
}

// handleTeamInvite serves POST /team/invite: mints a teammate key and renders the magic link
// inline. FAILS CLOSED on auth + CSRF. The teammate joins the SAME workspace (shared data);
// their key is labeled with their email and is individually revocable.
func (h *Handler) handleTeamInvite(w http.ResponseWriter, r *http.Request) {
	if h.inviteMinter == nil {
		http.Error(w, "team invites are not available in this deployment", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if !h.authorizeMutation(w, r) {
		return
	}

	d := h.newTeamData(r)
	email := strings.TrimSpace(r.PostFormValue("email"))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address == "" {
		d.FormErr = "Enter a valid email address for your teammate."
		w.WriteHeader(http.StatusBadRequest)
		h.renderTeam(w, &d)
		return
	}

	key, expiresAt, err := h.inviteMinter(r.Context(), addr.Address)
	if err != nil {
		// Never log the key; only the failure context.
		log.Printf("dashboard: team invite mint failed: %v", err)
		http.Error(w, "failed to create the invite", http.StatusInternalServerError)
		return
	}
	d.InvitedEmail = addr.Address
	d.InviteLink = inviteMagicLink(r, key)
	d.InviteExpiry = expiresAt.Format("Jan 2, 2006")
	h.renderTeam(w, &d)
}

func (h *Handler) renderTeam(w http.ResponseWriter, d *TeamData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.teamTmpl.Execute(w, d); err != nil {
		log.Printf("dashboard: team template execute: %v", err)
	}
}

// inviteMagicLink builds the absolute /access link for a freshly minted key from the
// request's host + forwarded scheme (correct behind Cloud Run's TLS-terminating front end).
func inviteMagicLink(r *http.Request, key string) string {
	scheme := "https"
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		scheme = xf
	} else if r.TLS == nil {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/access?key=%s", scheme, r.Host, key)
}

// teamContentTmpl is the Team page, rendered inside the (light-themed) app shell. Styles use
// the dashboard design tokens (--accent, --line, --ink, --r-md/lg, --e-1) for cohesion.
const teamContentTmpl = `{{define "content"}}
<style>
.team-panel{ background:#fff; border:1px solid var(--line); border-radius:var(--r-lg); padding:24px 26px; max-width:660px; box-shadow:var(--e-1); }
.team-panel h3{ font:650 17px/1.2 var(--font-sans); letter-spacing:-0.01em; }
.team-sub{ color:var(--ink-soft); font-size:14px; margin:6px 0 18px; line-height:1.5; }
.team-form{ display:flex; gap:10px; align-items:center; flex-wrap:wrap; }
.team-input{ flex:1; min-width:240px; border:1px solid var(--line); border-radius:var(--r-md); padding:11px 13px; font:inherit; font-size:14px; color:var(--ink); background:#fff; }
.team-input:focus{ outline:none; border-color:var(--accent); box-shadow:0 0 0 3px color-mix(in oklab,var(--accent) 18%,transparent); }
.team-btn{ background:var(--accent); color:#fff; border:0; border-radius:var(--r-md); padding:11px 18px; font:600 14px/1 var(--font-sans); cursor:pointer; white-space:nowrap; }
.team-btn:hover{ filter:brightness(1.06); }
.team-copy{ background:#fff; border:1px solid var(--line); color:var(--ink-2); }
.team-err{ background:#fdecec; color:#b42318; border-radius:var(--r-md); padding:10px 13px; font-size:13px; margin-bottom:14px; }
.team-result{ margin-top:22px; padding-top:18px; border-top:1px solid var(--line); }
.team-result-h{ display:flex; align-items:center; gap:8px; font:650 15px/1.2 var(--font-sans); color:var(--st-progress); }
.team-link-row{ display:flex; gap:10px; margin-top:12px; flex-wrap:wrap; }
.team-link{ font-family:ui-monospace,Menlo,Consolas,monospace; font-size:12.5px; color:var(--ink-2); }
.team-note{ font-size:13px; color:var(--ink-soft); margin-top:18px; line-height:1.5; }
</style>
<div class="page-head">
  <div class="eyebrow">Account</div>
  <h1>Team</h1>
  <p class="lead">Invite a teammate to share this workspace. They see the same opportunities and proposals, and you can revoke their access anytime.</p>
</div>
{{if .Enabled}}
<div class="team-panel">
  <h3>Invite a teammate</h3>
  <p class="team-sub">Enter their email and we'll create a private access link that signs them straight into this workspace — no Google sign-in or allow-list needed.</p>
  {{if .FormErr}}<div class="team-err">{{.FormErr}}</div>{{end}}
  <form method="POST" action="/team/invite" class="team-form">
    {{if .CSRFToken}}<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">{{end}}
    <input type="email" name="email" required autocomplete="off" placeholder="teammate@company.com" class="team-input">
    <button type="submit" class="team-btn">Create invite</button>
  </form>
  {{if .InviteLink}}
  <div class="team-result">
    <div class="team-result-h">` + iconCheck + `Invite ready for {{.InvitedEmail}}</div>
    <p class="team-sub">Send them this private link — it signs them straight in. Access expires {{.InviteExpiry}}; you can revoke it anytime.</p>
    <div class="team-link-row">
      <input type="text" readonly value="{{.InviteLink}}" id="team-link" class="team-input team-link" onclick="this.select()">
      <button type="button" class="team-btn team-copy" onclick="navigator.clipboard.writeText(document.getElementById('team-link').value);this.textContent='Copied ✓'">Copy link</button>
    </div>
  </div>
  {{end}}
  <p class="team-note">Each teammate gets their own link, so you can remove one person without affecting the rest. Everyone shares this company's opportunities and proposals.</p>
</div>
{{else}}
<div class="empty2"><div class="g">` + iconTeam + `</div><h3>Team invites aren't enabled</h3><p>This deployment isn't configured to mint teammate access. Contact your administrator.</p></div>
{{end}}
{{end}}`

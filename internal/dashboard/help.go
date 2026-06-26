package dashboard

import (
	"net/http"
	"strings"
)

// handleHelp serves GET /help: a public, UNGATED setup guide. It is mounted outside the
// product-key wrap (see internal/httpapi server wiring) so a tester can read how to get
// their SAM.gov API key before - or without - a session. It needs no profile or auth and
// renders a self-contained page (no external assets).
func (h *Handler) handleHelp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(helpPage))
}

// helpPage is helpHTML with the shared brand favicon + the Kai wave logo injected once at init,
// so the public help page matches the dashboard's branding.
var helpPage = strings.NewReplacer(
	"<!--FAVICON-->", string(FaviconLink()),
	"<!--MARK-->", sidebarMarkSVG,
).Replace(helpHTML)

// helpHTML is the standalone help/setup guide. Self-contained (inline CSS, no external
// assets) so it works under the strict CSP and renders identically whether or not the
// reader has a session.
const helpHTML = `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Kaimi - Help &amp; Setup Guide</title>
<!--FAVICON-->
<style>
:root{--bg:#0a0f1c;--surface:#0e1525;--border:#1e2a44;--ink:#e8eefc;--ink2:#aebbd6;--ink3:#7d8aa8;--blue:#3b82f6;--ok:#22c55e;}
*{box-sizing:border-box;margin:0;padding:0}
body{background:var(--bg);color:var(--ink);font:15px/1.6 -apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;padding:32px 16px;}
.wrap{max-width:760px;margin:0 auto;}
.brand{display:flex;align-items:center;gap:12px;margin-bottom:8px;}
.mk{width:36px;height:36px;border-radius:10px;flex:none;background:linear-gradient(150deg,#1D4ED8,#0A1B3D);display:grid;place-items:center;}
.brand h1{font-size:20px;} .brand .tag{font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:var(--ink3);}
h2{font-size:18px;margin:30px 0 10px;padding-bottom:6px;border-bottom:1px solid var(--border);}
h3{font-size:15px;margin:18px 0 6px;color:var(--ink);}
p,li{color:var(--ink2);} .lead{color:var(--ink2);margin:10px 0 4px;}
ol,ul{margin:8px 0 8px 22px;} li{margin:5px 0;}
.card{background:var(--surface);border:1px solid var(--border);border-radius:10px;padding:16px 18px;margin:14px 0;}
code{background:#11192c;border:1px solid var(--border);border-radius:5px;padding:1px 6px;font-family:ui-monospace,Menlo,Consolas,monospace;font-size:13px;color:#9ec1ff;}
a{color:#7cb3ff;} .note{font-size:13px;color:var(--ink3);}
.back{display:inline-block;margin-top:26px;color:#7cb3ff;text-decoration:none;}
.pill{display:inline-block;font-size:11px;font-weight:700;color:var(--ok);border:1px solid rgba(34,197,94,.4);border-radius:999px;padding:2px 9px;}
</style></head>
<body><div class="wrap">
  <div class="brand"><span class="mk"><!--MARK--></span><div><h1>Kaimi</h1><div class="tag">Help &amp; Setup Guide</div></div></div>
  <p class="lead">Everything you need to get set up and running. Most questions are about the SAM.gov API key - start there.</p>

  <h2>Getting your SAM.gov API key</h2>
  <p>Kaimi hunts live federal opportunities using <strong>your own</strong> SAM.gov API key, so your daily search quota is yours alone and never shared with another company.</p>
  <div class="card">
    <h3>Step by step</h3>
    <ol>
      <li>Sign in at <a href="https://sam.gov" target="_blank" rel="noopener">sam.gov</a> with your account (the one tied to your entity registration).</li>
      <li>Open your <strong>Account Details</strong> (top-right profile menu) → <strong>Workspace</strong>.</li>
      <li>Find the <strong>API Key</strong> section and choose <strong>Request / Generate API Key</strong> (you may be asked to re-enter your password).</li>
      <li>Copy the key - it's about <strong>40 characters</strong> of letters, digits, and <code>- _ .</code></li>
      <li>Back in Kaimi's <strong>Connect</strong> step, paste it into <strong>SAM.gov API key</strong> and click <strong>Save SAM.gov key</strong>.</li>
    </ol>
    <p class="note">Your key is stored encrypted in Google Secret Manager. Kaimi never displays, logs, or shares it. A fresh hunt runs as soon as you save it, then daily after that.</p>
  </div>
  <h3>Tips</h3>
  <ul>
    <li>Your SAM.gov account must be <strong>registered to an entity</strong> to get the standard daily quota (1,000 requests/day).</li>
    <li>If a hunt returns nothing, double-check the key was pasted with no extra spaces and that your NAICS codes are correct.</li>
  </ul>

  <h2>Your company profile &amp; NAICS codes</h2>
  <p>Kaimi scores every opportunity against your profile. The most important field is your <strong>NAICS codes</strong> - they decide which solicitations Kaimi pulls from SAM.gov.</p>
  <ul>
    <li>On the <strong>Profile</strong> step, search the NAICS picker by keyword (e.g. <code>computer systems</code>) or by code (e.g. <code>541512</code>) and select your codes.</li>
    <li>Mark each as <strong>primary</strong>, <strong>secondary</strong>, or <strong>tertiary</strong> to weight how strongly Kaimi favors it.</li>
    <li>Add a few <strong>competencies</strong> (one per line) - short phrases describing what you do; Kaimi matches these against opportunity text.</li>
  </ul>

  <h2>How Kaimi works</h2>
  <ol>
    <li><strong>It hunts</strong> live SAM.gov opportunities for your NAICS codes and scores each against your profile.</li>
    <li><strong>You pick</strong> what to pursue from the scored board.</li>
    <li><strong>It drafts</strong> the proposal - a team of agents outlines, writes, and checks it, flagging gaps for you to fill.</li>
    <li><strong>You stay in command.</strong> Every proposal pauses at one human review gate: yours. Nothing is ever submitted automatically.</li>
  </ol>

  <h2>Google Drive (optional)</h2>
  <p>You can connect your Google Drive on the Connect step to save finished proposals straight to your Workspace. It's optional - you can skip it and connect later.</p>

  <a class="back" href="/onboarding">← Back to setup</a>
</div></body></html>`

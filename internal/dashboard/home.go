package dashboard

import "net/http"

// handleHome serves GET /home: the public, UNGATED landing page. It is mounted outside the
// product-key wrap (see internal/httpapi server wiring) so anyone — including Google's OAuth
// reviewers — can reach it without a session. Google's consent-screen verification requires a
// homepage that describes the app and links to the privacy policy; this is that page.
// Self-contained (inline CSS, no external assets) so it renders under the strict CSP.
func (h *Handler) handleHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(homeHTML))
}

// homeHTML is the standalone landing page. Content is kept ACCURATE to what Kaimi does
// (finds SAM.gov opportunities with the user's own key, scores them, drafts proposals for
// human review — never auto-submits — and exports to Word/PDF or the user's Drive). It links
// the privacy policy, which Google's homepage requirement asks for.
const homeHTML = `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Kaimi — Federal BD, automated. You stay in control.</title>
<meta name="description" content="Kaimi finds the federal opportunities worth your time, scores them against your capabilities, and drafts compliant proposals — so your team decides and wins instead of searching and formatting.">
<style>
:root{--bg:#0a0f1c;--surface:#0e1525;--surface2:#111b30;--border:#1e2a44;--ink:#e8eefc;--ink2:#aebbd6;--ink3:#7d8aa8;--blue:#3b82f6;--cyan:#22b8cf;--ok:#22c55e;}
*{box-sizing:border-box;margin:0;padding:0}
body{background:radial-gradient(1200px 600px at 50% -10%,#10203c 0%,var(--bg) 55%);color:var(--ink);font:16px/1.6 -apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;min-height:100vh;}
.wrap{max-width:960px;margin:0 auto;padding:26px 18px 60px;}
header{display:flex;align-items:center;justify-content:space-between;margin-bottom:60px;}
.brand{display:flex;align-items:center;gap:11px;}
.mark{width:34px;height:34px;border-radius:9px;background:linear-gradient(135deg,var(--blue),var(--cyan));display:flex;align-items:center;justify-content:center;font-size:19px;font-weight:700;}
.brand b{font-size:18px;letter-spacing:-.01em;} .brand .by{font-size:11px;color:var(--ink3);}
nav a{color:var(--ink2);text-decoration:none;font-size:14px;margin-left:18px;} nav a:hover{color:var(--ink);}
.hero{text-align:center;padding:18px 0 8px;}
.eyebrow{font-size:12px;letter-spacing:.13em;text-transform:uppercase;color:var(--cyan);font-weight:700;}
h1{font-size:42px;line-height:1.1;letter-spacing:-.02em;margin:14px 0 16px;}
.hero p{color:var(--ink2);font-size:18px;max-width:660px;margin:0 auto 26px;}
.cta{display:flex;gap:12px;justify-content:center;flex-wrap:wrap;}
.btn{display:inline-block;padding:12px 22px;border-radius:10px;font-weight:600;font-size:15px;text-decoration:none;}
.btn-primary{background:linear-gradient(135deg,var(--blue),var(--cyan));color:#06101f;}
.btn-ghost{background:var(--surface);color:var(--ink);border:1px solid var(--border);}
.btn:hover{filter:brightness(1.07);}
.grid{display:grid;grid-template-columns:repeat(2,1fr);gap:14px;margin:54px 0 10px;}
.card{background:var(--surface);border:1px solid var(--border);border-radius:13px;padding:20px 22px;}
.card h3{font-size:16px;margin:0 0 6px;} .card p{color:var(--ink2);font-size:14.5px;}
.k{font-size:12px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:var(--cyan);}
h2{font-size:24px;text-align:center;margin:56px 0 6px;letter-spacing:-.01em;}
.sub{text-align:center;color:var(--ink3);font-size:14px;margin-bottom:24px;}
.steps{display:grid;grid-template-columns:repeat(3,1fr);gap:14px;}
.step{background:var(--surface2);border:1px solid var(--border);border-radius:13px;padding:18px 20px;}
.step .n{width:26px;height:26px;border-radius:50%;background:rgba(59,130,246,.18);color:#9ec1ff;display:flex;align-items:center;justify-content:center;font-weight:700;font-size:13px;margin-bottom:10px;}
.step h4{font-size:15px;margin:0 0 4px;} .step p{color:var(--ink2);font-size:14px;}
.privacy{margin:54px 0 0;background:var(--surface);border:1px solid var(--border);border-radius:13px;padding:22px 24px;text-align:center;}
.privacy h3{font-size:17px;margin-bottom:8px;} .privacy p{color:var(--ink2);font-size:14.5px;max-width:720px;margin:0 auto;}
.privacy a{color:#7cb3ff;}
footer{margin-top:54px;padding-top:22px;border-top:1px solid var(--border);display:flex;justify-content:space-between;flex-wrap:wrap;gap:10px;color:var(--ink3);font-size:13px;}
footer a{color:var(--ink2);text-decoration:none;margin-left:16px;} footer a:hover{color:var(--ink);}
@media(max-width:640px){.grid,.steps{grid-template-columns:1fr;}h1{font-size:32px;}.hero p{font-size:16px;}}
</style></head>
<body><div class="wrap">
  <header>
    <div class="brand"><span class="mark">&#8776;</span><div><b>Kaimi</b><div class="by">by BlueMeta Technologies</div></div></div>
    <nav><a href="/help">Help</a><a href="/privacy">Privacy</a><a href="/entry">Sign in</a></nav>
  </header>

  <section class="hero">
    <div class="eyebrow">Federal BD, automated — you stay in control</div>
    <h1>The agents hunt.<br>You make the calls.</h1>
    <p>Kaimi finds the federal opportunities worth your time, scores them against your capabilities, and drafts compliant proposals — so your team spends its hours deciding and winning, not searching and formatting.</p>
    <div class="cta">
      <a class="btn btn-primary" href="/entry">Enter your access link</a>
      <a class="btn btn-ghost" href="/help">Setup guide</a>
    </div>
  </section>

  <div class="grid">
    <div class="card"><div class="k">Find</div><h3>The right opportunities</h3><p>Hunts live SAM.gov solicitations with <strong>your own</strong> API key and surfaces the ones that fit your NAICS codes and set-aside eligibility — refreshed daily.</p></div>
    <div class="card"><div class="k">Score</div><h3>Bid or pass, with reasons</h3><p>Rates every opportunity against your capability profile and explains <em>why</em> it's a fit or not — so you triage in minutes.</p></div>
    <div class="card"><div class="k">Draft</div><h3>A real first draft</h3><p>Generates a structured, compliance-aware proposal draft you edit in place. A human always reviews and approves — Kaimi <strong>never</strong> submits on your behalf.</p></div>
    <div class="card"><div class="k">Deliver</div><h3>In the format you need</h3><p>Export to Word or PDF, get a requirements compliance matrix, or save an editable Google Doc straight to your Drive to share with your team.</p></div>
  </div>

  <h2>How it works</h2>
  <div class="sub">Set up once, then it runs every day.</div>
  <div class="steps">
    <div class="step"><div class="n">1</div><h4>Connect</h4><p>Add your company profile, NAICS codes, and your SAM.gov key. Optionally connect Google Drive.</p></div>
    <div class="step"><div class="n">2</div><h4>Review</h4><p>See scored opportunities and AI-drafted proposals, ready for your edits at the review gate.</p></div>
    <div class="step"><div class="n">3</div><h4>Win</h4><p>Approve, export, and submit on your terms. You stay in control end to end.</p></div>
  </div>

  <div class="privacy">
    <h3>Private by design</h3>
    <p>Built for contractors who can't pool their pipeline with competitors. Your data stays in your own environment. If you connect Google Drive, Kaimi uses the <strong>minimal</strong> permission — it can only see files it creates for you, never the rest of your Drive — and your information is never used to train AI models. Read our <a href="/privacy">Privacy Policy</a>.</p>
  </div>

  <footer>
    <div>&copy; 2026 BlueMeta Technologies</div>
    <div><a href="/help">Help</a><a href="/privacy">Privacy Policy</a><a href="mailto:malik@bluemetatech.com">Contact</a></div>
  </footer>
</div></body></html>`

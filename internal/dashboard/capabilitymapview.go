package dashboard

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"

	"github.com/Mawar2/Kaimi/internal/capabilitymap"
)

// This file implements the "Your capability map" view: a read-only page showing how
// Kaimi understands the tenant's business (the capabilitymap.CapabilityMap built from
// the onboarding profile + uploaded context documents). It is a standalone full-screen
// page (like the onboarding wizard), reachable at /capability-map and linked from the
// onboarding Done step. All fields render through html/template auto-escaping.

// WithCapabilityMap wires the capability-map reader so the /capability-map view can show
// the tenant's map. Without it the view reports the feature is unavailable.
func WithCapabilityMap(store capabilitymap.Store) Option {
	return func(h *Handler) { h.capMap = store }
}

// capabilityMapData is the view-model for the capability-map page.
type capabilityMapData struct {
	Unavailable  bool // no reader wired
	NotBuilt     bool // reader wired but no map yet
	Map          *capabilitymap.CapabilityMap
	GeneratedStr string // human-readable GeneratedAt
}

// handleCapabilityMap serves GET /capability-map. It loads the tenant's map (or reports
// "not built yet" / "unavailable") and renders the read-only view.
func (h *Handler) handleCapabilityMap(w http.ResponseWriter, _ *http.Request) {
	var data capabilityMapData
	if h.capMap == nil {
		data.Unavailable = true
	} else {
		m, err := h.capMap.Load()
		switch {
		case errors.Is(err, capabilitymap.ErrNotFound):
			data.NotBuilt = true
		case err != nil:
			fmt.Printf("capability map load failed: %v\n", err)
			http.Error(w, "failed to load the capability map", http.StatusInternalServerError)
			return
		default:
			data.Map = m
			if !m.GeneratedAt.IsZero() {
				data.GeneratedStr = m.GeneratedAt.Format("Jan 2, 2006 15:04 MST")
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.capMapTmpl.Execute(w, &data); err != nil {
		fmt.Printf("capability map template execution failed: %v\n", err)
	}
}

// capabilityMapTemplate compiles the standalone capability-map page. It does not use the
// dashboard shell — it is a focused full-screen view, like the onboarding wizard.
func capabilityMapTemplate(funcMap template.FuncMap) *template.Template {
	return template.Must(template.New("capabilitymap").Funcs(funcMap).Parse(capabilityMapContentTmpl))
}

// capabilityMapContentTmpl is the standalone capability-map page. All dynamic values
// render through html/template's contextual auto-escaping (no template.HTML), so document-
// derived text cannot inject markup.
const capabilityMapContentTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Kaimi — Your capability map</title>
<style>
  :root{--bg:#0b1220;--panel:#121a2b;--panel2:#0e1626;--border:#233047;--ink:#e8edf6;--ink3:#93a1bd;--accent:#3b82f6;--ok:#1a7f4b;}
  *{box-sizing:border-box;}
  body{margin:0;min-height:100vh;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;background:var(--bg);color:var(--ink);}
  .wrap{max-width:880px;margin:0 auto;padding:40px 28px 64px;}
  .top{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:8px;}
  a.back{color:var(--ink3);font-size:13px;text-decoration:none;}
  a.back:hover{color:var(--ink);}
  h1{font-size:28px;margin:6px 0 4px;}
  .sub{color:var(--ink3);font-size:14px;line-height:1.5;margin:0 0 24px;max-width:620px;}
  .note{background:var(--panel);border:1px solid var(--border);border-radius:12px;padding:20px;color:var(--ink3);font-size:14px;line-height:1.5;}
  .note a{color:#5aa2ff;}
  .summary{background:var(--panel);border:1px solid var(--border);border-radius:12px;padding:18px;font-size:15px;line-height:1.55;margin-bottom:22px;}
  h2{font-size:13px;text-transform:uppercase;letter-spacing:1.2px;color:var(--ink3);margin:26px 0 10px;}
  .comp{background:var(--panel);border:1px solid var(--border);border-radius:10px;padding:14px 16px;margin-bottom:10px;}
  .comp h3{margin:0 0 4px;font-size:15px;}
  .comp p{margin:0 0 6px;color:var(--ink3);font-size:13px;line-height:1.45;}
  .evidence{margin:6px 0 0;padding-left:18px;}
  .evidence li{font-size:12px;color:var(--ink3);margin:2px 0;}
  ul.plain{margin:0;padding-left:18px;}
  ul.plain li{font-size:14px;margin:4px 0;}
  .chips{display:flex;flex-wrap:wrap;gap:8px;}
  .chip{background:var(--panel2);border:1px solid var(--border);border-radius:999px;padding:4px 12px;font-size:13px;color:var(--ink);}
  .foot{margin-top:30px;color:var(--ink3);font-size:12px;border-top:1px solid var(--border);padding-top:14px;}
</style>
</head>
<body>
<div class="wrap">
  <div class="top">
    <div>
      <a class="back" href="/">← Back to Kaimi</a>
      <h1>Your capability map</h1>
    </div>
  </div>
  <p class="sub">How Kaimi understands your business — synthesized from your company profile and the context documents you uploaded. Kaimi uses this to qualify and score opportunities and to ground proposal drafting.</p>

  {{if .Unavailable}}
  <div class="note">The capability map is not available in this deployment.</div>
  {{else if .NotBuilt}}
  <div class="note">Not built yet. Complete onboarding — save your company profile and upload a capability statement or past performance — and Kaimi will build your capability map automatically. <a href="/onboarding">Go to onboarding →</a></div>
  {{else}}
  {{with .Map}}
  {{if .Summary}}<div class="summary">{{.Summary}}</div>{{end}}

  {{if .CoreCompetencies}}
  <h2>Core competencies</h2>
  {{range .CoreCompetencies}}
  <div class="comp">
    <h3>{{.Name}}</h3>
    {{if .Description}}<p>{{.Description}}</p>{{end}}
    {{if .Evidence}}<ul class="evidence">{{range .Evidence}}<li>{{.}}</li>{{end}}</ul>{{end}}
  </div>
  {{end}}
  {{end}}

  {{if .Differentiators}}<h2>Differentiators</h2><ul class="plain">{{range .Differentiators}}<li>{{.}}</li>{{end}}</ul>{{end}}

  {{if .Domains}}<h2>Mission domains</h2><div class="chips">{{range .Domains}}<span class="chip">{{.}}</span>{{end}}</div>{{end}}

  {{if .Certifications}}<h2>Certifications &amp; set-asides</h2><div class="chips">{{range .Certifications}}<span class="chip">{{.}}</span>{{end}}</div>{{end}}

  {{if .NAICS}}<h2>NAICS</h2><div class="chips">{{range .NAICS}}<span class="chip">{{.}}</span>{{end}}</div>{{end}}

  {{if .PastPerformance}}
  <h2>Past performance</h2>
  {{range .PastPerformance}}
  <div class="comp">
    <h3>{{if .Client}}{{.Client}}{{else}}Engagement{{end}}</h3>
    {{if .Scope}}<p>{{.Scope}}{{if .Value}} — {{.Value}}{{end}}</p>{{end}}
    {{if .Relevance}}<p>{{.Relevance}}</p>{{end}}
  </div>
  {{end}}
  {{end}}

  {{if .Keywords}}<h2>Matching vocabulary</h2><div class="chips">{{range .Keywords}}<span class="chip">{{.}}</span>{{end}}</div>{{end}}

  {{if .Sources}}<h2>Sources</h2><div class="chips">{{range .Sources}}<span class="chip">{{.}}</span>{{end}}</div>{{end}}
  {{end}}
  <div class="foot">Built by {{$.Map.Model}}{{if $.GeneratedStr}} · {{$.GeneratedStr}}{{end}}. Kaimi rebuilds it when you update your profile or upload documents.</div>
  {{end}}
</div>
</body>
</html>`

/* ============================================================
   KAIMI Desktop — bridge to the Go backend.
   Calls the Wails-bound App.ListOpportunities() and maps the
   dashboard rows into the design's opportunity shape. Falls back
   to the bundled demo queue when no local store data is present
   (or when running in a plain browser without Wails bindings).
   ============================================================ */
import { KAIMI_OPPS } from './data.js';

// recommendation proxy from fit band, until the Recommendation field
// is surfaced on the dashboard row view-model.
function recFromFit(fit){ return fit >= 70 ? "bid" : fit >= 40 ? "review" : "nobid"; }

// deadline label + escalation level from an ISO date string.
function deadlineInfo(iso){
  if(!iso) return { label:"—", level:"calm" };
  const d = new Date(iso);
  if(isNaN(d) || d.getFullYear() < 2) return { label:"—", level:"calm" };
  const days = Math.round((d - new Date()) / 86400000);
  if(days < 0) return { label:"closed", level:"crit" };
  const label = days === 0 ? "today" : days === 1 ? "1 day" : `${days} days`;
  const level = days < 7 ? "crit" : days < 14 ? "near" : days <= 30 ? "soon" : "calm";
  return { label, level };
}

export async function getOpportunities(){
  try {
    const App = window.go && window.go.main && window.go.main.App;
    if(!App || typeof App.ListOpportunities !== "function") return KAIMI_OPPS;
    const res = await App.ListOpportunities();
    if(!res || res.empty || !Array.isArray(res.rows) || res.rows.length === 0) return KAIMI_OPPS;
    return res.rows.map((r, i) => {
      const fit = Math.round((r.Score || 0) * 100);
      const dl = deadlineInfo(r.ResponseDeadline);
      return {
        id: r.ID || ("o" + i),
        title: r.Title || "(untitled opportunity)",
        agency: r.Agency || "",
        naics: r.NAICSCode || "",
        sol: "",
        fit,
        rec: recFromFit(fit),
        deadlineLabel: dl.label,
        deadlineLevel: r.DeadlineSoon ? "crit" : dl.level,
        isNew: true,
        day: "today",
        value: "",
      };
    });
  } catch (e) {
    return KAIMI_OPPS;
  }
}

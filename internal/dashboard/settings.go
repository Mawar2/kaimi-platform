package dashboard

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/Mawar2/Kaimi/internal/profile"
)

// SettingsData backs the Settings page. It carries the shell shared data, the CSRF
// token for the form, the flat company-profile form fields (pre-filled from the saved
// profile or the just-submitted values on a re-render), a validation error to show on
// a failed save, and the "saved" flag that drives the success banner after a PRG.
type SettingsData struct {
	shellData
	CSRFToken string

	// Company-profile form values. These mirror OnboardingData's profile fields so the
	// SAME fillFormFromProfile helper and parseProfileForm parser work unchanged.
	Company         string
	UEI             string
	CAGE            string
	NAICS           string // newline-separated "code|description|tier" lines
	Competencies    string // newline-separated
	PastPerformance string // newline-separated "client|scope|value" lines
	SetAside        profile.SetAsideStatus
	PrimaryNAICS    string // comma/newline-separated
	SecondaryNAICS  string
	CompetencyTags  string
	ScoringPP       string

	// FormErr is a validation error to render above the form (empty on success).
	FormErr string
	// Saved drives the success banner after the post/redirect/get round-trip.
	Saved bool
}

// newSettingsData builds the base view-model: the app shell, sidebar counts, and the
// CSRF token from the signed-in identity (empty when no session, in which case the
// form posts without a token and the mutation gate fails closed).
func (h *Handler) newSettingsData(r *http.Request) SettingsData {
	d := SettingsData{shellData: shellData{PageTitle: "Settings", ActiveNav: "settings"}}
	if ident, ok := h.resolveIdentity(r); ok {
		d.CSRFToken = ident.CSRFToken
	}
	h.fillShellCounts(r.Context(), &d.shellData)
	return d
}

// handleSettings serves GET /settings: the company-profile editor, pre-filled with the
// current profile. The onboarding gate guarantees a profile exists before this page is
// reachable, so Load is expected to succeed; a Load error (or no store) is handled
// gracefully by sending the operator back to onboarding rather than rendering a blank
// or erroring form.
func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	if h.profileStore == nil {
		http.Error(w, "settings are not available in this deployment", http.StatusServiceUnavailable)
		return
	}

	p, err := h.profileStore.Load()
	if err != nil {
		// No profile yet (or a transient read error): the operator belongs in onboarding,
		// where the profile is created. Redirect rather than render an empty editor.
		if !errors.Is(err, profile.ErrProfileNotFound) {
			log.Printf("dashboard: settings profile load failed: %v", err)
		}
		http.Redirect(w, r, onboardingPath, http.StatusSeeOther)
		return
	}

	d := h.newSettingsData(r)
	fillFormFromSettings(&d, p)
	// The success banner appears only right after a save (the ?saved=1 PRG redirect),
	// never on a normal page load.
	d.Saved = r.URL.Query().Get("saved") == "1"
	h.renderSettings(w, &d)
}

// handleSettingsSave serves POST /settings. It mirrors handleOnboardingProfile exactly:
// it FAILS CLOSED on auth + CSRF (a state-mutating endpoint re-checks identity here, not
// just upstream), parses the form with the SHARED parseProfileForm, validates with the
// SHARED profile.Validate, and persists via the SAME ProfileStore the pipeline and Writer
// read at runtime. On a validation failure it re-renders the form with the submitted
// values and the error (HTTP 400) and persists nothing; on success it refreshes the
// capability map (best-effort) and follows the PRG pattern, redirecting to
// /settings?saved=1 so a refresh does not re-POST.
func (h *Handler) handleSettingsSave(w http.ResponseWriter, r *http.Request) {
	if h.profileStore == nil {
		http.Error(w, "settings are not available in this deployment", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	// Fail-closed auth + CSRF gate. NOTHING is mutated until this passes.
	if !h.authorizeMutation(w, r) {
		return
	}

	p := parseProfileForm(r)

	if err := profile.Validate(p); err != nil {
		// Re-render with the submitted values and the error. Persist nothing.
		d := h.newSettingsData(r)
		fillFormFromSettings(&d, p)
		d.FormErr = err.Error()
		w.WriteHeader(http.StatusBadRequest)
		h.renderSettings(w, &d)
		return
	}

	if err := h.profileStore.Save(p); err != nil {
		log.Printf("dashboard: settings profile save failed: %v", err)
		http.Error(w, "failed to save profile", http.StatusInternalServerError)
		return
	}

	// The profile changed, so refresh the capability map (best-effort; never fails the save).
	h.triggerMapRebuild()

	// PRG: redirect so a refresh does not re-POST; the GET shows the success banner.
	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}

func (h *Handler) renderSettings(w http.ResponseWriter, d *SettingsData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.settingsTmpl.Execute(w, d); err != nil {
		log.Printf("dashboard: settings template execute: %v", err)
	}
}

// fillFormFromSettings copies a CapabilityProfile into the Settings view-model's flat
// form fields. It is the SettingsData analogue of fillFormFromProfile (which targets
// OnboardingData); both round-trip the same fields through the same format helpers so
// the two editors stay byte-for-byte consistent.
func fillFormFromSettings(d *SettingsData, p *profile.CapabilityProfile) {
	d.Company = p.Company
	d.UEI = p.UEI
	d.CAGE = p.CAGE
	d.SetAside = p.SetAside
	d.NAICS = formatNAICSLines(p.NAICSCodes)
	d.Competencies = strings.Join(p.Competencies, "\n")
	d.PastPerformance = formatPastPerformanceLines(p.PastPerformance)
	d.PrimaryNAICS = strings.Join(p.Scoring.PrimaryNAICS, ", ")
	d.SecondaryNAICS = strings.Join(p.Scoring.SecondaryNAICS, ", ")
	d.CompetencyTags = strings.Join(p.Scoring.CompetencyTags, "\n")
	d.ScoringPP = strings.Join(p.Scoring.PastPerformance, "\n")
}

// settingsContentTmpl is the Settings page, rendered inside the (light-themed) app shell.
// It posts the SAME field names the onboarding profile step does, so the shared
// parseProfileForm parses it unchanged. The NAICS picker mirrors the onboarding picker
// (typeahead over GET /api/v1/naics → chips synced into the hidden "naics" textarea),
// restyled for the light dashboard surface. Styles use the dashboard design tokens for
// cohesion with the Team page.
const settingsContentTmpl = `{{define "content"}}
<style>
.set-panel{ background:#fff; border:1px solid var(--line); border-radius:var(--r-lg); padding:24px 26px; max-width:720px; box-shadow:var(--e-1); }
.set-banner{ display:flex; align-items:center; gap:8px; background:#eafaf1; color:#0f7b46; border:1px solid #bfe9d2; border-radius:var(--r-md); padding:11px 14px; font-size:14px; margin-bottom:18px; max-width:720px; }
.set-banner svg{ width:18px; height:18px; flex:0 0 auto; }
.set-err{ background:#fdecec; color:#b42318; border:1px solid #f3c6c2; border-radius:var(--r-md); padding:11px 14px; font-size:14px; margin-bottom:18px; max-width:720px; }
.set-form label{ display:block; font:600 13px/1.3 var(--font-sans); color:var(--ink); margin-bottom:16px; }
.set-form .hint{ font-weight:400; color:var(--ink-soft); font-size:12px; }
.set-form input[type=text], .set-form textarea{ display:block; width:100%; margin-top:7px; border:1px solid var(--line); border-radius:var(--r-md); padding:10px 12px; font:inherit; font-size:14px; color:var(--ink); background:#fff; box-sizing:border-box; }
.set-form input[type=text]:focus, .set-form textarea:focus, .set-search:focus{ outline:none; border-color:var(--accent); box-shadow:0 0 0 3px color-mix(in oklab,var(--accent) 18%,transparent); }
.set-form textarea{ resize:vertical; min-height:64px; }
.set-row{ display:flex; gap:16px; flex-wrap:wrap; }
.set-row > label{ flex:1; min-width:200px; }
.set-form fieldset{ border:1px solid var(--line); border-radius:var(--r-md); padding:12px 14px; margin-bottom:16px; }
.set-form legend{ font:700 12px/1 var(--font-sans); color:var(--ink-soft); padding:0 6px; }
.set-chips{ display:flex; flex-wrap:wrap; gap:10px; }
.set-chk{ display:inline-flex; align-items:center; gap:7px; font:500 13px/1 var(--font-sans); color:var(--ink); margin:0; }
.set-btn{ background:var(--accent); color:#fff; border:0; border-radius:var(--r-md); padding:11px 18px; font:600 14px/1 var(--font-sans); cursor:pointer; display:inline-flex; align-items:center; gap:8px; }
.set-btn:hover{ filter:brightness(1.06); }
.set-btn svg{ width:16px; height:16px; }
/* NAICS picker (light theme). The hidden textarea is the canonical value carrier, kept
   in sync by the picker JS, so the existing server parser is unchanged. */
.naics-hidden{ display:block; }
body.naics-js .naics-hidden{ display:none; }
.naics-picker{ position:relative; }
.set-search{ display:block; width:100%; border:1px solid var(--line); border-radius:var(--r-md); padding:10px 12px; font:inherit; font-size:14px; color:var(--ink); background:#fff; box-sizing:border-box; }
.naics-results{ position:absolute; z-index:20; left:0; right:0; top:calc(100% + 4px); max-height:260px; overflow-y:auto; background:#fff; border:1px solid var(--line); border-radius:var(--r-md); box-shadow:0 8px 28px rgba(15,27,48,.16); }
.naics-result{ display:block; width:100%; text-align:left; padding:9px 12px; background:none; border:0; border-bottom:1px solid var(--line); color:var(--ink); font-size:13px; cursor:pointer; }
.naics-result:last-child{ border-bottom:0; }
.naics-result:hover{ background:color-mix(in oklab,var(--accent) 12%,transparent); }
.naics-result b{ color:var(--accent); margin-right:8px; font-variant-numeric:tabular-nums; }
.naics-chips{ display:flex; flex-wrap:wrap; gap:8px; margin-top:10px; }
.naics-chip{ display:inline-flex; align-items:center; gap:8px; padding:6px 8px 6px 10px; border:1px solid var(--line); border-radius:var(--r-md); background:var(--surface-2); font-size:13px; }
.naics-chip b{ color:var(--accent); font-variant-numeric:tabular-nums; }
.naics-chip-title{ color:var(--ink-soft); max-width:240px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.naics-chip-tier{ font-size:12px; padding:2px 4px; border-radius:6px; border:1px solid var(--line); }
.naics-chip-rm{ border:0; background:none; color:var(--ink-soft); font-size:16px; line-height:1; cursor:pointer; padding:0 2px; }
.naics-chip-rm:hover{ color:#b42318; }
</style>
<div class="page">
<div class="page-head">
  <div class="eyebrow">Account</div>
  <h1>Settings</h1>
  <p class="lead">Edit your company profile. Kaimi scores every opportunity against this profile, so your next hunt and your next drafts use whatever you save here.</p>
</div>
{{if .Saved}}<div class="set-banner">` + iconCheck + `Profile updated. Your next hunt and drafts will use these settings.</div>{{end}}
{{if .FormErr}}<div class="set-err">{{.FormErr}}</div>{{end}}
<div class="set-panel">
  <form class="set-form" method="POST" action="/settings">
    {{if .CSRFToken}}<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">{{end}}
    <label>Company name<input type="text" name="company" value="{{.Company}}" required></label>
    <div class="set-row">
      <label>UEI <span class="hint">(optional)</span><input type="text" name="uei" value="{{.UEI}}"></label>
      <label>CAGE <span class="hint">(optional)</span><input type="text" name="cage" value="{{.CAGE}}"></label>
    </div>
    <label>NAICS codes <span class="hint">· search and select your industry codes. This drives which solicitations Kaimi hunts.</span></label>
    <div class="naics-picker">
      <input type="text" id="naics-search" class="set-search" autocomplete="off" spellcheck="false" placeholder="Search by keyword or code, e.g. &#34;computer systems&#34; or 541512">
      <div id="naics-results" class="naics-results" hidden></div>
      <div id="naics-chips" class="naics-chips" aria-live="polite"></div>
    </div>
    <!-- Canonical value carrier (one "code|title|tier" per line), kept in sync by the
         picker JS so the existing server parser is unchanged. Hidden when JS runs. -->
    <textarea name="naics" id="naics-input" class="naics-hidden" aria-hidden="true" tabindex="-1" placeholder="541512|Computer Systems Design Services|primary">{{.NAICS}}</textarea>
    <fieldset><legend>Set-aside eligibility</legend><div class="set-chips">
      <label class="set-chk"><input type="checkbox" name="sa_small_business"{{if .SetAside.SmallBusiness}} checked{{end}}> Small business</label>
      <label class="set-chk"><input type="checkbox" name="sa_sdb"{{if .SetAside.SDB}} checked{{end}}> SDB</label>
      <label class="set-chk"><input type="checkbox" name="sa_eight_a"{{if .SetAside.EightA}} checked{{end}}> 8(a)</label>
      <label class="set-chk"><input type="checkbox" name="sa_sdvosb"{{if .SetAside.SDVOSB}} checked{{end}}> SDVOSB</label>
      <label class="set-chk"><input type="checkbox" name="sa_wosb"{{if .SetAside.WOSB}} checked{{end}}> WOSB</label>
      <label class="set-chk"><input type="checkbox" name="sa_hubzone"{{if .SetAside.HUBZone}} checked{{end}}> HUBZone</label>
    </div></fieldset>
    <label>Capabilities statement <span class="hint">· one competency per line</span>
      <textarea name="competencies" rows="3" placeholder="Cloud migration &amp; DevSecOps">{{.Competencies}}</textarea></label>
    <label>Past performance <span class="hint">· one record per line as client|scope|value</span>
      <textarea name="past_performance" rows="3" placeholder="DHS|Network modernization|$4.2M">{{.PastPerformance}}</textarea></label>
    <fieldset><legend>Scoring signals (optional)</legend>
      <label>Primary NAICS <span class="hint">· comma or newline separated</span>
        <input type="text" name="primary_naics" value="{{.PrimaryNAICS}}"></label>
      <label>Secondary NAICS <span class="hint">· comma or newline separated</span>
        <input type="text" name="secondary_naics" value="{{.SecondaryNAICS}}"></label>
      <label>Competency tags <span class="hint">· one per line</span>
        <textarea name="competency_tags" rows="2">{{.CompetencyTags}}</textarea></label>
      <label style="margin-bottom:0">Past-performance highlights <span class="hint">· one sentence per line, used as scoring signals</span>
        <textarea name="scoring_pp" rows="2">{{.ScoringPP}}</textarea></label>
    </fieldset>
    <div><button class="set-btn" type="submit">` + iconCheck + `Save changes</button></div>
  </form>
</div>
</div>
<script>
// NAICS picker: typeahead over /api/v1/naics (official 2022 taxonomy) → chips with a tier
// selector, kept in sync with the hidden "naics" textarea (one "code|title|tier" per line)
// so the server parser is unchanged. Falls back to the plain textarea if JS is unavailable.
// This mirrors the onboarding profile-step picker; the markup ids and field name are
// identical so the two surfaces behave the same.
(function(){
  var input=document.getElementById("naics-input");
  var search=document.getElementById("naics-search");
  var results=document.getElementById("naics-results");
  var chipsEl=document.getElementById("naics-chips");
  if(!input||!search||!results||!chipsEl){return;}
  document.body.classList.add("naics-js");
  var tiers=["primary","secondary","tertiary"];
  var model=[];
  function esc(s){var d=document.createElement("div");d.textContent=(s==null?"":s);return d.innerHTML;}
  function sync(){input.value=model.map(function(m){return m.code+"|"+m.title+"|"+m.tier;}).join("\n");}
  function add(code,title){
    for(var i=0;i<model.length;i++){if(model[i].code===code){return;}}
    model.push({code:code,title:title,tier:"primary"});renderChips();sync();
  }
  function renderChips(){
    chipsEl.innerHTML="";
    model.forEach(function(m,i){
      var chip=document.createElement("span");chip.className="naics-chip";
      var b=document.createElement("b");b.textContent=m.code;
      var t=document.createElement("span");t.className="naics-chip-title";t.textContent=m.title;
      var sel=document.createElement("select");sel.className="naics-chip-tier";
      tiers.forEach(function(tr){var o=document.createElement("option");o.value=tr;o.textContent=tr;if(tr===m.tier){o.selected=true;}sel.appendChild(o);});
      sel.addEventListener("change",function(){model[i].tier=sel.value;sync();});
      var rm=document.createElement("button");rm.type="button";rm.className="naics-chip-rm";rm.setAttribute("aria-label","Remove "+m.code);rm.textContent="×";
      rm.addEventListener("click",function(){model.splice(i,1);renderChips();sync();});
      chip.appendChild(b);chip.appendChild(t);chip.appendChild(sel);chip.appendChild(rm);
      chipsEl.appendChild(chip);
    });
  }
  function hideResults(){results.hidden=true;results.innerHTML="";}
  function renderResults(items){
    results.innerHTML="";
    if(!items.length){hideResults();return;}
    items.forEach(function(it){
      var row=document.createElement("button");row.type="button";row.className="naics-result";
      row.innerHTML="<b>"+esc(it.code)+"</b> "+esc(it.title);
      row.addEventListener("click",function(){add(it.code,it.title);search.value="";hideResults();search.focus();});
      results.appendChild(row);
    });
    results.hidden=false;
  }
  var timer=null,last=0;
  search.addEventListener("input",function(){
    var q=search.value.trim();
    if(timer){clearTimeout(timer);}
    if(q.length<2){hideResults();return;}
    timer=setTimeout(function(){
      var id=++last;
      fetch("/api/v1/naics?q="+encodeURIComponent(q)+"&limit=12",{credentials:"same-origin",headers:{"Accept":"application/json"}})
        .then(function(r){return r.ok?r.json():{results:[]};})
        .then(function(d){if(id===last){renderResults((d&&d.results)||[]);}})
        .catch(function(){hideResults();});
    },180);
  });
  document.addEventListener("click",function(ev){if(!ev.target.closest||!ev.target.closest(".naics-picker")){hideResults();}});
  // Seed chips from any pre-saved value (the edit case: every visit to Settings).
  (input.value||"").split(/\r?\n/).forEach(function(line){
    line=line.trim();if(!line){return;}
    var p=line.split("|");var code=(p[0]||"").trim();if(!code){return;}
    model.push({code:code,title:(p[1]||"").trim(),tier:(p[2]||"primary").trim()});
  });
  renderChips();sync();
})();
</script>
{{end}}`

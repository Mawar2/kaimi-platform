package dashboard

import (
	"fmt"
	"html/template"
	"strings"
)

// Icons for the proposal surfaces (design handoff lifecycle-components.jsx).
const (
	iconHand    = `<svg viewBox="0 0 24 24" fill="none"><path d="M8 11V5.5a1.5 1.5 0 0 1 3 0V10m0-1V4.5a1.5 1.5 0 0 1 3 0V10m0-.5V6a1.5 1.5 0 0 1 3 0v7c0 3.5-2.2 7-6.5 7C10 20 8.4 18.6 7 16.5l-2.2-3.4a1.5 1.5 0 0 1 2.4-1.8L8 12" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>`
	iconSpinner = `<svg viewBox="0 0 24 24" fill="none"><path d="M12 3v3M12 18v3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M3 12h3M18 12h3M5.6 18.4l2.1-2.1M16.3 7.7l2.1-2.1" stroke="currentColor" stroke-width="2.1" stroke-linecap="round"/></svg>`
	iconDot     = `<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="4" fill="currentColor"/></svg>`
	iconArrow   = `<svg viewBox="0 0 24 24" fill="none"><path d="M5 12h14M13 6l6 6-6 6" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/></svg>`
	iconDoc     = `<svg viewBox="0 0 24 24" fill="none"><path d="M7 3h7l5 5v13H7z" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/><path d="M14 3v5h5" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"/></svg>`
	iconHandoff = `<svg width="26" height="16" viewBox="0 0 26 16" fill="none"><path d="M2 8h20M17 3l5 5-5 5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`
)

// miniPipe renders the proposal card's five-dot pipeline (design handoff
// MiniPipe): segments fill green up to the current stage; the current node
// carries the state color.
func miniPipe(stageIndex int, state string) template.HTML {
	nodeState := func(i int) string {
		switch {
		case i < stageIndex:
			return " done"
		case i == stageIndex:
			switch state {
			case "human":
				return " human"
			case "done", "submitted":
				return " done"
			default:
				return " active"
			}
		default:
			return ""
		}
	}
	var b strings.Builder
	b.WriteString(`<div class="minipipe">`)
	for i := 0; i < 5; i++ {
		if i > 0 {
			seg := ""
			if i <= stageIndex {
				seg = " done"
			}
			fmt.Fprintf(&b, `<div class="seg%s"></div>`, seg)
		}
		fmt.Fprintf(&b, `<div class="node%s"></div>`, nodeState(i))
	}
	b.WriteString(`</div>`)
	// #nosec G203 -- built from constants and validated state strings.
	return template.HTML(b.String())
}

// wPipe renders the Workspace's five-node stage pipeline (design handoff
// WPipe): node 2 always carries the hand icon, node 4 the arrow until done.
func wPipe(stageIndex int, state string) template.HTML {
	nodeState := func(i int) string {
		switch {
		case i < stageIndex:
			return "done"
		case i == stageIndex:
			switch state {
			case "human":
				return "human"
			case "done", "submitted":
				return "done"
			case "failed":
				return "pending"
			default:
				return "progress"
			}
		default:
			return "pending"
		}
	}
	icon := func(i int, st string) string {
		switch {
		case i == 2:
			return iconHand
		case i == 4:
			if st == "done" {
				return iconCheck
			}
			return iconArrow
		case st == "done":
			return iconCheck
		case st == "progress":
			return iconSpinner
		default:
			return `<span style="opacity:.5">` + iconDot + `</span>`
		}
	}
	caption := func(st string) string {
		switch st {
		case "human":
			return "Needs you"
		case "progress":
			return "Working"
		case "done":
			return "Done"
		default:
			return "Pending"
		}
	}
	var b strings.Builder
	b.WriteString(`<div class="wpipe">`)
	for i := 0; i < 5; i++ {
		if i > 0 {
			on := "idle"
			if i <= stageIndex {
				on = "done"
			}
			fmt.Fprintf(&b, `<div class="wconn" data-on=%q></div>`, on)
		}
		st := nodeState(i)
		fmt.Fprintf(&b,
			`<div class="wnode" data-st=%q><div class="wring">%s</div><div class="wname">%s</div><div class="wstate">%s</div></div>`,
			st, icon(i, st), stageNames[i], caption(st))
	}
	b.WriteString(`</div>`)
	// #nosec G203 -- built from constants and validated state strings.
	return template.HTML(b.String())
}

// propChip renders the proposal card's right-cluster status chip.
func propChip(state string) template.HTML {
	var out string
	switch state {
	case "human":
		out = `<span class="needs-tag">` + iconHand + `Needs you</span>`
	case "submitted":
		out = `<span class="kbadge kbadge--done"><span class="dot"></span>Submitted</span>`
	case "done":
		out = `<span class="kbadge kbadge--done"><span class="dot"></span>Ready</span>`
	case "failed":
		out = `<span class="kbadge kbadge--failed"><span class="dot"></span>Failed</span>`
	default:
		out = `<span class="pc-working"><span class="pulse"></span>Working</span>`
	}
	// #nosec G203 -- constant markup selected by a validated state string.
	return template.HTML(out)
}

// proposalsContentTmpl is the Proposals command view (design handoff
// "4. Proposals"): "across everything, what needs me?"
const proposalsContentTmpl = `{{define "content"}}
<div class="page">
  <div class="page-head">
    <div class="eyebrow">Focus</div>
    <h1>Active proposals</h1>
    <p class="lead">Every proposal the agents are working right now. Most importantly, the ones waiting on you.</p>
    <div class="stats">
      <div class="stat"><div class="v">{{.InFlight}}<small> in flight</small></div><div class="k">Proposals being worked</div></div>
      <div class="stat"><div class="v">{{.AgentCount}}<small> agents</small></div><div class="k">Working across proposals</div></div>
      <div class="stat"><div class="v{{if gt .NeedsCount 0}} amber{{end}}">{{.NeedsCount}}<small> need you</small></div><div class="k">Paused at a review gate</div></div>
    </div>
  </div>

  {{if .Empty}}
  <div class="empty2">
    <div class="g">` + iconProps + `</div>
    <h3>No active proposals</h3>
    <p>Select an opportunity from your queue to spin up the agent workflow.</p>
  </div>
  {{end}}

  {{range .Groups}}
  <div class="section-h"><span class="lbl{{if .Amber}} amber{{end}}">{{.Label}}</span><span class="cnt">{{len .Cards}}</span><span class="ln"></span></div>
  <div class="prop-grid">
    {{range .Cards}}
    <a class="pcard{{if eq .State "human"}} needs{{end}}" href="/workspace/{{.ID}}">
      <div class="pc-body">
        <div class="pc-ttl">{{.Title}}</div>
        <div class="pc-agency">{{.Agency}} · {{.When}}</div>
      </div>
      <div class="pc-pipe">
        {{miniPipe .StageIndex .State}}
        <div class="stage-label{{if eq .State "human"}} human{{end}}">{{.StageLabel}}</div>
      </div>
      <div class="pc-right">
        {{propChip .State}}
        {{if .DeadlineLabel}}{{if .CalmDeadline}}{{deadlinePill .DeadlineLabel 99}}{{else}}{{deadlinePill .DeadlineLabel .DeadlineDays}}{{end}}{{end}}
        <span class="chev" style="width:18px;height:18px;color:var(--ink-4)">` + iconChev + `</span>
      </div>
    </a>
    {{end}}
  </div>
  {{end}}
</div>
{{end}}
`

// workspaceContentTmpl is the single-proposal Workspace (design handoff
// "5. Workspace"): the big stage pipeline plus exactly one state block —
// and at the gate, the real section editor.
const workspaceContentTmpl = `{{define "content"}}
<div class="ws">
  <a class="back" href="/proposals">` + iconBack + `All proposals</a>

  {{if .Flash}}<div class="ws-flash">` + iconCheck + `<span>{{.Flash}}</span></div>{{end}}

  <div class="ws-head">
    {{if .ScorePct}}{{fitRing .ScorePct 64}}{{end}}
    <div class="ws-id">
      <h1>{{.Opp.Title}}</h1>
      <div class="ws-meta">
        <span>{{.Opp.Agency}}</span><span class="sep"></span>
        {{if .DeadlineLabel}}{{deadlinePill .DeadlineLabel .DeadlineDays}}<span class="sep"></span>{{end}}
        <span>{{.Phrase}}</span>
      </div>
    </div>
  </div>

  {{wPipe .StageIndex .State}}

  {{if .AtGate}}
  <div class="review">
    <div class="r-head">
      <span class="r-badge">` + iconHand + `Needs you</span>
      <div>
        <h2>Tomás is handing you the draft</h2>
        <p>Read and edit the working draft below. Your edits are exactly what Vera reviews.</p>
      </div>
      <div class="r-hand">
        <span class="av" style="background:{{.Agent.HueBG}};color:{{.Agent.HueFG}}">{{.Agent.Initial}}</span>
        <span class="arrow">` + iconHandoff + `</span>
        <span class="you">` + iconHand + `</span>
      </div>
    </div>
    <div class="r-body">
      {{if .Doc}}
      <div class="r-sec-h">What Tomás produced</div>
      <div class="summary">Drafted {{len .Doc.Sections}} sections into the working draft. Export it for review, sharing, or submission — or edit it inline below.</div>
      <div class="art-row">
        <a class="artifact2" href="/workspace/{{.Opp.ID}}/proposal.docx" download>` + iconDoc + `Word (.docx)<span style="color:var(--ink-4);font-size:11px">editable</span></a>
        <a class="artifact2" href="/workspace/{{.Opp.ID}}/proposal.pdf" download>` + iconDoc + `PDF<span style="color:var(--ink-4);font-size:11px">for submission</span></a>
        <a class="artifact2" href="/workspace/{{.Opp.ID}}/draft.md" download>` + iconDoc + `Markdown<span style="color:var(--ink-4);font-family:var(--font-mono);font-size:11px">{{.VersionLabel}}</span></a>
      </div>

      {{range .OpenFlags}}
      <div class="gapflag">
        <div class="gf-ic">` + iconWarn + `</div>
        <div>
          <div class="gf-t">{{.Title}}</div>
          <div class="gf-d">{{.Detail}}</div>
        </div>
      </div>
      {{end}}

      {{if .Criteria}}
      <div class="r-sec-h" style="margin-top:24px">Check against criteria</div>
      <div class="crit2">
        {{range .Criteria}}
        <div class="citem {{if .OK}}ok{{else}}warn{{end}}">
          <span class="ci-ic">{{if .OK}}` + iconCheck + `{{else}}` + iconWarn + `{{end}}</span>
          <div><div class="ci-l">{{.Label}}</div>{{if .Note}}<div class="ci-n">{{.Note}}</div>{{end}}</div>
        </div>
        {{end}}
      </div>
      {{end}}

      <div class="r-sec-h" style="margin-top:24px">Working draft: edit sections directly <span id="savechip" class="ed-save-chip">Saved</span> <a class="artifact2" href="/editor/{{.Opp.ID}}">` + iconDoc + `Open full editor</a></div>
      {{range .Doc.Sections}}
      <section class="edsec">
        <div class="sec-head2"><h3>{{.Heading}}</h3><span class="reqtag">{{.Status}}</span></div>
        <form method="POST" action="/workspace/{{$.Opp.ID}}/section/{{.ID}}" data-autosave>
          <textarea name="body" rows="7"{{if gapTexts .Body}} class="gap-warn"{{end}}>{{.Body}}</textarea>
          <noscript><button class="kbtn kbtn--secondary kbtn--sm" style="margin-top:6px">Save section</button></noscript>
        </form>
        {{range gapTexts .Body}}
        <div class="ed-flag ed-gap">
          <span class="ef-ic">` + iconWarn + `</span>
          <div><b>Unresolved gap</b><p>{{.}}</p></div>
          <button type="button" class="kbtn kbtn--ghost kbtn--sm gap-jump" data-gap="{{.}}">Find in text</button>
        </div>
        {{end}}
      </section>
      {{end}}
      {{else}}
      <div class="summary">The working draft is not available yet.</div>
      {{end}}
    </div>
    <div class="r-actions">
      <form method="POST" action="/workspace/{{.Opp.ID}}/approve">
        <button class="kbtn kbtn--approve kbtn--lg">` + iconCheck + `Approve &amp; resume</button>
      </form>
      <form method="POST" action="/workspace/{{.Opp.ID}}/changes" style="display:flex;gap:9px;align-items:center">
        <input type="text" name="note" placeholder="What should Tomás change?" style="height:40px;padding:0 13px;border:1px solid var(--border);border-radius:var(--r-md);font:var(--t-small);min-width:230px">
        <button class="kbtn kbtn--changes kbtn--lg">` + iconBack + `Request changes</button>
      </form>
      <div class="note">Approving runs Vera&#39;s final pass on your edited draft. Requesting changes sends it back to Tomás.</div>
    </div>
  </div>

  {{else if eq .State "progress"}}
  <div class="ws-state">
    <span class="kava kava--lg" style="background:{{.Agent.HueBG}};color:{{.Agent.HueFG}}" data-working="1">{{.Agent.Initial}}<span class="kava-spin" aria-hidden="true"></span></span>
    <div>
      <h3>{{.Agent.Name}} is working</h3>
      <div class="role">{{.Agent.Role}} agent · {{.Phrase}}</div>
      <div class="desc">{{.AgentLine}}</div>
    </div>
  </div>

  {{else if eq .State "done"}}
  <div class="ws-state" style="border-color:color-mix(in oklab,var(--st-done) 40%,transparent);background:linear-gradient(180deg,var(--st-done-bg),var(--surface) 60%)">
    <span class="ws-av" style="background:linear-gradient(155deg,#2BD49A,#15A06B)">` + iconCheck + `</span>
    <div style="flex:1">
      <h3>Package ready to submit</h3>
      <div class="desc" style="margin-top:8px">All stages complete. Vera&#39;s final pass validated the draft as you left it. Final human submission to SAM.gov.</div>
      <div style="margin-top:16px">
        <form method="POST" action="/workspace/{{.Opp.ID}}/submit">
          <button class="kbtn kbtn--select kbtn--lg">` + iconArrow + `Submit to SAM.gov</button>
        </form>
      </div>
    </div>
  </div>

  {{else if eq .State "submitted"}}
  <div class="ws-state" style="border-color:color-mix(in oklab,var(--st-done) 40%,transparent)">
    <span class="ws-av" style="background:linear-gradient(155deg,#2BD49A,#15A06B)">` + iconCheck + `</span>
    <div>
      <h3>Submitted to SAM.gov</h3>
      <div class="role">Confirmation logged · the agents stand down on this one.</div>
      <div class="desc">Kaimi will watch for amendments and Q&amp;A updates on this solicitation and let you know if anything needs attention.</div>
    </div>
  </div>

  {{else}}
  <div class="ws-state" style="border-color:color-mix(in oklab,var(--st-failed) 40%,transparent)">
    <span class="ws-av" style="background:var(--st-failed-bg);color:var(--st-failed)">` + iconWarn + `</span>
    <div>
      <h3>{{.Phrase}}</h3>
      <div class="desc">The stage stopped with an error. Check the server logs, then re-select or request changes to retry.</div>
    </div>
  </div>
  {{end}}

  {{if and .Doc (not .AtGate)}}
  <div class="r-sec-h" style="margin-top:30px">
    Working draft: {{if eq .State "progress"}}review it while the agents work{{else}}final content{{end}}
    <span class="ed-save-chip">{{.VersionLabel}}</span>
  </div>
  {{range .Doc.Sections}}
  <section class="edsec">
    <div class="sec-head2"><h3>{{.Heading}}</h3><span class="reqtag">{{.Status}}</span></div>
    {{if .Body}}<div class="draft-body">{{highlightGaps .Body}}</div>{{else}}<div class="draft-pending">Tomás is drafting this section…</div>{{end}}
  </section>
  {{end}}
  {{end}}
</div>

<style>
  .ws-flash { display: flex; align-items: center; gap: 8px; margin: 12px 0 0; padding: 10px 14px; border-radius: var(--r-md); background: var(--st-done-bg); color: var(--st-done); border: 1px solid color-mix(in oklab, var(--st-done) 35%, transparent); font: var(--t-small); }
  .ws-flash svg { width: 18px; height: 18px; flex: none; }
  .edsec { margin-top: 14px; }
  .edsec textarea { width: 100%; min-height: 120px; font: var(--t-body); color: var(--ink); background: var(--surface); border: 1px solid var(--border); border-radius: var(--r-md); padding: 12px 14px; resize: vertical; box-sizing: border-box; }
  .edsec textarea:focus { outline: none; box-shadow: 0 0 0 3px var(--ring-focus); border-color: var(--blue-300); }
  .sec-head2 { display: flex; align-items: baseline; gap: 9px; margin: 0 0 7px; }
  .sec-head2 h3 { font: 650 15px/1.3 var(--font-sans); }
  .sec-head2 .reqtag { font: 500 11px/1 var(--font-mono); color: var(--ink-4); }
  .ed-save-chip { font: 600 11px/1 var(--font-sans); color: var(--st-done); margin-left: 8px; text-transform: none; letter-spacing: 0; }
  .ed-save-chip.saving { color: var(--ink-3); }
  .r-actions form { margin: 0; }
  .draft-body { white-space: pre-wrap; background: var(--surface); border: 1px solid var(--border); border-radius: var(--r-md); padding: 12px 14px; font: var(--t-body); color: var(--ink); }
  .draft-pending { border: 1px dashed var(--border); border-radius: var(--r-md); padding: 12px 14px; font: var(--t-small); color: var(--ink-3); font-style: italic; }
</style>
<script>
  // Autosave: debounce section edits into background POSTs so the draft.md
  // mirror stays current while the human writes (ux-spec: the workspace
  // editor is the one JS-enabled surface).
  document.querySelectorAll("form[data-autosave]").forEach(function (f) {
    var area = f.querySelector("textarea");
    var chip = document.getElementById("savechip");
    var timer;
    if (!area) return;
    area.addEventListener("input", function () {
      if (chip) { chip.textContent = "Saving…"; chip.classList.add("saving"); }
      clearTimeout(timer);
      timer = setTimeout(function () {
        fetch(f.action, {
          method: "POST",
          headers: { "Content-Type": "application/x-www-form-urlencoded" },
          body: new URLSearchParams(new FormData(f)).toString(),
          redirect: "manual",
        }).then(function (resp) {
          if (chip) {
            chip.textContent = resp.type === "opaqueredirect" || resp.ok ? "Saved" : "Save failed";
            chip.classList.remove("saving");
          }
        }).catch(function () {
          if (chip) { chip.textContent = "Save failed"; chip.classList.remove("saving"); }
        });
      }, 900);
    });
  });
  // Jump-to-gap: select the [GAP: ...] marker inside the section's textarea so
  // the browser scrolls to it and the human sees exactly what is missing.
  document.querySelectorAll(".gap-jump").forEach(function (b) {
    b.addEventListener("click", function () {
      var area = b.closest("section").querySelector("textarea");
      if (!area) return;
      var idx = area.value.indexOf(b.getAttribute("data-gap"));
      var start = idx < 0 ? area.value.indexOf("[GAP:") : area.value.lastIndexOf("[GAP:", idx);
      if (start < 0) return;
      var end = area.value.indexOf("]", start);
      area.focus();
      area.setSelectionRange(start, end < 0 ? area.value.length : end + 1);
      area.scrollIntoView({ behavior: "smooth", block: "center" });
    });
  });
</script>
{{end}}
`

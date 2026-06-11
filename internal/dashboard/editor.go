package dashboard

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/finalreview"
)

// This file implements the full-page draft editor (the new Kaimi App.html
// "editor" route, desktop-editor.jsx + editor.css): a focused, full-bleed
// writing surface — a section rail on the left and the working draft on the
// right, with inline gap callouts. It is a standalone page (no app shell) and
// reuses the proposal Document plus the workspace's section autosave endpoint,
// so editing the draft here is identical to editing it at the review gate.

// EditorData is the view-model for GET /editor/{id}.
type EditorData struct {
	Title        string // page title
	OppID        string
	OppTitle     string
	Meta         string // "DHS · CISA · SOL# … · click any paragraph to edit"
	VersionLabel string
	Sections     []EditorSection
}

// EditorSection is one editable section plus any gap flag attached to it.
type EditorSection struct {
	ID      string
	Heading string
	Status  string
	Body    string
	Flag    *document.Flag // non-nil when the section carries an open review flag
	Gaps    []string       // missing-fact text of each Writer [GAP: ...] marker in Body
}

// gapFlagTitle is the Title the proposal service puts on section-anchored
// unresolved-gap flags (see proposal.flagsFromResult). The editor identifies
// gap flags by it so their callouts can derive from the live body instead.
const gapFlagTitle = "Unresolved gap"

// highlightGaps renders a section body for the read-only draft view with every
// Writer [GAP: ...] marker wrapped in <mark class="gap-mark">. The body is
// HTML-escaped first; escaping never alters the marker's "[GAP:" / "]"
// delimiters, so the boundaries survive.
func highlightGaps(body string) template.HTML {
	escaped := template.HTMLEscapeString(body)
	var b strings.Builder
	for {
		before, after, found := strings.Cut(escaped, "[GAP:")
		if !found {
			b.WriteString(escaped)
			break
		}
		gapText, rest, closed := strings.Cut(after, "]")
		if !closed {
			gapText, rest = after, ""
		}
		b.WriteString(before)
		b.WriteString(`<mark class="gap-mark">[GAP:`)
		b.WriteString(gapText)
		if closed {
			b.WriteString("]")
		}
		b.WriteString(`</mark>`)
		escaped = rest
	}
	return template.HTML(b.String()) //nolint:gosec // input is escaped above; only our own <mark> wrapper is added
}

// handleEditor renders the full-page draft editor for a selected proposal.
func (h *Handler) handleEditor(w http.ResponseWriter, r *http.Request) {
	if h.proposals == nil {
		http.Error(w, "the editor is not enabled on this server", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if !opportunityIDPattern.MatchString(id) {
		h.renderNotFound(w, id)
		return
	}
	opp, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.renderNotFound(w, id)
		return
	}
	doc, err := h.proposals.Document(id)
	if err != nil || doc == nil {
		h.renderNotFound(w, id)
		return
	}

	// Index open flags by the section they belong to. Unresolved-gap flags are
	// skipped: the gap callouts derive from the live section body instead, so
	// they appear before Vera has run and disappear the moment the human fills
	// the gap — a persisted flag would lag both ways.
	flagBySection := map[string]*document.Flag{}
	for i := range doc.Flags {
		f := &doc.Flags[i]
		if f.Resolved || f.SectionID == "" || f.Title == gapFlagTitle {
			continue
		}
		if _, seen := flagBySection[f.SectionID]; !seen {
			flagBySection[f.SectionID] = f
		}
	}

	data := EditorData{
		Title:        "Editor — " + opp.Title,
		OppID:        id,
		OppTitle:     opp.Title,
		VersionLabel: versionLabel(doc),
	}
	meta := opp.Agency
	if opp.SolicitationNum != "" {
		meta += " · SOL# " + opp.SolicitationNum
	}
	data.Meta = meta + " · click any section to edit"
	for i := range doc.Sections {
		s := &doc.Sections[i]
		data.Sections = append(data.Sections, EditorSection{
			ID: s.ID, Heading: s.Heading, Status: s.Status, Body: s.Body,
			Flag: flagBySection[s.ID],
			Gaps: finalreview.GapTexts(s.Body),
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.editorTmpl.Execute(w, data); err != nil {
		fmt.Printf("editor template failed: %v\n", err)
	}
}

// editorPageTmpl is the standalone full-page editor — it deliberately does NOT
// use the app shell (no sidebar); the design's "editor" route is a focused
// full-bleed surface. Section edits autosave through the shared workspace
// endpoint, so the draft stays identical to the review-gate view.
const editorPageTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Kaimi — {{.Title}}</title>
  {{faviconLink}}
  {{styleTag}}
</head>
<body>
<div class="ed-fullpage route-fade">
  <div class="ed">
    <div class="ed-rail">
      <div class="er-h">Sections</div>
      {{range .Sections}}
      <a class="ed-sec{{if or .Flag .Gaps}} warn{{end}}" href="#sec-{{.ID}}"><span class="dot"></span><b>{{.Heading}}</b></a>
      {{end}}
    </div>
    <div class="ed-main">
      <div class="ed-top">
        <a class="kbtn kbtn--ghost kbtn--sm" href="/workspace/{{.OppID}}" style="text-decoration:none">` + iconBack + `Back to review</a>
        <div class="ed-name">Working draft<span>{{.OppTitle}}</span></div>
        <span class="ed-ver you">{{.VersionLabel}}</span>
        <span id="edsave" class="ed-save">Saved</span>
      </div>
      <div class="ed-scroll">
        <div class="ed-doc">
          <div class="ed-title">{{.OppTitle}} — Technical Volume</div>
          <div class="ed-meta">{{.Meta}}</div>
          {{range .Sections}}
          <section id="sec-{{.ID}}" class="edsec">
            <div class="sec-head2"><h3>{{.Heading}}</h3>{{if .Status}}<span class="reqtag">{{.Status}}</span>{{end}}</div>
            <form method="POST" action="/workspace/{{$.OppID}}/section/{{.ID}}" data-autosave>
              <textarea name="body" rows="8"{{if .Gaps}} class="gap-warn"{{end}}>{{.Body}}</textarea>
              <noscript><button class="kbtn kbtn--secondary kbtn--sm" style="margin-top:6px">Save section</button></noscript>
            </form>
            {{range .Gaps}}
            <div class="ed-flag ed-gap">
              <span class="ef-ic">` + iconWarn + `</span>
              <div><b>Unresolved gap</b><p>{{.}}</p></div>
              <button type="button" class="kbtn kbtn--ghost kbtn--sm gap-jump" data-gap="{{.}}">Find in text</button>
            </div>
            {{end}}
            {{if .Flag}}
            <div class="ed-flag">
              <span class="ef-ic">` + iconWarn + `</span>
              <div><b>{{.Flag.Title}}</b><p>{{.Flag.Detail}}</p></div>
            </div>
            {{end}}
          </section>
          {{end}}
        </div>
      </div>
    </div>
  </div>
</div>
<style>
  .ed-rail .ed-sec{ text-decoration:none; color:inherit; }
  .ed-main .ed-top .kbtn{ color:inherit; }
  .edsec textarea{ width:100%; min-height:120px; font:var(--t-body); color:var(--ink); background:var(--surface); border:1px solid var(--border); border-radius:var(--r-md); padding:12px 14px; resize:vertical; box-sizing:border-box; }
  .edsec textarea:focus{ outline:none; box-shadow:0 0 0 3px var(--ring-focus); border-color:var(--blue-300); }
  .sec-head2{ display:flex; align-items:baseline; gap:9px; margin:0 0 7px; }
  .sec-head2 h3{ font:650 15px/1.3 var(--font-sans); }
  .sec-head2 .reqtag{ font:500 11px/1 var(--font-mono); color:var(--ink-4); }
</style>
<script>
  // Debounced autosave — posts each edited section to the shared workspace
  // endpoint so the draft.md mirror stays current (the one JS-enabled surface).
  var chip = document.getElementById("edsave");
  document.querySelectorAll("form[data-autosave]").forEach(function (f) {
    var area = f.querySelector("textarea"); var timer;
    if (!area) return;
    area.addEventListener("input", function () {
      if (chip) { chip.textContent = "Saving…"; chip.classList.add("saving"); }
      clearTimeout(timer);
      timer = setTimeout(function () {
        fetch(f.action, { method:"POST", headers:{"Content-Type":"application/x-www-form-urlencoded"},
          body: new URLSearchParams(new FormData(f)).toString(), redirect:"manual" })
          .then(function (resp) { if (chip) { chip.textContent = (resp.type==="opaqueredirect"||resp.ok) ? "Saved" : "Save failed"; chip.classList.remove("saving"); } })
          .catch(function () { if (chip) { chip.textContent = "Save failed"; chip.classList.remove("saving"); } });
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
</body>
</html>
`

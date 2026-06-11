package dashboard

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"

	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/zone2view"
)

// This file implements the Zone 2 web surfaces (GitHub issue #156, epic
// #153): the Proposals command view, the Workspace with the human review
// gate and section editor, and the human-action endpoints. All lifecycle
// behavior lives in internal/proposal; these handlers translate HTTP to
// service calls and render the designed markup (app-proposals.jsx /
// app-workspace.jsx from the design handoff).

// Stage names of the five-node proposal pipeline, per the design handoff.
// stageNames aliases the shared Zone 2 pipeline vocabulary (internal/zone2view),
// the single source of truth shared with the desktop app.
var stageNames = zone2view.StageNames

// agentIdentity is the named-teammate vocabulary from the design handoff.
// Status copy uses their names — the gate is a warm handoff, not an alarm.
type agentIdentity struct {
	Name, Role, Initial string
	// HueBG is the avatar's background — a linear-gradient. It is typed
	// template.CSS (not string) so html/template interpolates it verbatim in a
	// style attribute instead of sanitizing the gradient to ZgotmplZ. The values
	// are static map constants below, never user input, so this is safe.
	HueBG template.CSS
	HueFG string
}

var agents = map[string]agentIdentity{
	"outline": {"Noa", "Outline", "N", "linear-gradient(155deg,#5B9BFF,#2563EB)", "#fff"},
	"writer":  {"Tomás", "Technical Writer", "T", "linear-gradient(155deg,#67E0F4,#0EA5C4)", "#062a33"},
	"review":  {"Vera", "Final Review", "V", "linear-gradient(155deg,#A99BFF,#7C6BF5)", "#fff"},
}

// workingAgent maps a pipeline stage to the teammate working it.
func workingAgent(stageIndex int) agentIdentity {
	switch stageIndex {
	case 0:
		return agents["outline"]
	case 3:
		return agents["review"]
	default:
		return agents["writer"]
	}
}

// PropCard is the view-model for one proposal card on the command view.
type PropCard struct {
	ID            string
	Title         string
	Agency        string
	When          string
	StageIndex    int
	State         string // human | progress | done | submitted | failed
	StageLabel    string
	DeadlineLabel string
	DeadlineDays  int
	CalmDeadline  bool // submitted cards show a calm pill regardless of level
}

// PropGroup is one section of the command view, in fixed display order.
type PropGroup struct {
	Label string
	Amber bool
	Cards []PropCard
}

// ProposalsData is the view-model for the Proposals command view.
type ProposalsData struct {
	shellData
	InFlight   int
	AgentCount int
	Groups     []PropGroup
	Empty      bool
}

// WorkspaceData is the view-model for the single-proposal Workspace.
type WorkspaceData struct {
	shellData
	Opp           *opportunity.Opportunity
	Doc           *document.Document
	ScorePct      int
	StageIndex    int
	State         string
	Phrase        string
	DeadlineLabel string
	DeadlineDays  int
	Agent         agentIdentity
	AgentLine     string
	Criteria      []zone2view.Criterion
	OpenFlags     []document.Flag
	VersionLabel  string
	AtGate        bool
	// Flash is a one-shot confirmation banner shown after a gate action
	// (issue #246 B4), derived from the ?flash= redirect marker.
	Flash string
}

// gateFlashMessage maps a ?flash= redirect marker to the confirmation banner the
// workspace shows after a gate action, so Request changes / Approve / Submit
// never read as "nothing happened".
func gateFlashMessage(flash string) string {
	switch flash {
	case "changes":
		return "Sent back to Tomás — he will revise the draft and return it to you."
	case "approve":
		return "Approved — Vera is running the final review on your draft."
	case "submit":
		return "Submitted to SAM.gov."
	default:
		return ""
	}
}

// agentLines are the working-state description sentences from the handoff.
var agentLines = map[string]string{
	"outline": "Mapping the solicitation into a section plan and matching each requirement to an evaluation factor.",
	"writer":  "Drafting the technical volume from the outline — section by section, grounded only in the facts on file.",
	"review":  "Running the final compliance and consistency pass. Validating every requirement and cross-reference.",
}

func (h *Handler) handleSelect(w http.ResponseWriter, r *http.Request) {
	if h.proposals == nil {
		http.Error(w, "proposal actions are not enabled on this server", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if !opportunityIDPattern.MatchString(id) {
		h.renderNotFound(w, id)
		return
	}
	if _, err := h.svc.Get(r.Context(), id); err != nil {
		h.renderNotFound(w, id)
		return
	}
	if err := h.proposals.Select(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/workspace/"+id, http.StatusSeeOther)
}

func (h *Handler) handleProposals(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.List(r.Context(), ListOptions{Now: h.Now()})
	if err != nil {
		fmt.Printf("proposals list failed: %v\n", err)
		http.Error(w, "failed to load proposals", http.StatusInternalServerError)
		return
	}

	data := ProposalsData{shellData: shellData{PageTitle: "Active proposals", ActiveNav: "proposals"}}
	data.QueueCount = len(rows)
	groups := map[string]*PropGroup{
		"human":     {Label: "Waiting on you", Amber: true},
		"progress":  {Label: "Agents working"},
		"done":      {Label: "Ready to submit"},
		"submitted": {Label: "Submitted"},
		"failed":    {Label: "Needs attention"},
	}
	now := h.Now()
	for i := range rows {
		row := &rows[i]
		if row.Stage == StageHunted || row.Stage == StageScored {
			continue
		}
		// Derive the card state from the SAME raw status the workspace uses, via
		// zone2view, so the two views can never disagree (issue #246 B2).
		stageIndex, state := zone2view.View(row.ProposalStatus)
		card := PropCard{
			ID:         row.ID,
			Title:      row.Title,
			Agency:     row.Agency,
			When:       zone2view.StatusPhrase(stageIndex, state),
			StageIndex: stageIndex,
			State:      state,
			StageLabel: stageLabelFor(stageIndex, state),
		}
		if !row.ResponseDeadline.IsZero() {
			card.DeadlineLabel, card.DeadlineDays = deadlineDisplay(row.ResponseDeadline, now)
			card.CalmDeadline = state == "submitted"
		}
		g := groups[state]
		if g == nil {
			g = groups["progress"]
		}
		g.Cards = append(g.Cards, card)
		switch state {
		case "human":
			data.NeedsCount++
			data.InFlight++
			data.AgentCount++ // the gate holds one handoff
		case "submitted":
			data.SubmittedCount++
		default:
			data.InFlight++
			data.AgentCount++
		}
		data.ActiveCount++
	}
	for _, key := range []string{"human", "progress", "done", "submitted", "failed"} {
		if len(groups[key].Cards) > 0 {
			data.Groups = append(data.Groups, *groups[key])
		}
	}
	data.Empty = len(data.Groups) == 0

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.proposalsTmpl.Execute(w, data); err != nil {
		fmt.Printf("proposals template execution failed: %v\n", err)
	}
}

// stageLabelFor renders the mini-pipeline caption.
func stageLabelFor(stageIndex int, state string) string {
	switch state {
	case "human":
		return "Human Review"
	case "submitted":
		return "Submitted to SAM.gov"
	case "done":
		return "Ready to submit"
	case "failed":
		return stageNames[stageIndex] + " failed"
	default:
		return stageNames[stageIndex]
	}
}

func (h *Handler) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !opportunityIDPattern.MatchString(id) {
		h.renderNotFound(w, id)
		return
	}
	opp, err := h.svc.Get(r.Context(), id)
	if err != nil || !opp.Selected {
		h.renderNotFound(w, id)
		return
	}

	now := h.Now()
	stageIndex, state := zone2view.View(opp.ProposalStatus)
	data := WorkspaceData{
		shellData:  shellData{PageTitle: opp.Title, ActiveNav: "proposals"},
		Opp:        opp,
		StageIndex: stageIndex,
		State:      state,
		Phrase:     zone2view.StatusPhrase(stageIndex, state),
		Agent:      workingAgent(stageIndex),
		AtGate:     state == "human",
	}
	data.AgentLine = agentLines[agentKeyFor(stageIndex)]
	data.Flash = gateFlashMessage(r.URL.Query().Get("flash"))
	// The sidebar shows the same queue/needs/active counts here as on every
	// other page (issue #246 B1).
	h.fillShellCounts(r.Context(), &data.shellData)
	if opp.Score > 0 {
		data.ScorePct = int(0.5 + opp.Score*100)
	}
	if !opp.ResponseDeadline.IsZero() {
		data.DeadlineLabel, data.DeadlineDays = deadlineDisplay(opp.ResponseDeadline, now)
	}

	if h.proposals != nil {
		if doc, err := h.proposals.Document(id); err == nil {
			data.Doc = doc
			data.VersionLabel = versionLabel(doc)
			data.Criteria = deriveCriteria(opp, doc)
			for _, f := range doc.Flags {
				if !f.Resolved {
					data.OpenFlags = append(data.OpenFlags, f)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.workspaceTmpl.Execute(w, data); err != nil {
		fmt.Printf("workspace template execution failed: %v\n", err)
	}
}

func agentKeyFor(stageIndex int) string {
	switch stageIndex {
	case 0:
		return "outline"
	case 3:
		return "review"
	default:
		return "writer"
	}
}

// versionLabel renders the editor's version chip ("v4 · edited by you").
func versionLabel(doc *document.Document) string {
	if len(doc.Revisions) == 0 {
		return ""
	}
	last := doc.Revisions[len(doc.Revisions)-1]
	if last.Actor == "human" {
		return fmt.Sprintf("v%d · edited by you", doc.Version)
	}
	name := last.Actor
	if a, ok := agents[last.Actor]; ok {
		name = a.Name
	}
	return fmt.Sprintf("v%d · %s", doc.Version, name)
}

// deriveCriteria checks each must-have requirement against the current draft
// content — honest, derived state, never fabricated. It delegates to
// zone2view.DeriveCriteria (the single source of truth shared with the desktop
// app), which defers to the Final Review's open flags and otherwise uses
// finalreview.RequirementAddressed rather than a verbatim substring match.
func deriveCriteria(opp *opportunity.Opportunity, doc *document.Document) []zone2view.Criterion {
	return zone2view.DeriveCriteria(opp.Requirements, strings.ToLower(doc.Markdown()), doc.OpenFlagTexts())
}

// handleAction dispatches the gate decisions and submit.
func (h *Handler) handleAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.proposals == nil {
			http.Error(w, "proposal actions are not enabled on this server", http.StatusServiceUnavailable)
			return
		}
		id := r.PathValue("id")
		if !opportunityIDPattern.MatchString(id) {
			h.renderNotFound(w, id)
			return
		}
		var err error
		switch action {
		case "approve":
			err = h.proposals.Approve(r.Context(), id)
		case "changes":
			err = h.proposals.RequestChanges(r.Context(), id, r.FormValue("note"))
		case "submit":
			err = h.proposals.Submit(r.Context(), id)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		// Redirect with a flash marker so the workspace confirms the action
		// (issue #246 B4); action is one of the validated literals above.
		http.Redirect(w, r, "/workspace/"+id+"?flash="+action, http.StatusSeeOther)
	}
}

// handleDraftDownload serves the proposal's working draft as a Markdown file so
// the workspace's "draft.md" is a real, openable artifact instead of a dead
// label (issue #246 B3). The internal document.json is intentionally not exposed.
func (h *Handler) handleDraftDownload(w http.ResponseWriter, r *http.Request) {
	if h.proposals == nil {
		http.Error(w, "proposal actions are not enabled on this server", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if !opportunityIDPattern.MatchString(id) {
		h.renderNotFound(w, id)
		return
	}
	doc, err := h.proposals.Document(id)
	if err != nil {
		h.renderNotFound(w, id)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", id+"-draft.md"))
	_, _ = io.WriteString(w, doc.Markdown())
}

func (h *Handler) handleSectionSave(w http.ResponseWriter, r *http.Request) {
	if h.proposals == nil {
		http.Error(w, "proposal actions are not enabled on this server", http.StatusServiceUnavailable)
		return
	}
	id, sid := r.PathValue("id"), r.PathValue("sid")
	if !opportunityIDPattern.MatchString(id) || !opportunityIDPattern.MatchString(sid) {
		h.renderNotFound(w, id)
		return
	}
	if _, err := h.proposals.UpdateSection(r.Context(), id, sid, r.FormValue("body")); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	// Plain form posts navigate back to the workspace; the autosave script
	// sends fetch requests and ignores the redirect.
	http.Redirect(w, r, "/workspace/"+id, http.StatusSeeOther)
}

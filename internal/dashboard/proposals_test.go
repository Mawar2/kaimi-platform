package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/finalreview"
	"github.com/Mawar2/Kaimi/internal/googledocs"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/proposal"
	"github.com/Mawar2/Kaimi/internal/store"
	"github.com/Mawar2/Kaimi/internal/writer"
)

// newProposalHandler wires the dashboard handler to a REAL proposal service:
// real Outline agent (cached docs client), real stub Writer, real Final
// Review. This is the web product flow under test, minus only the LLM.
func newProposalHandler(t *testing.T) (*dashboard.Handler, *proposal.Service, store.Store) {
	t.Helper()
	dir := t.TempDir()
	opps, err := store.NewJSONStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	docs, err := document.NewStore(dir)
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	docsClient, err := googledocs.NewClient(context.Background(), googledocs.Config{UseCached: true})
	if err != nil {
		t.Fatalf("docs client: %v", err)
	}
	svc := proposal.NewService(&proposal.Deps{
		Opportunities: opps,
		Documents:     docs,
		Outline:       outline.New(docsClient),
		Writer:        writer.New(),
		Review:        finalreview.New(),
	})
	now := time.Now()
	opp := &opportunity.Opportunity{
		ID: "zta-1", Title: "Zero Trust Architecture Modernization",
		Agency: "DHS CISA", NAICSCode: "541512",
		Description:      "Modernize zero trust architecture.",
		ResponseDeadline: now.Add(30 * 24 * time.Hour),
		Score:            0.87, Recommendation: "BID",
		Requirements: []string{"FedRAMP High"},
		ScoredAt:     &now, CreatedAt: now, UpdatedAt: now,
	}
	if err := opps.Save(context.Background(), opp); err != nil {
		t.Fatalf("seed: %v", err)
	}
	h := dashboard.NewHandler(dashboard.NewService(opps), dashboard.WithProposals(svc))
	h.Now = func() time.Time { return now }
	return h, svc, opps
}

func postForm(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func get(t *testing.T, h http.Handler, path string) string {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", path, http.NoBody))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, rr.Code)
	}
	return rr.Body.String()
}

// TestProposalFlowOverHTTP drives the full product loop through the web
// handlers: select, agents draft, gate, human edit, approve, real final
// review, ready, submit.
func TestProposalFlowOverHTTP(t *testing.T) {
	h, svc, _ := newProposalHandler(t)

	// 1. Select to pursue (the bridge event).
	rr := postForm(t, h, "/opportunity/zta-1/select", url.Values{})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("select: status %d, want 303", rr.Code)
	}
	svc.Wait()

	// 2. Proposals command view shows the proposal waiting on the human.
	body := get(t, h, "/proposals")
	for _, want := range []string{
		"Active proposals", "Waiting on you", "needs-tag",
		"Zero Trust Architecture Modernization", "minipipe",
	} {
		if !contains(body, want) {
			t.Errorf("/proposals missing %q", want)
		}
	}

	// 3. Workspace shows the gate: review card plus editable sections.
	body = get(t, h, "/workspace/zta-1")
	for _, want := range []string{
		`data-st="human"`,       // pipeline node at the gate
		"handing you the draft", // warm handoff heading
		"<textarea",             // the editor is real
		"Approve &amp; resume",  // gate actions
		"Request changes",
		"Executive Summary", // outline sections present
	} {
		if !contains(body, want) {
			t.Errorf("workspace gate missing %q", want)
		}
	}

	// 4. Human edits a section to satisfy the must-have requirement.
	doc, err := svc.Document("zta-1")
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	secID := doc.Sections[0].ID
	rr = postForm(t, h, "/workspace/zta-1/section/"+secID, url.Values{
		"body": {"We will use FedRAMP High authorized tooling end to end."},
	})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("section save: status %d, want 303", rr.Code)
	}
	doc, _ = svc.Document("zta-1")
	if doc.Revisions[len(doc.Revisions)-1].Actor != "human" {
		t.Errorf("section edit must be attributed to the human")
	}

	// 5. Approve: the REAL final review runs on the edited revision.
	rr = postForm(t, h, "/workspace/zta-1/approve", url.Values{})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("approve: status %d, want 303", rr.Code)
	}
	svc.Wait()

	body = get(t, h, "/workspace/zta-1")
	if !contains(body, "Package ready to submit") || !contains(body, "Submit to SAM.gov") {
		t.Fatalf("workspace should be at the ready state after a clean final review")
	}

	// 6. Submit, the human act.
	rr = postForm(t, h, "/workspace/zta-1/submit", url.Values{})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("submit: status %d, want 303", rr.Code)
	}
	body = get(t, h, "/workspace/zta-1")
	if !contains(body, "Submitted to SAM.gov") {
		t.Errorf("workspace should show the submitted state")
	}
}

// TestRequestChangesOverHTTP exercises the gate's other decision.
func TestRequestChangesOverHTTP(t *testing.T) {
	h, svc, opps := newProposalHandler(t)
	postForm(t, h, "/opportunity/zta-1/select", url.Values{})
	svc.Wait()

	rr := postForm(t, h, "/workspace/zta-1/changes", url.Values{
		"note": {"Tighten the technical approach."},
	})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("changes: status %d, want 303", rr.Code)
	}
	svc.Wait()
	opp, _ := opps.Get(context.Background(), "zta-1")
	if opp.ProposalStatus != proposal.StatusGate {
		t.Errorf("after request-changes the proposal must return to the gate, got %q", opp.ProposalStatus)
	}
}

// TestWorkspaceSurfacesUseDesignTokens proves the workspace editor surfaces
// consume the --surface token instead of hardcoded #fff (issue #210). #fff
// bypasses the token system and stays white in the dark Focus theme; the
// section textarea and the read-only draft body must follow --surface so the
// workspace is theme-correct and on one vocabulary.
func TestWorkspaceSurfacesUseDesignTokens(t *testing.T) {
	h, svc, _ := newProposalHandler(t)
	if rr := postForm(t, h, "/opportunity/zta-1/select", url.Values{}); rr.Code != http.StatusSeeOther {
		t.Fatalf("select: status %d, want 303", rr.Code)
	}
	svc.Wait()

	body := get(t, h, "/workspace/zta-1")
	if !contains(body, "<textarea") {
		t.Fatalf("workspace gate must render the section editor")
	}
	// The two workspace-specific editor surfaces must route through --surface
	// (theme-aware), not a hardcoded #fff. Assert the exact rule fragments so
	// the check targets the workspace template, not the design-system tokens in
	// StyleTag (which legitimately define #fff token values).
	for _, want := range []string{
		// .edsec textarea rule (resize:vertical is unique to it)
		"background: var(--surface); border: 1px solid var(--border); border-radius: var(--r-md); padding: 12px 14px; resize: vertical",
		// .draft-body rule (white-space:pre-wrap is unique to it)
		"white-space: pre-wrap; background: var(--surface)",
	} {
		if !contains(body, want) {
			t.Errorf("workspace editor surface must use var(--surface); missing rule %q", want)
		}
	}
}

// TestWorkspaceSidebarShowsCounts proves the workspace page populates the
// shared sidebar counts (issue #246 B1). The bug: handleWorkspace built
// shellData with only PageTitle/ActiveNav, so every sidebar badge rendered 0
// even though /, /opportunity, and /proposals show the real counts. The fix
// routes every page through one shell-count helper so the sidebar never drifts.
//
// Under the self-cleaning queue (#224) selecting the only opportunity moves it
// out of the queue and to the human gate, so the proof the counts now populate
// is the Proposals "needs" badge reading 1 (it was a hard 0 before the fix).
func TestWorkspaceSidebarShowsCounts(t *testing.T) {
	h, svc, _ := newProposalHandler(t)
	if rr := postForm(t, h, "/opportunity/zta-1/select", url.Values{}); rr.Code != http.StatusSeeOther {
		t.Fatalf("select: status %d, want 303", rr.Code)
	}
	svc.Wait() // the draft pipeline runs to the human gate

	body := get(t, h, "/workspace/zta-1")
	// At the gate the proposal awaits human review → the Proposals amber badge
	// must show 1. Before the fix the workspace shell had no counts at all.
	if !contains(body, `<span class="needs">1</span>`) {
		t.Errorf("workspace sidebar should show the needs-review badge (1), got a zeroed nav")
	}
}

// TestProposalCardsResetLinkStyling proves the whole-card <a class="pcard">
// link gets the same text-decoration/color reset the other card and nav links
// get (issue #207). Without it the proposal cards render as default underlined
// link-blue text instead of the designed navy, non-underlined cards — a clear
// drift from 03-proposals-command.png. The reset lives once in the shared shell.
func TestProposalCardsResetLinkStyling(t *testing.T) {
	h, svc, _ := newProposalHandler(t)
	if rr := postForm(t, h, "/opportunity/zta-1/select", url.Values{}); rr.Code != http.StatusSeeOther {
		t.Fatalf("select: status %d, want 303", rr.Code)
	}
	svc.Wait()

	body := get(t, h, "/proposals")
	// The card link must render so there is something to reset.
	if !contains(body, `class="pcard`) {
		t.Fatalf("/proposals did not render a pcard link")
	}
	// The shared link reset must list a.pcard so card text inherits --ink and
	// drops the underline (the rule also covers a.orow / a.nav-item).
	if !contains(body, "a.pcard") {
		t.Errorf("proposal card link a.pcard must be in the text-decoration/color reset")
	}
}

// TestListAndWorkspaceAgreeOnState proves the proposals list and the workspace
// derive proposal state from the SAME source of truth — the raw ProposalStatus
// — so they can't contradict each other (issue #246 B2). The bug: the list ran
// the status through DeriveStage→rowStatus, a lossy round-trip that collapsed
// outline-running, final-review, and ALL failures into "writer:in_progress", so
// a proposal that had failed at Outline showed "Tomás drafting now / Agents
// working" in the list while the workspace correctly showed "Outline hit a
// problem".
func TestListAndWorkspaceAgreeOnState(t *testing.T) {
	dir := t.TempDir()
	opps, err := store.NewJSONStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	now := time.Now()
	opp := &opportunity.Opportunity{
		ID: "fail-1", Title: "Failed At Outline", Agency: "GSA",
		Selected: true, SelectedAt: &now, ProposalStatus: "outline:failed",
		ResponseDeadline: now.Add(20 * 24 * time.Hour),
		ScoredAt:         &now, CreatedAt: now, UpdatedAt: now,
	}
	if err := opps.Save(context.Background(), opp); err != nil {
		t.Fatalf("seed: %v", err)
	}

	docs, err := document.NewStore(dir)
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	docsClient, err := googledocs.NewClient(context.Background(), googledocs.Config{UseCached: true})
	if err != nil {
		t.Fatalf("docs client: %v", err)
	}
	svc := proposal.NewService(&proposal.Deps{
		Opportunities: opps, Documents: docs,
		Outline: outline.New(docsClient), Writer: writer.New(), Review: finalreview.New(),
	})
	h := dashboard.NewHandler(dashboard.NewService(opps), dashboard.WithProposals(svc))
	h.Now = func() time.Time { return now }

	list := get(t, h, "/proposals")
	ws := get(t, h, "/workspace/fail-1")

	// The workspace tells the truth: the outline stage failed.
	if !contains(ws, "Outline hit a problem") {
		t.Fatalf("workspace should show the outline failure")
	}
	// The list must agree — it must NOT claim the writer is happily working.
	if contains(list, "Tomás drafting now") || contains(list, "Agents working") {
		t.Errorf("proposals list contradicts the workspace: shows the writer working for an outline:failed proposal")
	}
	// And it should surface the failure under the needs-attention group.
	if !contains(list, "Needs attention") {
		t.Errorf("proposals list should surface the outline failure under Needs attention")
	}
}

// TestGateActionFeedback proves the gate decisions give the human visible
// confirmation (issue #246 B4): Request changes / Approve redirect with a flash
// marker and the workspace renders a confirmation banner, so the action never
// reads as "nothing happened" (the stub writer's invisible redraft made it look
// like a no-op).
func TestGateActionFeedback(t *testing.T) {
	h, svc, _ := newProposalHandler(t)
	postForm(t, h, "/opportunity/zta-1/select", url.Values{})
	svc.Wait() // reaches the gate

	rr := postForm(t, h, "/workspace/zta-1/changes", url.Values{"note": {"Tighten it."}})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("changes: status %d, want 303", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/workspace/zta-1?flash=changes" {
		t.Errorf("changes redirect = %q, want /workspace/zta-1?flash=changes", loc)
	}
	svc.Wait()

	body := get(t, h, "/workspace/zta-1?flash=changes")
	if !contains(body, "Sent back to Tom") { // "Sent back to Tomás…"
		t.Errorf("workspace should show a confirmation banner after request-changes")
	}

	// Approve also confirms (Vera is reviewing).
	rr = postForm(t, h, "/workspace/zta-1/approve", url.Values{})
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("approve: status %d, want 303", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/workspace/zta-1?flash=approve" {
		t.Errorf("approve redirect = %q, want /workspace/zta-1?flash=approve", loc)
	}
	svc.Wait() // let the final-review goroutine finish before TempDir cleanup
}

// TestDraftDownloadArtifact proves the gate surfaces the working draft as a
// real, downloadable artifact rather than dead "draft.md"/"document.json" labels
// (issue #246 B3): the workspace links draft.md to a download endpoint that
// serves the Markdown, and the internal document.json is no longer shown.
func TestDraftDownloadArtifact(t *testing.T) {
	dir := t.TempDir()
	opps, err := store.NewJSONStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	docs, err := document.NewStore(dir)
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	docsClient, err := googledocs.NewClient(context.Background(), googledocs.Config{UseCached: true})
	if err != nil {
		t.Fatalf("docs client: %v", err)
	}
	svc := proposal.NewService(&proposal.Deps{
		Opportunities: opps, Documents: docs,
		Outline: outline.New(docsClient), Writer: writer.New(), Review: finalreview.New(),
	})
	now := time.Now()
	opp := &opportunity.Opportunity{
		ID: "dl-1", Title: "Download Opp", Agency: "GSA",
		Selected: true, SelectedAt: &now, ProposalStatus: proposal.StatusGate,
		ResponseDeadline: now.Add(20 * 24 * time.Hour), UpdatedAt: now,
	}
	if err := opps.Save(context.Background(), opp); err != nil {
		t.Fatalf("seed opp: %v", err)
	}
	doc := &document.Document{
		OpportunityID: "dl-1", Title: "Download Opp — Technical Volume",
		Sections: []document.Section{
			{ID: "approach", Heading: "Technical Approach",
				Body: "Zero trust rollout details for download.", Status: "drafted"},
		},
	}
	if err := docs.Create(doc, "writer", "draft"); err != nil {
		t.Fatalf("create doc: %v", err)
	}

	h := dashboard.NewHandler(dashboard.NewService(opps), dashboard.WithProposals(svc))
	h.Now = func() time.Time { return now }

	body := get(t, h, "/workspace/dl-1")
	if !contains(body, `href="/workspace/dl-1/draft.md"`) {
		t.Errorf("gate should link draft.md to a real download endpoint")
	}
	if contains(body, "document.json") {
		t.Errorf("the internal document.json must not be surfaced as an artifact")
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/workspace/dl-1/draft.md", http.NoBody))
	if rr.Code != http.StatusOK {
		t.Fatalf("draft.md download: status %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("draft.md content-type = %q, want text/markdown", ct)
	}
	if !contains(rr.Body.String(), "Zero trust rollout details for download.") {
		t.Errorf("draft.md download should contain the drafted section body")
	}
}

// TestGateCriteriaMatchesParaphrase renders the gate and proves the criteria
// grid (issue #246 B6): a must-have the draft addresses in different words shows
// as met, and a genuinely-absent one reads "could not auto-confirm" rather than
// falsely asserting it is missing. Seeds a gated proposal directly so the test
// needs no live agents.
func TestGateCriteriaMatchesParaphrase(t *testing.T) {
	dir := t.TempDir()
	opps, err := store.NewJSONStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	docs, err := document.NewStore(dir)
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	docsClient, err := googledocs.NewClient(context.Background(), googledocs.Config{UseCached: true})
	if err != nil {
		t.Fatalf("docs client: %v", err)
	}
	svc := proposal.NewService(&proposal.Deps{
		Opportunities: opps, Documents: docs,
		Outline: outline.New(docsClient), Writer: writer.New(), Review: finalreview.New(),
	})
	now := time.Now()
	opp := &opportunity.Opportunity{
		ID: "crit-1", Title: "Criteria Opp", Agency: "DHS",
		Selected: true, SelectedAt: &now, ProposalStatus: proposal.StatusGate,
		Requirements:     []string{"FedRAMP High authorization", "ISO 27001 certification"},
		ResponseDeadline: now.Add(20 * 24 * time.Hour), UpdatedAt: now,
	}
	if err := opps.Save(context.Background(), opp); err != nil {
		t.Fatalf("seed opp: %v", err)
	}
	doc := &document.Document{
		OpportunityID: "crit-1", Title: "Criteria Opp — Technical Volume",
		Sections: []document.Section{
			{ID: "approach", Heading: "Technical Approach",
				Body:   "We deploy only FedRAMP High authorized tooling across the environment.",
				Status: "drafted"},
		},
	}
	if err := docs.Create(doc, "writer", "draft"); err != nil {
		t.Fatalf("create doc: %v", err)
	}

	h := dashboard.NewHandler(dashboard.NewService(opps), dashboard.WithProposals(svc))
	h.Now = func() time.Time { return now }
	body := get(t, h, "/workspace/crit-1")

	// The paraphrased requirement is recognized as met (at least one ok item).
	if !contains(body, "citem ok") {
		t.Errorf("paraphrased must-have should render as met (citem ok)")
	}
	// The genuinely-absent requirement is honest: not asserted missing.
	if !contains(body, "could not auto-confirm") {
		t.Errorf("an unconfirmed must-have should read 'could not auto-confirm', not assert it is missing")
	}
	if contains(body, "Not yet addressed in the draft") {
		t.Errorf("old false-assertion copy should be gone")
	}
}

// TestProposalGuards covers method/id/state validation.
func TestProposalGuards(t *testing.T) {
	h, svc, _ := newProposalHandler(t)

	if rr := postForm(t, h, "/opportunity/nope/select", url.Values{}); rr.Code != http.StatusNotFound {
		t.Errorf("select unknown id: status %d, want 404", rr.Code)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/opportunity/zta-1/select", http.NoBody))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET select: status %d, want 405", rr.Code)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/workspace/missing-doc", http.NoBody))
	if rr.Code != http.StatusNotFound {
		t.Errorf("workspace for unselected opp: status %d, want 404", rr.Code)
	}
	if rr := postForm(t, h, "/workspace/zta-1/approve", url.Values{}); rr.Code != http.StatusConflict {
		t.Errorf("approve before gate: status %d, want 409", rr.Code)
	}

	// Detail page shows the select CTA before selection, and the pursued
	// state after.
	body := get(t, h, "/opportunity/zta-1")
	if !contains(body, "Select to pursue") {
		t.Errorf("detail should offer the select CTA")
	}
	postForm(t, h, "/opportunity/zta-1/select", url.Values{})
	svc.Wait()
	body = get(t, h, "/opportunity/zta-1")
	if !contains(body, "In your proposals") {
		t.Errorf("pursued detail should show the in-your-proposals state")
	}
}

// TestActionsWithoutProposalService keeps the read-only deployment valid.
func TestActionsWithoutProposalService(t *testing.T) {
	opps, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	h := dashboard.NewHandler(dashboard.NewService(opps))
	if rr := postForm(t, h, "/opportunity/x/select", url.Values{}); rr.Code != http.StatusServiceUnavailable {
		t.Errorf("select without service: status %d, want 503", rr.Code)
	}
}

// TestWorkspaceShowsLivingDocumentWhileAgentsWork (issue #158): once Noa has
// built the skeleton the human sees the actual document — read-only — while
// Tomás is still drafting.
func TestWorkspaceShowsLivingDocumentWhileAgentsWork(t *testing.T) {
	dir := t.TempDir()
	opps, err := store.NewJSONStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	docs, err := document.NewStore(dir)
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	docsClient, err := googledocs.NewClient(context.Background(), googledocs.Config{UseCached: true})
	if err != nil {
		t.Fatalf("docs client: %v", err)
	}
	svc := proposal.NewService(&proposal.Deps{
		Opportunities: opps,
		Documents:     docs,
		Outline:       outline.New(docsClient),
		Writer:        writer.New(),
		Review:        finalreview.New(),
	})

	// Mid-writer snapshot: skeleton exists, first section drafted, second
	// still outlined, status writer:in_progress.
	now := time.Now()
	opp := &opportunity.Opportunity{
		ID: "mid-1", Title: "Mid-Draft Opp", Agency: "GSA",
		Selected: true, SelectedAt: &now, ProposalStatus: "writer:in_progress",
		ResponseDeadline: now.Add(20 * 24 * time.Hour), UpdatedAt: now,
	}
	if err := opps.Save(context.Background(), opp); err != nil {
		t.Fatalf("seed: %v", err)
	}
	doc := &document.Document{
		OpportunityID: "mid-1",
		Title:         "Mid-Draft Opp — Technical Volume",
		Sections: []document.Section{
			{ID: "exec_summary", Heading: "Executive Summary", Body: "Already drafted.", Status: "drafted"},
			{ID: "technical_approach", Heading: "Technical Approach", Status: "outlined"},
		},
	}
	if err := docs.Create(doc, "outline", "skeleton"); err != nil {
		t.Fatalf("create doc: %v", err)
	}

	h := dashboard.NewHandler(dashboard.NewService(opps), dashboard.WithProposals(svc))
	body := get(t, h, "/workspace/mid-1")

	for _, want := range []string{
		"Tomás is working",      // the calm working card stays
		"Executive Summary",     // the document is visible…
		"Already drafted.",      // …with drafted content…
		"Technical Approach",    // …and the not-yet-drafted section…
		"drafting this section", // …shows the placeholder
	} {
		if !contains(body, want) {
			t.Errorf("mid-draft workspace missing %q", want)
		}
	}
	if contains(body, "<textarea") {
		t.Errorf("the document must be read-only outside the gate")
	}
}

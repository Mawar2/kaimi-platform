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

package dashboard_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Mawar2/Kaimi/internal/proposal"
)

// TestDraftEditorPage verifies the standalone full-page draft editor: it loads
// the selected proposal's document, renders the section rail + editable doc, and
// is NOT wrapped in the app shell (no sidebar).
func TestDraftEditorPage(t *testing.T) {
	h, svc, _ := newProposalHandler(t)
	if rr := postForm(t, h, "/opportunity/zta-1/select", url.Values{}); rr.Code != http.StatusSeeOther {
		t.Fatalf("select: status %d, want 303", rr.Code)
	}
	svc.Wait()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/editor/zta-1", http.NoBody))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /editor/zta-1: status %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		`class="ed-fullpage`, // the focused full-page surface
		`class="ed-rail"`,    // section rail
		"Back to review",     // returns to the workspace
		"<textarea",          // editable sections
		`data-autosave`,      // reuses the workspace autosave
		"Executive Summary",  // a real document section
	} {
		if !contains(body, want) {
			t.Errorf("/editor missing %q", want)
		}
	}
	// Standalone page: no app-shell sidebar.
	if contains(body, `class="side"`) {
		t.Errorf("editor must be a standalone page (no app shell sidebar)")
	}
}

// TestEditorRequiresDocument 404s a proposal that was never selected.
func TestEditorRequiresDocument(t *testing.T) {
	h, _, _ := newProposalHandler(t)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/editor/zta-1", http.NoBody))
	if rr.Code != http.StatusNotFound {
		t.Errorf("editor for an unselected opp: status %d, want 404", rr.Code)
	}
}

// gapBody is a section body holding one unresolved Writer gap marker plus a
// script tag, so the same fixture proves both the callout and the escaping.
const gapBody = "Staffed by [GAP: number of cleared staff] engineers. <script>alert(1)</script>"

// seedGapSection selects zta-1 and writes gapBody into its first section.
func seedGapSection(t *testing.T, h http.Handler, svc *proposal.Service) string {
	t.Helper()
	if rr := postForm(t, h, "/opportunity/zta-1/select", url.Values{}); rr.Code != http.StatusSeeOther {
		t.Fatalf("select: status %d, want 303", rr.Code)
	}
	svc.Wait()
	doc, err := svc.Document("zta-1")
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	secID := doc.Sections[0].ID
	if rr := postForm(t, h, "/workspace/zta-1/section/"+secID, url.Values{"body": {gapBody}}); rr.Code != http.StatusSeeOther {
		t.Fatalf("section save: status %d, want 303", rr.Code)
	}
	return secID
}

// TestEditorHighlightsUnresolvedGaps: a section holding a [GAP: ...] marker
// gets the amber textarea tint, a per-gap callout with a jump control, and a
// warn mark in the section rail — and the gap text is HTML-escaped.
func TestEditorHighlightsUnresolvedGaps(t *testing.T) {
	h, svc, _ := newProposalHandler(t)
	seedGapSection(t, h, svc)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/editor/zta-1", http.NoBody))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /editor/zta-1: status %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		`class="gap-warn"`,                   // amber textarea tint
		"Unresolved gap",                     // callout title
		"number of cleared staff",            // the missing-fact text
		`data-gap="number of cleared staff"`, // jump-to-gap hook
		`class="ed-sec warn"`,                // section rail warn mark
	} {
		if !contains(body, want) {
			t.Errorf("/editor missing %q", want)
		}
	}
	if contains(body, "<script>alert(1)</script>") {
		t.Errorf("section body with markup must be HTML-escaped")
	}
}

// TestEditorNoGaps_NoWarnUI: a clean draft renders without any gap UI.
func TestEditorNoGaps_NoWarnUI(t *testing.T) {
	h, svc, _ := newProposalHandler(t)
	if rr := postForm(t, h, "/opportunity/zta-1/select", url.Values{}); rr.Code != http.StatusSeeOther {
		t.Fatalf("select: status %d, want 303", rr.Code)
	}
	svc.Wait()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/editor/zta-1", http.NoBody))
	body := rr.Body.String()
	for _, reject := range []string{`class="gap-warn"`, "Unresolved gap"} {
		if contains(body, reject) {
			t.Errorf("clean draft must not render %q", reject)
		}
	}
}

// TestWorkspaceGateHighlightsGaps: the review-gate section editors get the
// same gap treatment as the full editor.
func TestWorkspaceGateHighlightsGaps(t *testing.T) {
	h, svc, _ := newProposalHandler(t)
	seedGapSection(t, h, svc)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/workspace/zta-1", http.NoBody))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /workspace/zta-1: status %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{
		`class="gap-warn"`,
		"Unresolved gap",
		"number of cleared staff",
		`data-gap="number of cleared staff"`,
	} {
		if !contains(body, want) {
			t.Errorf("/workspace gate missing %q", want)
		}
	}
}

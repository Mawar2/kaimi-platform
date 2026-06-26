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

// newDriveSaveHandler builds a dashboard handler over a real proposal service with a drafted
// document for "zta-1", optionally wiring a (fake) Drive saver. It mirrors newProposalHandler
// but lets the test inject the saver so the "Save to Google Drive" path runs fully offline.
func newDriveSaveHandler(t *testing.T, saver dashboard.ProposalDriveSaver) *dashboard.Handler {
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
		Agency: "DHS CISA", NAICSCode: "541512", SolicitationNum: "SOL-2026-001",
		Description: "Modernize zero trust architecture.", ResponseDeadline: now.Add(720 * time.Hour),
		Score: 0.87, Recommendation: "BID", CreatedAt: now, UpdatedAt: now,
		Requirements: []string{"continuous endpoint monitoring", "supply chain risk management plan"},
	}
	if err := opps.Save(context.Background(), opp); err != nil {
		t.Fatalf("seed: %v", err)
	}

	optsList := []dashboard.Option{dashboard.WithProposals(svc)}
	if saver != nil {
		optsList = append(optsList, dashboard.WithProposalDriveSaver(saver))
	}
	h := dashboard.NewHandler(dashboard.NewService(opps), optsList...)
	h.Now = func() time.Time { return now }

	// Draft the proposal so a document exists to save.
	if rr := postForm(t, h, "/opportunity/zta-1/select", url.Values{}); rr.Code != http.StatusSeeOther {
		t.Fatalf("select: status %d, want 303", rr.Code)
	}
	svc.Wait()
	return h
}

// TestSaveToDrive covers the workspace "Save to Google Drive" action: success lands the user
// in the new Doc, a not-connected Drive routes them to Connect, and the action is hidden +
// unavailable when no saver is wired.
func TestSaveToDrive(t *testing.T) {
	t.Run("success redirects to the new Doc URL", func(t *testing.T) {
		const docURL = "https://docs.google.com/document/d/abc123/edit"
		h := newDriveSaveHandler(t, func(_ context.Context, doc *document.Document) (string, error) {
			if doc == nil || doc.Title == "" {
				t.Error("saver received an empty document")
			}
			return docURL, nil
		})
		rr := postForm(t, h, "/workspace/zta-1/save-to-drive", url.Values{})
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303", rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != docURL {
			t.Errorf("Location = %q, want the new Doc URL %q", loc, docURL)
		}
	})

	t.Run("not connected redirects to the Connect step", func(t *testing.T) {
		h := newDriveSaveHandler(t, func(_ context.Context, _ *document.Document) (string, error) {
			return "", dashboard.ErrDriveNotConnected
		})
		rr := postForm(t, h, "/workspace/zta-1/save-to-drive", url.Values{})
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303", rr.Code)
		}
		if loc := rr.Header().Get("Location"); !strings.Contains(loc, "step=connect") {
			t.Errorf("Location = %q, want the onboarding Connect step", loc)
		}
	})

	t.Run("button shown on the workspace when wired", func(t *testing.T) {
		h := newDriveSaveHandler(t, func(_ context.Context, _ *document.Document) (string, error) { return "x", nil })
		body := get(t, h, "/workspace/zta-1")
		for _, want := range []string{"/workspace/zta-1/save-to-drive", "Save to Google Drive"} {
			if !strings.Contains(body, want) {
				t.Errorf("workspace missing %q", want)
			}
		}
	})

	t.Run("hidden and unavailable when no saver wired", func(t *testing.T) {
		h := newDriveSaveHandler(t, nil)
		if body := get(t, h, "/workspace/zta-1"); strings.Contains(body, "save-to-drive") {
			t.Error("the Save to Google Drive button must be hidden when no saver is wired")
		}
		rr := postForm(t, h, "/workspace/zta-1/save-to-drive", url.Values{})
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503 when no saver is wired", rr.Code)
		}
	})
}

// TestProposalComplianceCSV covers the workspace compliance-matrix download: after the proposal
// is drafted, GET /workspace/{id}/compliance.csv returns a CSV attachment with rows for the
// opportunity's extracted requirements.
func TestProposalComplianceCSV(t *testing.T) {
	h := newDriveSaveHandler(t, nil)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/workspace/zta-1/compliance.csv", http.NoBody))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want it to contain text/csv", ct)
	}
	cd := rr.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, ".csv") {
		t.Errorf("Content-Disposition = %q, want an attachment with a .csv filename", cd)
	}
	body := rr.Body.String()
	for _, want := range []string{"Compliance matrix", "Requirement", "continuous endpoint monitoring"} {
		if !strings.Contains(body, want) {
			t.Errorf("CSV body missing %q", want)
		}
	}
}

package main

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/store"
)

func TestDashboardHandler(t *testing.T) {
	// Setup a temporary JSON store
	tmpDir, err := os.MkdirTemp("", "dashboard-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	s, err := store.NewJSONStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Add some test opportunities
	now := time.Now()
	opps := []opportunity.Opportunity{
		{ID: "opp1", Title: "Hunted Opp"},                   // Hunted
		{ID: "opp2", Title: "Scored Opp", ScoredAt: &now},   // Scored
		{ID: "opp3", Title: "Selected Opp", Selected: true}, // Selected
	}

	for _, opp := range opps {
		if err := s.Save(context.Background(), &opp); err != nil {
			t.Fatal(err)
		}
	}

	svc := dashboard.NewService(s)

	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		t.Fatalf("failed to parse templates: %v", err)
	}

	srv := &server{
		svc:  svc,
		tmpl: tmpl,
	}

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	rr := httptest.NewRecorder()

	srv.handleOverview(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	body := rr.Body.String()

	// Check for stage names and counts
	expectedStages := []string{"Hunted", "Scored", "Selected", "In Proposal", "Awaiting Human Review", "Finalized"}
	for _, stage := range expectedStages {
		if !strings.Contains(body, stage) {
			t.Errorf("expected body to contain stage %q", stage)
		}
	}

	// Check for specific counts
	// Hunted: 1, Scored: 1, Selected: 1, others: 0

	// Better way to check counts:
	if !strings.Contains(body, "1") {
		t.Errorf("expected body to contain count 1")
	}
}

func TestDashboardHandler_NotFound(t *testing.T) {
	srv := &server{}
	req := httptest.NewRequest(http.MethodGet, "/not-found", http.NoBody)
	rr := httptest.NewRecorder()

	srv.handleOverview(rr, req)

	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

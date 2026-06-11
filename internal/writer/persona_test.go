package writer

import (
	"context"
	"testing"

	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
)

// TestWriter_SurfacesThomasPersona verifies the Writer carries its human-facing
// persona ("Thomas") in the AgentResult metadata so the dashboard can attribute
// a draft to its author. It holds in stub mode too (persona is the role, not the
// model).
func TestWriter_SurfacesThomasPersona(t *testing.T) {
	_, res, err := New().Run(context.Background(), Input{
		Opportunity: &opportunity.Opportunity{ID: "OPP-1", Title: "Test Opportunity"},
		Outline:     &outline.Outline{Sections: []outline.Section{{ID: "intro", Title: "Introduction"}}},
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got := res.Flags["persona"]; got != "Thomas" {
		t.Errorf("persona flag = %q, want %q", got, "Thomas")
	}
}

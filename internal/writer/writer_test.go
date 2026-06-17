package writer

import (
	"context"
	"strings"
	"testing"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/scorer"
)

// TestBuildSectionPrompt_IncludesRevisionNote proves a human change request is
// surfaced to the model on a revision, and absent on a fresh draft (#249 follow-up).
func TestBuildSectionPrompt_IncludesRevisionNote(t *testing.T) {
	opp := &opportunity.Opportunity{ID: "x", Title: "T", Agency: "A"}
	section := outline.Section{Title: "Past Performance"}
	const note = "Name a teaming partner for past performance at this scale."

	withNote := buildSectionPrompt(opp, &scorer.CapabilityProfile{}, section, nil, nil, note)
	if !strings.Contains(withNote, note) {
		t.Errorf("revision prompt must include the reviewer change request:\n%s", withNote)
	}
	if !strings.Contains(withNote, "change request") {
		t.Errorf("revision prompt should label the change request block")
	}
	fresh := buildSectionPrompt(opp, &scorer.CapabilityProfile{}, section, nil, nil, "")
	if strings.Contains(fresh, "change request") {
		t.Errorf("a fresh draft must not carry a change-request block")
	}
}

func newValidInput() Input {
	opp := &opportunity.Opportunity{
		ID:    "opp-123",
		Title: "Cloud Migration Project",
	}
	out := &outline.Outline{
		OpportunityID: "opp-123",
		Title:         "Migration Outline",
		Sections: []outline.Section{
			{ID: "s1", Title: "Executive Summary"},
			{ID: "s2", Title: "Technical Approach"},
			{ID: "s3", Title: "Timeline"},
		},
		FormattingRules: &outline.FormattingRules{},
	}
	return Input{
		Opportunity: opp,
		Outline:     out,
	}
}

func TestNew_ReturnsAgent(t *testing.T) {
	a := New()
	if a == nil {
		t.Fatal("expected agent to be non-nil")
	}
}

func TestRun_ValidInput_Success(t *testing.T) {
	ctx := context.Background()
	a := New()
	in := newValidInput()

	draft, res, err := a.Run(ctx, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != agent.StatusSuccess {
		t.Errorf("expected status %s, got %s", agent.StatusSuccess, res.Status)
	}
	if res.AgentName != "writer" {
		t.Errorf("expected agent name writer, got %s", res.AgentName)
	}
	if res.NoticeID != in.Opportunity.ID {
		t.Errorf("expected NoticeID %s, got %s", in.Opportunity.ID, res.NoticeID)
	}

	if draft == "" {
		t.Error("expected non-empty draft")
	}
	for _, s := range in.Outline.Sections {
		if !strings.Contains(draft, s.Title) {
			t.Errorf("draft missing section title: %s", s.Title)
		}
	}

	expectedCount := "3"
	if res.Flags["section_count"] != expectedCount {
		t.Errorf("expected flag section_count %s, got %s", expectedCount, res.Flags["section_count"])
	}
}

func TestRun_NilOpportunity_Failed(t *testing.T) {
	ctx := context.Background()
	a := New()
	in := newValidInput()
	in.Opportunity = nil

	draft, res, err := a.Run(ctx, in)
	if err == nil {
		t.Error("expected error for nil opportunity")
	}
	if draft != "" {
		t.Errorf("expected empty draft on failure, got %q", draft)
	}
	if res == nil || res.Status != agent.StatusFailed {
		t.Errorf("expected result status failed, got %+v", res)
	}
}

func TestRun_NilOutline_Failed(t *testing.T) {
	ctx := context.Background()
	a := New()
	in := newValidInput()
	in.Outline = nil

	draft, res, err := a.Run(ctx, in)
	if err == nil {
		t.Error("expected error for nil outline")
	}
	if draft != "" {
		t.Errorf("expected empty draft on failure, got %q", draft)
	}
	if res == nil || res.Status != agent.StatusFailed {
		t.Errorf("expected result status failed, got %+v", res)
	}
}

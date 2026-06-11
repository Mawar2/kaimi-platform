package document

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	base := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	tick := 0
	s.Now = func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Minute)
	}
	return s
}

func skeleton() *Document {
	return &Document{
		OpportunityID: "opp-1",
		Title:         "Zero Trust Modernization — Technical Volume",
		Sections: []Section{
			{ID: "exec_summary", Heading: "Executive Summary"},
			{ID: "technical_approach", Heading: "Technical Approach", RequirementIDs: []string{"req-1"}},
			{ID: "past_performance", Heading: "Past Performance"},
		},
	}
}

func TestCreateAndGetRoundTrip(t *testing.T) {
	s := newTestStore(t)
	if err := s.Create(skeleton(), "outline", "Outline skeleton: 3 sections"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Get("opp-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if len(got.Sections) != 3 || got.Sections[1].ID != "technical_approach" {
		t.Errorf("section order not preserved: %+v", got.Sections)
	}
	if len(got.Revisions) != 1 || got.Revisions[0].Actor != "outline" {
		t.Errorf("revision attribution missing: %+v", got.Revisions)
	}
	if got.Sections[1].RequirementIDs[0] != "req-1" {
		t.Errorf("requirement links lost")
	}
}

func TestCreateRejectsDuplicateAndBadIDs(t *testing.T) {
	s := newTestStore(t)
	if err := s.Create(skeleton(), "outline", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Create(skeleton(), "outline", ""); err == nil {
		t.Errorf("duplicate Create should fail")
	}
	bad := skeleton()
	bad.OpportunityID = "../escape"
	if err := s.Create(bad, "outline", ""); err == nil {
		t.Errorf("path-shaped opportunity id must be rejected")
	}
}

func TestReplaceSectionsAttributesAgent(t *testing.T) {
	s := newTestStore(t)
	if err := s.Create(skeleton(), "outline", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	doc, err := s.ReplaceSections("opp-1", map[string]string{
		"exec_summary":       "Example Federal Co will deliver.",
		"technical_approach": "Zero trust pillars.",
	}, "writer", "Draft v1")
	if err != nil {
		t.Fatalf("ReplaceSections: %v", err)
	}
	if doc.Version != 2 {
		t.Errorf("Version = %d, want 2", doc.Version)
	}
	if doc.Sections[0].Body != "Example Federal Co will deliver." {
		t.Errorf("section body not applied")
	}
	if doc.Sections[2].Body != "" {
		t.Errorf("untouched section must keep its body")
	}
	last := doc.Revisions[len(doc.Revisions)-1]
	if last.Actor != "writer" || last.Version != 2 {
		t.Errorf("agent revision not recorded: %+v", last)
	}
}

func TestUpdateSectionIsHumanAttributed(t *testing.T) {
	s := newTestStore(t)
	if err := s.Create(skeleton(), "outline", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	doc, err := s.UpdateSection("opp-1", "exec_summary", "My edited summary.", "human")
	if err != nil {
		t.Fatalf("UpdateSection: %v", err)
	}
	if doc.Sections[0].Body != "My edited summary." {
		t.Errorf("edit not applied")
	}
	last := doc.Revisions[len(doc.Revisions)-1]
	if last.Actor != "human" {
		t.Errorf("human attribution missing: %+v", last)
	}
	if _, err := s.UpdateSection("opp-1", "nope", "x", "human"); err == nil {
		t.Errorf("unknown section id should fail")
	}
}

func TestMarkdownMirrorRewrittenOnEverySave(t *testing.T) {
	s := newTestStore(t)
	if err := s.Create(skeleton(), "outline", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.UpdateSection("opp-1", "exec_summary", "Edited body here.", "human"); err != nil {
		t.Fatalf("UpdateSection: %v", err)
	}
	md, err := os.ReadFile(filepath.Join(s.dir("opp-1"), "draft.md"))
	if err != nil {
		t.Fatalf("draft.md missing: %v", err)
	}
	text := string(md)
	for _, want := range []string{
		"# Zero Trust Modernization — Technical Volume",
		"## Executive Summary",
		"Edited body here.",
		"## Technical Approach",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("draft.md missing %q", want)
		}
	}
}

func TestSetFlags(t *testing.T) {
	s := newTestStore(t)
	if err := s.Create(skeleton(), "outline", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	doc, err := s.SetFlags("opp-1", []Flag{
		{SectionID: "past_performance", Title: "No past performance at this scale", Detail: "Flagged by final review"},
	}, "final-review", "Review issues")
	if err != nil {
		t.Fatalf("SetFlags: %v", err)
	}
	if len(doc.Flags) != 1 || doc.Flags[0].Resolved {
		t.Errorf("flags not stored: %+v", doc.Flags)
	}
}

func TestVersionsIncrementAcrossActors(t *testing.T) {
	s := newTestStore(t)
	if err := s.Create(skeleton(), "outline", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.ReplaceSections("opp-1", map[string]string{"exec_summary": "draft"}, "writer", ""); err != nil {
		t.Fatalf("ReplaceSections: %v", err)
	}
	doc, err := s.UpdateSection("opp-1", "exec_summary", "human edit", "human")
	if err != nil {
		t.Fatalf("UpdateSection: %v", err)
	}
	if doc.Version != 3 || len(doc.Revisions) != 3 {
		t.Errorf("want version 3 with 3 revisions, got v%d / %d revisions", doc.Version, len(doc.Revisions))
	}
	// Re-read from disk to prove persistence of the full history.
	got, err := s.Get("opp-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Version != 3 || got.Revisions[2].Actor != "human" {
		t.Errorf("history not persisted: v%d %+v", got.Version, got.Revisions)
	}
}

func TestGetMissingIsNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get("absent")
	if err == nil {
		t.Fatalf("Get on missing document should error")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(Get missing) = false, want true")
	}
}

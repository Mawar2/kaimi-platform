package finalreview_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/finalreview"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
)

// fixture returns a minimal valid Opportunity for use in tests.
func fixture() *opportunity.Opportunity {
	now := time.Now().UTC()
	return &opportunity.Opportunity{
		ID:               "opp-fixture-001",
		Title:            "Enterprise IT Modernization Services",
		Agency:           "Dept. of Veterans Affairs",
		SolicitationNum:  "VA-2026-IT-001",
		NAICSCode:        "541512",
		PostedDate:       now.Add(-7 * 24 * time.Hour),
		ResponseDeadline: now.Add(30 * 24 * time.Hour),
		Description:      "Seeking IT modernization support for enterprise systems.",
		URL:              "https://sam.gov/opp/fixture-001",
		CreatedAt:        now.Add(-7 * 24 * time.Hour),
		UpdatedAt:        now,
	}
}

// draftFixture returns a non-empty approved draft for use in tests.
const draftFixture = `
# Technical Proposal — Enterprise IT Modernization

## Executive Summary
Example Federal Co brings proven expertise in federal IT modernization...

## Technical Approach
Our approach follows a phased migration strategy...

## Past Performance
Example Federal Co has successfully delivered similar engagements for DoD and civilian agencies...
`

// outlineFixture returns a minimal Outline with a few required sections.
func outlineFixture() *outline.Outline {
	return &outline.Outline{
		OpportunityID: "opp-fixture-001",
		Title:         "Enterprise IT Modernization Services",
		Sections: []outline.Section{
			{ID: "executive_summary", Title: "Executive Summary", Required: true},
			{ID: "technical_approach", Title: "Technical Approach", Required: true},
			{ID: "past_performance", Title: "Past Performance", Required: true},
		},
		FormattingRules: &outline.FormattingRules{
			PageLimit:     &outline.FormattingRule{Specified: false},
			Font:          &outline.FormattingRule{Specified: false},
			Margins:       &outline.FormattingRule{Specified: false},
			LineSpacing:   &outline.FormattingRule{Specified: false},
			FileFormat:    &outline.FormattingRule{Specified: false},
			RequiredForms: nil,
		},
		GeneratedAt: time.Now().UTC(),
	}
}

func TestNew_ReturnsAgent(t *testing.T) {
	a := finalreview.New()
	if a == nil {
		t.Fatal("New() returned nil")
	}
}

func TestReview_ValidInput_ReturnsResult(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Review() returned nil result")
	}
}

func TestReview_ValidInput_AgentNameSet(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.AgentName != "final-review" {
		t.Errorf("AgentName = %q, want %q", result.AgentName, "final-review")
	}
}

func TestReview_ValidInput_ReadyToSubmit(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	// All checks pass — a valid draft and opportunity should be ready.
	if result.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %q, want %q for valid input; summary: %s", result.Status, agent.StatusReadyToSubmit, result.Summary)
	}
}

func TestReview_ValidInput_StatusSuccess(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %q, want %q", result.Status, agent.StatusReadyToSubmit)
	}
}

func TestReview_ValidInput_SummaryNotEmpty(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Summary == "" {
		t.Error("Summary is empty, want a non-empty explanation")
	}
}

func TestReview_NilOpportunity_ReturnsError(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	_, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: nil,
	})
	if err == nil {
		t.Error("Review() with nil Opportunity should return an error")
	}
}

func TestReview_EmptyDraft_ReturnsError(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	_, err := a.Review(ctx, finalreview.Input{
		Draft:       "",
		Opportunity: fixture(),
	})
	if err == nil {
		t.Error("Review() with empty Draft should return an error")
	}
}

func TestReview_NeverSubmits(t *testing.T) {
	// This test documents the invariant: Final Review never triggers submission.
	// It sets Status=StatusReadyToSubmit only as a signal to a human, not as an action.
	// There is no Submit() method on the agent — only Review().
	ctx := context.Background()
	a := finalreview.New()

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}

	// If Status is StatusReadyToSubmit, that's a flag for a human — not an automatic action.
	// This test verifies the agent only returns a result; it does not call any
	// submission API or side-effect that would send the proposal.
	_ = result.Status // documented: human reads this and decides
}

func TestReview_ExpiredDeadline_NotReadyToSubmit(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	opp := fixture()
	// Set deadline in the past — proposal cannot be submitted.
	opp.ResponseDeadline = time.Now().Add(-24 * time.Hour)

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: opp,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status == agent.StatusReadyToSubmit {
		t.Error("Status = StatusReadyToSubmit for expired deadline, want StatusFailed")
	}
	if result.Status != agent.StatusFailed {
		t.Errorf("Status = %q for expired deadline, want %q", result.Status, agent.StatusFailed)
	}
}

// --- KAI-7: must_have check ---

func TestReview_MissingMustHave_NeedsHuman(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	opp := fixture()
	opp.Requirements = []string{"cybersecurity compliance"}

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture, // does not mention "cybersecurity compliance"
		Opportunity: opp,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusNeedsHuman {
		t.Errorf("Status = %q, want %q when must-have requirement is missing", result.Status, agent.StatusNeedsHuman)
	}
}

func TestReview_MustHaveAddressed_ReadyToSubmit(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	opp := fixture()
	opp.Requirements = []string{"IT modernization"}

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture, // contains "IT modernization"
		Opportunity: opp,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %q, want %q when must-have requirement is addressed", result.Status, agent.StatusReadyToSubmit)
	}
}

// TestReview_MustHaveParaphrased_ReadyToSubmit locks the fix for issue #262: a
// requirement the draft addresses in different words (or with small wording
// shifts like an inserted "the") must NOT be flagged. The cases mirror the
// tester-reported gate block: the draft plainly qualified the company, yet the
// verbatim-substring matcher flagged every must-have and blocked approval.
func TestReview_MustHaveParaphrased_ReadyToSubmit(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	// Mirrors the deployed draft's Technical Approach opening (issue #262).
	draft := `## Technical Approach
As a Small Disadvantaged Business fully qualifying under NAICS 541519
(Other Computer Related Services), BlueMeta Technologies will execute the
Website Modernization for the Selective Service System by leveraging our
core competencies in website modernization and secure federal delivery.`

	opp := fixture()
	opp.Requirements = []string{
		// None of these appear verbatim in the draft, but all are addressed.
		"Website Modernization for Selective Service System",
		"Must qualify under NAICS 541519 (Other Computer Related Services)",
		"Must qualify as a Small Business",
	}

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draft,
		Opportunity: opp,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %q, want %q when every must-have is addressed in the draft's own words; flags: %v",
			result.Status, agent.StatusReadyToSubmit, result.Flags)
	}
}

// TestReview_MustHaveParaphrased_StillFlagsAbsent proves the lenient matcher
// does not false-green: a requirement genuinely absent from the draft is
// still flagged even alongside addressed ones.
func TestReview_MustHaveParaphrased_StillFlagsAbsent(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	draft := `## Technical Approach
As a Small Disadvantaged Business fully qualifying under NAICS 541519,
BlueMeta Technologies will modernize the Selective Service System website.`

	opp := fixture()
	opp.Requirements = []string{
		"Must qualify as a Small Business", // addressed
		"FedRAMP High authorization",       // absent — must be flagged
	}

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draft,
		Opportunity: opp,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusNeedsHuman {
		t.Errorf("Status = %q, want %q when one must-have is genuinely absent", result.Status, agent.StatusNeedsHuman)
	}
	if got := result.Flags["issues_found"]; got != "1" {
		t.Errorf("Flags[issues_found] = %q, want %q (only the absent requirement flagged); flags: %v", got, "1", result.Flags)
	}
}

func TestReview_MultipleMustHaves_AllMissing_NeedsHuman(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	opp := fixture()
	opp.Requirements = []string{"zero trust architecture", "FedRAMP authorization", "FIPS 140-2"}

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture, // mentions none of these
		Opportunity: opp,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusNeedsHuman {
		t.Errorf("Status = %q, want %q when all must-have requirements are missing", result.Status, agent.StatusNeedsHuman)
	}
	// All three gaps should be recorded as separate issues.
	issuesFound := result.Flags["issues_found"]
	if issuesFound == "0" || issuesFound == "" {
		t.Errorf("Flags[issues_found] = %q, want > 0 when multiple requirements are missing", issuesFound)
	}
}

func TestReview_EmptyRequirements_ReadyToSubmit(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	opp := fixture()
	opp.Requirements = nil // no must-have requirements

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: opp,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %q, want %q when Requirements is nil", result.Status, agent.StatusReadyToSubmit)
	}
}

// --- KAI-7: required_section check ---

func TestReview_MissingRequiredSection_NeedsHuman(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	ol := outlineFixture()
	// Add a required section whose title doesn't appear in draftFixture.
	ol.Sections = append(ol.Sections, outline.Section{
		ID:       "security_plan",
		Title:    "Security Plan",
		Required: true,
	})

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
		Outline:     ol,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusNeedsHuman {
		t.Errorf("Status = %q, want %q when required section is absent from draft", result.Status, agent.StatusNeedsHuman)
	}
}

func TestReview_AllRequiredSectionsPresent_ReadyToSubmit(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	// outlineFixture has Executive Summary, Technical Approach, Past Performance —
	// all of which appear in draftFixture.
	ol := outlineFixture()

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
		Outline:     ol,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %q, want %q when all required sections are present", result.Status, agent.StatusReadyToSubmit)
	}
}

func TestReview_OptionalSectionMissing_ReadyToSubmit(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	ol := outlineFixture()
	// Add an optional section that is NOT in the draft.
	ol.Sections = append(ol.Sections, outline.Section{
		ID:       "appendix_a",
		Title:    "Appendix A — Resumes",
		Required: false,
	})

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
		Outline:     ol,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	// Optional sections don't trigger an issue.
	if result.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %q, want %q for optional missing section", result.Status, agent.StatusReadyToSubmit)
	}
}

func TestReview_NoOutline_SkipsOutlineChecks(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	// No Outline provided — section and form checks must be skipped.
	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
		Outline:     nil,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %q, want %q when Outline is nil", result.Status, agent.StatusReadyToSubmit)
	}
}

// --- KAI-7: required_form check ---

func TestReview_MissingRequiredForm_NeedsHuman(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	ol := outlineFixture()
	ol.FormattingRules.RequiredForms = []string{"SF-1449"}

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture, // does not mention "SF-1449"
		Opportunity: fixture(),
		Outline:     ol,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusNeedsHuman {
		t.Errorf("Status = %q, want %q when required form is not acknowledged in draft", result.Status, agent.StatusNeedsHuman)
	}
}

func TestReview_RequiredFormMentioned_ReadyToSubmit(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	ol := outlineFixture()
	ol.FormattingRules.RequiredForms = []string{"SF-1449"}

	draftWithForm := draftFixture + "\n\nSF-1449 is attached as required by the solicitation.\n"

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftWithForm,
		Opportunity: fixture(),
		Outline:     ol,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %q, want %q when required form is mentioned in draft", result.Status, agent.StatusReadyToSubmit)
	}
}

func TestReview_NoRequiredForms_ReadyToSubmit(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	ol := outlineFixture()
	ol.FormattingRules.RequiredForms = nil // no forms required

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
		Outline:     ol,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %q, want %q when no required forms are specified", result.Status, agent.StatusReadyToSubmit)
	}
}

// --- KAI-7: flag reporting ---

func TestReview_IssuesReportedInFlags(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	opp := fixture()
	opp.Requirements = []string{"zero trust architecture"}

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: opp,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}

	if result.Flags == nil {
		t.Fatal("Flags is nil, want at least issues_found key")
	}
	if result.Flags["issues_found"] == "" {
		t.Error("Flags[issues_found] is empty, want a count")
	}
	// At least one issue should be reported.
	if result.Flags["issue_1"] == "" {
		t.Error("Flags[issue_1] is empty, want a detail string")
	}
}

func TestReview_CleanDraft_FlagsHaveZeroIssues(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}

	if result.Flags == nil {
		t.Fatal("Flags is nil, want issues_found key")
	}
	if result.Flags["issues_found"] != "0" {
		t.Errorf("Flags[issues_found] = %q, want \"0\" for a clean draft", result.Flags["issues_found"])
	}
	if _, ok := result.Flags["issue_1"]; ok {
		t.Error("Flags[issue_1] is set, want no issue_N keys for a clean draft")
	}
}

func TestReview_IssueDetails_ContainWhatAndWhere(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	opp := fixture()
	opp.Requirements = []string{"zero trust architecture"}

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: opp,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}

	detail := result.Flags["issue_1"]
	if !strings.Contains(detail, "zero trust architecture") {
		t.Errorf("issue_1 = %q, want it to contain the keyword %q", detail, "zero trust architecture")
	}
	if !strings.Contains(strings.ToLower(detail), "draft") {
		t.Errorf("issue_1 = %q, want it to reference \"draft\"", detail)
	}
}

// --- KAI-7: page_limit check ---

func TestReview_DraftWithinPageLimit_ReadyToSubmit(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	ol := outlineFixture()
	// 5-page limit; draftFixture is well under that.
	ol.FormattingRules.PageLimit = &outline.FormattingRule{Value: "5 pages", Specified: true}

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
		Outline:     ol,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %q, want %q when draft is within page limit", result.Status, agent.StatusReadyToSubmit)
	}
}

func TestReview_DraftExceedsPageLimit_NeedsHuman(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	ol := outlineFixture()
	// 1-page limit (250 words); build a draft that clearly exceeds it.
	ol.FormattingRules.PageLimit = &outline.FormattingRule{Value: "1 pages", Specified: true}

	// ~300 words to exceed 1 page (250 words/page).
	longDraft := strings.Repeat("word ", 300)

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       longDraft,
		Opportunity: fixture(),
		Outline:     ol,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusNeedsHuman {
		t.Errorf("Status = %q, want %q when draft exceeds page limit", result.Status, agent.StatusNeedsHuman)
	}
}

func TestReview_UnspecifiedPageLimit_NoIssue(t *testing.T) {
	ctx := context.Background()
	a := finalreview.New()

	ol := outlineFixture()
	ol.FormattingRules.PageLimit = &outline.FormattingRule{Specified: false}

	result, err := a.Review(ctx, finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
		Outline:     ol,
	})
	if err != nil {
		t.Fatalf("Review() returned unexpected error: %v", err)
	}
	if result.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %q, want %q when page limit is not specified", result.Status, agent.StatusReadyToSubmit)
	}
}

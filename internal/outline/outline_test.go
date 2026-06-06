package outline

import (
	"context"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/opportunity"
)

var testTime = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

// baseOpportunity returns a minimal but valid Opportunity for testing.
func baseOpportunity() *opportunity.Opportunity {
	return &opportunity.Opportunity{
		ID:               "TEST-001",
		Title:            "IT Systems Design Services",
		SolicitationNum:  "SOL-2026-TEST-001",
		Agency:           "Department of Defense",
		Office:           "Office of the CIO",
		PostedDate:       testTime,
		ResponseDeadline: testTime.Add(30 * 24 * time.Hour),
		NAICSCode:        "541512",
		NAICSDescription: "Computer Systems Design Services",
		SetAsideCode:     "",
		Description:      "Provide IT systems design and integration services.",
		Type:             "Solicitation",
		URL:              "https://sam.gov/test/001",
		CreatedAt:        testTime,
		UpdatedAt:        testTime,
	}
}

// TestOutlineAgent_HappyPath verifies the agent returns a valid Outline and success result.
func TestOutlineAgent_HappyPath(t *testing.T) {
	ctx := context.Background()
	a := New()

	outline, result, err := a.Run(ctx, baseOpportunity())

	if err != nil {
		t.Fatalf("Run() returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Run() returned nil result")
	}
	if result.Status != agent.StatusSuccess {
		t.Errorf("Status = %q, want %q", result.Status, agent.StatusSuccess)
	}
	if result.AgentName != agentName {
		t.Errorf("AgentName = %q, want %q", result.AgentName, agentName)
	}
	const wantSummary = "generated 5 sections for opportunity TEST-001"
	if result.Summary != wantSummary {
		t.Errorf("Summary = %q, want %q", result.Summary, wantSummary)
	}
	if outline == nil {
		t.Fatal("Run() returned nil outline on success")
	}
	if outline.OpportunityID != "TEST-001" {
		t.Errorf("OpportunityID = %q, want %q", outline.OpportunityID, "TEST-001")
	}
	if len(outline.Sections) == 0 {
		t.Error("Outline must contain at least one section")
	}
	if outline.GeneratedAt.IsZero() {
		t.Error("GeneratedAt must be set")
	}
}

// TestOutlineAgent_NilOpportunity verifies the agent returns a failed result and nil outline.
func TestOutlineAgent_NilOpportunity(t *testing.T) {
	ctx := context.Background()
	a := New()

	outline, result, err := a.Run(ctx, nil)

	if err == nil {
		t.Fatal("Run() should return an error when opportunity is nil")
	}
	if result == nil {
		t.Fatal("Run() should still return a Result even on failure")
	}
	if result.Status != agent.StatusFailed {
		t.Errorf("Status = %q, want %q", result.Status, agent.StatusFailed)
	}
	if result.AgentName != agentName {
		t.Errorf("AgentName = %q, want %q", result.AgentName, agentName)
	}
	const wantSummary = "opportunity must not be nil"
	if result.Summary != wantSummary {
		t.Errorf("Summary = %q, want %q", result.Summary, wantSummary)
	}
	if outline != nil {
		t.Error("Run() should return nil outline on failure")
	}
}

// TestBuildSections_Basesections verifies that the five standard federal proposal
// volumes are always present, even for a sparse opportunity.
func TestBuildSections_BaseSections(t *testing.T) {
	opp := baseOpportunity()
	opp.Description = "" // sparse: no description
	opp.SetAsideCode = ""

	sections := buildSections(opp)

	required := []string{
		"executive_summary",
		"technical_approach",
		"management_approach",
		"past_performance",
		"price_cost_volume",
	}
	ids := sectionIDs(sections)
	for _, id := range required {
		if !contains(ids, id) {
			t.Errorf("base section %q missing from sparse opportunity", id)
		}
	}
}

// TestBuildSections_SetAside verifies a small business subcontracting plan is added
// when a set-aside code is present.
func TestBuildSections_SetAside(t *testing.T) {
	opp := baseOpportunity()
	opp.SetAsideCode = "SBA"

	sections := buildSections(opp)

	if !contains(sectionIDs(sections), "small_business_subcontracting") {
		t.Error("expected small_business_subcontracting section for SBA set-aside")
	}
}

// TestBuildSections_NoSetAside verifies the subcontracting plan is omitted when
// there is no set-aside, including case-insensitive "none" variants from external systems.
func TestBuildSections_NoSetAside(t *testing.T) {
	for _, code := range []string{"", "NONE", "none", "None"} {
		opp := baseOpportunity()
		opp.SetAsideCode = code

		sections := buildSections(opp)

		if contains(sectionIDs(sections), "small_business_subcontracting") {
			t.Errorf("unexpected small_business_subcontracting section for SetAsideCode=%q", code)
		}
	}
}

// TestBuildSections_KeywordsAddSections verifies that description keywords trigger
// the correct conditional sections.
func TestBuildSections_KeywordsAddSections(t *testing.T) {
	cases := []struct {
		name        string
		description string
		wantSection string
	}{
		{
			name:        "key personnel",
			description: "Contractor shall provide key personnel as defined in Section H.",
			wantSection: "key_personnel",
		},
		{
			name:        "quality assurance",
			description: "A Quality Assurance Surveillance Plan (QASP) is required.",
			wantSection: "quality_assurance",
		},
		{
			name:        "security clearance",
			description: "Personnel must hold an active Secret clearance.",
			wantSection: "security_plan",
		},
		{
			name:        "transition scenario",
			description: "Offeror must describe its transition plan from the incumbent contractor.",
			wantSection: "transition_plan",
		},
		{
			name:        "data rights",
			description: "All technical data produced under this contract is subject to unlimited data rights.",
			wantSection: "data_rights",
		},
		{
			name:        "ip keyword",
			description: "All IP generated under this contract is property of the government.",
			wantSection: "data_rights",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opp := baseOpportunity()
			opp.Description = tc.description

			sections := buildSections(opp)

			if !contains(sectionIDs(sections), tc.wantSection) {
				t.Errorf("expected section %q when description is %q", tc.wantSection, tc.description)
			}
		})
	}
}

// TestBuildSections_SectionRationaleSet verifies every returned section has a non-empty
// rationale so downstream agents and the UI can explain the structure.
func TestBuildSections_SectionRationaleSet(t *testing.T) {
	opp := baseOpportunity()
	opp.SetAsideCode = "8A"
	opp.Description = "key personnel required. quality assurance plan mandatory."

	for _, s := range buildSections(opp) {
		if s.Rationale == "" {
			t.Errorf("section %q has empty Rationale", s.ID)
		}
	}
}

// TestExtractFormattingRules_NothingSpecified verifies that an opportunity with no
// formatting language returns all fields as Specified=false.
func TestExtractFormattingRules_NothingSpecified(t *testing.T) {
	opp := baseOpportunity()
	opp.Description = "Provide IT systems design and integration services."

	rules := extractFormattingRules(opp)

	if rules == nil {
		t.Fatal("extractFormattingRules() returned nil")
	}
	for _, f := range []*FormattingRule{rules.PageLimit, rules.Font, rules.Margins, rules.LineSpacing, rules.FileFormat} {
		if f == nil {
			t.Fatal("all FormattingRule fields must be non-nil")
		}
		if f.Specified {
			t.Errorf("expected Specified=false for sparse description, got Value=%q", f.Value)
		}
	}
	if len(rules.RequiredForms) != 0 {
		t.Errorf("expected no RequiredForms, got %v", rules.RequiredForms)
	}
}

// TestExtractFormattingRules_PageLimit verifies common page-limit phrasings are extracted.
func TestExtractFormattingRules_PageLimit(t *testing.T) {
	cases := []struct {
		desc      string
		wantValue string
	}{
		{"Proposals shall not to exceed 25 pages in length.", "25 pages"},
		{"Submissions are limited to no more than 10 pages.", "10 pages"},
		{"The technical volume is limited to 15 pages.", "15 pages"},
		{"A maximum of 30 pages is allowed.", "30 pages"},
	}
	for _, tc := range cases {
		t.Run(tc.wantValue, func(t *testing.T) {
			opp := baseOpportunity()
			opp.Description = tc.desc
			rules := extractFormattingRules(opp)
			if !rules.PageLimit.Specified {
				t.Fatalf("PageLimit.Specified=false for %q", tc.desc)
			}
			if rules.PageLimit.Value != tc.wantValue {
				t.Errorf("PageLimit.Value = %q, want %q", rules.PageLimit.Value, tc.wantValue)
			}
		})
	}
}

// TestExtractFormattingRules_Font verifies font extraction.
func TestExtractFormattingRules_Font(t *testing.T) {
	cases := []struct {
		desc      string
		wantValue string
	}{
		{"Text must use Arial 12-point font.", "Arial 12-point"},
		{"Use Times New Roman 11-point throughout.", "Times New Roman 11-point"},
		{"Calibri 12 point is required.", "Calibri 12-point"},
	}
	for _, tc := range cases {
		t.Run(tc.wantValue, func(t *testing.T) {
			opp := baseOpportunity()
			opp.Description = tc.desc
			rules := extractFormattingRules(opp)
			if !rules.Font.Specified {
				t.Fatalf("Font.Specified=false for %q", tc.desc)
			}
			if rules.Font.Value != tc.wantValue {
				t.Errorf("Font.Value = %q, want %q", rules.Font.Value, tc.wantValue)
			}
		})
	}
}

// TestExtractFormattingRules_Margins verifies margin extraction.
func TestExtractFormattingRules_Margins(t *testing.T) {
	opp := baseOpportunity()
	opp.Description = "All pages must have 1-inch margins."
	rules := extractFormattingRules(opp)
	if !rules.Margins.Specified {
		t.Fatal("Margins.Specified=false")
	}
	if rules.Margins.Value != "1-inch margins" {
		t.Errorf("Margins.Value = %q, want %q", rules.Margins.Value, "1-inch margins")
	}
}

// TestExtractFormattingRules_LineSpacing verifies line spacing extraction.
func TestExtractFormattingRules_LineSpacing(t *testing.T) {
	cases := []struct {
		desc      string
		wantValue string
	}{
		{"The proposal must be single-spaced.", "single-spaced"},
		{"All text shall be double-spaced.", "double-spaced"},
		{"Use 1.5-spaced text.", "1.5-spaced"},
	}
	for _, tc := range cases {
		t.Run(tc.wantValue, func(t *testing.T) {
			opp := baseOpportunity()
			opp.Description = tc.desc
			rules := extractFormattingRules(opp)
			if !rules.LineSpacing.Specified {
				t.Fatalf("LineSpacing.Specified=false for %q", tc.desc)
			}
			if rules.LineSpacing.Value != tc.wantValue {
				t.Errorf("LineSpacing.Value = %q, want %q", rules.LineSpacing.Value, tc.wantValue)
			}
		})
	}
}

// TestExtractFormattingRules_FileFormat verifies file format extraction.
func TestExtractFormattingRules_FileFormat(t *testing.T) {
	cases := []struct {
		desc      string
		wantValue string
	}{
		{"Submit the proposal in PDF format.", "PDF"},
		{"Proposals submitted as pdf will be accepted.", "PDF"},
		{"Submit as Microsoft Word.", "Microsoft Word"},
		{"Submissions must be in .docx format.", "Microsoft Word"},
		{"Older .doc files are also accepted.", "Microsoft Word"},
	}
	for _, tc := range cases {
		t.Run(tc.wantValue, func(t *testing.T) {
			opp := baseOpportunity()
			opp.Description = tc.desc
			rules := extractFormattingRules(opp)
			if !rules.FileFormat.Specified {
				t.Fatalf("FileFormat.Specified=false for %q", tc.desc)
			}
			if rules.FileFormat.Value != tc.wantValue {
				t.Errorf("FileFormat.Value = %q, want %q", rules.FileFormat.Value, tc.wantValue)
			}
		})
	}
}

// TestExtractFormattingRules_RequiredForms verifies government form numbers are extracted
// and deduplicated.
func TestExtractFormattingRules_RequiredForms(t *testing.T) {
	opp := baseOpportunity()
	opp.Description = "Offeror must submit SF-330, SF 1449, and DD Form 254. SF-330 is required again."

	rules := extractFormattingRules(opp)

	want := []string{"SF-330", "SF-1449", "DD-254"}
	if len(rules.RequiredForms) != len(want) {
		t.Fatalf("RequiredForms = %v, want %v", rules.RequiredForms, want)
	}
	for i, f := range rules.RequiredForms {
		if f != want[i] {
			t.Errorf("RequiredForms[%d] = %q, want %q", i, f, want[i])
		}
	}
}

// TestExtractFormattingRules_Partial verifies that when only some rules are stated,
// the stated ones are extracted and the rest remain Specified=false.
func TestExtractFormattingRules_Partial(t *testing.T) {
	opp := baseOpportunity()
	// Only page limit and file format are stated; font/margins/spacing are silent.
	opp.Description = "Proposals shall not to exceed 20 pages and must be submitted in PDF format."

	rules := extractFormattingRules(opp)

	if !rules.PageLimit.Specified || rules.PageLimit.Value != "20 pages" {
		t.Errorf("PageLimit = {%v, %q}, want {true, \"20 pages\"}", rules.PageLimit.Specified, rules.PageLimit.Value)
	}
	if !rules.FileFormat.Specified || rules.FileFormat.Value != "PDF" {
		t.Errorf("FileFormat = {%v, %q}, want {true, \"PDF\"}", rules.FileFormat.Specified, rules.FileFormat.Value)
	}
	for name, f := range map[string]*FormattingRule{
		"Font": rules.Font, "Margins": rules.Margins, "LineSpacing": rules.LineSpacing,
	} {
		if f.Specified {
			t.Errorf("%s should be unspecified in partial description", name)
		}
	}
}

// TestOutlineAgent_FormattingRulesNonNil verifies the Outline returned by Run always
// has a non-nil FormattingRules, even for a sparse opportunity.
func TestOutlineAgent_FormattingRulesNonNil(t *testing.T) {
	opp := baseOpportunity()
	opp.Description = ""

	outline, _, err := New().Run(context.Background(), opp)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if outline.FormattingRules == nil {
		t.Error("Outline.FormattingRules must never be nil")
	}
}

// sectionIDs extracts the ID field from a slice of sections.
func sectionIDs(sections []Section) []string {
	ids := make([]string, len(sections))
	for i, s := range sections {
		ids[i] = s.ID
	}
	return ids
}

// contains reports whether slice contains target.
func contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// Package outline implements the Outline agent for Zone 2 of the Kaimi pipeline.
//
// The Outline agent is responsible for generating a structured proposal outline
// from a selected Opportunity. It is the first agent triggered by the Manager
// after a human selects an opportunity from the queue.
package outline

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/opportunity"
)

const agentName = "outline"

// Section represents a single required section in a federal proposal.
type Section struct {
	ID        string // short identifier, e.g. "technical_approach"
	Title     string // display title, e.g. "Technical Approach"
	Required  bool   // whether this section is mandatory for this opportunity
	Rationale string // why this section was included, derived from opportunity data
}

// FormattingRule represents one formatting requirement extracted from the solicitation.
// Specified is false when the solicitation is silent on this requirement — downstream
// agents must not invent a value when Specified is false.
type FormattingRule struct {
	Value     string // stated value; empty when Specified is false
	Specified bool   // true only if the solicitation explicitly states this requirement
}

// FormattingRules captures the formatting requirements extracted from the solicitation.
// Every field is non-nil so callers can always check Specified without a nil guard.
type FormattingRules struct {
	PageLimit     *FormattingRule // e.g. "25 pages per volume"
	Font          *FormattingRule // e.g. "Arial 12-point"
	Margins       *FormattingRule // e.g. "1-inch on all sides"
	LineSpacing   *FormattingRule // e.g. "single-spaced"
	FileFormat    *FormattingRule // e.g. "PDF"
	RequiredForms []string        // government form numbers, e.g. ["SF-330", "SF-1449"]
}

// Outline is the structured output produced by the Outline agent.
// It is the input the next agent in Zone 2 consumes.
type Outline struct {
	OpportunityID   string
	Title           string // opportunity title, carried for context
	Sections        []Section
	FormattingRules *FormattingRules
	GeneratedAt     time.Time
}

// Agent is the Outline agent.
type Agent struct{}

// New creates a new Outline agent.
func New() *Agent {
	return &Agent{}
}

// Run takes a selected Opportunity and produces a structured Outline and a Result.
//
// Returns a non-nil Outline on success. Returns a failed Result (and nil Outline)
// on unrecoverable errors. Sparse opportunities get a best-effort outline rather than
// a failure.
//
// TODO(phase-3): Replace buildSections with a Gemini call once LLM integration lands.
func (a *Agent) Run(ctx context.Context, opp *opportunity.Opportunity) (*Outline, *agent.Result, error) {
	if opp == nil {
		return nil, &agent.Result{
			AgentName: agentName,
			Status:    agent.StatusFailed,
			Summary:   "opportunity must not be nil",
		}, fmt.Errorf("outline agent: opportunity must not be nil")
	}

	sections := buildSections(opp)
	formatting := extractFormattingRules(opp)

	outline := &Outline{
		OpportunityID:   opp.ID,
		Title:           opp.Title,
		Sections:        sections,
		FormattingRules: formatting,
		GeneratedAt:     time.Now().UTC(),
	}

	result := &agent.Result{
		AgentName: agentName,
		Status:    agent.StatusSuccess,
		Summary:   fmt.Sprintf("generated %d sections for opportunity %s", len(sections), opp.ID),
		OutputRef: "", // TODO(phase-3): set to Google Doc URL once KAI-5 is built
	}

	return outline, result, nil
}

// Regexes for extracting formatting values from solicitation text.
// Compiled once at package level for efficiency.
var (
	// page limits: "not to exceed 25 pages", "no more than 10 pages", "limited to 15 pages"
	pageLimitRE = regexp.MustCompile(`(?i)(?:not to exceed|no more than|limited to|maximum of)\s+(\d+)\s+pages?`)

	// fonts: "Arial 12-point", "Times New Roman 11-point", "Calibri 12 point"
	fontRE = regexp.MustCompile(`(?i)(arial|times new roman|calibri|courier new)\s+(\d+)\s*-?\s*point`)

	// margins: "1-inch margins", "0.5-inch margins", "minimum 1-inch margin"
	marginRE = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*-?\s*inch\s+(?:minimum\s+)?margins?`)

	// line spacing: "single-spaced", "double-spaced", "1.5-spaced"
	spacingRE = regexp.MustCompile(`(?i)(single|double|1\.5)\s*-?\s*spaced`)

	// file format: "submitted as PDF", "in PDF format", "Microsoft Word", ".doc", ".docx"
	// .docx? requires a non-word preceding char (space, start, punctuation) so it does
	// not false-positive on filenames like "proposal.docx".
	fileFormatRE = regexp.MustCompile(`(?i)\b(pdf|microsoft word)\b|(?:^|[\s(,])(\.docx?)\b`)

	// government forms: "SF-330", "SF 1449", "DD Form 254", "DD-1423"
	formRE = regexp.MustCompile(`(?i)\b(SF|DD(?:\s+Form)?)\s*-?\s*(\d+)\b`)

	// ip as a standalone abbreviation: matches "IP." "(IP)" "IP rights" but not "zip" or "tip"
	ipRE = regexp.MustCompile(`(?i)\bip\b`)
)

// extractFormattingRules parses the opportunity description for stated formatting
// requirements. Fields not mentioned in the solicitation are returned with
// Specified=false and an empty Value — callers must not invent defaults for these.
func extractFormattingRules(opp *opportunity.Opportunity) *FormattingRules {
	desc := opp.Description

	rules := &FormattingRules{
		PageLimit:   unspecified(),
		Font:        unspecified(),
		Margins:     unspecified(),
		LineSpacing: unspecified(),
		FileFormat:  unspecified(),
	}

	if m := pageLimitRE.FindStringSubmatch(desc); m != nil {
		rules.PageLimit = specified(m[1] + " pages")
	}

	if m := fontRE.FindStringSubmatch(desc); m != nil {
		rules.Font = specified(m[1] + " " + m[2] + "-point")
	}

	if m := marginRE.FindStringSubmatch(desc); m != nil {
		rules.Margins = specified(m[1] + "-inch margins")
	}

	if m := spacingRE.FindStringSubmatch(desc); m != nil {
		rules.LineSpacing = specified(m[1] + "-spaced")
	}

	if m := fileFormatRE.FindStringSubmatch(desc); m != nil {
		val := strings.ToLower(m[1])
		if val == "" {
			val = strings.ToLower(m[2])
		}
		canonical := "PDF"
		if strings.HasPrefix(val, ".doc") || val == "microsoft word" {
			canonical = "Microsoft Word"
		}
		rules.FileFormat = specified(canonical)
	}

	// Collect all government form numbers mentioned, deduplicated.
	seen := map[string]bool{}
	for _, m := range formRE.FindAllStringSubmatch(desc, -1) {
		// m[1] may be "DD Form" — keep only the first word (the prefix).
		prefix := strings.ToUpper(strings.Fields(m[1])[0])
		form := prefix + "-" + m[2]
		if !seen[form] {
			rules.RequiredForms = append(rules.RequiredForms, form)
			seen[form] = true
		}
	}

	return rules
}

// specified returns a FormattingRule with a known value.
func specified(value string) *FormattingRule {
	return &FormattingRule{Value: value, Specified: true}
}

// unspecified returns a FormattingRule marking a requirement as not stated.
func unspecified() *FormattingRule {
	return &FormattingRule{Specified: false}
}

// buildSections derives the required proposal sections from the opportunity.
//
// Uses rule-based logic against the opportunity's own fields — type, contract type,
// set-aside code, and description keywords. No section list is hardcoded; every
// inclusion is traceable back to a field value.
//
// Returns at least the five standard federal proposal volumes even for sparse input.
func buildSections(opp *opportunity.Opportunity) []Section {
	desc := strings.ToLower(opp.Description)

	sections := []Section{
		{
			ID:        "executive_summary",
			Title:     "Executive Summary",
			Required:  true,
			Rationale: "standard section for federal proposals",
		},
		{
			ID:        "technical_approach",
			Title:     "Technical Approach",
			Required:  true,
			Rationale: "standard section for federal proposals",
		},
		{
			ID:        "management_approach",
			Title:     "Management Approach",
			Required:  true,
			Rationale: "standard section for federal proposals",
		},
		{
			ID:        "past_performance",
			Title:     "Past Performance",
			Required:  true,
			Rationale: "standard section for federal proposals",
		},
		{
			ID:        "price_cost_volume",
			Title:     "Price/Cost Volume",
			Required:  true,
			Rationale: "standard section for federal proposals",
		},
	}

	// Set-aside programs require a small business subcontracting plan.
	if opp.SetAsideCode != "" && !strings.EqualFold(opp.SetAsideCode, "NONE") {
		sections = append(sections, Section{
			ID:        "small_business_subcontracting",
			Title:     "Small Business Subcontracting Plan",
			Required:  true,
			Rationale: fmt.Sprintf("required by set-aside code %q", opp.SetAsideCode),
		})
	}

	// Key personnel requirements surface in the description.
	if containsAny(desc, "key personnel", "named individual", "key staff") {
		sections = append(sections, Section{
			ID:        "key_personnel",
			Title:     "Key Personnel",
			Required:  true,
			Rationale: "opportunity description references key personnel requirements",
		})
	}

	// Quality assurance plans are often explicitly required.
	if containsAny(desc, "quality assurance", "qasp", "quality control", "qcp") {
		sections = append(sections, Section{
			ID:        "quality_assurance",
			Title:     "Quality Assurance Plan",
			Required:  true,
			Rationale: "opportunity description references quality assurance requirements",
		})
	}

	// Security and clearance requirements drive a dedicated section.
	if containsAny(desc, "secret", "clearance", "classified", "security plan") {
		sections = append(sections, Section{
			ID:        "security_plan",
			Title:     "Security Plan",
			Required:  true,
			Rationale: "opportunity description references security or clearance requirements",
		})
	}

	// Recompetes and transitions require a transition plan.
	if containsAny(desc, "transition", "recompete", "incumbent", "phase-in") {
		sections = append(sections, Section{
			ID:        "transition_plan",
			Title:     "Transition Plan",
			Required:  true,
			Rationale: "opportunity description indicates a transition or recompete scenario",
		})
	}

	// Data rights appear in technology and software contracts.
	if containsAny(desc, "data right", "intellectual property", "technical data") || ipRE.MatchString(desc) {
		sections = append(sections, Section{
			ID:        "data_rights",
			Title:     "Data Rights and Intellectual Property",
			Required:  true,
			Rationale: "opportunity description references data rights or intellectual property",
		})
	}

	return sections
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/googledocs"
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

// SectionPlanner produces the proposal section structure for an opportunity.
//
// Two implementations exist: the deterministic planner (default) derives sections
// from the opportunity and solicitation text with rules and keyword matching; the
// Gemini planner (gemini-3.5-flash) asks the model to identify the required
// sections from the solicitation. Formatting-rule extraction is deliberately NOT
// part of this interface — it stays deterministic (reliable regex) regardless of
// which planner is used.
type SectionPlanner interface {
	// PlanSections returns the ordered proposal sections for the opportunity.
	// source is the combined opportunity description plus extracted document text.
	// It must return at least one section or a non-nil error — never an empty,
	// error-free result (which would yield a silently empty outline).
	PlanSections(ctx context.Context, opp *opportunity.Opportunity, source string) ([]Section, error)
}

// deterministicPlanner is the default SectionPlanner: it derives sections from the
// opportunity and source text with the rule-based buildSections. It never errors.
type deterministicPlanner struct{}

// PlanSections implements SectionPlanner using the rule-based buildSections.
func (deterministicPlanner) PlanSections(_ context.Context, opp *opportunity.Opportunity, source string) ([]Section, error) {
	return buildSections(opp, source), nil
}

// DocsClientResolver returns the Google Docs client to use for the current draft.
// Resolving the client per draft (rather than caching one at construction) lets a
// freshly connected or changed customer Drive take effect on the very next draft
// with no restart — the cause of the boot-time-cached bug (#73).
type DocsClientResolver func(context.Context) (googledocs.Client, error)

// staticDocs wraps a fixed client in a resolver that always returns it, so the
// New/NewWithPlanner constructors keep their existing signatures and behavior.
func staticDocs(c googledocs.Client) DocsClientResolver {
	return func(context.Context) (googledocs.Client, error) { return c, nil }
}

// Agent is the Outline agent.
type Agent struct {
	resolveDocs DocsClientResolver
	planner     SectionPlanner
}

// New creates a new Outline agent that saves generated outlines to Google Docs
// via the given client. It uses the deterministic section planner — no model call.
func New(docsClient googledocs.Client) *Agent {
	return &Agent{resolveDocs: staticDocs(docsClient), planner: deterministicPlanner{}}
}

// NewWithPlanner creates an Outline agent that plans sections with the given
// SectionPlanner (e.g. the gemini-3.5-flash planner) instead of the deterministic
// rules. Formatting extraction stays deterministic either way.
func NewWithPlanner(docsClient googledocs.Client, planner SectionPlanner) *Agent {
	return &Agent{resolveDocs: staticDocs(docsClient), planner: planner}
}

// NewWithResolver creates an Outline agent that resolves its Google Docs client
// per draft via resolveDocs, instead of binding a fixed client at construction.
// This is what makes a newly connected / changed customer Drive take effect on the
// next draft without a restart (#73). A nil planner falls back to the deterministic
// planner. The resolver is invoked once per Run, inside Run's context.
func NewWithResolver(resolveDocs DocsClientResolver, planner SectionPlanner) *Agent {
	if planner == nil {
		planner = deterministicPlanner{}
	}
	return &Agent{resolveDocs: resolveDocs, planner: planner}
}

// resolveDocsClient returns the Docs client for the current draft, guarding the
// misconfigured cases (a zero-value Agent with no resolver, or a resolver that
// yields a nil client) so Run fails cleanly with a reason instead of panicking on
// a nil interface.
func (a *Agent) resolveDocsClient(ctx context.Context) (googledocs.Client, error) {
	if a.resolveDocs == nil {
		return nil, fmt.Errorf("no docs client resolver configured")
	}
	c, err := a.resolveDocs(ctx)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("docs client resolver returned a nil client")
	}
	return c, nil
}

// Run takes a selected Opportunity and produces a structured Outline and a Result.
//
// On success, the outline is saved to a Google Doc and the Result's OutputRef is
// set to the Doc's URL. Returns a non-nil Outline on success. Returns a failed
// Result on unrecoverable errors.
//
// If the opportunity itself is invalid, returns a nil Outline — there is nothing
// to save. If the Outline was generated but the Google Doc could not be created,
// the Outline is still returned alongside the failed Result so the caller can
// retry or persist it elsewhere — the outline must never be lost silently.
//
// Sparse opportunities get a best-effort outline rather than a failure.
//
// documents maps a solicitation document filename to its extracted text
// (populated by the Manager's ingest stage). When present, its text is scanned
// alongside the opportunity description for required sections and formatting
// rules — the real Section L/M instructions live in the RFP documents, not the
// thin SAM.gov summary. A nil/empty map reproduces the previous behavior exactly.
//
// The section structure comes from the configured SectionPlanner (deterministic
// by default, gemini-3.5-flash when constructed with NewWithPlanner); formatting
// rules are always extracted deterministically.
func (a *Agent) Run(ctx context.Context, opp *opportunity.Opportunity, documents map[string]string) (*Outline, *agent.Result, error) {
	if opp == nil {
		return nil, &agent.Result{
			AgentName: agentName,
			Status:    agent.StatusFailed,
			Summary:   "opportunity must not be nil",
		}, fmt.Errorf("outline agent: opportunity must not be nil")
	}

	planner := a.planner
	if planner == nil {
		// Defensive default for zero-value Agents (e.g. constructed via struct
		// literal in a test) so Run never dereferences a nil planner.
		planner = deterministicPlanner{}
	}

	source := combinedSource(opp, documents)
	sections, err := planner.PlanSections(ctx, opp, source)
	if err != nil {
		return nil, &agent.Result{
			AgentName:   agentName,
			Status:      agent.StatusFailed,
			NoticeID:    opp.ID,
			Summary:     fmt.Sprintf("failed to plan sections for %s", opp.ID),
			Error:       fmt.Sprintf("planning sections: %v", err),
			CompletedAt: time.Now().UTC(),
		}, fmt.Errorf("outline agent: plan sections: %w", err)
	}
	// A planner that returns no sections without erroring would yield a silently
	// empty outline; treat it as a failure rather than save an empty Doc.
	if len(sections) == 0 {
		return nil, &agent.Result{
			AgentName:   agentName,
			Status:      agent.StatusFailed,
			NoticeID:    opp.ID,
			Summary:     fmt.Sprintf("planner returned no sections for %s", opp.ID),
			Error:       "section planner returned zero sections",
			CompletedAt: time.Now().UTC(),
		}, fmt.Errorf("outline agent: planner returned zero sections for %s", opp.ID)
	}
	formatting := extractFormattingRules(source)

	outline := &Outline{
		OpportunityID:   opp.ID,
		Title:           opp.Title,
		Sections:        sections,
		FormattingRules: formatting,
		GeneratedAt:     time.Now().UTC(),
	}

	// Resolve the Docs client now, per draft, so a Drive connected or re-targeted
	// since this Agent was constructed is honored immediately, with no restart (#73).
	docsClient, err := a.resolveDocsClient(ctx)
	if err != nil {
		// Don't lose the outline: return it alongside the failed Result.
		return outline, &agent.Result{
			AgentName:   agentName,
			Status:      agent.StatusFailed,
			NoticeID:    opp.ID,
			Summary:     fmt.Sprintf("generated outline for %s but failed to resolve the Docs client", opp.ID),
			Error:       fmt.Sprintf("resolving docs client: %v", err),
			CompletedAt: time.Now().UTC(),
		}, fmt.Errorf("outline agent: resolve docs client: %w", err)
	}

	created, err := docsClient.CreateDoc(ctx, googledocs.Document{
		Title:    outline.Title,
		Sections: toDocSections(outline),
	})
	if err != nil {
		// Don't lose the outline silently: return it alongside the failed Result
		// so the caller can retry Doc creation or persist the outline elsewhere.
		return outline, &agent.Result{
			AgentName:   agentName,
			Status:      agent.StatusFailed,
			NoticeID:    opp.ID,
			Summary:     fmt.Sprintf("generated outline for %s but failed to create Google Doc", opp.ID),
			Error:       fmt.Sprintf("creating google doc: %v", err),
			CompletedAt: time.Now().UTC(),
		}, fmt.Errorf("outline agent: create google doc: %w", err)
	}

	result := &agent.Result{
		AgentName: agentName,
		Status:    agent.StatusSuccess,
		NoticeID:  opp.ID,
		Summary:   fmt.Sprintf("generated %d sections for opportunity %s", len(sections), opp.ID),
		OutputRef: created.URL,
		Flags: map[string]string{
			"doc_id":        created.ID,
			"section_count": strconv.Itoa(len(sections)),
		},
		CompletedAt: time.Now().UTC(),
	}

	return outline, result, nil
}

// toDocSections renders an Outline's sections and formatting rules into the flat
// heading/body shape the googledocs client writes to a Doc.
func toDocSections(o *Outline) []googledocs.DocSection {
	docSections := make([]googledocs.DocSection, 0, len(o.Sections)+1)

	for _, sec := range o.Sections {
		required := "Required: no"
		if sec.Required {
			required = "Required: yes"
		}
		docSections = append(docSections, googledocs.DocSection{
			Heading: sec.Title,
			Body:    required + "\n" + sec.Rationale,
		})
	}

	docSections = append(docSections, googledocs.DocSection{
		Heading: "Formatting Requirements",
		Body:    formatFormattingRules(o.FormattingRules),
	})

	return docSections
}

// formatFormattingRules renders each formatting requirement as one line: its
// stated value when Specified, or an explicit "not specified" placeholder
// otherwise — never inventing a value the solicitation didn't state.
func formatFormattingRules(f *FormattingRules) string {
	lines := []string{
		formatRuleLine("Page limit", f.PageLimit),
		formatRuleLine("Font", f.Font),
		formatRuleLine("Margins", f.Margins),
		formatRuleLine("Line spacing", f.LineSpacing),
		formatRuleLine("File format", f.FileFormat),
	}

	if len(f.RequiredForms) > 0 {
		lines = append(lines, "Required forms: "+strings.Join(f.RequiredForms, ", "))
	} else {
		lines = append(lines, "Required forms: Not specified in solicitation")
	}

	return strings.Join(lines, "\n")
}

// formatRuleLine renders a single formatting rule as "<label>: <value>", using
// "Not specified in solicitation" when the rule's value was not stated.
func formatRuleLine(label string, rule *FormattingRule) string {
	if rule.Specified {
		return label + ": " + rule.Value
	}
	return label + ": Not specified in solicitation"
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

// combinedSource concatenates the opportunity description with the extracted text
// of every ingested solicitation document, in a stable filename order, to form the
// single body of text the deterministic parsers scan. With no documents it returns
// just the description, so behavior is unchanged.
func combinedSource(opp *opportunity.Opportunity, documents map[string]string) string {
	if len(documents) == 0 {
		return opp.Description
	}
	names := make([]string, 0, len(documents))
	for name := range documents {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder
	sb.WriteString(opp.Description)
	for _, name := range names {
		sb.WriteString("\n")
		sb.WriteString(documents[name])
	}
	return sb.String()
}

// extractFormattingRules parses the solicitation source text for stated formatting
// requirements. Fields not mentioned in the solicitation are returned with
// Specified=false and an empty Value — callers must not invent defaults for these.
func extractFormattingRules(desc string) *FormattingRules {
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
// source is the combined solicitation text (description plus any ingested document
// text) scanned for the keyword-driven optional sections.
func buildSections(opp *opportunity.Opportunity, source string) []Section {
	desc := strings.ToLower(source)

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

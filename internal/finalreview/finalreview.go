package finalreview

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
)

const agentName = "final-review"

// wordsPerPage is the assumed density used for page-count estimation.
const wordsPerPage = 250

// Input holds everything the Final Review agent needs to do its job.
//
// Draft is the proposal text approved by the human reviewer. Opportunity is
// the source federal opportunity, used to verify deadline and context.
// Outline is optional; when nil, section and form checks are skipped so
// existing callers that don't have an Outline are not broken.
type Input struct {
	// Draft is the human-approved proposal text. Must not be empty.
	Draft string

	// Opportunity is the federal opportunity this proposal responds to.
	// Must not be nil.
	Opportunity *opportunity.Opportunity

	// Outline is the structured outline produced by the Outline agent.
	// When nil, the required_section, required_form, and page_limit checks
	// are skipped — only deadline and must_have checks run.
	Outline *outline.Outline

	// Documents maps a solicitation document filename to its extracted text
	// (populated by the Manager's ingest stage). It is threaded through now so the
	// LLM compliance pass (#164) can vet the draft against the full solicitation;
	// the current deterministic checks do not use it.
	Documents map[string]string
}

// Agent is the Final Review agent.
//
// It performs automated pre-submission checks and returns an AgentResult
// indicating whether the proposal is ready for a human to submit. With a
// ComplianceChecker configured (NewWithComplianceChecker), it additionally runs
// an LLM compliance pass that vets the draft against the full solicitation
// document set; without one (New) it runs only the fast deterministic checks.
type Agent struct {
	checker ComplianceChecker // optional LLM compliance pass; nil = deterministic checks only
}

// New returns a Final Review agent that runs only the deterministic checks.
func New() *Agent {
	return &Agent{}
}

// NewWithComplianceChecker returns a Final Review agent that runs the deterministic
// checks as a fast pre-filter and then an LLM compliance pass (via checker) over
// the full solicitation documents. The deterministic checks always run first.
func NewWithComplianceChecker(checker ComplianceChecker) *Agent {
	return &Agent{checker: checker}
}

// Review runs the final automated checks on an approved proposal draft.
//
// It validates that the draft is non-empty and the opportunity exists, then
// runs five checks: deadline, must_have, required_section, required_form,
// and page_limit. All issues are collected into AgentResult.Flags.
//
// Review returns an error only for invalid input (nil opportunity, empty
// draft). Soft failures are expressed through the AgentResult status so the
// Manager can route them appropriately without crashing the pipeline.
//
// The agent NEVER submits anything. StatusReadyToSubmit is a signal to a human.
func (a *Agent) Review(ctx context.Context, in Input) (*agent.Result, error) {
	// Validate required inputs at the system boundary.
	if in.Opportunity == nil {
		return nil, fmt.Errorf("final-review: opportunity must not be nil")
	}
	if in.Draft == "" {
		return nil, fmt.Errorf("final-review: draft must not be empty")
	}

	// Expired deadline is an unrecoverable hard failure — the proposal cannot
	// be submitted regardless of content quality.
	if err := checkDeadline(in.Opportunity); err != nil {
		return &agent.Result{
			AgentName:   agentName,
			Status:      agent.StatusFailed,
			NoticeID:    in.Opportunity.ID,
			Summary:     fmt.Sprintf("proposal cannot be submitted: %v", err),
			CompletedAt: time.Now().UTC(),
		}, nil
	}

	// Collect all soft issues found during content checks.
	var issues []string

	issues = append(issues, checkMustHave(in.Draft, in.Opportunity.Requirements)...)

	if in.Outline != nil {
		issues = append(issues, checkRequiredSections(in.Draft, in.Outline.Sections)...)
		issues = append(issues, checkRequiredForms(in.Draft, in.Outline.FormattingRules)...)
		issues = append(issues, checkPageLimit(in.Draft, in.Outline.FormattingRules)...)
	}

	// LLM compliance pass (optional). Runs after the deterministic pre-filter
	// whenever a checker is configured. Grounding depends on what's available:
	// the full solicitation documents when the ingest stage provided them,
	// otherwise the opportunity's own summary (description + requirements) — so
	// the review never silently degrades to string checks alone (issue #264).
	// Any unmet requirement it finds — or a failure to run the check — becomes
	// an issue, routing the proposal to needs_human.
	if a.checker != nil {
		issues = append(issues, a.runCompliance(ctx, in)...)
	}

	flags := buildFlags(issues)

	if len(issues) > 0 {
		return &agent.Result{
			AgentName:   agentName,
			Status:      agent.StatusNeedsHuman,
			NoticeID:    in.Opportunity.ID,
			Summary:     fmt.Sprintf("%d issue(s) found; human review required before submission", len(issues)),
			Flags:       flags,
			CompletedAt: time.Now().UTC(),
		}, nil
	}

	return &agent.Result{
		AgentName:   agentName,
		Status:      agent.StatusReadyToSubmit,
		NoticeID:    in.Opportunity.ID,
		Summary:     "all automated checks passed; proposal is ready for human submission",
		Flags:       flags,
		CompletedAt: time.Now().UTC(),
	}, nil
}

// checkDeadline returns an error if the opportunity's response deadline has
// already passed. A proposal submitted after the deadline is invalid.
func checkDeadline(opp *opportunity.Opportunity) error {
	if opp.ResponseDeadline.Before(time.Now()) {
		return fmt.Errorf("response deadline %s has passed",
			opp.ResponseDeadline.Format(time.DateOnly))
	}
	return nil
}

// checkMustHave verifies each must-have requirement is addressed in the draft.
// Any requirement the matcher cannot confirm is reported as a must_have issue.
// Matching is term-overlap (RequirementAddressed), not verbatim: a verbatim
// full-phrase check falsely flagged paraphrases — the tester-reported gate
// block in issue #262 — because drafts restate requirements in their own words.
func checkMustHave(draft string, requirements []string) []string {
	draftLower := strings.ToLower(draft)
	var issues []string
	for _, req := range requirements {
		if !RequirementAddressed(draftLower, req) {
			issues = append(issues, fmt.Sprintf(
				"[must_have] requirement %q not addressed in draft", req,
			))
		}
	}
	return issues
}

// requirementStopwords are common words dropped before term matching so the
// signal comes from the requirement's meaningful terms, not filler.
var requirementStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "must": true,
	"shall": true, "will": true, "have": true, "from": true, "that": true,
	"this": true, "any": true, "all": true, "are": true, "per": true,
	"including": true, "provide": true, "required": true,
}

// RequirementAddressed reports whether the draft plausibly addresses a
// requirement. A verbatim full-phrase match falsely flags paraphrases as
// missing; instead this scores the overlap of the requirement's significant
// terms against the draft, comparing stems so "authorization" is satisfied by
// "authorized". A requirement counts as addressed when at least two-thirds of
// its significant terms appear — lenient enough to tolerate a synonym swap,
// strict enough not to match on noise. The draft must already be lowercased.
//
// This is the single source of truth for must-have matching: the Final Review
// must_have check and the gate's criteria grid (zone2view) both use it, so the
// two can never disagree (issue #262).
func RequirementAddressed(draftLower, requirement string) bool {
	terms := significantRequirementTerms(requirement)
	if len(terms) == 0 {
		// No meaningful terms (e.g. a requirement of only stopwords): fall back
		// to the whole-phrase check rather than claiming a spurious match.
		return strings.Contains(draftLower, strings.ToLower(strings.TrimSpace(requirement)))
	}
	hits := 0
	for _, term := range terms {
		if strings.Contains(draftLower, stemTerm(term)) {
			hits++
		}
	}
	return hits*3 >= len(terms)*2
}

// significantRequirementTerms splits a requirement into lowercased, meaningful
// terms: alphanumeric runs of length >= 3 that are not stopwords.
func significantRequirementTerms(requirement string) []string {
	var terms []string
	for _, field := range strings.FieldsFunc(strings.ToLower(requirement), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		if len(field) < 3 || requirementStopwords[field] {
			continue
		}
		terms = append(terms, field)
	}
	return terms
}

// stemTerm trims a few common English suffixes so inflected forms match a shared
// stem ("authorization"/"authorized" -> "author", "modernization"/"modernize"
// -> "modern"). Longest suffixes are checked first; the length guard keeps short
// words intact.
func stemTerm(term string) string {
	for _, suf := range []string{"ization", "isation", "ation", "izing", "ized", "izes", "ing", "ed", "es", "s"} {
		if len(term) > len(suf)+2 && strings.HasSuffix(term, suf) {
			return term[:len(term)-len(suf)]
		}
	}
	return term
}

// checkRequiredSections verifies every Required section from the outline has
// a matching title (case-insensitive substring) in the draft.
func checkRequiredSections(draft string, sections []outline.Section) []string {
	draftLower := strings.ToLower(draft)
	var issues []string
	for _, sec := range sections {
		if !sec.Required {
			continue
		}
		if !strings.Contains(draftLower, strings.ToLower(sec.Title)) {
			issues = append(issues, fmt.Sprintf(
				"[required_section] section %q not found in draft", sec.Title,
			))
		}
	}
	return issues
}

// checkRequiredForms confirms each form number listed in FormattingRules is
// acknowledged somewhere in the draft text.
func checkRequiredForms(draft string, rules *outline.FormattingRules) []string {
	if rules == nil {
		return nil
	}
	draftLower := strings.ToLower(draft)
	var issues []string
	for _, form := range rules.RequiredForms {
		if !strings.Contains(draftLower, strings.ToLower(form)) {
			issues = append(issues, fmt.Sprintf(
				"[required_form] form %q not acknowledged in draft", form,
			))
		}
	}
	return issues
}

// checkPageLimit estimates the draft's page count at wordsPerPage words/page
// and flags it when the draft exceeds the solicitation's stated limit.
// A FormattingRule with Specified=false is silently ignored.
func checkPageLimit(draft string, rules *outline.FormattingRules) []string {
	if rules == nil || rules.PageLimit == nil || !rules.PageLimit.Specified {
		return nil
	}

	limit, ok := parsePageCount(rules.PageLimit.Value)
	if !ok || limit <= 0 {
		return nil
	}

	wordCount := len(strings.Fields(draft))
	estimatedPages := (wordCount + wordsPerPage - 1) / wordsPerPage // ceiling division

	if estimatedPages > limit {
		return []string{fmt.Sprintf(
			"[page_limit] draft is ~%d page(s) but limit is %d page(s); reduce length in draft",
			estimatedPages, limit,
		)}
	}
	return nil
}

// parsePageCount extracts the integer page count from a string like "25 pages" or "1 pages".
// Returns (0, false) when the value cannot be parsed.
func parsePageCount(value string) (int, bool) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, false
	}
	return n, true
}

// buildFlags converts the collected issues into the Flags map used in AgentResult.
// issues_found holds the count; issue_1, issue_2, … hold the detail strings.
func buildFlags(issues []string) map[string]string {
	flags := map[string]string{
		"issues_found": strconv.Itoa(len(issues)),
	}
	for i, detail := range issues {
		flags[fmt.Sprintf("issue_%d", i+1)] = detail
	}
	return flags
}

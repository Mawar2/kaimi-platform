package finalreview

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

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
// indicating whether the proposal is ready for a human to submit.
// Instantiate with New().
type Agent struct{}

// New returns a new Final Review agent.
func New() *Agent {
	return &Agent{}
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

// checkMustHave scans the draft for each keyword in requirements.
// Any keyword absent from the draft is reported as a must_have issue.
func checkMustHave(draft string, requirements []string) []string {
	draftLower := strings.ToLower(draft)
	var issues []string
	for _, req := range requirements {
		if !strings.Contains(draftLower, strings.ToLower(req)) {
			issues = append(issues, fmt.Sprintf(
				"[must_have] requirement %q not addressed in draft", req,
			))
		}
	}
	return issues
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

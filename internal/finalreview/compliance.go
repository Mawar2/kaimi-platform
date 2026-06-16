package finalreview

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// complianceSystemInstruction is delivered as a system instruction (not in the
// user prompt) so the compliance discipline resists drift on long solicitation
// text. It pins the reviewer to grounded analysis and a strict output shape.
const complianceSystemInstruction = "You are Vera, Kaimi's U.S. federal proposal compliance reviewer. " +
	"Compare the PROPOSAL DRAFT against the SOLICITATION MATERIALS provided in the user message — either " +
	"full solicitation documents, or a solicitation summary when full documents are unavailable. " +
	"Identify every mandatory requirement the solicitation imposes — the 'shall', 'must', and 'will' " +
	"instructions in Section L (instructions), Section M (evaluation criteria), and the SOW/PWS deliverables, " +
	"or the stated mandatory requirements of a summary. " +
	"For each, decide whether the draft addresses it. A requirement is addressed when the draft substantively " +
	"covers it in its own words; do not demand verbatim phrasing. " +
	"CRITICAL: Use ONLY the requirements that actually appear in the provided materials — do NOT invent, " +
	"assume, or import requirements from general knowledge. If the materials state no mandatory requirements, " +
	"return an empty findings array. " +
	"Respond with ONLY a JSON object of the form " +
	`{"findings":[{"requirement":"<short text>","source":"<e.g. Section L>","addressed":true|false,"note":"<where addressed, or what is missing>"}]} ` +
	"and no prose, preamble, or markdown."

// ComplianceChecker runs the LLM compliance review. It receives the static system
// instruction (the compliance discipline) and a grounded user prompt (the draft +
// solicitation document text) and returns the model's raw JSON response. The
// production implementation is GeminiComplianceChecker; tests inject a mock.
type ComplianceChecker interface {
	CheckCompliance(ctx context.Context, systemInstruction, prompt string) (string, error)
}

// complianceFinding is one requirement the reviewer evaluated.
type complianceFinding struct {
	Requirement string `json:"requirement"`
	Source      string `json:"source"`
	Addressed   bool   `json:"addressed"`
	Note        string `json:"note"`
}

// complianceResponse is the model's structured output.
type complianceResponse struct {
	Findings []complianceFinding `json:"findings"`
}

// runCompliance runs the LLM compliance pass and returns one issue string per
// unmet requirement. A failure to run or parse the check is itself returned as a
// single issue (routing the proposal to needs_human) rather than a Go error, so a
// transient model problem never crashes the pipeline or lets a draft through
// unchecked.
func (a *Agent) runCompliance(ctx context.Context, in Input) []string {
	// Ground on the full solicitation documents when the ingest stage provided
	// them; otherwise fall back to the opportunity's own summary so the LLM pass
	// still runs in deployments without document ingestion (issue #264).
	var prompt string
	if len(in.Documents) > 0 {
		prompt = buildCompliancePrompt(in.Draft, in.Documents)
	} else {
		prompt = buildOpportunityCompliancePrompt(in.Draft, in.Opportunity)
	}

	raw, err := a.checker.CheckCompliance(ctx, complianceSystemInstruction, prompt)
	if err != nil {
		return []string{fmt.Sprintf("[compliance_error] compliance review could not run (verify manually): %v", err)}
	}

	findings, err := parseComplianceResponse(raw)
	if err != nil {
		return []string{fmt.Sprintf("[compliance_error] compliance response could not be parsed (verify manually): %v", err)}
	}

	var issues []string
	for _, f := range findings {
		if f.Addressed {
			continue
		}
		detail := strings.TrimSpace(f.Note)
		if detail == "" {
			detail = "not addressed in the draft"
		}
		issues = append(issues, fmt.Sprintf("[compliance] %s (%s): %s", f.Requirement, f.Source, detail))
	}
	return issues
}

// buildCompliancePrompt assembles the grounded user prompt: the draft followed by
// the full solicitation document text, in a stable filename order.
func buildCompliancePrompt(draft string, documents map[string]string) string {
	var sb strings.Builder

	sb.WriteString("## Proposal draft to evaluate\n")
	sb.WriteString(draft)
	sb.WriteString("\n\n")

	sb.WriteString("## Solicitation documents (the only source of mandatory requirements)\n")
	names := make([]string, 0, len(documents))
	for name := range documents {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		text := strings.TrimSpace(documents[name])
		if text == "" {
			continue
		}
		fmt.Fprintf(&sb, "### %s\n%s\n\n", name, text)
	}

	return sb.String()
}

// buildOpportunityCompliancePrompt assembles the fallback user prompt for
// deployments without document ingestion: the draft followed by a solicitation
// summary built from the opportunity's own fields. The summary is clearly
// labeled so the model knows it is not seeing full solicitation documents.
func buildOpportunityCompliancePrompt(draft string, opp *opportunity.Opportunity) string {
	var sb strings.Builder

	sb.WriteString("## Proposal draft to evaluate\n")
	sb.WriteString(draft)
	sb.WriteString("\n\n")

	sb.WriteString("## Solicitation summary (the only source of mandatory requirements; full documents unavailable)\n")
	fmt.Fprintf(&sb, "Title: %s\n", opp.Title)
	fmt.Fprintf(&sb, "Agency: %s\n", opp.Agency)
	if opp.SolicitationNum != "" {
		fmt.Fprintf(&sb, "Solicitation number: %s\n", opp.SolicitationNum)
	}
	if opp.NAICSCode != "" {
		fmt.Fprintf(&sb, "NAICS: %s %s\n", opp.NAICSCode, opp.NAICSDescription)
	}
	if opp.SetAsideCode != "" {
		fmt.Fprintf(&sb, "Set-aside: %s\n", opp.SetAsideCode)
	}
	fmt.Fprintf(&sb, "\nDescription:\n%s\n", strings.TrimSpace(opp.Description))

	if len(opp.Requirements) > 0 {
		sb.WriteString("\nMandatory requirements:\n")
		for _, req := range opp.Requirements {
			fmt.Fprintf(&sb, "- %s\n", req)
		}
	}

	return sb.String()
}

// parseComplianceResponse decodes the model's JSON, tolerating a ```json fenced
// code block wrapper that models sometimes add despite instructions.
func parseComplianceResponse(raw string) ([]complianceFinding, error) {
	cleaned := stripCodeFence(strings.TrimSpace(raw))
	if cleaned == "" {
		return nil, fmt.Errorf("empty response")
	}

	var resp complianceResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return resp.Findings, nil
}

// stripCodeFence removes a leading ```json / ``` fence and trailing ``` if present.
func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Drop the first line (``` or ```json) and any trailing fence.
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}

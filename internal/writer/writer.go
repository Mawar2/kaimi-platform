// Package writer provides the Zone 2 agent that turns an Outline into draft prose.
//
// It runs in two modes:
//   - Skeleton/stub mode (New): produces a deterministic placeholder draft with no
//     model call. Used as a fallback and in fast tests.
//   - Generation mode (NewWithGenerator): drafts each outline section with a
//     Generator (Gemini in production), grounded strictly in the Opportunity and
//     the Capability Profile.
//
// Grounding rule (KAI-9): the Writer passes only the facts present in the
// Opportunity and Capability Profile into the prompt and enforces, via a system
// instruction, that the model never invents past performance, contract numbers,
// client names, certifications, or compliance claims. Missing facts are flagged as
// gaps, never fabricated.
package writer

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/scorer"
)

const agentName = "writer"

// writerPersona is the Writer's human-facing name, carried in the AgentResult
// metadata so the dashboard can attribute a draft to its author. The Go package
// stays "writer"; "Thomas" is only the persona shown to users.
const writerPersona = "Thomas"

// gapMarker is the placeholder the model is instructed to emit when a section
// needs a fact that is not present in the grounding inputs — instead of inventing it.
const gapMarker = "[GAP:"

// defaultCompany is the generic company phrasing used when the profile does not
// supply a company name, so the system instruction never falls back to a hardcoded
// real company.
const defaultCompany = "the offeror"

// buildSystemInstruction builds the system instruction carrying the critical
// anti-fabrication rules, addressing the proposal to the configured company. It is
// delivered as a system instruction (not appended to the user prompt) so it is
// robust against instruction drift when the opportunity text is long or complex.
// The company name comes from the Capability Profile, not a literal, so the Writer
// drafts for whichever company is configured.
func buildSystemInstruction(company string) string {
	company = strings.TrimSpace(company)
	if company == "" {
		company = defaultCompany
	}
	return fmt.Sprintf("You are drafting sections of a U.S. federal proposal for %s. ", company) +
		"CRITICAL RULES: " +
		"Use ONLY the facts provided in the user message. " +
		"Do NOT invent past performance, contract numbers, client names, dollar amounts, certifications, dates, or compliance claims. " +
		"If a section needs a fact that is not provided, do NOT fabricate it — insert a placeholder of the exact form " + gapMarker + " what is missing] and continue. " +
		"Write only the prose for the requested section: no preamble and no markdown headers."
}

// Generator produces prose for a single proposal section from a system instruction
// (the static anti-fabrication rules) and a grounded user prompt (the per-section
// facts). The production implementation is GeminiGenerator; tests inject a mock.
// Implementations must not add facts beyond what the prompt provides.
type Generator interface {
	GenerateSection(ctx context.Context, systemInstruction, prompt string) (string, error)
}

// Input contains the context the Writer needs to generate a draft.
type Input struct {
	// Opportunity provides the solicitation facts and title. Required.
	Opportunity *opportunity.Opportunity
	// Outline defines the sections and formatting rules for the draft. Required.
	Outline *outline.Outline
	// Profile supplies the configured company's real facts (company name, past
	// performance, competencies) used to ground the draft. Required in generation
	// mode; ignored in stub mode.
	Profile *scorer.CapabilityProfile
	// Documents maps a solicitation document filename to its extracted text
	// (populated by the Manager's ingest stage). When present it is additional
	// grounded source material the model may use; it is never required.
	Documents map[string]string
	// RevisionNote carries the human reviewer's change request when this run is a
	// revision (Request changes at the gate). When present the writer must
	// address it in the new draft; empty on a fresh draft.
	RevisionNote string
}

// Agent transforms an outline into a proposal draft. A nil generator selects
// stub mode; a non-nil generator selects grounded generation mode.
type Agent struct {
	gen Generator
}

// New creates a Writer in stub mode (no model calls).
func New() *Agent {
	return &Agent{}
}

// NewWithGenerator creates a Writer that drafts sections with the given Generator.
func NewWithGenerator(g Generator) *Agent {
	return &Agent{gen: g}
}

// Run produces a proposal draft for the given input.
//
// Stub mode returns a deterministic placeholder draft. Generation mode drafts each
// section via the Generator, grounded in the Opportunity and Capability Profile.
// On any failure it returns a failed Result and a non-nil error with an empty
// draft — it never emits a silent empty draft.
func (a *Agent) Run(ctx context.Context, in Input) (string, *agent.Result, error) {
	if in.Opportunity == nil {
		return "", failed("", "missing opportunity data", "opportunity cannot be nil"), fmt.Errorf("opportunity is required")
	}
	if in.Outline == nil {
		return "", failed(in.Opportunity.ID, "missing outline data", "outline cannot be nil"), fmt.Errorf("outline is required")
	}

	if a.gen == nil {
		return a.runStub(in)
	}
	return a.runGenerated(ctx, in)
}

// runStub produces the deterministic placeholder draft (skeleton behavior).
func (a *Agent) runStub(in Input) (string, *agent.Result, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Proposal Draft: %s\n", in.Opportunity.Title)
	for _, section := range in.Outline.Sections {
		fmt.Fprintf(&sb, "\n## %s\n", section.Title)
		fmt.Fprintf(&sb, "[Stub draft for %s -- real generation lands in KAI-9]\n", section.Title)
	}
	return sb.String(), successResult(in, len(in.Outline.Sections), "true"), nil
}

// runGenerated drafts each section with the Generator, grounded in the inputs.
func (a *Agent) runGenerated(ctx context.Context, in Input) (string, *agent.Result, error) {
	if in.Profile == nil {
		return "", failed(in.Opportunity.ID, "missing capability profile", "profile cannot be nil in generation mode"), fmt.Errorf("profile is required for grounded drafting")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Proposal Draft: %s\n", in.Opportunity.Title)

	// Address the proposal to the configured company, not a hardcoded name.
	systemInstruction := buildSystemInstruction(in.Profile.Company)

	for _, section := range in.Outline.Sections {
		if err := ctx.Err(); err != nil {
			return "", failed(in.Opportunity.ID, "draft generation cancelled", err.Error()), err
		}

		prompt := buildSectionPrompt(in.Opportunity, in.Profile, section, in.Outline.FormattingRules, in.Documents, in.RevisionNote)
		text, err := a.gen.GenerateSection(ctx, systemInstruction, prompt)
		if err != nil {
			return "", failed(
				in.Opportunity.ID,
				fmt.Sprintf("failed to generate section %q", section.Title),
				err.Error(),
			), fmt.Errorf("writer: generate section %q: %w", section.ID, err)
		}

		// Treat a whitespace-only section as a failure rather than silently
		// emitting a heading with no content (no silent empty draft).
		text = strings.TrimSpace(text)
		if text == "" {
			return "", failed(
				in.Opportunity.ID,
				fmt.Sprintf("empty draft for section %q", section.Title),
				"generator returned no content for the section",
			), fmt.Errorf("writer: empty draft for section %q", section.ID)
		}

		fmt.Fprintf(&sb, "\n## %s\n", section.Title)
		sb.WriteString(text)
		sb.WriteString("\n")
	}

	return sb.String(), successResult(in, len(in.Outline.Sections), "false"), nil
}

// buildSectionPrompt builds the grounded user prompt for one section. It includes
// only the facts present in the Opportunity, the Capability Profile, and the
// ingested solicitation documents; the anti-fabrication rules are delivered
// separately via systemInstruction.
func buildSectionPrompt(opp *opportunity.Opportunity, profile *scorer.CapabilityProfile, section outline.Section, rules *outline.FormattingRules, documents map[string]string, revisionNote string) string {
	var sb strings.Builder

	// A human reviewer's change request takes priority: it is the reason this
	// section is being redrafted, so it leads the prompt.
	if note := strings.TrimSpace(revisionNote); note != "" {
		sb.WriteString("## Reviewer change request (address this in your draft)\n")
		fmt.Fprintf(&sb, "%s\n\n", note)
	}

	sb.WriteString("## Section to draft\n")
	fmt.Fprintf(&sb, "Title: %s\n", section.Title)
	if section.Rationale != "" {
		fmt.Fprintf(&sb, "Why this section is required: %s\n", section.Rationale)
	}
	sb.WriteString("\n")

	sb.WriteString("## Opportunity facts (the only solicitation facts you may use)\n")
	fmt.Fprintf(&sb, "Title: %s\n", opp.Title)
	fmt.Fprintf(&sb, "Agency: %s\n", opp.Agency)
	fmt.Fprintf(&sb, "NAICS: %s (%s)\n", opp.NAICSCode, opp.NAICSDescription)
	if opp.Description != "" {
		fmt.Fprintf(&sb, "Description: %s\n", opp.Description)
	}
	if len(opp.Requirements) > 0 {
		fmt.Fprintf(&sb, "Stated requirements: %s\n", strings.Join(opp.Requirements, "; "))
	}
	sb.WriteString("\n")

	writeSolicitationDocuments(&sb, documents)

	sb.WriteString("## Company facts (the only company facts you may use)\n")
	fmt.Fprintf(&sb, "Competencies: %s\n", joinOrNone(profile.CompetencyTags))
	fmt.Fprintf(&sb, "Past performance: %s\n", joinOrNone(profile.PastPerformance))
	fmt.Fprintf(&sb, "Primary NAICS: %s\n", joinOrNone(profile.PrimaryNAICS))
	// Only assert SDB status when true — printing "false" can prompt the model to
	// write an unnecessary negative claim.
	if profile.SDBStatus {
		sb.WriteString("Small Disadvantaged Business: yes\n")
	}
	sb.WriteString("\n")

	if rules != nil {
		if rules.PageLimit != nil && rules.PageLimit.Specified {
			fmt.Fprintf(&sb, "Formatting: respect the page limit (%s).\n", rules.PageLimit.Value)
		}
		if rules.Font != nil && rules.Font.Specified {
			fmt.Fprintf(&sb, "Formatting: use the required font (%s).\n", rules.Font.Value)
		}
	}

	return sb.String()
}

// writeSolicitationDocuments appends the ingested solicitation document text to
// the prompt, in a stable filename order so prompts are deterministic. When no
// documents were ingested it writes nothing, so the prompt never implies source
// material that is not present.
func writeSolicitationDocuments(sb *strings.Builder, documents map[string]string) {
	if len(documents) == 0 {
		return
	}
	names := make([]string, 0, len(documents))
	for name := range documents {
		names = append(names, name)
	}
	sort.Strings(names)

	sb.WriteString("## Solicitation documents (extracted text — additional grounded source material)\n")
	for _, name := range names {
		text := strings.TrimSpace(documents[name])
		if text == "" {
			continue
		}
		fmt.Fprintf(sb, "### %s\n%s\n\n", name, text)
	}
}

// joinOrNone joins items with ", " or returns "(none provided)" when empty, so the
// prompt never implies a fact the profile does not contain.
func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "(none provided)"
	}
	return strings.Join(items, ", ")
}

// successResult builds the success Result for a completed draft.
func successResult(in Input, sectionCount int, stub string) *agent.Result {
	return &agent.Result{
		AgentName: agentName,
		Status:    agent.StatusSuccess,
		NoticeID:  in.Opportunity.ID,
		Summary:   fmt.Sprintf("generated draft with %d sections for opportunity %s", sectionCount, in.Opportunity.ID),
		Flags: map[string]string{
			"section_count": fmt.Sprintf("%d", sectionCount),
			"stub":          stub,
			"persona":       writerPersona,
		},
		CompletedAt: time.Now().UTC(),
	}
}

// failed builds a failed Result with the given notice ID, summary, and error detail.
func failed(noticeID, summary, errMsg string) *agent.Result {
	return &agent.Result{
		AgentName:   agentName,
		Status:      agent.StatusFailed,
		NoticeID:    noticeID,
		Summary:     summary,
		Error:       errMsg,
		CompletedAt: time.Now().UTC(),
	}
}

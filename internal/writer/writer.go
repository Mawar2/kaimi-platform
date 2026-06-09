// Package writer provides the Zone 2 agent that turns an Outline into draft prose.
// This is a skeleton implementation that produces a stubbed draft to prove the
// interface; actual content generation will be implemented in KAI-9.
// It returns an agent.Result consistent with other agents in the system.
package writer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
)

const agentName = "writer"

// Input contains the necessary context for the writer agent to generate a draft.
type Input struct {
	// Opportunity provides the high-level project details and title.
	Opportunity *opportunity.Opportunity
	// Outline defines the required sections and formatting rules for the draft.
	Outline *outline.Outline
}

// Agent handles the transformation of an outline into a prose document.
type Agent struct{}

// New creates a new instance of the Writer agent.
func New() *Agent {
	return &Agent{}
}

// Run generates a stubbed proposal draft based on the provided input.
func (a *Agent) Run(ctx context.Context, in Input) (string, *agent.Result, error) {
	if in.Opportunity == nil {
		res := &agent.Result{
			AgentName:   agentName,
			Status:      agent.StatusFailed,
			Summary:     "missing opportunity data",
			Error:       "opportunity cannot be nil",
			CompletedAt: time.Now().UTC(),
		}
		return "", res, fmt.Errorf("opportunity is required")
	}

	if in.Outline == nil {
		res := &agent.Result{
			AgentName:   agentName,
			Status:      agent.StatusFailed,
			Summary:     "missing outline data",
			Error:       "outline cannot be nil",
			CompletedAt: time.Now().UTC(),
		}
		return "", res, fmt.Errorf("outline is required")
	}

	// TODO(phase-3): replace the stub with a Gemini call (KAI-9).
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Proposal Draft: %s\n", in.Opportunity.Title)

	for _, section := range in.Outline.Sections {
		fmt.Fprintf(&sb, "\n## %s\n", section.Title)
		fmt.Fprintf(&sb, "[Stub draft for %s -- real generation lands in KAI-9]\n", section.Title)
	}

	draft := sb.String()
	res := &agent.Result{
		AgentName: agentName,
		Status:    agent.StatusSuccess,
		NoticeID:  in.Opportunity.ID,
		Summary:   fmt.Sprintf("generated stub draft with %d sections for opportunity %s", len(in.Outline.Sections), in.Opportunity.ID),
		Flags: map[string]string{
			"section_count": fmt.Sprintf("%d", len(in.Outline.Sections)),
			"stub":          "true",
		},
		CompletedAt: time.Now().UTC(),
	}

	return draft, res, nil
}

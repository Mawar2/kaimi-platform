// Package agent provides the core interface contract for all Kaimi agents.
//
// The AgentResult type is the standardized return value for every agent in both
// Zone 1 (scheduled pipeline) and Zone 2 (per-proposal orchestration). This contract
// enables the Manager agent to coordinate specialist agents without tight coupling.
//
// Key design principles:
//   - Status enum makes outcomes explicit and actionable
//   - OutputRef is flexible (file path, URL, ID) to support different storage backends
//   - Flags are extensible metadata without schema changes
//   - Error field captures failure details without throwing exceptions
package agent

import "time"

// Status represents the outcome of an agent's execution.
type Status string

const (
	// StatusSuccess indicates the agent completed successfully and produced output.
	StatusSuccess Status = "success"

	// StatusFailed indicates the agent encountered an error and could not complete.
	StatusFailed Status = "failed"

	// StatusNeedsHuman indicates the agent requires human intervention to proceed.
	// Used when the agent detects ambiguity, conflicting requirements, or needs clarification.
	StatusNeedsHuman Status = "needs_human"

	// StatusReadyToSubmit indicates a Zone 2 agent has completed review and the
	// proposal is ready for human approval and submission. Only used by Final Review agent.
	StatusReadyToSubmit Status = "ready_to_submit"
)

// AgentResult is the standardized return type for all Kaimi agents.
//
// Every agent (Hunter, Scorer, Outline, Writer, Final Review) returns an AgentResult
// to communicate its outcome, output location, and any metadata or errors.
//
// Example usage (Scorer agent):
//
//	result := &agent.AgentResult{
//	    AgentName:  "scorer",
//	    Status:     agent.StatusSuccess,
//	    NoticeID:   "ABC-123-2026",
//	    Summary:    "Scored 87/100 - Strong NAICS match, relevant past performance",
//	    OutputRef:  "opportunities/ABC-123-2026.json",
//	    Flags:      map[string]string{"score": "87", "recommendation": "BID"},
//	    CompletedAt: time.Now(),
//	}
//
// Example usage (agent failure):
//
//	result := &agent.AgentResult{
//	    AgentName: "hunter",
//	    Status:    agent.StatusFailed,
//	    Error:     "SAM.gov API returned 429 (rate limit exceeded)",
//	    CompletedAt: time.Now(),
//	}
type AgentResult struct {
	// AgentName identifies which agent produced this result.
	// Examples: "hunter", "scorer", "outline", "writer", "final-review"
	AgentName string `json:"agent_name"`

	// Status indicates the outcome of the agent's execution.
	// See Status constants for valid values.
	Status Status `json:"status"`

	// NoticeID is the SAM.gov notice/opportunity ID this result relates to.
	// Empty for agents that operate across multiple opportunities.
	NoticeID string `json:"notice_id,omitempty"`

	// Summary is a human-readable description of what the agent did.
	// Should explain the outcome in 1-2 sentences.
	// Required for StatusSuccess and StatusNeedsHuman.
	Summary string `json:"summary,omitempty"`

	// OutputRef points to where the agent's output is stored.
	// Format depends on the agent:
	//   - Hunter/Scorer: file path to updated Opportunity JSON
	//   - Outline: Google Docs URL
	//   - Final Review: validation report file path
	// Empty if Status is StatusFailed.
	OutputRef string `json:"output_ref,omitempty"`

	// Flags are extensible key-value metadata for agent-specific information.
	// Examples:
	//   - Scorer: {"score": "87", "recommendation": "BID"}
	//   - Outline: {"section_count": "5", "doc_id": "abc123"}
	//   - Final Review: {"issues_found": "3", "ready": "false"}
	// Allows agents to communicate structured data without schema changes.
	Flags map[string]string `json:"flags,omitempty"`

	// Error contains the error message if Status is StatusFailed.
	// Should include enough context for debugging (what failed, why).
	// Empty for non-failed statuses.
	Error string `json:"error,omitempty"`

	// CompletedAt is when the agent finished execution.
	CompletedAt time.Time `json:"completed_at"`
}

// IsSuccess returns true if the agent completed successfully.
func (r *AgentResult) IsSuccess() bool {
	return r.Status == StatusSuccess || r.Status == StatusReadyToSubmit
}

// IsFailed returns true if the agent failed.
func (r *AgentResult) IsFailed() bool {
	return r.Status == StatusFailed
}

// NeedsHuman returns true if the agent requires human intervention.
func (r *AgentResult) NeedsHuman() bool {
	return r.Status == StatusNeedsHuman
}

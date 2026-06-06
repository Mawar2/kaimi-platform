package agent

import (
	"context"
	"fmt"
	"time"
)

// StubAgent is a minimal agent implementation that proves the AgentResult contract works.
// It's used for testing and as a reference for building real agents.
//
// Real agents will follow this pattern:
//  1. Take input (Opportunity, config, etc.)
//  2. Do work (call APIs, run LLM, etc.)
//  3. Return AgentResult with status and output
type StubAgent struct {
	name string
}

// NewStubAgent creates a stub agent with the given name.
func NewStubAgent(name string) *StubAgent {
	return &StubAgent{name: name}
}

// Execute runs the stub agent and returns a successful AgentResult.
// This proves the contract shape works without doing real work.
func (a *StubAgent) Execute(ctx context.Context, noticeID string) (*AgentResult, error) {
	// Simulate some work
	select {
	case <-ctx.Done():
		return &AgentResult{
			AgentName:   a.name,
			Status:      StatusFailed,
			NoticeID:    noticeID,
			Error:       "context cancelled",
			CompletedAt: time.Now(),
		}, ctx.Err()
	case <-time.After(10 * time.Millisecond):
		// Continue
	}

	// Return successful result
	return &AgentResult{
		AgentName:   a.name,
		Status:      StatusSuccess,
		NoticeID:    noticeID,
		Summary:     fmt.Sprintf("Stub agent '%s' completed successfully for notice %s", a.name, noticeID),
		OutputRef:   fmt.Sprintf("output/%s/%s.json", a.name, noticeID),
		Flags:       map[string]string{"stub": "true", "version": "1.0"},
		CompletedAt: time.Now(),
	}, nil
}

// ExecuteWithError simulates an agent failure.
// Useful for testing error handling in the Manager.
func (a *StubAgent) ExecuteWithError(ctx context.Context, noticeID, errMsg string) (*AgentResult, error) {
	return &AgentResult{
		AgentName:   a.name,
		Status:      StatusFailed,
		NoticeID:    noticeID,
		Error:       errMsg,
		CompletedAt: time.Now(),
	}, fmt.Errorf("agent failed: %s", errMsg)
}

// ExecuteNeedsHuman simulates an agent that requires human intervention.
// Useful for testing the Manager's human review gate.
func (a *StubAgent) ExecuteNeedsHuman(ctx context.Context, noticeID, reason string) (*AgentResult, error) {
	return &AgentResult{
		AgentName:   a.name,
		Status:      StatusNeedsHuman,
		NoticeID:    noticeID,
		Summary:     reason,
		Flags:       map[string]string{"intervention_needed": "true"},
		CompletedAt: time.Now(),
	}, nil
}

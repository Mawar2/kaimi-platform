package outline

import (
	"context"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/opportunity"
)

var testTime = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

// cachedOpportunity returns a realistic Opportunity for use in tests.
// This is the "cached test fixture" — no live API calls needed.
func cachedOpportunity() *opportunity.Opportunity {
	return &opportunity.Opportunity{
		ID:               "CACHED-TEST-001",
		Title:            "IT Systems Design Services",
		SolicitationNum:  "SOL-2026-TEST-001",
		Agency:           "Department of Defense",
		Office:           "Office of the CIO",
		PostedDate:       testTime,
		ResponseDeadline: testTime.Add(30 * 24 * time.Hour),
		NAICSCode:        "541512",
		NAICSDescription: "Computer Systems Design Services",
		SetAsideCode:     "SBA",
		Description:      "Provide IT systems design and integration services.",
		Type:             "Solicitation",
		URL:              "https://sam.gov/test/cached-001",
		CreatedAt:        testTime,
		UpdatedAt:        testTime,
	}
}

// TestOutlineAgent_HappyPath verifies the agent returns success when given a valid Opportunity.
func TestOutlineAgent_HappyPath(t *testing.T) {
	ctx := context.Background()
	a := New()

	result, err := a.Run(ctx, cachedOpportunity())

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
	const wantSummary = "outline stub complete for opportunity CACHED-TEST-001: IT Systems Design Services"
	if result.Summary != wantSummary {
		t.Errorf("Summary = %q, want %q", result.Summary, wantSummary)
	}
}

// TestOutlineAgent_NilOpportunity verifies the agent returns failed when given nil input.
func TestOutlineAgent_NilOpportunity(t *testing.T) {
	ctx := context.Background()
	a := New()

	result, err := a.Run(ctx, nil)

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
}

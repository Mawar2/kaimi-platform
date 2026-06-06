package agent

import (
	"context"
	"testing"
	"time"
)

func TestStubAgent_Execute(t *testing.T) {
	agent := NewStubAgent("test-agent")
	ctx := context.Background()

	result, err := agent.Execute(ctx, "TEST-123")
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	// Verify result fields
	if result.AgentName != "test-agent" {
		t.Errorf("AgentName = %v, want test-agent", result.AgentName)
	}
	if result.Status != StatusSuccess {
		t.Errorf("Status = %v, want success", result.Status)
	}
	if result.NoticeID != "TEST-123" {
		t.Errorf("NoticeID = %v, want TEST-123", result.NoticeID)
	}
	if result.Summary == "" {
		t.Error("Summary should not be empty")
	}
	if result.OutputRef == "" {
		t.Error("OutputRef should not be empty")
	}
	if result.Flags["stub"] != "true" {
		t.Errorf("Flags[stub] = %v, want true", result.Flags["stub"])
	}
	if result.CompletedAt.IsZero() {
		t.Error("CompletedAt should be set")
	}

	// Verify result methods
	if !result.IsSuccess() {
		t.Error("IsSuccess() should be true")
	}
	if result.IsFailed() {
		t.Error("IsFailed() should be false")
	}
	if result.NeedsHuman() {
		t.Error("NeedsHuman() should be false")
	}
}

func TestStubAgent_ExecuteWithCancellation(t *testing.T) {
	agent := NewStubAgent("test-agent")
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	result, err := agent.Execute(ctx, "TEST-123")
	if err != context.Canceled {
		t.Errorf("Execute() error = %v, want context.Canceled", err)
	}

	// Verify failed result
	if result.Status != StatusFailed {
		t.Errorf("Status = %v, want failed", result.Status)
	}
	if result.Error == "" {
		t.Error("Error should not be empty for cancelled context")
	}
	if !result.IsFailed() {
		t.Error("IsFailed() should be true for cancelled execution")
	}
}

func TestStubAgent_ExecuteWithError(t *testing.T) {
	agent := NewStubAgent("test-agent")
	ctx := context.Background()
	errorMsg := "simulated API failure"

	result, err := agent.ExecuteWithError(ctx, "TEST-456", errorMsg)
	if err == nil {
		t.Fatal("ExecuteWithError() should return error")
	}

	// Verify failed result
	if result.Status != StatusFailed {
		t.Errorf("Status = %v, want failed", result.Status)
	}
	if result.NoticeID != "TEST-456" {
		t.Errorf("NoticeID = %v, want TEST-456", result.NoticeID)
	}
	if result.Error != errorMsg {
		t.Errorf("Error = %v, want %v", result.Error, errorMsg)
	}
	if !result.IsFailed() {
		t.Error("IsFailed() should be true")
	}
}

func TestStubAgent_ExecuteNeedsHuman(t *testing.T) {
	agent := NewStubAgent("test-agent")
	ctx := context.Background()
	reason := "Requirements are ambiguous"

	result, err := agent.ExecuteNeedsHuman(ctx, "TEST-789", reason)
	if err != nil {
		t.Fatalf("ExecuteNeedsHuman() error = %v, want nil", err)
	}

	// Verify needs_human result
	if result.Status != StatusNeedsHuman {
		t.Errorf("Status = %v, want needs_human", result.Status)
	}
	if result.NoticeID != "TEST-789" {
		t.Errorf("NoticeID = %v, want TEST-789", result.NoticeID)
	}
	if result.Summary != reason {
		t.Errorf("Summary = %v, want %v", result.Summary, reason)
	}
	if result.Flags["intervention_needed"] != "true" {
		t.Errorf("Flags[intervention_needed] = %v, want true", result.Flags["intervention_needed"])
	}
	if !result.NeedsHuman() {
		t.Error("NeedsHuman() should be true")
	}
}

func TestStubAgent_ConcurrentExecution(t *testing.T) {
	// Test that multiple stub agents can run concurrently without issues
	agent := NewStubAgent("concurrent-test")
	ctx := context.Background()

	done := make(chan bool, 3)

	for i := 0; i < 3; i++ {
		go func(id int) {
			noticeID := "NOTICE-" + string(rune('A'+id))
			result, err := agent.Execute(ctx, noticeID)
			if err != nil {
				t.Errorf("Execute() error = %v", err)
			}
			if result.Status != StatusSuccess {
				t.Errorf("Status = %v, want success", result.Status)
			}
			done <- true
		}(i)
	}

	// Wait for all to complete with timeout
	timeout := time.After(1 * time.Second)
	for i := 0; i < 3; i++ {
		select {
		case <-done:
			// Success
		case <-timeout:
			t.Fatal("Concurrent execution timed out")
		}
	}
}

package fallback_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Mawar2/Kaimi/internal/fallback"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/scorer"
)

func init() {
	// Keep retry-driven tests instant.
	fallback.RetryBackoff = 0
}

// recordingCall implements both Generator.GenerateSection and
// ComplianceChecker.CheckCompliance (identical signatures), counting invocations and
// optionally failing a number of times before succeeding.
type recordingCall struct {
	out        string
	err        error
	failNTimes int // fail this many calls (transiently) before returning out
	calls      int
}

func (r *recordingCall) GenerateSection(_ context.Context, _, _ string) (string, error) {
	return r.invoke()
}

func (r *recordingCall) CheckCompliance(_ context.Context, _, _ string) (string, error) {
	return r.invoke()
}

func (r *recordingCall) invoke() (string, error) {
	r.calls++
	if r.failNTimes > 0 {
		r.failNTimes--
		return "", errors.New("503 unavailable") // transient
	}
	return r.out, r.err
}

func TestGenerator_PrimarySucceeds_BackupUntouched(t *testing.T) {
	primary := &recordingCall{out: "primary"}
	backup := &recordingCall{out: "backup"}
	g := fallback.NewGenerator(primary, backup)

	out, err := g.GenerateSection(context.Background(), "sys", "prompt")
	if err != nil || out != "primary" {
		t.Fatalf("got (%q, %v), want (primary, nil)", out, err)
	}
	if backup.calls != 0 {
		t.Errorf("backup called %d times, want 0", backup.calls)
	}
}

func TestGenerator_TransientThenRecovers_SameOption(t *testing.T) {
	// Primary fails once transiently, then succeeds on retry — backup never used.
	primary := &recordingCall{out: "primary", failNTimes: 1}
	backup := &recordingCall{out: "backup"}
	g := fallback.NewGenerator(primary, backup)

	out, err := g.GenerateSection(context.Background(), "sys", "prompt")
	if err != nil || out != "primary" {
		t.Fatalf("got (%q, %v), want (primary, nil)", out, err)
	}
	if primary.calls != 2 {
		t.Errorf("primary called %d times, want 2 (1 transient fail + 1 success)", primary.calls)
	}
	if backup.calls != 0 {
		t.Errorf("backup called %d times, want 0", backup.calls)
	}
}

func TestGenerator_NonTransient_FailsOverImmediately(t *testing.T) {
	// Non-transient primary error should NOT retry the primary; go straight to backup.
	primary := &recordingCall{err: errors.New("safety blocked")}
	backup := &recordingCall{out: "backup"}
	g := fallback.NewGenerator(primary, backup)

	out, err := g.GenerateSection(context.Background(), "sys", "prompt")
	if err != nil || out != "backup" {
		t.Fatalf("got (%q, %v), want (backup, nil)", out, err)
	}
	if primary.calls != 1 {
		t.Errorf("primary called %d times, want 1 (no retry on non-transient)", primary.calls)
	}
}

func TestGenerator_AllFail_ReturnsLastError_NoStub(t *testing.T) {
	primary := &recordingCall{err: errors.New("primary down")}
	backup := &recordingCall{err: errors.New("backup down")}
	g := fallback.NewGenerator(primary, backup)

	out, err := g.GenerateSection(context.Background(), "sys", "prompt")
	if out != "" {
		t.Errorf("expected empty output when all options fail (no stub), got %q", out)
	}
	if err == nil || err.Error() != "backup down" {
		t.Fatalf("want last error 'backup down', got %v", err)
	}
}

// recordingPlanner implements outline.SectionPlanner, counting invocations and
// optionally failing a number of times (transiently) before succeeding.
type recordingPlanner struct {
	sections   []outline.Section
	err        error
	failNTimes int
	calls      int
}

func (r *recordingPlanner) PlanSections(_ context.Context, _ *opportunity.Opportunity, _ *scorer.CapabilityProfile, _ string) ([]outline.Section, error) {
	r.calls++
	if r.failNTimes > 0 {
		r.failNTimes--
		return nil, errors.New("429 rate limit") // transient
	}
	return r.sections, r.err
}

func TestPlanner_PrimarySucceeds_BackupUntouched(t *testing.T) {
	primary := &recordingPlanner{sections: []outline.Section{{ID: "exec", Title: "Executive Summary"}}}
	backup := &recordingPlanner{sections: []outline.Section{{ID: "other", Title: "Other"}}}
	p := fallback.NewPlanner(primary, backup)

	got, err := p.PlanSections(context.Background(), nil, nil, "source")
	if err != nil || len(got) != 1 || got[0].ID != "exec" {
		t.Fatalf("got (%v, %v), want primary's sections", got, err)
	}
	if backup.calls != 0 {
		t.Errorf("backup called %d times, want 0", backup.calls)
	}
}

func TestPlanner_TransientThenRecovers_SameOption(t *testing.T) {
	// Primary fails once transiently, then succeeds on retry — backup never used.
	primary := &recordingPlanner{sections: []outline.Section{{ID: "exec", Title: "Executive Summary"}}, failNTimes: 1}
	backup := &recordingPlanner{sections: []outline.Section{{ID: "other", Title: "Other"}}}
	p := fallback.NewPlanner(primary, backup)

	got, err := p.PlanSections(context.Background(), nil, nil, "source")
	if err != nil || len(got) != 1 || got[0].ID != "exec" {
		t.Fatalf("got (%v, %v), want primary's sections after retry", got, err)
	}
	if primary.calls != 2 {
		t.Errorf("primary called %d times, want 2 (1 transient fail + 1 success)", primary.calls)
	}
	if backup.calls != 0 {
		t.Errorf("backup called %d times, want 0", backup.calls)
	}
}

func TestPlanner_NonTransient_FailsOverImmediately(t *testing.T) {
	primary := &recordingPlanner{err: errors.New("safety blocked")}
	backup := &recordingPlanner{sections: []outline.Section{{ID: "other", Title: "Other"}}}
	p := fallback.NewPlanner(primary, backup)

	got, err := p.PlanSections(context.Background(), nil, nil, "source")
	if err != nil || len(got) != 1 || got[0].ID != "other" {
		t.Fatalf("got (%v, %v), want backup's sections", got, err)
	}
	if primary.calls != 1 {
		t.Errorf("primary called %d times, want 1 (no retry on non-transient)", primary.calls)
	}
}

func TestPlanner_AllFail_ReturnsLastError_NoStub(t *testing.T) {
	primary := &recordingPlanner{err: errors.New("primary down")}
	backup := &recordingPlanner{err: errors.New("backup down")}
	p := fallback.NewPlanner(primary, backup)

	got, err := p.PlanSections(context.Background(), nil, nil, "source")
	if got != nil {
		t.Errorf("expected nil sections when all options fail (no stub), got %v", got)
	}
	if err == nil || err.Error() != "backup down" {
		t.Fatalf("want last error 'backup down', got %v", err)
	}
}

func TestChecker_FailsOverToBackup(t *testing.T) {
	primary := &recordingCall{err: errors.New("primary down")}
	backup := &recordingCall{out: `{"findings":[]}`}
	c := fallback.NewChecker(primary, backup)

	out, err := c.CheckCompliance(context.Background(), "sys", "prompt")
	if err != nil || out != `{"findings":[]}` {
		t.Fatalf("got (%q, %v), want backup JSON", out, err)
	}
}

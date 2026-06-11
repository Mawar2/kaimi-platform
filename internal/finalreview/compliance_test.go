package finalreview_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/finalreview"
)

// mockChecker is a test double for finalreview.ComplianceChecker.
type mockChecker struct {
	resp      string
	err       error
	calls     int
	gotPrompt string
}

func (m *mockChecker) CheckCompliance(_ context.Context, _, prompt string) (string, error) {
	m.calls++
	m.gotPrompt = prompt
	return m.resp, m.err
}

func docs() map[string]string {
	return map[string]string{
		"RFP_Section_L.pdf": "Offerors shall submit a past performance volume of no more than 5 pages.",
	}
}

func TestReview_Compliance_UnmetRequirement_NeedsHuman(t *testing.T) {
	mc := &mockChecker{resp: `{"findings":[
		{"requirement":"submit a past performance volume","source":"Section L","addressed":false,"note":"draft has no separate past performance volume"}
	]}`}
	a := finalreview.NewWithComplianceChecker(mc)

	res, err := a.Review(context.Background(), finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
		Documents:   docs(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != agent.StatusNeedsHuman {
		t.Fatalf("Status = %s, want needs_human; flags=%v", res.Status, res.Flags)
	}
	// The unmet requirement is surfaced as a compliance issue flag.
	var found bool
	for _, v := range res.Flags {
		if strings.Contains(v, "[compliance]") && strings.Contains(v, "past performance volume") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a [compliance] flag naming the unmet requirement; flags=%v", res.Flags)
	}
	// The model was grounded on the draft and the solicitation document text.
	if !strings.Contains(mc.gotPrompt, "Offerors shall submit a past performance volume") {
		t.Error("compliance prompt did not include the solicitation document text")
	}
	if !strings.Contains(mc.gotPrompt, "Technical Approach") {
		t.Error("compliance prompt did not include the proposal draft")
	}
}

func TestReview_Compliance_AllAddressed_ReadyToSubmit(t *testing.T) {
	mc := &mockChecker{resp: `{"findings":[
		{"requirement":"submit a past performance volume","source":"Section L","addressed":true,"note":"Past Performance section present"}
	]}`}
	a := finalreview.NewWithComplianceChecker(mc)

	res, err := a.Review(context.Background(), finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(), // no Requirements, no Outline => no deterministic issues
		Documents:   docs(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %s, want ready_to_submit; flags=%v", res.Status, res.Flags)
	}
}

func TestReview_Compliance_CheckerError_NeedsHuman(t *testing.T) {
	mc := &mockChecker{err: errors.New("model unavailable")}
	a := finalreview.NewWithComplianceChecker(mc)

	res, err := a.Review(context.Background(), finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
		Documents:   docs(),
	})
	if err != nil {
		t.Fatalf("a checker error should not be a Go error: %v", err)
	}
	if res.Status != agent.StatusNeedsHuman {
		t.Fatalf("Status = %s, want needs_human on checker failure", res.Status)
	}
	var found bool
	for _, v := range res.Flags {
		if strings.Contains(v, "[compliance_error]") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a [compliance_error] flag; flags=%v", res.Flags)
	}
}

// TestReview_Compliance_NoDocuments_RunsOnOpportunity locks issue #264: with a
// checker configured but no ingested solicitation documents (production today —
// #162 infra is unprovisioned), Vera must still run her LLM pass, grounded on
// the opportunity's own summary instead of silently degrading to the
// deterministic string checks.
func TestReview_Compliance_NoDocuments_RunsOnOpportunity(t *testing.T) {
	mc := &mockChecker{resp: `{"findings":[]}`}
	a := finalreview.NewWithComplianceChecker(mc)

	opp := fixture()
	opp.Requirements = []string{"IT modernization"} // present in draftFixture

	res, err := a.Review(context.Background(), finalreview.Input{
		Draft:       draftFixture,
		Opportunity: opp,
		// no Documents — the pass must ground on the opportunity instead
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mc.calls != 1 {
		t.Fatalf("checker calls = %d, want 1 (LLM pass must run without documents)", mc.calls)
	}
	// Grounding: the prompt carries the opportunity summary and the draft.
	for _, want := range []string{
		"Seeking IT modernization support", // opportunity description
		"IT modernization",                 // the mandatory requirement
		"Dept. of Veterans Affairs",        // agency
		"Technical Approach",               // the draft itself
	} {
		if !strings.Contains(mc.gotPrompt, want) {
			t.Errorf("compliance prompt missing %q", want)
		}
	}
	if res.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %s, want ready_to_submit (no findings, no deterministic issues)", res.Status)
	}
}

// TestReview_Compliance_NoDocuments_UnmetFindingFlags proves an unmet finding
// from the opportunity-grounded pass routes to needs_human like the
// documents-grounded path does.
func TestReview_Compliance_NoDocuments_UnmetFindingFlags(t *testing.T) {
	mc := &mockChecker{resp: `{"findings":[
		{"requirement":"name a dedicated program manager","source":"solicitation summary","addressed":false,"note":"draft names no PM"}
	]}`}
	a := finalreview.NewWithComplianceChecker(mc)

	res, err := a.Review(context.Background(), finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
		// no Documents
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != agent.StatusNeedsHuman {
		t.Fatalf("Status = %s, want needs_human; flags=%v", res.Status, res.Flags)
	}
	var found bool
	for _, v := range res.Flags {
		if strings.Contains(v, "[compliance]") && strings.Contains(v, "program manager") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a [compliance] flag naming the unmet requirement; flags=%v", res.Flags)
	}
}

func TestReview_Compliance_DeterministicPreFilterStillRuns(t *testing.T) {
	// A missing must-have requirement (deterministic) must still route to
	// needs_human even when the compliance pass finds everything addressed.
	mc := &mockChecker{resp: `{"findings":[{"requirement":"x","source":"L","addressed":true}]}`}
	a := finalreview.NewWithComplianceChecker(mc)

	opp := fixture()
	opp.Requirements = []string{"zero trust architecture"} // not present in draftFixture

	res, err := a.Review(context.Background(), finalreview.Input{
		Draft:       draftFixture,
		Opportunity: opp,
		Documents:   docs(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != agent.StatusNeedsHuman {
		t.Fatalf("Status = %s, want needs_human (deterministic must-have miss)", res.Status)
	}
	var hasMustHave bool
	for _, v := range res.Flags {
		if strings.Contains(v, "must_have") {
			hasMustHave = true
		}
	}
	if !hasMustHave {
		t.Errorf("deterministic must_have check did not run alongside compliance; flags=%v", res.Flags)
	}
}

func TestReview_NoChecker_DeterministicOnly_Unchanged(t *testing.T) {
	// The plain agent (New) ignores Documents entirely.
	a := finalreview.New()
	res, err := a.Review(context.Background(), finalreview.Input{
		Draft:       draftFixture,
		Opportunity: fixture(),
		Documents:   docs(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != agent.StatusReadyToSubmit {
		t.Errorf("Status = %s, want ready_to_submit (no checker, no deterministic issues)", res.Status)
	}
}

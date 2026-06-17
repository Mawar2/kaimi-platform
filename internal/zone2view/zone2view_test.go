package zone2view

import (
	"testing"

	"github.com/Mawar2/Kaimi/internal/proposal"
)

// TestView locks the single source of truth for proposal pipeline position +
// display state (issue #246 B2 / #249). Both the web dashboard and the desktop
// derive their card/workspace state from this, so they cannot disagree.
func TestView(t *testing.T) {
	cases := []struct {
		status string
		idx    int
		state  string
	}{
		{proposal.StatusOutlineRunning, 0, "progress"},
		{proposal.StatusWriterRunning, 1, "progress"},
		{proposal.StatusGate, 2, "human"},
		{proposal.StatusReviewNeedsHuman, 2, "human"},
		{proposal.StatusReviewRunning, 3, "progress"},
		{proposal.StatusReadyToSubmit, 4, "done"},
		{proposal.StatusSubmitted, 4, "submitted"},
		{"outline:failed", 0, "failed"},
		{"writer:failed", 1, "failed"},
		{"final-review:failed", 3, "failed"},
		{"", 0, "progress"},
	}
	for _, c := range cases {
		idx, st := View(c.status)
		if idx != c.idx || st != c.state {
			t.Errorf("View(%q) = (%d,%q), want (%d,%q)", c.status, idx, st, c.idx, c.state)
		}
	}
}

// TestStatusPhrase locks the named-teammate present-tense status copy.
func TestStatusPhrase(t *testing.T) {
	cases := []struct {
		idx   int
		state string
		want  string
	}{
		{2, "human", "Paused for your review"},
		{4, "done", "Ready to submit"},
		{4, "submitted", "Submitted"},
		{0, "failed", "Outline hit a problem"},
		{0, "progress", "Noa outlining now"},
		{1, "progress", "Tomás drafting now"},
		{3, "progress", "Vera finalizing"},
		{2, "progress", "Human Review in progress"},
	}
	for _, c := range cases {
		if got := StatusPhrase(c.idx, c.state); got != c.want {
			t.Errorf("StatusPhrase(%d,%q) = %q, want %q", c.idx, c.state, got, c.want)
		}
	}
}

// TestRequirementAddressed locks the criteria matcher (issue #246 B6): term
// overlap + light stemming, not a verbatim phrase match. Cases cover verbatim,
// paraphrase, and genuinely-absent.
func TestRequirementAddressed(t *testing.T) {
	cases := []struct {
		name  string
		draft string // lowercased, as deriveCriteria passes it
		req   string
		want  bool
	}{
		{"verbatim", "we use fedramp high authorization controls", "FedRAMP High authorization", true},
		{"paraphrase stem", "deployed only fedramp high authorized tooling", "FedRAMP High authorization", true},
		{"absent", "a general cloud security posture", "FedRAMP High authorization", false},
		{"single term present", "fedramp high authorized tooling", "FedRAMP", true},
		{"single term absent", "fedramp high authorized tooling", "ISO 27001 certification", false},
		{"paraphrase modernize", "we will modernize the architecture", "architecture modernization", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RequirementAddressed(c.draft, c.req); got != c.want {
				t.Errorf("RequirementAddressed(%q, %q) = %v, want %v", c.draft, c.req, got, c.want)
			}
		})
	}
}

// TestDeriveCriteria locks the shared gate criteria (issue #246 B6): a
// paraphrased must-have reads as met (no note), and a genuinely-absent one
// carries the honest "could not auto-confirm" copy — never asserted missing.
func TestDeriveCriteria(t *testing.T) {
	if DeriveCriteria(nil, "anything", nil) != nil {
		t.Errorf("no requirements should yield nil criteria")
	}
	draft := "we deploy only fedramp high authorized tooling across the environment."
	got := DeriveCriteria([]string{"FedRAMP High authorization", "ISO 27001 certification"}, draft, nil)
	if len(got) != 2 {
		t.Fatalf("want 2 criteria, got %d", len(got))
	}
	if !got[0].OK || got[0].Note != "" {
		t.Errorf("paraphrased must-have should be met with no note, got %+v", got[0])
	}
	if got[1].OK || got[1].Note != "Kaimi could not auto-confirm this — verify in the draft" {
		t.Errorf("absent must-have should carry honest copy, got %+v", got[1])
	}
}

// TestDeriveCriteria_DefersToFlags proves the criteria grid never contradicts the
// Final Review (tester-reported): when the review flags a must-have as not
// addressed, the grid shows it not-addressed too — even if the lenient
// term-overlap matcher would otherwise pass it. One source of truth at the gate.
func TestDeriveCriteria_DefersToFlags(t *testing.T) {
	// The draft contains the requirement's keywords, so RequirementAddressed
	// (term-overlap) would return true — but the Final Review flagged it.
	draft := "we deploy fedramp high authorized tooling across the environment."
	flags := []string{`[must_have] requirement "FedRAMP High authorization" not addressed in draft`}
	got := DeriveCriteria([]string{"FedRAMP High authorization"}, draft, flags)
	if len(got) != 1 {
		t.Fatalf("want 1 criterion, got %d", len(got))
	}
	if got[0].OK {
		t.Errorf("a flagged must-have must show NOT addressed even when the matcher would pass, got %+v", got[0])
	}
	if got[0].Note == "" {
		t.Errorf("a flagged must-have needs an explanatory note")
	}
}

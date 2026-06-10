package dashboard

import (
	"fmt"
	"html/template"
	"strings"
	"testing"
)

// TestAgentGradientIsStyleSafe proves each agent's HueBG (a linear-gradient)
// survives interpolation into a style attribute. If HueBG were a plain string,
// html/template's CSS sanitizer would reject the gradient and emit ZgotmplZ,
// blanking the avatar background in the workspace progress + gate states. Typing
// HueBG as template.CSS (the values are static constants) keeps it verbatim.
func TestAgentGradientIsStyleSafe(t *testing.T) {
	tmpl := template.Must(template.New("avatar").Parse(`<span style="background:{{.HueBG}}"></span>`))
	for key, a := range agents {
		var b strings.Builder
		if err := tmpl.Execute(&b, a); err != nil {
			t.Fatalf("execute agent %q: %v", key, err)
		}
		got := b.String()
		if strings.Contains(got, "ZgotmplZ") {
			t.Errorf("agent %q: HueBG sanitized to ZgotmplZ — it must be template.CSS, got:\n%s", key, got)
		}
		if !strings.Contains(got, "linear-gradient(") {
			t.Errorf("agent %q: gradient missing from the style attr, got:\n%s", key, got)
		}
	}
}

func TestStatusBadgeKinds(t *testing.T) {
	tests := []struct {
		kind      StatusKind
		wantClass string
	}{
		{StatusPending, "kbadge--pending"},
		{StatusProgress, "kbadge--progress"},
		{StatusDone, "kbadge--done"},
		{StatusHuman, "kbadge--human"},
		{StatusFailed, "kbadge--failed"},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			got := string(StatusBadge(tt.kind, "Some Label"))
			assertWellFormedXML(t, got)
			if !strings.Contains(got, tt.wantClass) {
				t.Errorf("StatusBadge(%q) missing class %q in:\n%s", tt.kind, tt.wantClass, got)
			}
			if !strings.Contains(got, "Some Label") {
				t.Errorf("StatusBadge(%q) missing label in:\n%s", tt.kind, got)
			}
			if !strings.Contains(got, `class="dot"`) {
				t.Errorf("StatusBadge(%q) missing leading dot in:\n%s", tt.kind, got)
			}
		})
	}
}

func TestStatusBadgeUnknownKindFallsBackToPending(t *testing.T) {
	got := string(StatusBadge(StatusKind("nonsense"), "X"))
	if !strings.Contains(got, "kbadge--pending") {
		t.Errorf("unknown status kind should render as pending (the quietest), got:\n%s", got)
	}
}

func TestStatusBadgeEscapesLabel(t *testing.T) {
	got := string(StatusBadge(StatusDone, `A & B <i>sneaky</i>`))
	assertWellFormedXML(t, got)
	if !strings.Contains(got, "A &amp; B") || strings.Contains(got, "<i>") {
		t.Errorf("StatusBadge must HTML-escape the label, got:\n%s", got)
	}
}

func TestRecommendationPill(t *testing.T) {
	tests := []struct {
		rec       string
		wantClass string
		wantLabel string
	}{
		{"BID", "krec--bid", "Bid"},
		{"NO_BID", "krec--nobid", "No Bid"},
		{"REVIEW", "krec--review", "Review"},
	}
	for _, tt := range tests {
		t.Run(tt.rec, func(t *testing.T) {
			got := string(RecommendationPill(tt.rec))
			assertWellFormedXML(t, got)
			if !strings.Contains(got, tt.wantClass) || !strings.Contains(got, tt.wantLabel) {
				t.Errorf("RecommendationPill(%q) want class %q and label %q, got:\n%s",
					tt.rec, tt.wantClass, tt.wantLabel, got)
			}
			if !strings.Contains(got, "<svg") {
				t.Errorf("RecommendationPill(%q) missing icon, got:\n%s", tt.rec, got)
			}
		})
	}
}

func TestRecommendationPillUnknownIsEmpty(t *testing.T) {
	if got := RecommendationPill("MAYBE"); got != "" {
		t.Errorf("unknown recommendation should render nothing, got:\n%s", got)
	}
	if got := RecommendationPill(""); got != "" {
		t.Errorf("empty recommendation should render nothing, got:\n%s", got)
	}
}

func TestUrgencyForBoundaries(t *testing.T) {
	tests := []struct {
		days int
		want UrgencyLevel
	}{
		{-1, UrgencyCrit}, {0, UrgencyCrit}, {6, UrgencyCrit},
		{7, UrgencyNear}, {14, UrgencyNear},
		{15, UrgencySoon}, {30, UrgencySoon},
		{31, UrgencyCalm}, {90, UrgencyCalm},
	}
	for _, tt := range tests {
		if got := UrgencyFor(tt.days); got != tt.want {
			t.Errorf("UrgencyFor(%d) = %q, want %q", tt.days, got, tt.want)
		}
	}
}

func TestDeadlinePill(t *testing.T) {
	calm := string(DeadlinePill("Apr 30", 45))
	assertWellFormedXML(t, calm)
	if !strings.Contains(calm, `class="kdead"`) {
		t.Errorf("calm deadline should use the base kdead class only, got:\n%s", calm)
	}
	crit := string(DeadlinePill("Closes in 2d", 2))
	if !strings.Contains(crit, "kdead--crit") {
		t.Errorf("critical deadline should carry kdead--crit, got:\n%s", crit)
	}
	if !strings.Contains(crit, "<svg") {
		t.Errorf("deadline pill should carry the clock icon, got:\n%s", crit)
	}
	escaped := string(DeadlinePill("a<b & c", 2))
	assertWellFormedXML(t, escaped)
	if !strings.Contains(escaped, "a&lt;b &amp; c") {
		t.Errorf("DeadlinePill must HTML-escape the label, got:\n%s", escaped)
	}
}

func TestFitBandForBoundaries(t *testing.T) {
	tests := []struct {
		score int
		want  FitBand
	}{
		{100, FitStrong}, {80, FitStrong},
		{79, FitGood}, {60, FitGood},
		{59, FitFair}, {40, FitFair},
		{39, FitWeak}, {0, FitWeak},
	}
	for _, tt := range tests {
		if got := FitBandFor(tt.score); got != tt.want {
			t.Errorf("FitBandFor(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestFitRingGeometryMatchesDesignSpecimen(t *testing.T) {
	// The 44px / score-82 specimen in Kaimi Design System.html:
	// r=19, stroke 5, dasharray 119.4, dashoffset 21.5, band strong, number 82.
	got := string(FitRing(82, 44))
	assertWellFormedXML(t, got)
	for _, want := range []string{
		`data-band="strong"`,
		`r="19"`,
		`stroke-width="5"`,
		`stroke-dasharray="119.4"`,
		`stroke-dashoffset="21.5"`,
		`>82<`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("FitRing(82, 44) missing %q in:\n%s", want, got)
		}
	}
}

func TestFitRingSublabel(t *testing.T) {
	if got := string(FitRing(82, 92)); !strings.Contains(got, ">FIT<") {
		t.Errorf("FitRing at >=92px should carry the FIT sublabel, got:\n%s", got)
	}
	if got := string(FitRing(82, 44)); strings.Contains(got, ">FIT<") {
		t.Errorf("FitRing below 92px should not carry the FIT sublabel, got:\n%s", got)
	}
}

func TestFitRingClampsScore(t *testing.T) {
	over := string(FitRing(150, 44))
	if !strings.Contains(over, `stroke-dashoffset="0.0"`) || !strings.Contains(over, ">100<") {
		t.Errorf("scores above 100 should clamp to a full ring, got:\n%s", over)
	}
	under := string(FitRing(-5, 44))
	if !strings.Contains(under, `stroke-dashoffset="119.4"`) || !strings.Contains(under, ">0<") {
		t.Errorf("scores below 0 should clamp to an empty ring, got:\n%s", under)
	}
}

func TestMetaTagEscapes(t *testing.T) {
	got := string(MetaTag("NAICS <541512> & co"))
	assertWellFormedXML(t, got)
	if !strings.Contains(got, `class="ktag"`) || !strings.Contains(got, "NAICS &lt;541512&gt; &amp; co") {
		t.Errorf("MetaTag must use ktag and escape text, got:\n%s", got)
	}
}

func TestAllComponentsAreWellFormedXML(t *testing.T) {
	outputs := []string{
		string(StatusBadge(StatusHuman, "Needs Human")),
		string(RecommendationPill("BID")),
		string(RecommendationPill("NO_BID")),
		string(RecommendationPill("REVIEW")),
		string(DeadlinePill("9 days", 9)),
		string(FitRing(65, 52)),
		string(FitRing(22, 132)),
		string(MetaTag("SOL# 70RCSA24R0123")),
	}
	for i, out := range outputs {
		t.Run(fmt.Sprintf("output_%d", i), func(t *testing.T) {
			assertWellFormedXML(t, out)
		})
	}
}

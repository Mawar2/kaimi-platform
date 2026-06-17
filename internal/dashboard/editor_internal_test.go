package dashboard

import (
	"strings"
	"testing"
)

// TestHighlightGaps verifies the read-only draft rendering: gap markers are
// wrapped in <mark>, surrounding prose is preserved, and the body is
// HTML-escaped before the wrapper is added (no injection path).
func TestHighlightGaps(t *testing.T) {
	got := string(highlightGaps("Staffed by [GAP: cleared staff count] engineers."))
	want := `Staffed by <mark class="gap-mark">[GAP: cleared staff count]</mark> engineers.`
	if got != want {
		t.Errorf("highlightGaps:\n got %q\nwant %q", got, want)
	}
}

func TestHighlightGaps_EscapesMarkup(t *testing.T) {
	got := string(highlightGaps(`<script>x</script> [GAP: a <b> tag]`))
	if strings.Contains(got, "<script>") || strings.Contains(got, "<b>") {
		t.Errorf("body markup must be escaped, got %q", got)
	}
	if !strings.Contains(got, `<mark class="gap-mark">`) {
		t.Errorf("gap marker must still be wrapped, got %q", got)
	}
}

func TestHighlightGaps_NoMarker_PlainEscape(t *testing.T) {
	got := string(highlightGaps("No gaps here & none expected."))
	if strings.Contains(got, "<mark") {
		t.Errorf("no marker, no mark: %q", got)
	}
	if !strings.Contains(got, "&amp;") {
		t.Errorf("plain text must still be escaped: %q", got)
	}
}

package export

import (
	"bytes"
	"testing"
	"time"

	"github.com/go-pdf/fpdf"

	"github.com/Mawar2/Kaimi/internal/document"
)

// sampleDoc builds a small but representative proposal: a title and two sections,
// one carrying a [GAP: …] marker and one carrying typographic Unicode (a smart
// quote and an em-dash) that must survive cp1252 translation.
func sampleDoc() *document.Document {
	return &document.Document{
		OpportunityID: "OPP-123",
		Title:         "Cloud Modernization Services",
		Sections: []document.Section{
			{
				ID:      "technical_approach",
				Heading: "Technical Approach",
				Body:    "Our team delivers a phased migration.\n\nPricing detail: [GAP: pricing] must be filled before submission.",
			},
			{
				ID:      "past_performance",
				Heading: "Past Performance",
				Body:    "We ’ve delivered similar work — on time and on budget — for federal clients.",
			},
		},
	}
}

func TestRenderPDF_ValidPDF(t *testing.T) {
	opts := Options{
		CompanyName:        "Acme Federal LLC",
		SolicitationNumber: "W912-26-R-0001",
		Date:               time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC),
	}

	out, err := RenderPDF(sampleDoc(), opts)
	if err != nil {
		t.Fatalf("RenderPDF returned error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("RenderPDF returned empty bytes")
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("output does not start with %%PDF header; got %q", firstBytes(out, 8))
	}
	// A cover + TOC + two section pages is well over a few hundred bytes.
	if len(out) < 500 {
		t.Fatalf("PDF suspiciously small (%d bytes); expected a non-trivial document", len(out))
	}
}

// TestRenderPDF_NilDoc verifies a nil document yields a valid cover-only PDF and
// never panics.
func TestRenderPDF_NilDoc(t *testing.T) {
	out, err := RenderPDF(nil, Options{})
	if err != nil {
		t.Fatalf("RenderPDF(nil) returned error: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("nil-doc output is not a valid PDF; got %q", firstBytes(out, 8))
	}
}

// TestRenderPDF_EmptySections verifies a document with no sections still renders
// a valid PDF (cover + empty TOC).
func TestRenderPDF_EmptySections(t *testing.T) {
	doc := &document.Document{Title: "Bare Proposal"}
	out, err := RenderPDF(doc, Options{})
	if err != nil {
		t.Fatalf("RenderPDF returned error: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatalf("empty-sections output is not a valid PDF; got %q", firstBytes(out, 8))
	}
}

// TestRenderPDF_EmptyBodyEmitsHeading verifies that a section with an empty body
// still produces a non-trivial PDF (the heading is emitted). We can't easily read
// text back out of the binary PDF, so we assert the section's presence makes the
// document materially larger than a sections-less one.
func TestRenderPDF_EmptyBodyEmitsHeading(t *testing.T) {
	withSection := &document.Document{
		Title:    "Proposal",
		Sections: []document.Section{{ID: "s1", Heading: "Empty Section", Body: ""}},
	}
	without := &document.Document{Title: "Proposal"}

	a, err := RenderPDF(withSection, Options{})
	if err != nil {
		t.Fatalf("RenderPDF(withSection) error: %v", err)
	}
	b, err := RenderPDF(without, Options{})
	if err != nil {
		t.Fatalf("RenderPDF(without) error: %v", err)
	}
	// The extra section page makes the output larger; this confirms the heading
	// path runs for an empty body rather than skipping the section entirely.
	if len(a) <= len(b) {
		t.Fatalf("empty-body section did not add a page: with=%d bytes, without=%d bytes", len(a), len(b))
	}
}

// TestCP1252Translation verifies that the cp1252 translator maps typographic
// Unicode (em-dash, smart quote) to a single renderable cp1252 byte rather than
// dropping it or producing UTF-8 mojibake. This guards the core mechanism that
// lets RenderPDF show real punctuation without embedding a font.
func TestCP1252Translation(t *testing.T) {
	pdf := fpdf.New("P", "mm", "Letter", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	cases := []struct {
		name string
		in   string
		// want is the expected single cp1252 byte for the (single-rune) input.
		want byte
	}{
		{"em-dash", "—", 0x97},            // — -> cp1252 0x97
		{"en-dash", "–", 0x96},            // – -> cp1252 0x96
		{"right-single-quote", "’", 0x92}, // ’ -> cp1252 0x92
		{"left-double-quote", "“", 0x93},  // “ -> cp1252 0x93
		{"e-acute", "é", 0xE9},            // é -> cp1252 0xE9 (same code point)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := tr(c.in)
			// The UTF-8 source is multi-byte; a correct translation collapses it
			// to the single cp1252 byte (no mojibake, no drop).
			if len(got) != 1 {
				t.Fatalf("translation of %q produced %d bytes (% x); want 1 cp1252 byte 0x%02x",
					c.in, len(got), []byte(got), c.want)
			}
			if got[0] != c.want {
				t.Fatalf("translation of %q = 0x%02x; want 0x%02x", c.in, got[0], c.want)
			}
		})
	}
}

// TestGapMarkerSurvivesTranslation confirms the cp1252 translator passes a
// [GAP: …] marker through unchanged (it is plain ASCII).
func TestGapMarkerSurvivesTranslation(t *testing.T) {
	pdf := fpdf.New("P", "mm", "Letter", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	const marker = "[GAP: pricing]"
	if got := tr(marker); got != marker {
		t.Fatalf("GAP marker was altered by translation: got %q want %q", got, marker)
	}
}

// firstBytes returns up to n bytes for diagnostic messages.
func firstBytes(b []byte, n int) []byte {
	if len(b) < n {
		return b
	}
	return b[:n]
}

package export

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/document"
)

// readZipEntry returns the bytes of the named entry from a ZIP archive given as bytes.
func readZipEntry(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("returned bytes are not a valid ZIP (.docx): %v", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %q: %v", name, err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %q: %v", name, err)
		}
		return b
	}
	t.Fatalf("zip entry %q not found in .docx", name)
	return nil
}

// TestRenderDOCX verifies the rendered file is a valid .docx (a ZIP with word/document.xml)
// and that the proposal title, a section heading, and any [GAP: …] marker all survive into the
// document body. The [GAP: …] assertion is the load-bearing one: gaps are human placeholders
// and must never be stripped.
func TestRenderDOCX(t *testing.T) {
	doc := &document.Document{
		OpportunityID: "TEST-001",
		Title:         "Cybersecurity Modernization Proposal",
		Sections: []document.Section{
			{
				ID:      "technical_approach",
				Heading: "Technical Approach",
				Body:    "We deliver a zero-trust architecture.\n\nPhased rollout over twelve months.",
			},
			{
				ID:      "pricing",
				Heading: "Pricing",
				Body:    "Our pricing is competitive.\n\n[GAP: pricing] Final rate card pending contracts review.",
			},
		},
	}

	opts := Options{
		CompanyName:        "Acme Federal Systems",
		SolicitationNumber: "W912-26-R-0001",
		// Fixed date keeps the test deterministic.
		Date: time.Date(2026, time.June, 24, 0, 0, 0, 0, time.UTC),
	}

	data, err := RenderDOCX(doc, opts)
	if err != nil {
		t.Fatalf("RenderDOCX returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("RenderDOCX returned empty bytes")
	}

	// A .docx is a ZIP; word/document.xml holds the body content.
	xml := string(readZipEntry(t, data, "word/document.xml"))

	wantSubstrings := []string{
		"Cybersecurity Modernization Proposal", // proposal title (cover page)
		"Technical Approach",                   // a section heading
		"[GAP: pricing]",                       // the gap marker must survive verbatim
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(xml, want) {
			t.Errorf("word/document.xml missing expected text %q", want)
		}
	}
}

// TestRenderDOCX_NilDoc verifies a nil document still produces a valid cover-only .docx and
// never panics.
func TestRenderDOCX_NilDoc(t *testing.T) {
	data, err := RenderDOCX(nil, Options{})
	if err != nil {
		t.Fatalf("RenderDOCX(nil) returned error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("RenderDOCX(nil) returned empty bytes")
	}
	// Must still be a valid .docx with the body part present.
	_ = readZipEntry(t, data, "word/document.xml")
}

package export

import (
	"bytes"
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"

	"github.com/Mawar2/Kaimi/internal/document"
)

// PDF page geometry, in millimetres (fpdf's working unit). Federal proposals are
// printed on US Letter, and 1" margins are the conventional default — each
// solicitation's Section L overrides format, so we ship a clean baseline the user
// adapts per-RFP rather than a fixed template.
const (
	pdfPageSize   = "Letter"
	pdfMarginMM   = 25.4 // 1 inch
	pdfFontFamily = "Helvetica"
)

// dateLayout is the human-friendly cover-page date format (Go reference time).
const dateLayout = "January 2, 2006"

// RenderPDF renders a proposal document to a PDF as a byte slice, ready to be
// written to a file or streamed to an HTTP response.
//
// The layout is: a centered cover page (company, title, optional solicitation
// number, date), a table of contents, then each section with a bold heading and
// wrapped body prose. Every body page carries a "Page X of Y" footer.
//
// Text is emitted through fpdf's cp1252 translator so smart quotes, em/en dashes,
// and accented Latin characters render correctly with the built-in core fonts
// without embedding a font asset. Characters outside cp1252 (emoji, CJK) are out
// of scope and will not render — that is acceptable for federal English prose.
//
// [GAP: …] markers are preserved verbatim and never stripped; they signal
// unfinished content the human must resolve before submission.
//
// A nil document or one with no sections still produces a valid cover-only PDF;
// RenderPDF never panics on empty input.
func RenderPDF(doc *document.Document, opts Options) ([]byte, error) {
	// "P"ortrait, millimetres, Letter, no external font directory (core fonts only).
	pdf := fpdf.New("P", "mm", pdfPageSize, "")
	pdf.SetMargins(pdfMarginMM, pdfMarginMM, pdfMarginMM)
	pdf.SetAutoPageBreak(true, pdfMarginMM)

	// tr translates UTF-8 to cp1252 so the core fonts render typographic
	// punctuation and accented Latin. Empty descriptor == cp1252 default.
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	// The footer alias is substituted with the final page count when the
	// document is closed; "{nb}" is fpdf's conventional placeholder.
	pdf.AliasNbPages("")

	// Resolve cover-page identity once.
	company := opts.CompanyOrDefault()
	title := TitleOrDefault(doc)
	date := opts.Date
	if date.IsZero() {
		date = time.Now()
	}

	// The footer renders on every page added after it is set. We set it before
	// the body pages but render the cover page first with the footer suppressed,
	// so the cover stays clean (no page number on a title page).
	footerOn := false
	solNum := opts.SolicitationNumber
	pdf.SetFooterFunc(func() {
		if !footerOn {
			return
		}
		pdf.SetY(-15) // 15mm up from the bottom
		pdf.SetFont(pdfFontFamily, "I", 8)
		pdf.SetTextColor(120, 120, 120)
		label := fmt.Sprintf("Page %d of {nb}", pdf.PageNo())
		if solNum != "" {
			label = tr(solNum) + "   |   " + label
		}
		pdf.CellFormat(0, 10, label, "", 0, "C", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})

	// --- Cover page ---
	pdf.AddPage()
	renderCover(pdf, tr, company, title, solNum, date)

	// --- Body ---
	footerOn = true
	pdf.AddPage()
	renderTableOfContents(pdf, tr, doc)
	renderSections(pdf, tr, doc)

	// Collect the bytes. fpdf accumulates an internal error; Output surfaces it.
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("render pdf: %w", err)
	}
	return buf.Bytes(), nil
}

// renderCover lays out the centered title page. tr translates each string to
// cp1252 before it is drawn.
func renderCover(pdf *fpdf.Fpdf, tr func(string) string, company, title, solNum string, date time.Time) {
	_, pageH := pdf.GetPageSize()

	// Push the block roughly into the vertical upper-third for a title-page feel.
	pdf.SetY(pageH * 0.30)

	pdf.SetFont(pdfFontFamily, "B", 28)
	pdf.MultiCell(0, 12, tr(company), "", "C", false)

	pdf.Ln(10)
	pdf.SetFont(pdfFontFamily, "", 18)
	pdf.MultiCell(0, 9, tr(title), "", "C", false)

	if solNum != "" {
		pdf.Ln(8)
		pdf.SetFont(pdfFontFamily, "", 12)
		pdf.MultiCell(0, 7, tr("Solicitation No. "+solNum), "", "C", false)
	}

	pdf.Ln(8)
	pdf.SetFont(pdfFontFamily, "", 12)
	pdf.MultiCell(0, 7, tr(date.Format(dateLayout)), "", "C", false)
}

// renderTableOfContents prints a "Table of Contents" heading followed by one
// line per section heading. It is a static list (no live page links) — the
// section ordering matches the body below it.
func renderTableOfContents(pdf *fpdf.Fpdf, tr func(string) string, doc *document.Document) {
	pdf.SetFont(pdfFontFamily, "B", 18)
	pdf.MultiCell(0, 10, tr("Table of Contents"), "", "L", false)
	pdf.Ln(4)

	pdf.SetFont(pdfFontFamily, "", 12)
	if doc == nil || len(doc.Sections) == 0 {
		pdf.SetTextColor(120, 120, 120)
		pdf.MultiCell(0, 7, tr("(No sections)"), "", "L", false)
		pdf.SetTextColor(0, 0, 0)
		return
	}
	for i := range doc.Sections {
		heading := doc.Sections[i].Heading
		if heading == "" {
			heading = fmt.Sprintf("Section %d", i+1)
		}
		pdf.MultiCell(0, 7, tr(fmt.Sprintf("%d.  %s", i+1, heading)), "", "L", false)
	}
}

// renderSections prints each section: a bold heading on its own page, then the
// body broken into wrapped paragraphs. An empty body still emits the heading.
// [GAP: …] markers in the body are drawn verbatim.
func renderSections(pdf *fpdf.Fpdf, tr func(string) string, doc *document.Document) {
	if doc == nil {
		return
	}
	for i := range doc.Sections {
		sec := doc.Sections[i]

		// Each section starts on a fresh page so headings stay anchored to their
		// content, matching how reviewers expect compliance sections to read.
		pdf.AddPage()

		heading := sec.Heading
		if heading == "" {
			heading = fmt.Sprintf("Section %d", i+1)
		}
		pdf.SetFont(pdfFontFamily, "B", 16)
		pdf.MultiCell(0, 9, tr(fmt.Sprintf("%d.  %s", i+1, heading)), "", "L", false)
		pdf.Ln(3)

		pdf.SetFont(pdfFontFamily, "", 11)
		for _, para := range bodyParagraphs(sec.Body) {
			// tr preserves "[GAP: …]" verbatim — those bytes are plain ASCII and
			// pass through cp1252 translation unchanged.
			pdf.MultiCell(0, 6, tr(para), "", "L", false)
			pdf.Ln(3)
		}
	}
}

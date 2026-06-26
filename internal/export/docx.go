package export

import (
	"bytes"
	"fmt"
	"time"

	"github.com/gomutex/godocx"

	"github.com/Mawar2/Kaimi/internal/document"
)

// coverDateLayout is the human-friendly cover-page date format (e.g. "January 2, 2006").
const coverDateLayout = "January 2, 2006"

// RenderDOCX renders a finished proposal Document into a Microsoft Word (.docx) file and
// returns it as bytes. The output has a cover page (company, title, optional solicitation
// number, date), a static Table of Contents listing each section in order, and then every
// section rendered as a level-1 heading followed by its body paragraphs.
//
// Any "[GAP: …]" markers in section bodies are preserved verbatim — they are placeholders the
// human fills before submission and must never be silently dropped. A nil doc or a doc with no
// sections still produces a valid cover-only .docx (this function never panics).
func RenderDOCX(doc *document.Document, opts Options) ([]byte, error) {
	d, err := godocx.NewDocument()
	if err != nil {
		return nil, fmt.Errorf("create docx document: %w", err)
	}

	// 1. Cover page: company name as the title, proposal title beneath it, then the optional
	// solicitation number and the generation date. A page break ends the cover.
	if _, err := d.AddHeading(opts.CompanyOrDefault(), 0); err != nil {
		return nil, fmt.Errorf("write cover company heading: %w", err)
	}
	if _, err := d.AddHeading(TitleOrDefault(doc), 1); err != nil {
		return nil, fmt.Errorf("write cover title heading: %w", err)
	}
	if opts.SolicitationNumber != "" {
		d.AddParagraph("Solicitation No. " + opts.SolicitationNumber)
	}
	coverDate := opts.Date
	if coverDate.IsZero() {
		coverDate = time.Now()
	}
	d.AddParagraph(coverDate.Format(coverDateLayout))
	d.AddPageBreak()

	// 2. Table of Contents: a static list — one paragraph per section heading, in order. This
	// is intentionally a plain list, not a live Word TOC field, which keeps the renderer
	// dependency-free and the output adaptable per-RFP.
	if _, err := d.AddHeading("Table of Contents", 1); err != nil {
		return nil, fmt.Errorf("write table of contents heading: %w", err)
	}
	if doc != nil {
		for i := range doc.Sections {
			d.AddParagraph(doc.Sections[i].Heading)
		}
	}

	// 3. Each section: its heading (level 1) followed by one paragraph per body paragraph. An
	// empty body still emits the heading so the document keeps its compliance structure.
	if doc != nil {
		for i := range doc.Sections {
			sec := doc.Sections[i]
			if _, err := d.AddHeading(sec.Heading, 1); err != nil {
				return nil, fmt.Errorf("write section heading %q: %w", sec.Heading, err)
			}
			// bodyParagraphs preserves [GAP: …] markers verbatim; we add each paragraph as-is.
			for _, para := range bodyParagraphs(sec.Body) {
				d.AddParagraph(para)
			}
		}
	}

	// 4. Serialize to bytes. godocx writes the .docx ZIP to an io.Writer; we capture it in a
	// buffer so the caller gets the file as []byte (no temp file on disk).
	var buf bytes.Buffer
	if err := d.Write(&buf); err != nil {
		return nil, fmt.Errorf("serialize docx: %w", err)
	}
	return buf.Bytes(), nil
}

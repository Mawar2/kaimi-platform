// Package export renders a finished proposal (internal/document.Document) into the
// downloadable, shareable formats federal users need — Microsoft Word (.docx) and PDF —
// from one structured model. It preserves the proposal's compliance structure: sections in
// order, real headings, and any [GAP: …] markers left for the human to fill (never silently
// dropped). It also adds a cover page and a table of contents.
//
// No external services or APIs: the renderers are pure-Go (docx.go via gomutex/godocx, MIT;
// pdf.go via go-pdf/fpdf, MIT). Federal proposals have no universal template — each
// solicitation's Section L dictates format — so this produces clean, conventionally
// formatted documents (1" margins, a standard serif/sans body, page numbers) the user
// adapts per-RFP, not a fixed one-size template.
package export

import (
	"strings"
	"time"

	"github.com/Mawar2/Kaimi/internal/document"
)

// defaultCompanyLabel is the neutral cover-page label when no company name is supplied, so a
// rendered document never bakes in the wrong (or a placeholder) company.
const defaultCompanyLabel = "The Offeror"

// Options carries the cover-page identity fields that are not part of the document model.
type Options struct {
	// CompanyName is the offeror shown on the cover page. Empty falls back to defaultCompanyLabel.
	CompanyName string
	// SolicitationNumber is the RFP/solicitation number for the cover page + footers. Optional.
	SolicitationNumber string
	// Date is the generation date shown on the cover page. Zero falls back to the renderer's now.
	Date time.Time
}

// CompanyOrDefault returns the cover-page company name, or the neutral default when unset.
func (o Options) CompanyOrDefault() string {
	if c := strings.TrimSpace(o.CompanyName); c != "" {
		return c
	}
	return defaultCompanyLabel
}

// TitleOrDefault returns the document title, or a neutral default when the document has none.
func TitleOrDefault(doc *document.Document) string {
	if doc != nil {
		if t := strings.TrimSpace(doc.Title); t != "" {
			return t
		}
	}
	return "Proposal"
}

// bodyParagraphs splits a section body into paragraphs on blank lines, trimming each, so both
// renderers lay prose out the same way. Empty paragraphs are dropped.
func bodyParagraphs(body string) []string {
	var out []string
	for _, para := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n") {
		if p := strings.TrimSpace(para); p != "" {
			out = append(out, p)
		}
	}
	return out
}

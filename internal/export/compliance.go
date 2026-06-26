package export

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Mawar2/Kaimi/internal/document"
)

// complianceHeader is the data-grid header for the compliance matrix CSV. The
// columns trace each must-have requirement to where (if anywhere) the proposal
// addresses it, plus the coverage status the user works the gaps from.
var complianceHeader = []string{"#", "Requirement", "Source", "Status", "Addressed in", "Notes"}

// emptyMatrixNote is the single row written when no requirements were extracted,
// so the download is always a usable manual template rather than an empty file.
const emptyMatrixNote = "No must-have requirements were extracted — fill this matrix from the solicitation's Section L/M."

// sourceParen extracts a leading-or-embedded solicitation reference like
// "(Section L)", "(Section M.3)" or "(SOW 3.2)" from a flag's text, so the
// matrix can attribute a gap to its part of the solicitation when the Final
// Review named it.
var sourceParen = regexp.MustCompile(`\((?:Section\s+[LM]|SOW|PWS|Section\s+C)[^)]*\)`)

// wordSplit tokenizes a requirement into comparable words for the keyword
// overlap test (lowercase alphanumerics; punctuation dropped).
var wordSplit = regexp.MustCompile(`[^a-z0-9]+`)

// stopWords are common words excluded from requirement-to-section keyword
// matching so a shared "the"/"and" never counts as coverage.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "shall": true,
	"must": true, "will": true, "that": true, "this": true, "from": true,
	"all": true, "any": true, "are": true, "per": true, "into": true,
	"a": true, "an": true, "of": true, "to": true, "in": true, "on": true,
	"or": true, "as": true, "at": true, "by": true, "be": true, "is": true,
}

// RenderComplianceCSV renders a compliance matrix CSV that traces each must-have
// requirement (extracted by the Scorer) to the proposal's coverage. It is pure
// stdlib (encoding/csv) and reads only the document's section bodies and its
// unresolved compliance flags — never the unpopulated RequirementIDs links.
//
// Status per requirement is one of:
//   - GAP: an unresolved Final-Review flag names the requirement (Notes carries
//     the flag text; Source is the solicitation reference parsed from it, if any).
//   - Addressed: a section body covers the requirement (Addressed in names the
//     matching section heading(s)).
//   - Review: neither flagged nor clearly covered — the user must check it.
//
// A nil document is treated as having no sections or flags. When requirements is
// empty the matrix still renders its header plus one template row, never a crash.
func RenderComplianceCSV(doc *document.Document, requirements []string, opts Options) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	date := opts.Date
	if date.IsZero() {
		date = time.Now()
	}

	// Title block: a few single-cell rows above the data grid so the file opens
	// with context in any spreadsheet, then a blank row, then the header.
	rows := [][]string{
		{"Compliance matrix"},
		{TitleOrDefault(doc)},
	}
	if s := strings.TrimSpace(opts.SolicitationNumber); s != "" {
		rows = append(rows, []string{"Solicitation No. " + s})
	}
	rows = append(rows,
		[]string{"Generated " + date.Format("January 2, 2006")},
		[]string{}, // blank separator row
		complianceHeader,
	)
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return nil, fmt.Errorf("write compliance matrix header: %w", err)
		}
	}

	// Pre-compute the unresolved flag texts once (stable order) so each
	// requirement scans the same list deterministically.
	var flags []string
	if doc != nil {
		flags = doc.OpenFlagTexts()
	}

	// Build the data rows. Trim + skip empty requirements but keep the visible
	// numbering contiguous over the rows we actually emit.
	wrote := false
	num := 0
	for _, raw := range requirements {
		req := strings.TrimSpace(raw)
		if req == "" {
			continue
		}
		num++
		wrote = true
		status, source, addressedIn, notes := classifyRequirement(req, doc, flags)
		if err := w.Write([]string{fmt.Sprintf("%d", num), req, source, status, addressedIn, notes}); err != nil {
			return nil, fmt.Errorf("write compliance matrix row: %w", err)
		}
	}

	if !wrote {
		if err := w.Write([]string{"1", emptyMatrixNote, "", "Review", "", ""}); err != nil {
			return nil, fmt.Errorf("write compliance matrix template row: %w", err)
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("flush compliance matrix: %w", err)
	}
	return buf.Bytes(), nil
}

// classifyRequirement decides one requirement's status and supporting columns.
// flags is the document's pre-computed unresolved flag texts. It is deterministic
// (no map iteration over output; sections scanned in document order).
func classifyRequirement(req string, doc *document.Document, flags []string) (status, source, addressedIn, notes string) {
	reqNorm := strings.ToLower(req)

	// GAP wins: an unresolved Final-Review flag that names this requirement.
	for _, f := range flags {
		if strings.Contains(strings.ToLower(f), reqNorm) {
			src := ""
			if m := sourceParen.FindString(f); m != "" {
				// Strip the surrounding parentheses for a clean cell value.
				src = strings.TrimSpace(strings.Trim(m, "()"))
			}
			return "GAP", src, "", strings.TrimSpace(f)
		}
	}

	// Addressed: a section body mentions the requirement — either the whole
	// requirement as a substring, or a strong keyword overlap with its
	// significant words (so a paraphrase still counts).
	var headings []string
	if doc != nil {
		keywords := significantWords(reqNorm)
		for i := range doc.Sections {
			body := strings.ToLower(doc.Sections[i].Body)
			if body == "" {
				continue
			}
			if strings.Contains(body, reqNorm) || keywordOverlap(body, keywords) {
				h := strings.TrimSpace(doc.Sections[i].Heading)
				if h == "" {
					h = strings.TrimSpace(doc.Sections[i].ID)
				}
				if h != "" {
					headings = append(headings, h)
				}
			}
		}
	}
	if len(headings) > 0 {
		return "Addressed", "", strings.Join(headings, "; "), ""
	}

	// Otherwise the requirement is not clearly covered — flag it for review.
	return "Review", "", "", ""
}

// significantWords returns the de-duplicated content words of a normalized
// requirement (lowercase), dropping stop words and tokens shorter than three
// characters so the overlap test keys on meaningful terms.
func significantWords(reqNorm string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, tok := range wordSplit.Split(reqNorm, -1) {
		if len(tok) < 3 || stopWords[tok] || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	return out
}

// keywordOverlap reports whether a section body covers a requirement's keywords.
// It requires that the majority of the requirement's significant words appear in
// the body (and at least two), so an incidental single shared word is not counted
// as coverage. With one keyword it requires that single word to be present.
func keywordOverlap(body string, keywords []string) bool {
	if len(keywords) == 0 {
		return false
	}
	hits := 0
	for _, kw := range keywords {
		if strings.Contains(body, kw) {
			hits++
		}
	}
	if len(keywords) == 1 {
		return hits == 1
	}
	// Majority of keywords present, with a floor of two, treats a paraphrase as
	// coverage while rejecting a single coincidental word match.
	return hits >= 2 && hits*2 >= len(keywords)
}

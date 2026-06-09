// outline-probe fetches opportunities matching BlueMeta's capability profile,
// downloads attached solicitation PDFs, extracts their text with pdftotext, and
// runs the Outline agent to show what formatting rules and sections are produced.
//
// Usage:
//
//	# Cached mode (no API key needed — uses test/fixtures/samgov_response.json)
//	go run ./cmd/outline-probe
//
//	# Live mode: searches SAM.gov using BlueMeta's NAICS codes
//	SAM_API_KEY=your-key go run ./cmd/outline-probe --mode=live --limit=3
//
//	# Local PDF: extract text from a file on disk and run the outline agent
//	go run ./cmd/outline-probe --pdf-file=path/to/solicitation.pdf
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Mawar2/Kaimi/internal/googledocs"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/profile"
	"github.com/Mawar2/Kaimi/internal/samgov"
)

// htmlTagRE strips HTML tags from SAM.gov description pointer responses.
var htmlTagRE = regexp.MustCompile(`<[^>]+>`)

// pdfToText is the name of the Poppler pdftotext binary, resolved via $PATH so
// the tool works across developer machines and CI runners rather than assuming
// a single OS-specific install location.
const pdfToText = "pdftotext"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "outline-probe: %v\n", err)
		os.Exit(1)
	}
}

// run parses flags and dispatches to PDF-file mode or SAM.gov (cached/live) mode.
func run() error {
	mode := flag.String("mode", "cached", "Mode: cached or live")
	limit := flag.Int("limit", 3, "Max number of opportunities to process")
	pdfFile := flag.String("pdf-file", "", "Path to a PDF file to parse directly (bypasses SAM.gov fetch)")
	flag.Parse()

	ctx := context.Background()

	if *pdfFile != "" {
		return runPDFMode(ctx, *pdfFile)
	}
	return runSAMMode(ctx, *mode, *limit)
}

// runSAMMode fetches opportunities from SAM.gov (cached or live), resolves the
// best available description text for each in live mode, and runs the outline
// agent over the result.
func runSAMMode(ctx context.Context, mode string, limit int) error {
	apiKey := os.Getenv("SAM_API_KEY")
	if mode == "live" && apiKey == "" {
		return fmt.Errorf("SAM_API_KEY environment variable required for live mode")
	}

	prof := profile.BlueMeta
	fmt.Printf("Capability profile: BlueMeta Technologies\n")
	fmt.Printf("NAICS codes: %s\n", strings.Join(prof.NAICSCodes, ", "))
	fmt.Println(strings.Repeat("=", 60))

	client, err := samgov.NewClient(samgov.Config{
		APIKey:    apiKey,
		UseCached: mode == "cached",
	})
	if err != nil {
		return fmt.Errorf("create SAM.gov client: %w", err)
	}

	opps, err := client.FetchByNAICS(ctx, prof.NAICSCodes)
	if err != nil {
		return fmt.Errorf("fetch opportunities: %w", err)
	}

	if len(opps) == 0 {
		fmt.Println("No opportunities found.")
		return nil
	}

	if len(opps) > limit {
		opps = opps[:limit]
	}

	fmt.Printf("Processing %d opportunity/ies (mode=%s)\n\n", len(opps), mode)

	tmpDir, err := os.MkdirTemp("", "outline-probe-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if mode == "live" {
		for _, opp := range opps {
			resolveDescription(opp, apiKey, tmpDir)
		}
	}

	return processOpportunities(ctx, opps)
}

// runPDFMode extracts text from a local PDF file and runs the outline agent on it.
func runPDFMode(ctx context.Context, pdfPath string) error {
	fmt.Printf("PDF file: %s\n\n", pdfPath)

	tmpDir, err := os.MkdirTemp("", "outline-probe-pdf-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	text, err := extractPDFText(pdfPath, tmpDir)
	if err != nil {
		return err
	}
	fmt.Printf("Extracted %d chars of PDF text.\n\n", len(text))

	opp := &opportunity.Opportunity{
		ID:          "PDF-PROBE",
		Title:       filepath.Base(pdfPath),
		Agency:      "(from local PDF)",
		Description: text,
	}

	return processOpportunities(ctx, []*opportunity.Opportunity{opp})
}

// resolveDescription fills in the best available description text for an
// opportunity fetched in live mode, in priority order: PDF attachment →
// inline description → SAM.gov description pointer URL.
func resolveDescription(opp *opportunity.Opportunity, apiKey, tmpDir string) {
	// Priority 1: download the first PDF attachment and extract its text.
	if len(opp.Attachments) > 0 {
		for i, attURL := range opp.Attachments {
			fmt.Printf("Fetching attachment %d/%d for %s...\n", i+1, len(opp.Attachments), opp.Title)
			text, err := downloadAndExtractPDF(attURL, tmpDir)
			if err != nil {
				fmt.Printf("  Warning: %v\n", err)
				continue
			}
			fmt.Printf("  Extracted %d chars of PDF text.\n", len(text))
			opp.Description = text
			return
		}
	}

	// Priority 2: follow a SAM.gov description pointer URL.
	if strings.HasPrefix(opp.Description, "https://api.sam.gov") {
		fmt.Printf("Fetching full description from pointer URL for %s...\n", opp.Title)
		fullDesc, err := fetchDescription(opp.Description, apiKey)
		if err != nil {
			fmt.Printf("  Warning: could not fetch description: %v\n", err)
			return
		}
		opp.Description = fullDesc
		fmt.Printf("  Got %d chars of description text.\n", len(fullDesc))
	}
}

// downloadAndExtractPDF downloads a URL, saves it as a PDF, and returns extracted plain text.
func downloadAndExtractPDF(url, tmpDir string) (string, error) {
	resp, err := http.Get(url) //nolint:noctx // probe-only CLI tool; no request cancellation needed
	if err != nil {
		return "", fmt.Errorf("HTTP get: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close response body: %v\n", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "pdf") && !strings.HasSuffix(strings.ToLower(url), ".pdf") {
		return "", fmt.Errorf("not a PDF (Content-Type: %s)", ct)
	}

	pdfPath := filepath.Join(tmpDir, "attachment.pdf")
	f, err := os.Create(pdfPath)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	if _, err = io.Copy(f, resp.Body); err != nil {
		_ = f.Close() // best-effort close on write error path
		return "", fmt.Errorf("write PDF: %w", err)
	}
	if err = f.Close(); err != nil {
		return "", fmt.Errorf("close PDF file: %w", err)
	}

	return extractPDFText(pdfPath, tmpDir)
}

// extractPDFText runs pdftotext on pdfPath and returns the resulting plain text.
func extractPDFText(pdfPath, tmpDir string) (string, error) {
	txtPath := filepath.Join(tmpDir, "extracted.txt")
	cmd := exec.Command(pdfToText, pdfPath, txtPath) //nolint:gosec // G204: pdfToText is a compile-time constant, not user input
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pdftotext: %w — %s", err, strings.TrimSpace(string(out)))
	}
	raw, err := os.ReadFile(txtPath)
	if err != nil {
		return "", fmt.Errorf("read extracted text: %w", err)
	}
	return strings.Join(strings.Fields(string(raw)), " "), nil
}

// fetchDescription follows a SAM.gov description pointer URL and returns plain text.
func fetchDescription(url, apiKey string) (string, error) {
	if !strings.Contains(url, "api_key=") {
		sep := "&"
		if !strings.Contains(url, "?") {
			sep = "?"
		}
		url = url + sep + "api_key=" + apiKey
	}

	resp, err := http.Get(url) //nolint:noctx // probe-only CLI tool; no request cancellation needed
	if err != nil {
		return "", fmt.Errorf("HTTP get: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close response body: %v\n", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("SAM.gov returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	// SAM.gov wraps the description in {"description":"<html>..."}.
	text := string(body)
	text = strings.TrimPrefix(text, `{"description":"`)
	text = strings.TrimSuffix(text, `"}`)
	text = strings.ReplaceAll(text, `\n`, " ")
	text = strings.ReplaceAll(text, `\r`, "")
	text = strings.ReplaceAll(text, `\"`, `"`)
	text = htmlTagRE.ReplaceAllString(text, " ")
	return strings.Join(strings.Fields(text), " "), nil
}

// processOpportunities runs the outline agent against each opportunity in turn
// and prints the resulting sections, formatting rules, and a description excerpt.
func processOpportunities(ctx context.Context, opps []*opportunity.Opportunity) error {
	// outline-probe is a diagnostic tool and does not persist to Drive; use the
	// cached Docs client so Run() can complete without live credentials.
	docsClient, err := googledocs.NewClient(ctx, googledocs.Config{UseCached: true})
	if err != nil {
		return fmt.Errorf("create cached docs client: %w", err)
	}
	ag := outline.New(docsClient)

	for i, opp := range opps {
		printDivider()
		fmt.Printf("[%d/%d] %s\n", i+1, len(opps), opp.Title)
		fmt.Printf("  ID:        %s\n", opp.ID)
		fmt.Printf("  Agency:    %s\n", opp.Agency)
		if opp.SetAsideCode != "" {
			fmt.Printf("  Set-aside: %s\n", opp.SetAsideCode)
		} else {
			fmt.Printf("  Set-aside: (none — full and open)\n")
		}
		fmt.Println()

		ol, result, err := ag.Run(ctx, opp)
		if err != nil {
			fmt.Printf("  ERROR: %v\n\n", err)
			continue
		}
		if result.IsFailed() {
			fmt.Printf("  FAILED: %s\n\n", result.Summary)
			continue
		}

		fmt.Printf("  Generated: %s\n\n", ol.GeneratedAt.Format(time.RFC3339))
		printSections(ol.Sections)
		printFormattingRules(ol.FormattingRules)
		printDescriptionExcerpt(opp.Description)
	}

	printDivider()
	fmt.Printf("Done. %d opportunit%s processed.\n", len(opps), pluralSuffix(len(opps)))
	return nil
}

// printSections writes the derived section list to stdout.
func printSections(sections []outline.Section) {
	fmt.Printf("  Sections (%d):\n", len(sections))
	for _, s := range sections {
		fmt.Printf("    • [%s] %s\n", s.ID, s.Title)
		fmt.Printf("        Rationale: %s\n", s.Rationale)
	}
	fmt.Println()
}

// printFormattingRules writes the extracted formatting requirements to stdout.
func printFormattingRules(rules *outline.FormattingRules) {
	fmt.Println("  Formatting rules:")
	printRule("Page limit", rules.PageLimit)
	printRule("Font", rules.Font)
	printRule("Margins", rules.Margins)
	printRule("Line spacing", rules.LineSpacing)
	printRule("File format", rules.FileFormat)

	if len(rules.RequiredForms) > 0 {
		fmt.Printf("    %-16s %s\n", "Required forms:", strings.Join(rules.RequiredForms, ", "))
	} else {
		fmt.Printf("    %-16s (none specified)\n", "Required forms:")
	}
	fmt.Println()
}

// printRule prints a single formatting rule, noting when it is unspecified.
func printRule(label string, rule *outline.FormattingRule) {
	key := label + ":"
	if rule.Specified {
		fmt.Printf("    %-16s %s\n", key, rule.Value)
	} else {
		fmt.Printf("    %-16s (not specified)\n", key)
	}
}

// printDescriptionExcerpt writes the first 500 characters of the resolved
// description text, so the developer can see what text the agent worked from.
func printDescriptionExcerpt(desc string) {
	fmt.Println("  Description excerpt (first 500 chars):")
	if len(desc) > 500 {
		desc = desc[:500] + "..."
	}
	fmt.Printf("    %s\n\n", desc)
}

// printDivider prints a visual separator between opportunity blocks.
func printDivider() {
	fmt.Println(strings.Repeat("─", 60))
}

// pluralSuffix returns "y" for count==1 and "ies" otherwise, for the word "opportunit".
func pluralSuffix(count int) string {
	if count == 1 {
		return "y"
	}
	return "ies"
}

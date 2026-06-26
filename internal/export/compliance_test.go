package export_test

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/export"
)

// fixedDate gives the renderer a deterministic generation date so golden/byte
// comparisons do not depend on the wall clock.
var fixedDate = time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC)

// parseRows reads the CSV back and returns all records, failing the test on any
// parse error (which would mean malformed quoting).
func parseRows(t *testing.T, data []byte) [][]string {
	t.Helper()
	r := csv.NewReader(bytes.NewReader(data))
	// The file has a single-cell title block above the data grid, so rows are
	// intentionally ragged; -1 lets the reader accept varying field counts.
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v", err)
	}
	return recs
}

// dataRows returns only the requirement rows (everything after the header row).
func dataRows(t *testing.T, data []byte) [][]string {
	t.Helper()
	recs := parseRows(t, data)
	for i, r := range recs {
		if len(r) > 0 && r[0] == "#" {
			return recs[i+1:]
		}
	}
	t.Fatalf("no header row found in output")
	return nil
}

func TestRenderComplianceCSV_AllAddressed(t *testing.T) {
	doc := &document.Document{
		Title: "Zero Trust Modernization",
		Sections: []document.Section{
			{ID: "tech", Heading: "Technical Approach", Body: "We provide continuous monitoring of all endpoints."},
			{ID: "mgmt", Heading: "Management Plan", Body: "Our staffing plan ensures qualified personnel coverage."},
		},
	}
	reqs := []string{"continuous monitoring of endpoints", "staffing plan for personnel"}

	data, err := export.RenderComplianceCSV(doc, reqs, export.Options{Date: fixedDate})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	rows := dataRows(t, data)
	if len(rows) != 2 {
		t.Fatalf("got %d data rows, want 2", len(rows))
	}
	// Row 0 -> Technical Approach, Row 1 -> Management Plan.
	if rows[0][3] != "Addressed" {
		t.Errorf("req 1 status = %q, want Addressed", rows[0][3])
	}
	if !strings.Contains(rows[0][4], "Technical Approach") {
		t.Errorf("req 1 Addressed in = %q, want it to name Technical Approach", rows[0][4])
	}
	if rows[1][3] != "Addressed" {
		t.Errorf("req 2 status = %q, want Addressed", rows[1][3])
	}
	if !strings.Contains(rows[1][4], "Management Plan") {
		t.Errorf("req 2 Addressed in = %q, want it to name Management Plan", rows[1][4])
	}
}

func TestRenderComplianceCSV_Gap(t *testing.T) {
	doc := &document.Document{
		Title: "Cyber Support",
		Sections: []document.Section{
			{ID: "tech", Heading: "Technical Approach", Body: "Generic narrative without the specific commitment."},
		},
		Flags: []document.Flag{
			{Title: "Missing SCRM plan (Section L)", Detail: "The supply chain risk management plan is not present.", Resolved: false},
			{Title: "Resolved already", Detail: "section 508 accessibility was addressed", Resolved: true},
		},
	}
	reqs := []string{"supply chain risk management plan", "section 508 accessibility"}

	data, err := export.RenderComplianceCSV(doc, reqs, export.Options{Date: fixedDate})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	rows := dataRows(t, data)
	if len(rows) != 2 {
		t.Fatalf("got %d data rows, want 2", len(rows))
	}
	// First requirement is named by an unresolved flag -> GAP.
	if rows[0][3] != "GAP" {
		t.Errorf("req 1 status = %q, want GAP", rows[0][3])
	}
	if !strings.Contains(rows[0][5], "supply chain risk management plan") {
		t.Errorf("req 1 Notes = %q, want the flag detail", rows[0][5])
	}
	if rows[0][2] != "Section L" {
		t.Errorf("req 1 Source = %q, want \"Section L\" parsed from the flag", rows[0][2])
	}
	// Second requirement is only named by a RESOLVED flag -> must NOT be GAP.
	if rows[1][3] == "GAP" {
		t.Errorf("req 2 status = GAP, but its only flag is resolved and must be ignored")
	}
}

func TestRenderComplianceCSV_Review(t *testing.T) {
	doc := &document.Document{
		Title: "Logistics",
		Sections: []document.Section{
			{ID: "tech", Heading: "Technical Approach", Body: "Unrelated prose about scheduling and timelines."},
		},
	}
	reqs := []string{"warehouse inventory barcode tracking"}

	data, err := export.RenderComplianceCSV(doc, reqs, export.Options{Date: fixedDate})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	rows := dataRows(t, data)
	if len(rows) != 1 {
		t.Fatalf("got %d data rows, want 1", len(rows))
	}
	if rows[0][3] != "Review" {
		t.Errorf("status = %q, want Review", rows[0][3])
	}
	if rows[0][4] != "" {
		t.Errorf("Addressed in = %q, want empty for a Review row", rows[0][4])
	}
}

func TestRenderComplianceCSV_EmptyRequirements(t *testing.T) {
	doc := &document.Document{Title: "Empty", Sections: []document.Section{{ID: "x", Heading: "X", Body: "body"}}}

	data, err := export.RenderComplianceCSV(doc, nil, export.Options{Date: fixedDate})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	rows := dataRows(t, data)
	if len(rows) != 1 {
		t.Fatalf("got %d data rows, want exactly 1 template row", len(rows))
	}
	if rows[0][3] != "Review" {
		t.Errorf("template row status = %q, want Review", rows[0][3])
	}
	if !strings.Contains(rows[0][1], "No must-have requirements were extracted") {
		t.Errorf("template row requirement = %q, want the manual-template note", rows[0][1])
	}
}

func TestRenderComplianceCSV_WellFormed(t *testing.T) {
	doc := &document.Document{
		Title: "Quoting, \"Edge\" Cases",
		Sections: []document.Section{
			{ID: "tech", Heading: "Tech, Approach", Body: "We deliver reporting, dashboards, and \"analytics\"."},
		},
	}
	// Requirements deliberately contain a comma and an embedded quote to catch
	// any CSV quoting bug.
	reqs := []string{
		`reporting, dashboards, and "analytics"`,
		"unrelated requirement",
	}

	data, err := export.RenderComplianceCSV(doc, reqs, export.Options{
		Date: fixedDate, SolicitationNumber: "SOL-2026-001",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	recs := parseRows(t, data)

	// Find the header and assert every data row matches its column count.
	headerIdx := -1
	for i, r := range recs {
		if len(r) > 0 && r[0] == "#" {
			headerIdx = i
			break
		}
	}
	if headerIdx == -1 {
		t.Fatal("no header row")
	}
	want := len(recs[headerIdx])
	for i := headerIdx + 1; i < len(recs); i++ {
		if len(recs[i]) != want {
			t.Errorf("data row %d has %d columns, want %d (quoting bug?)", i, len(recs[i]), want)
		}
	}
	// The comma+quote requirement round-trips intact.
	rows := dataRows(t, data)
	if rows[0][1] != reqs[0] {
		t.Errorf("requirement round-trip = %q, want %q", rows[0][1], reqs[0])
	}
}

func TestRenderComplianceCSV_Deterministic(t *testing.T) {
	doc := &document.Document{
		Title: "Repeatable",
		Sections: []document.Section{
			{ID: "a", Heading: "Alpha", Body: "the alpha covers monitoring endpoints"},
			{ID: "b", Heading: "Beta", Body: "beta narrative"},
		},
		Flags: []document.Flag{{Title: "Missing risk register", Detail: "the risk register is absent", Resolved: false}},
	}
	reqs := []string{"monitoring endpoints", "risk register", "uncovered item"}
	opts := export.Options{Date: fixedDate, SolicitationNumber: "SOL-9"}

	a, err := export.RenderComplianceCSV(doc, reqs, opts)
	if err != nil {
		t.Fatalf("render a: %v", err)
	}
	b, err := export.RenderComplianceCSV(doc, reqs, opts)
	if err != nil {
		t.Fatalf("render b: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("output is not deterministic across identical inputs")
	}
}

func TestRenderComplianceCSV_NilDoc(t *testing.T) {
	data, err := export.RenderComplianceCSV(nil, []string{"some requirement"}, export.Options{Date: fixedDate})
	if err != nil {
		t.Fatalf("render with nil doc: %v", err)
	}
	rows := dataRows(t, data)
	if len(rows) != 1 {
		t.Fatalf("got %d data rows, want 1", len(rows))
	}
	if rows[0][3] != "Review" {
		t.Errorf("nil-doc status = %q, want Review (no sections to address it)", rows[0][3])
	}
}

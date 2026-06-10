package opportunity

import (
	"encoding/json"
	"testing"
	"time"
)

// TestOpportunity_JSONRoundTrip verifies that the Opportunity struct can be
// marshaled to JSON and back without data loss.
//
// This test ensures the schema is JSON-serializable, which is required for
// storing opportunities in the queue (currently JSON files, later Firestore).
func TestOpportunity_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second) // Truncate to avoid subsecond precision issues

	original := Opportunity{
		ID:                 "TEST-123",
		Title:              "Test Opportunity",
		SolicitationNum:    "SOL-2026-001",
		Agency:             "Department of Test",
		Office:             "Office of Testing",
		PostedDate:         now,
		ResponseDeadline:   now.Add(30 * 24 * time.Hour),
		NAICSCode:          "541512",
		NAICSDescription:   "Computer Systems Design Services",
		SetAsideCode:       "SBA",
		PlaceOfPerformance: "Washington, DC",
		Description:        "Test opportunity description",
		Type:               "Solicitation",
		ContractType:       "Firm Fixed Price",
		URL:                "https://sam.gov/test/123",
		Attachments:        []string{"https://sam.gov/test/123/rfp.pdf"},
		Score:              0.85,
		ScoreReasoning:     "Strong technical fit",
		Selected:           false,
		ProposalStatus:     "",
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal Opportunity: %v", err)
	}

	// Unmarshal back to struct
	var decoded Opportunity
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Opportunity: %v", err)
	}

	// Verify critical fields match
	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: got %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Title != original.Title {
		t.Errorf("Title mismatch: got %q, want %q", decoded.Title, original.Title)
	}
	if !decoded.PostedDate.Equal(original.PostedDate) {
		t.Errorf("PostedDate mismatch: got %v, want %v", decoded.PostedDate, original.PostedDate)
	}
	if !decoded.ResponseDeadline.Equal(original.ResponseDeadline) {
		t.Errorf("ResponseDeadline mismatch: got %v, want %v", decoded.ResponseDeadline, original.ResponseDeadline)
	}
	if decoded.Score != original.Score {
		t.Errorf("Score mismatch: got %f, want %f", decoded.Score, original.Score)
	}
}

// TestSolicitationDoc_JSONRoundTrip verifies that a SolicitationDoc — the
// post-ingestion record of a solicitation attachment stored in GCS — survives a
// JSON marshal/unmarshal without losing any field. The ingest stage (Ticket C)
// populates these; the Store persists them as part of the Opportunity.
func TestSolicitationDoc_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	original := SolicitationDoc{
		Filename:    "RFP_Section_L.pdf",
		SourceURL:   "https://sam.gov/test/123/RFP_Section_L.pdf",
		ContentType: "application/pdf",
		RawObject:   "gs://kaimi-solicitations/TEST-123/raw/RFP_Section_L.pdf",
		TextObject:  "gs://kaimi-solicitations/TEST-123/text/RFP_Section_L.pdf.txt",
		SHA256:      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Bytes:       204_800,
		IngestedAt:  now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal SolicitationDoc: %v", err)
	}

	var decoded SolicitationDoc
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal SolicitationDoc: %v", err)
	}

	if decoded != original {
		t.Errorf("SolicitationDoc round-trip mismatch:\n got  %+v\n want %+v", decoded, original)
	}
}

// TestOpportunity_DocumentsJSONRoundTrip verifies that the Documents slice on an
// Opportunity round-trips through JSON. This is the forward-compatible schema
// hook the ingest stage and Manager (Tickets C/D) populate; Attachments (the raw
// SAM.gov URL list) is retained alongside it, unchanged.
func TestOpportunity_DocumentsJSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	original := Opportunity{
		ID:          "TEST-123",
		Title:       "Test Opportunity",
		Attachments: []string{"https://sam.gov/test/123/rfp.pdf"},
		Documents: []SolicitationDoc{
			{
				Filename:   "rfp.pdf",
				SourceURL:  "https://sam.gov/test/123/rfp.pdf",
				RawObject:  "gs://kaimi-solicitations/TEST-123/raw/rfp.pdf",
				TextObject: "gs://kaimi-solicitations/TEST-123/text/rfp.pdf.txt",
				SHA256:     "abc123",
				IngestedAt: now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal Opportunity: %v", err)
	}

	var decoded Opportunity
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Opportunity: %v", err)
	}

	if len(decoded.Documents) != 1 {
		t.Fatalf("Documents length mismatch: got %d, want 1", len(decoded.Documents))
	}
	if decoded.Documents[0] != original.Documents[0] {
		t.Errorf("Documents[0] mismatch:\n got  %+v\n want %+v", decoded.Documents[0], original.Documents[0])
	}
	// Attachments must be retained unchanged alongside Documents.
	if len(decoded.Attachments) != 1 || decoded.Attachments[0] != original.Attachments[0] {
		t.Errorf("Attachments not preserved: got %v, want %v", decoded.Attachments, original.Attachments)
	}
}

// TestOpportunity_EmptyInitialization verifies that an Opportunity can be
// created with zero values and that optional fields handle nil properly.
func TestOpportunity_EmptyInitialization(t *testing.T) {
	var opp Opportunity

	// Verify zero values are safe
	if opp.ID != "" {
		t.Errorf("Expected empty ID, got %q", opp.ID)
	}
	if opp.Score != 0.0 {
		t.Errorf("Expected zero Score, got %f", opp.Score)
	}
	if opp.Selected {
		t.Error("Expected Selected to be false")
	}

	// Verify pointer fields are nil
	if opp.ScoredAt != nil {
		t.Error("Expected ScoredAt to be nil")
	}
	if opp.SelectedAt != nil {
		t.Error("Expected SelectedAt to be nil")
	}
}

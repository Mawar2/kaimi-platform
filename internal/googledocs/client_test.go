package googledocs

import (
	"context"
	"strings"
	"testing"
)

// TestConfig_Defaults verifies that Config has sensible zero values.
func TestConfig_Defaults(t *testing.T) {
	var cfg Config

	if len(cfg.CredentialsJSON) != 0 {
		t.Errorf("Expected empty CredentialsJSON, got %q", cfg.CredentialsJSON)
	}
	if cfg.DestinationID != "" {
		t.Errorf("Expected empty DestinationID, got %q", cfg.DestinationID)
	}
	if cfg.UseCached {
		t.Error("Expected UseCached to be false")
	}
}

// TestNewClient_CachedMode verifies that NewClient creates a cached client correctly.
func TestNewClient_CachedMode(t *testing.T) {
	cfg := Config{
		UseCached: true,
	}

	client, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Failed to create cached client: %v", err)
	}
	if client == nil {
		t.Error("Expected non-nil client")
	}
}

// TestNewClient_LiveModeNoCredentials verifies that creating a live client without
// credentials fails with a clear error before any network call is attempted.
func TestNewClient_LiveModeNoCredentials(t *testing.T) {
	cfg := Config{
		UseCached:     false,
		DestinationID: "destination-id",
	}

	_, err := NewClient(context.Background(), cfg)
	if err == nil {
		t.Fatal("Expected error when creating live client without credentials")
	}
}

// TestNewClient_ADCMode_NoDestination verifies that ADC mode still requires a
// DestinationID — ADC only changes how credentials are resolved, not other validation.
func TestNewClient_ADCMode_NoDestination(t *testing.T) {
	cfg := Config{
		UseADC: true,
	}

	_, err := NewClient(context.Background(), cfg)
	if err == nil {
		t.Fatal("Expected error when creating ADC client without a DestinationID")
	}
}

// TestNewClient_LiveModeNoDestination verifies that creating a live client without
// a configured destination folder ID fails with a clear error.
func TestNewClient_LiveModeNoDestination(t *testing.T) {
	cfg := Config{
		UseCached:       false,
		CredentialsJSON: []byte(`{"type": "service_account"}`),
	}

	_, err := NewClient(context.Background(), cfg)
	if err == nil {
		t.Fatal("Expected error when creating live client without a destination folder ID")
	}
}

// TestCachedClient_CreateDoc verifies that the cached client returns a deterministic
// CreatedDoc derived from the fixture, without making any network calls.
func TestCachedClient_CreateDoc(t *testing.T) {
	ctx := context.Background()

	client, err := newCachedClient()
	if err != nil {
		t.Fatalf("Failed to create cached client: %v", err)
	}

	doc := Document{
		Title: "Proposal Outline: IT Systems Design Services",
		Sections: []DocSection{
			{Heading: "Executive Summary", Body: "Required: yes\nStandard section."},
		},
	}

	created, err := client.CreateDoc(ctx, doc)
	if err != nil {
		t.Fatalf("CreateDoc() returned unexpected error: %v", err)
	}
	if created == nil {
		t.Fatal("CreateDoc() returned nil result")
	}
	if created.ID == "" {
		t.Error("Expected non-empty doc ID")
	}
	wantURL := "https://docs.google.com/document/d/" + created.ID + "/edit"
	if created.URL != wantURL {
		t.Errorf("URL = %q, want %q", created.URL, wantURL)
	}
}

// TestCachedClient_CreateDoc_EmptyTitle verifies that the cached client validates
// input the same way the live client would, rather than silently accepting it.
func TestCachedClient_CreateDoc_EmptyTitle(t *testing.T) {
	ctx := context.Background()

	client, err := newCachedClient()
	if err != nil {
		t.Fatalf("Failed to create cached client: %v", err)
	}

	_, err = client.CreateDoc(ctx, Document{Title: ""})
	if err == nil {
		t.Error("Expected error when creating a document with an empty title")
	}
}

// TestBuildRequests verifies that buildRequests produces InsertText and
// UpdateParagraphStyle requests in the correct order, with cursor indices that
// account for the inserted text at each step.
func TestBuildRequests(t *testing.T) {
	doc := Document{
		Title: "Test Document",
		Sections: []DocSection{
			{Heading: "Executive Summary", Body: "Required: yes\nSummary body."},
			{Heading: "Technical Approach", Body: "Required: yes\nApproach body."},
		},
	}

	reqs := buildRequests(doc)

	// Each section produces 3 requests: InsertText (heading), UpdateParagraphStyle
	// (heading), InsertText (body).
	wantCount := len(doc.Sections) * 3
	if len(reqs) != wantCount {
		t.Fatalf("len(requests) = %d, want %d", len(reqs), wantCount)
	}

	index := int64(1)
	for i, sec := range doc.Sections {
		base := i * 3

		insertHeading := reqs[base].InsertText
		if insertHeading == nil {
			t.Fatalf("requests[%d]: expected InsertText request for heading", base)
		}
		headingText := sec.Heading + "\n"
		if insertHeading.Text != headingText {
			t.Errorf("requests[%d].InsertText.Text = %q, want %q", base, insertHeading.Text, headingText)
		}
		if insertHeading.Location == nil || insertHeading.Location.Index != index {
			t.Errorf("requests[%d].InsertText.Location.Index = %v, want %d", base, insertHeading.Location, index)
		}

		style := reqs[base+1].UpdateParagraphStyle
		if style == nil {
			t.Fatalf("requests[%d]: expected UpdateParagraphStyle request for heading", base+1)
		}
		if style.ParagraphStyle == nil || style.ParagraphStyle.NamedStyleType != "HEADING_1" {
			t.Errorf("requests[%d].UpdateParagraphStyle.ParagraphStyle.NamedStyleType = %v, want HEADING_1", base+1, style.ParagraphStyle)
		}
		wantStart := index
		wantEnd := index + int64(len(headingText))
		if style.Range == nil || style.Range.StartIndex != wantStart || style.Range.EndIndex != wantEnd {
			t.Errorf("requests[%d].UpdateParagraphStyle.Range = %v, want [%d, %d)", base+1, style.Range, wantStart, wantEnd)
		}
		index += int64(len(headingText))

		insertBody := reqs[base+2].InsertText
		if insertBody == nil {
			t.Fatalf("requests[%d]: expected InsertText request for body", base+2)
		}
		bodyText := sec.Body + "\n\n"
		if insertBody.Text != bodyText {
			t.Errorf("requests[%d].InsertText.Text = %q, want %q", base+2, insertBody.Text, bodyText)
		}
		if insertBody.Location == nil || insertBody.Location.Index != index {
			t.Errorf("requests[%d].InsertText.Location.Index = %v, want %d", base+2, insertBody.Location, index)
		}
		index += int64(len(bodyText))
	}
}

// TestBuildRequests_Empty verifies that a document with no sections produces no requests.
func TestBuildRequests_Empty(t *testing.T) {
	reqs := buildRequests(Document{Title: "Empty Document"})
	if len(reqs) != 0 {
		t.Errorf("len(requests) = %d, want 0 for a document with no sections", len(reqs))
	}
}

// TestCreatedDoc_URLFormat verifies the cached client constructs URLs in the
// expected, deterministic format rather than depending on Drive's webViewLink.
func TestCreatedDoc_URLFormat(t *testing.T) {
	ctx := context.Background()

	client, err := newCachedClient()
	if err != nil {
		t.Fatalf("Failed to create cached client: %v", err)
	}

	created, err := client.CreateDoc(ctx, Document{Title: "Some Title"})
	if err != nil {
		t.Fatalf("CreateDoc() returned unexpected error: %v", err)
	}

	if !strings.HasPrefix(created.URL, "https://docs.google.com/document/d/") {
		t.Errorf("URL = %q, want prefix %q", created.URL, "https://docs.google.com/document/d/")
	}
	if !strings.HasSuffix(created.URL, "/edit") {
		t.Errorf("URL = %q, want suffix %q", created.URL, "/edit")
	}
}

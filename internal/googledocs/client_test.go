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

// TestRenderDocumentHTML verifies that renderDocumentHTML emits the title as an
// <h1>, each section heading as an <h2>, and each body line as its own <p>; that
// HTML in the content is escaped (so it can't be interpreted as markup); and that
// "[GAP: …]" markers survive verbatim.
func TestRenderDocumentHTML(t *testing.T) {
	doc := Document{
		Title: "Proposal Outline",
		Sections: []DocSection{
			{Heading: "Executive Summary", Body: "First line\nSecond line"},
			{Heading: "Pricing", Body: "[GAP: pricing] needs <script>alert(1)</script> input"},
		},
	}

	out := renderDocumentHTML(doc)

	wantContains := []string{
		"<h1>Proposal Outline</h1>",
		"<h2>Executive Summary</h2>",
		"<h2>Pricing</h2>",
		"<p>First line</p>",
		"<p>Second line</p>",
		"&lt;script&gt;", // the <script> tag is escaped, not interpreted
		"[GAP: pricing]", // GAP markers survive verbatim
	}
	for _, want := range wantContains {
		if !strings.Contains(out, want) {
			t.Errorf("rendered HTML missing %q\ngot: %s", want, out)
		}
	}

	// The raw, unescaped <script> tag must never appear in the output.
	if strings.Contains(out, "<script>") {
		t.Errorf("rendered HTML contains an unescaped <script> tag\ngot: %s", out)
	}

	// A two-line body must yield exactly two <p> elements for that section. The
	// first section has two lines; combined with the second section's single body
	// paragraph, that is three <p> total.
	if got := strings.Count(out, "<p>"); got != 3 {
		t.Errorf("<p> count = %d, want 3 (two from the first section, one from the second)", got)
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

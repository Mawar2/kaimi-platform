package samgov

import (
	"context"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// TestConfig_Defaults verifies that Config has sensible zero values.
func TestConfig_Defaults(t *testing.T) {
	var cfg Config

	if cfg.APIKey != "" {
		t.Errorf("Expected empty APIKey, got %q", cfg.APIKey)
	}
	if cfg.BaseURL != "" {
		t.Errorf("Expected empty BaseURL, got %q", cfg.BaseURL)
	}
	if cfg.UseCached {
		t.Error("Expected UseCached to be false")
	}
}

// TestConfig_CachedMode verifies that cached mode can be configured.
func TestConfig_CachedMode(t *testing.T) {
	cfg := Config{
		UseCached: true,
	}

	if !cfg.UseCached {
		t.Error("Expected UseCached to be true")
	}
}

// TestNewClient_CachedMode verifies that NewClient creates a cached client correctly.
func TestNewClient_CachedMode(t *testing.T) {
	cfg := Config{
		UseCached: true,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create cached client: %v", err)
	}

	if client == nil {
		t.Error("Expected non-nil client")
	}
}

// TestNewClient_LiveMode verifies that NewClient creates a live client correctly.
func TestNewClient_LiveMode(t *testing.T) {
	cfg := Config{
		APIKey:    "test-api-key",
		UseCached: false,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create live client: %v", err)
	}

	if client == nil {
		t.Error("Expected non-nil client")
	}
}

// TestNewClient_LiveModeNoAPIKey verifies that creating a live client without an API key fails.
func TestNewClient_LiveModeNoAPIKey(t *testing.T) {
	cfg := Config{
		UseCached: false,
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("Expected error when creating live client without API key")
	}
}

// TestCachedClient_FetchByNAICS verifies fetching opportunities by NAICS code from cached data.
func TestCachedClient_FetchByNAICS(t *testing.T) {
	ctx := context.Background()

	client, err := newCachedClient()
	if err != nil {
		t.Fatalf("Failed to create cached client: %v", err)
	}

	tests := []struct {
		name          string
		naicsCodes    []string
		expectedCount int
		shouldError   bool
	}{
		{
			name:          "single NAICS code - 541512",
			naicsCodes:    []string{"541512"},
			expectedCount: 3, // All three opportunities in fixture have 541512 (either primary or secondary)
			shouldError:   false,
		},
		{
			name:          "single NAICS code - 541519",
			naicsCodes:    []string{"541519"},
			expectedCount: 2, // Two opportunities in fixture have 541519
			shouldError:   false,
		},
		{
			name:          "multiple NAICS codes",
			naicsCodes:    []string{"541512", "541519"},
			expectedCount: 3, // All three opportunities in fixture
			shouldError:   false,
		},
		{
			name:          "no matching NAICS code",
			naicsCodes:    []string{"999999"},
			expectedCount: 0,
			shouldError:   false,
		},
		{
			name:          "empty NAICS codes",
			naicsCodes:    []string{},
			expectedCount: 0,
			shouldError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opportunities, err := client.FetchByNAICS(ctx, tt.naicsCodes)

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(opportunities) != tt.expectedCount {
				t.Errorf("Expected %d opportunities, got %d", tt.expectedCount, len(opportunities))
			}

			// Verify all opportunities have required fields
			for i, opp := range opportunities {
				if opp.ID == "" {
					t.Errorf("Opportunity %d has empty ID", i)
				}
				if opp.Title == "" {
					t.Errorf("Opportunity %d has empty Title", i)
				}
				if opp.Agency == "" {
					t.Errorf("Opportunity %d has empty Agency", i)
				}
				if opp.NAICSCode == "" {
					t.Errorf("Opportunity %d has empty NAICSCode", i)
				}
			}
		})
	}
}

// TestCachedClient_FetchByID verifies fetching a single opportunity by ID from cached data.
func TestCachedClient_FetchByID(t *testing.T) {
	ctx := context.Background()

	client, err := newCachedClient()
	if err != nil {
		t.Fatalf("Failed to create cached client: %v", err)
	}

	tests := []struct {
		name        string
		noticeID    string
		shouldError bool
		expectedID  string
	}{
		{
			name:        "valid notice ID - first opportunity",
			noticeID:    "a1b2c3d4e5f6",
			shouldError: false,
			expectedID:  "a1b2c3d4e5f6",
		},
		{
			name:        "valid notice ID - second opportunity",
			noticeID:    "f6e5d4c3b2a1",
			shouldError: false,
			expectedID:  "f6e5d4c3b2a1",
		},
		{
			name:        "invalid notice ID",
			noticeID:    "nonexistent",
			shouldError: true,
		},
		{
			name:        "empty notice ID",
			noticeID:    "",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opp, err := client.FetchByID(ctx, tt.noticeID)

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if opp.ID != tt.expectedID {
				t.Errorf("Expected opportunity ID %q, got %q", tt.expectedID, opp.ID)
			}

			// Verify required fields are populated
			if opp.Title == "" {
				t.Error("Opportunity has empty Title")
			}
			if opp.Agency == "" {
				t.Error("Opportunity has empty Agency")
			}
		})
	}
}

// TestTransformOpportunity verifies that SAM.gov API data is correctly transformed
// to internal Opportunity structs.
func TestTransformOpportunity(t *testing.T) {
	// Create resource links as JSON
	resourceLinksJSON := []byte(`[{"name":"RFP.pdf","url":"https://sam.gov/rfp.pdf"}]`)

	data := &opportunityData{
		NoticeID:           "test-123",
		Title:              "Test Opportunity",
		SolicitationNumber: "SOL-001",
		Department:         "Department of Test",
		Office:             "Test Office",
		PostedDate:         "2026-05-15",
		ResponseDeadLine:   "2026-07-15T16:00:00-05:00",
		NAICSCode:          "541512",
		NAICSCodes:         []string{"541512", "541519"},
		TypeOfSetAside:     "SBA",
		Description:        "Test description",
		Type:               "Solicitation",
		UILink:             "https://sam.gov/opp/test-123/view",
		PlaceOfPerformance: placeOfPerformance{
			City:  locationInfo{Name: "Washington"},
			State: locationInfo{Code: "DC", Name: "District of Columbia"},
		},
		ResourceLinks: resourceLinksJSON,
	}

	opp, err := transformOpportunity(data)
	if err != nil {
		t.Fatalf("Failed to transform opportunity: %v", err)
	}

	// Verify all fields are correctly mapped
	if opp.ID != "test-123" {
		t.Errorf("Expected ID %q, got %q", "test-123", opp.ID)
	}
	if opp.Title != "Test Opportunity" {
		t.Errorf("Expected Title %q, got %q", "Test Opportunity", opp.Title)
	}
	if opp.SolicitationNum != "SOL-001" {
		t.Errorf("Expected SolicitationNum %q, got %q", "SOL-001", opp.SolicitationNum)
	}
	if opp.Agency != "Department of Test" {
		t.Errorf("Expected Agency %q, got %q", "Department of Test", opp.Agency)
	}
	if opp.NAICSCode != "541512" {
		t.Errorf("Expected NAICSCode %q, got %q", "541512", opp.NAICSCode)
	}
	if opp.SetAsideCode != "SBA" {
		t.Errorf("Expected SetAsideCode %q, got %q", "SBA", opp.SetAsideCode)
	}
	if len(opp.Attachments) != 1 {
		t.Errorf("Expected 1 attachment, got %d", len(opp.Attachments))
	}
	if opp.URL != "https://sam.gov/opp/test-123/view" {
		t.Errorf("Expected URL %q, got %q", "https://sam.gov/opp/test-123/view", opp.URL)
	}

	// Verify dates were parsed
	if opp.PostedDate.IsZero() {
		t.Error("PostedDate should not be zero")
	}
	if opp.ResponseDeadline.IsZero() {
		t.Error("ResponseDeadline should not be zero")
	}

	// Verify timestamps are set
	if opp.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if opp.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
}

// TestTransformOpportunity_ResourceLinksVariants verifies that resourceLinks can be parsed
// as both array and string (handling SAM.gov API inconsistency).
func TestTransformOpportunity_ResourceLinksVariants(t *testing.T) {
	baseData := opportunityData{
		NoticeID:           "test-123",
		Title:              "Test Opportunity",
		SolicitationNumber: "SOL-001",
		Department:         "Department of Test",
		Office:             "Test Office",
		PostedDate:         "2026-05-15",
		ResponseDeadLine:   "2026-07-15T16:00:00-05:00",
		NAICSCode:          "541512",
		TypeOfSetAside:     "SBA",
		Description:        "Test description",
		Type:               "Solicitation",
		UILink:             "https://sam.gov/opp/test-123/view",
		PlaceOfPerformance: placeOfPerformance{
			City:  locationInfo{Name: "Washington"},
			State: locationInfo{Code: "DC"},
		},
	}

	tests := []struct {
		name              string
		resourceLinks     []byte
		expectedAttachments int
	}{
		{
			name:              "resourceLinks as array",
			resourceLinks:     []byte(`[{"name":"RFP.pdf","url":"https://sam.gov/rfp.pdf"},{"name":"Addendum.pdf","url":"https://sam.gov/addendum.pdf"}]`),
			expectedAttachments: 2,
		},
		{
			name:              "resourceLinks as string (should not fail)",
			resourceLinks:     []byte(`"some string value"`),
			expectedAttachments: 0,
		},
		{
			name:              "resourceLinks as empty array",
			resourceLinks:     []byte(`[]`),
			expectedAttachments: 0,
		},
		{
			name:              "resourceLinks as null",
			resourceLinks:     []byte(`null`),
			expectedAttachments: 0,
		},
		{
			name:              "resourceLinks empty",
			resourceLinks:     nil,
			expectedAttachments: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := baseData
			data.ResourceLinks = tt.resourceLinks

			opp, err := transformOpportunity(&data)
			if err != nil {
				t.Fatalf("transformOpportunity should not fail: %v", err)
			}

			if len(opp.Attachments) != tt.expectedAttachments {
				t.Errorf("Expected %d attachments, got %d", tt.expectedAttachments, len(opp.Attachments))
			}
		})
	}
}

// TestFormatPlaceOfPerformance verifies place of performance formatting.
func TestFormatPlaceOfPerformance(t *testing.T) {
	tests := []struct {
		name     string
		pop      placeOfPerformance
		expected string
	}{
		{
			name: "full address",
			pop: placeOfPerformance{
				StreetAddress: "1800 F Street NW",
				City:          locationInfo{Name: "Washington"},
				State:         locationInfo{Code: "DC"},
				Zip:           "20405",
			},
			expected: "1800 F Street NW, Washington, DC, 20405",
		},
		{
			name: "city and state only",
			pop: placeOfPerformance{
				City:  locationInfo{Name: "Washington"},
				State: locationInfo{Code: "DC"},
			},
			expected: "Washington, DC",
		},
		{
			name: "country only",
			pop: placeOfPerformance{
				Country: locationInfo{Name: "United States"},
			},
			expected: "United States",
		},
		{
			name:     "empty",
			pop:      placeOfPerformance{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPlaceOfPerformance(&tt.pop)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestDeduplicateOpportunities verifies deduplication logic.
func TestDeduplicateOpportunities(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	opportunities := []*opportunity.Opportunity{
		{ID: "opp-1", Title: "Opportunity 1", CreatedAt: now, UpdatedAt: now, Agency: "A", PostedDate: now, ResponseDeadline: now, NAICSCode: "541512"},
		{ID: "opp-2", Title: "Opportunity 2", CreatedAt: now, UpdatedAt: now, Agency: "B", PostedDate: now, ResponseDeadline: now, NAICSCode: "541512"},
		{ID: "opp-1", Title: "Opportunity 1 Duplicate", CreatedAt: now, UpdatedAt: now, Agency: "A", PostedDate: now, ResponseDeadline: now, NAICSCode: "541512"},
		{ID: "opp-3", Title: "Opportunity 3", CreatedAt: now, UpdatedAt: now, Agency: "C", PostedDate: now, ResponseDeadline: now, NAICSCode: "541512"},
		{ID: "opp-2", Title: "Opportunity 2 Duplicate", CreatedAt: now, UpdatedAt: now, Agency: "B", PostedDate: now, ResponseDeadline: now, NAICSCode: "541512"},
	}

	unique := deduplicateOpportunities(opportunities)

	if len(unique) != 3 {
		t.Errorf("Expected 3 unique opportunities, got %d", len(unique))
	}

	// Verify IDs are unique
	seen := make(map[string]bool)
	for _, opp := range unique {
		if seen[opp.ID] {
			t.Errorf("Duplicate ID found: %s", opp.ID)
		}
		seen[opp.ID] = true
	}
}

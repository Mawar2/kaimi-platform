package samgov

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// failingRoundTripper is a test http.RoundTripper that always fails the request.
// It lets us force the l.client.Do(req) error path deterministically (no network),
// so we can assert that the api_key in the request URL never leaks into the error.
type failingRoundTripper struct{}

// RoundTrip always returns an error, mirroring how net/http wraps transport
// failures in a *url.Error whose Error() string embeds the full request URL.
func (failingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("simulated transport failure")
}

// TestLiveClient_FetchErrorRedactsAPIKey verifies that when an HTTP fetch fails,
// the api_key embedded in the request URL is never exposed in the returned error.
// This is the regression test for issue #129 (api_key leaking into error messages/logs).
func TestLiveClient_FetchErrorRedactsAPIKey(t *testing.T) {
	const fakeKey = "SECRET_TEST_KEY_123"

	newLeakyClient := func() *liveClient {
		return &liveClient{
			apiKey:  fakeKey,
			baseURL: "https://api.sam.gov/opportunities/v2",
			client:  &http.Client{Transport: failingRoundTripper{}},
		}
	}

	assertRedacted := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected an error from the forced fetch failure, got nil")
		}
		msg := err.Error()
		if strings.Contains(msg, fakeKey) {
			t.Errorf("error message leaked api_key %q: %s", fakeKey, msg)
		}
		if !strings.Contains(msg, "REDACTED") {
			t.Errorf("expected error to contain a redaction marker %q, got: %s", "REDACTED", msg)
		}
	}

	t.Run("FetchByNAICS", func(t *testing.T) {
		_, err := newLeakyClient().FetchByNAICS(context.Background(), []string{"541512"})
		assertRedacted(t, err)
	})

	t.Run("FetchByID", func(t *testing.T) {
		_, err := newLeakyClient().FetchByID(context.Background(), "abc123")
		assertRedacted(t, err)
	})
}

// TestLiveClient_SearchQueryContract pins the exact query the live client sends
// to the SAM.gov Opportunities v2 search endpoint (issue #268). The official
// spec filters NAICS via `ncode` — there is no `naics` parameter, and SAM.gov
// silently ignores unknown params, returning the ENTIRE unfiltered 30-day
// corpus. That bug burned the full 1,000 req/day quota every Hunter run and
// polluted the queue with out-of-scope opportunities. A regression to the
// wrong param name (or a tiny page size) must fail CI.
func TestLiveClient_SearchQueryContract(t *testing.T) {
	var gotQueries []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQueries = append(gotQueries, r.URL.Query())
		// A short (empty) page terminates the pagination loop immediately.
		_, _ = w.Write([]byte(`{"totalRecords":0,"limit":1000,"offset":0,"opportunitiesData":[]}`))
	}))
	defer server.Close()

	client := &liveClient{apiKey: "test-key", baseURL: server.URL, client: server.Client()}
	if _, err := client.FetchByNAICS(context.Background(), []string{"541512"}); err != nil {
		t.Fatalf("FetchByNAICS failed: %v", err)
	}
	if len(gotQueries) != 1 {
		t.Fatalf("expected exactly 1 request, got %d", len(gotQueries))
	}
	q := gotQueries[0]

	if got := q.Get("ncode"); got != "541512" {
		t.Errorf("ncode = %q, want %q (the spec's NAICS filter param)", got, "541512")
	}
	if q.Has("naics") {
		t.Errorf("query still sends the unsupported naics param (%q) — SAM.gov ignores it and returns the unfiltered corpus", q.Get("naics"))
	}
	if got := q.Get("limit"); got != "1000" {
		t.Errorf("limit = %q, want %q (spec max; small pages waste the 1,000 req/day quota)", got, "1000")
	}
	for _, required := range []string{"postedFrom", "postedTo", "api_key"} {
		if !q.Has(required) {
			t.Errorf("query missing required param %q", required)
		}
	}
}

// TestLiveClient_PaginationAdvancesOffset verifies the pagination loop still
// advances offset by the page size and terminates on a short page (issue #268
// keeps the loop as defensive code for codes that exceed one max-size page).
func TestLiveClient_PaginationAdvancesOffset(t *testing.T) {
	// One minimal record the transform step accepts; repeated to fill a page.
	const record = `{"noticeId":"n-%d","title":"t","postedDate":"2026-05-20","type":"Solicitation"}`

	var gotOffsets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		gotOffsets = append(gotOffsets, q.Get("offset"))

		limit, err := strconv.Atoi(q.Get("limit"))
		if err != nil || limit < 1 {
			t.Errorf("bad limit param %q", q.Get("limit"))
			limit = 1
		}
		if q.Get("offset") == "0" {
			// Full page -> the client must request another page.
			records := make([]string, limit)
			for i := range records {
				records[i] = fmt.Sprintf(record, i)
			}
			_, _ = fmt.Fprintf(w, `{"totalRecords":%d,"opportunitiesData":[%s]}`, limit+1, strings.Join(records, ","))
			return
		}
		// Second page is short -> the loop must terminate.
		_, _ = fmt.Fprintf(w, `{"totalRecords":0,"opportunitiesData":[]}`)
	}))
	defer server.Close()

	client := &liveClient{apiKey: "test-key", baseURL: server.URL, client: server.Client()}
	if _, err := client.FetchByNAICS(context.Background(), []string{"541512"}); err != nil {
		t.Fatalf("FetchByNAICS failed: %v", err)
	}

	if len(gotOffsets) != 2 {
		t.Fatalf("expected 2 requests (full page then short page), got %d: %v", len(gotOffsets), gotOffsets)
	}
	if gotOffsets[0] != "0" {
		t.Errorf("first request offset = %q, want \"0\"", gotOffsets[0])
	}
	if gotOffsets[1] != "1000" {
		t.Errorf("second request offset = %q, want \"1000\" (offset must advance by the page size)", gotOffsets[1])
	}
}

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
		name                string
		resourceLinks       []byte
		expectedAttachments int
	}{
		{
			name:                "resourceLinks as array",
			resourceLinks:       []byte(`[{"name":"RFP.pdf","url":"https://sam.gov/rfp.pdf"},{"name":"Addendum.pdf","url":"https://sam.gov/addendum.pdf"}]`),
			expectedAttachments: 2,
		},
		{
			name:                "resourceLinks as string (should not fail)",
			resourceLinks:       []byte(`"some string value"`),
			expectedAttachments: 0,
		},
		{
			name:                "resourceLinks as empty array",
			resourceLinks:       []byte(`[]`),
			expectedAttachments: 0,
		},
		{
			name:                "resourceLinks as null",
			resourceLinks:       []byte(`null`),
			expectedAttachments: 0,
		},
		{
			name:                "resourceLinks empty",
			resourceLinks:       nil,
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

// TestLiveClient_RequestCapBoundsHunt proves the per-hunt request budget hard-stops
// pagination, so a pathologically broad NAICS code (endless full pages) can never exhaust
// the tenant's 1,000/day SAM quota in a single hunt. The server always returns a FULL page,
// so natural pagination would never terminate; only the cap stops it.
func TestLiveClient_RequestCapBoundsHunt(t *testing.T) {
	const record = `{"noticeId":"n-%d","title":"t","postedDate":"2026-05-20","type":"Solicitation"}`
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit < 1 {
			limit = 1
		}
		recs := make([]string, limit)
		for i := range recs {
			recs[i] = fmt.Sprintf(record, i)
		}
		_, _ = fmt.Fprintf(w, `{"totalRecords":1000000,"opportunitiesData":[%s]}`, strings.Join(recs, ","))
	}))
	defer server.Close()

	client := &liveClient{apiKey: "test-key", baseURL: server.URL, client: server.Client(), maxSearchRequests: 3}
	if _, err := client.FetchByNAICS(context.Background(), []string{"541512"}); err != nil {
		t.Fatalf("FetchByNAICS failed: %v", err)
	}
	if requests != 3 {
		t.Errorf("made %d requests, want exactly 3 (the configured cap) — unbounded pagination would blow the daily quota", requests)
	}
}

// TestLiveClient_CapSpansAllNAICSCodes proves the budget is shared across NAICS codes, so
// the cap bounds the WHOLE hunt, not each code independently. With a cap of 2 and three
// codes each offering endless pages, the hunt must stop after 2 total requests.
func TestLiveClient_CapSpansAllNAICSCodes(t *testing.T) {
	const record = `{"noticeId":"n-%d","title":"t","postedDate":"2026-05-20","type":"Solicitation"}`
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit < 1 {
			limit = 1
		}
		recs := make([]string, limit)
		for i := range recs {
			recs[i] = fmt.Sprintf(record, i)
		}
		_, _ = fmt.Fprintf(w, `{"totalRecords":1000000,"opportunitiesData":[%s]}`, strings.Join(recs, ","))
	}))
	defer server.Close()

	client := &liveClient{apiKey: "test-key", baseURL: server.URL, client: server.Client(), maxSearchRequests: 2}
	if _, err := client.FetchByNAICS(context.Background(), []string{"541512", "541511", "518210"}); err != nil {
		t.Fatalf("FetchByNAICS failed: %v", err)
	}
	if requests != 2 {
		t.Errorf("made %d requests across 3 NAICS codes, want exactly 2 (shared cap, not per-code)", requests)
	}
}

// TestLiveClient_LookbackWindowConfigurable proves the search window honors the configured
// LookbackDays (a short window keeps a daily incremental hunt cheap) and defaults to 30 days
// when unset, while postedTo stays "today".
func TestLiveClient_LookbackWindowConfigurable(t *testing.T) {
	check := func(lookbackDays, wantDaysAgo int) {
		t.Helper()
		var gotFrom, gotTo string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotFrom = r.URL.Query().Get("postedFrom")
			gotTo = r.URL.Query().Get("postedTo")
			_, _ = w.Write([]byte(`{"totalRecords":0,"opportunitiesData":[]}`))
		}))
		defer server.Close()

		client := &liveClient{apiKey: "k", baseURL: server.URL, client: server.Client(), lookbackDays: lookbackDays}
		if _, err := client.FetchByNAICS(context.Background(), []string{"541512"}); err != nil {
			t.Fatalf("FetchByNAICS: %v", err)
		}
		now := time.Now()
		if want := now.AddDate(0, 0, -wantDaysAgo).Format("01/02/2006"); gotFrom != want {
			t.Errorf("lookbackDays=%d: postedFrom=%q, want %q", lookbackDays, gotFrom, want)
		}
		if want := now.Format("01/02/2006"); gotTo != want {
			t.Errorf("lookbackDays=%d: postedTo=%q, want %q (today)", lookbackDays, gotTo, want)
		}
	}
	check(2, 2)  // configured short window for cheap daily hunts
	check(0, 30) // unset -> default 30-day window (first/backfill hunt)
}

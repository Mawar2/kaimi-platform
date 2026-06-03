// Package samgov provides a client for interacting with the SAM.gov Opportunities API.
//
// The client supports two modes:
// - Live mode: makes real HTTP requests to the SAM.gov API
// - Cached mode: uses pre-recorded fixtures for fast, deterministic testing
//
// This package will be fully implemented when the Hunter agent is built.
// This is Phase 0 scaffolding.
package samgov

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// Client provides methods for fetching federal contracting opportunities from SAM.gov.
type Client interface {
	// FetchByNAICS retrieves all active opportunities matching the given NAICS codes.
	// Returns a slice of opportunities, or an error if the API call fails.
	//
	// TODO(phase-0): Implement in Hunter agent ticket.
	FetchByNAICS(ctx context.Context, naicsCodes []string) ([]*opportunity.Opportunity, error)

	// FetchByID retrieves a single opportunity by its SAM.gov notice ID.
	// Returns the opportunity if found, or an error if not found or API call fails.
	//
	// TODO(phase-0): Implement in Hunter agent ticket.
	FetchByID(ctx context.Context, noticeID string) (*opportunity.Opportunity, error)
}

// Config holds configuration for the SAM.gov API client.
type Config struct {
	// APIKey is the SAM.gov API key for authentication.
	// Required for live mode.
	APIKey string

	// BaseURL is the SAM.gov API base URL.
	// Defaults to the production API if empty.
	BaseURL string

	// UseCached indicates whether to use cached fixtures instead of live API.
	// When true, the client reads from test/fixtures/ instead of making HTTP requests.
	UseCached bool
}

// NewClient creates a new SAM.gov API client based on the provided configuration.
//
// If config.UseCached is true, returns a client that reads from test/fixtures/samgov_response.json.
// Otherwise, returns a client that makes real HTTP requests to the SAM.gov API.
func NewClient(config Config) (Client, error) {
	if config.UseCached {
		return newCachedClient()
	}
	return newLiveClient(config)
}

// cachedClient implements Client using pre-recorded fixture data.
type cachedClient struct {
	fixtureData *samgovResponse
}

// newCachedClient creates a client that reads from test/fixtures/samgov_response.json.
func newCachedClient() (*cachedClient, error) {
	// Try to find the fixture file - it may be in different locations depending on
	// where the test is run from (package directory vs project root)
	possiblePaths := []string{
		"test/fixtures/samgov_response.json",
		"../../test/fixtures/samgov_response.json",
	}

	var data []byte
	var err error
	for _, path := range possiblePaths {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read cached fixture (tried: %v): %w", possiblePaths, err)
	}

	// Parse fixture data
	var response samgovResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse cached fixture: %w", err)
	}

	return &cachedClient{
		fixtureData: &response,
	}, nil
}

// FetchByNAICS returns opportunities from the cached fixture, filtered by NAICS codes.
func (c *cachedClient) FetchByNAICS(ctx context.Context, naicsCodes []string) ([]*opportunity.Opportunity, error) {
	if len(naicsCodes) == 0 {
		return nil, fmt.Errorf("at least one NAICS code is required")
	}

	// Create a set of NAICS codes for fast lookup
	naicsSet := make(map[string]bool)
	for _, code := range naicsCodes {
		naicsSet[code] = true
	}

	// Filter and transform opportunities
	var opportunities []*opportunity.Opportunity
	for i := range c.fixtureData.OpportunitiesData {
		oppData := &c.fixtureData.OpportunitiesData[i]
		// Check if this opportunity matches any requested NAICS code
		if !naicsSet[oppData.NAICSCode] {
			// Also check naicsCodes array
			matched := false
			for _, code := range oppData.NAICSCodes {
				if naicsSet[code] {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// Transform to Opportunity struct
		opp, err := transformOpportunity(oppData)
		if err != nil {
			// Log error but continue processing other opportunities
			fmt.Fprintf(os.Stderr, "Warning: failed to transform opportunity %s: %v\n", oppData.NoticeID, err)
			continue
		}

		opportunities = append(opportunities, opp)
	}

	return opportunities, nil
}

// FetchByID returns a single opportunity from the cached fixture by notice ID.
func (c *cachedClient) FetchByID(ctx context.Context, noticeID string) (*opportunity.Opportunity, error) {
	if noticeID == "" {
		return nil, fmt.Errorf("notice ID is required")
	}

	// Search for the opportunity in cached data
	for i := range c.fixtureData.OpportunitiesData {
		oppData := &c.fixtureData.OpportunitiesData[i]
		if oppData.NoticeID == noticeID {
			return transformOpportunity(oppData)
		}
	}

	return nil, fmt.Errorf("opportunity %s not found in cached data", noticeID)
}

// liveClient implements Client using real HTTP requests to the SAM.gov API.
type liveClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// newLiveClient creates a client that makes real HTTP requests to SAM.gov.
func newLiveClient(config Config) (*liveClient, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required for live mode")
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.sam.gov/opportunities/v2"
	}

	return &liveClient{
		apiKey:  config.APIKey,
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// FetchByNAICS retrieves opportunities from SAM.gov API filtered by NAICS codes.
//
// The SAM.gov Opportunities API supports pagination and filtering. This implementation
// handles pagination automatically to retrieve all matching opportunities.
func (l *liveClient) FetchByNAICS(ctx context.Context, naicsCodes []string) ([]*opportunity.Opportunity, error) {
	if len(naicsCodes) == 0 {
		return nil, fmt.Errorf("at least one NAICS code is required")
	}

	var allOpportunities []*opportunity.Opportunity

	// Fetch opportunities for each NAICS code
	// SAM.gov API requires separate queries per NAICS code
	for _, naicsCode := range naicsCodes {
		opportunities, err := l.fetchByNAICSCode(ctx, naicsCode)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch opportunities for NAICS %s: %w", naicsCode, err)
		}
		allOpportunities = append(allOpportunities, opportunities...)
	}

	// Deduplicate opportunities (same opportunity may match multiple NAICS codes)
	return deduplicateOpportunities(allOpportunities), nil
}

// fetchByNAICSCode fetches opportunities for a single NAICS code with pagination.
func (l *liveClient) fetchByNAICSCode(ctx context.Context, naicsCode string) ([]*opportunity.Opportunity, error) {
	var allOpportunities []*opportunity.Opportunity
	limit := 100 // SAM.gov default page size
	offset := 0

	for {
		// Calculate date range (last 30 days)
		now := time.Now()
		postedTo := now.Format("01/02/2006")
		postedFrom := now.AddDate(0, 0, -30).Format("01/02/2006")

		// Build query URL
		url := fmt.Sprintf("%s/search?naics=%s&limit=%d&offset=%d&postedFrom=%s&postedTo=%s&api_key=%s",
			l.baseURL, naicsCode, limit, offset, postedFrom, postedTo, l.apiKey)

		// Make HTTP request
		req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := l.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request: %w", err)
		}

		// Check response status
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return nil, fmt.Errorf("SAM.gov API returned status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var response samgovResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		// Close response body immediately after use
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close response body: %v\n", closeErr)
		}

		// Transform opportunities
		for i := range response.OpportunitiesData {
			oppData := &response.OpportunitiesData[i]
			opp, err := transformOpportunity(oppData)
			if err != nil {
				// Log error but continue processing
				fmt.Fprintf(os.Stderr, "Warning: failed to transform opportunity %s: %v\n", oppData.NoticeID, err)
				continue
			}
			allOpportunities = append(allOpportunities, opp)
		}

		// Check if there are more pages
		if len(response.OpportunitiesData) < limit {
			break
		}
		offset += limit

		// Rate limiting: sleep between requests
		time.Sleep(200 * time.Millisecond)
	}

	return allOpportunities, nil
}

// FetchByID retrieves a single opportunity from SAM.gov by notice ID.
func (l *liveClient) FetchByID(ctx context.Context, noticeID string) (*opportunity.Opportunity, error) {
	if noticeID == "" {
		return nil, fmt.Errorf("notice ID is required")
	}

	// Build query URL
	url := fmt.Sprintf("%s/search?noticeid=%s&api_key=%s", l.baseURL, noticeID, l.apiKey)

	// Make HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close response body: %v\n", closeErr)
		}
	}()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SAM.gov API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response samgovResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check if opportunity was found
	if len(response.OpportunitiesData) == 0 {
		return nil, fmt.Errorf("opportunity %s not found", noticeID)
	}

	// Transform and return first opportunity
	return transformOpportunity(&response.OpportunitiesData[0])
}

// samgovResponse represents the JSON response structure from SAM.gov Opportunities API.
type samgovResponse struct {
	TotalRecords      int               `json:"totalRecords"`
	Limit             int               `json:"limit"`
	Offset            int               `json:"offset"`
	OpportunitiesData []opportunityData `json:"opportunitiesData"`
}

// opportunityData represents a single opportunity in the SAM.gov API response.
type opportunityData struct {
	NoticeID                  string             `json:"noticeId"`
	Title                     string             `json:"title"`
	SolicitationNumber        string             `json:"solicitationNumber"`
	Department                string             `json:"department"`
	SubTier                   string             `json:"subTier"`
	Office                    string             `json:"office"`
	PostedDate                string             `json:"postedDate"`
	Type                      string             `json:"type"`
	BaseType                  string             `json:"baseType"`
	TypeOfSetAsideDescription string             `json:"typeOfSetAsideDescription"`
	TypeOfSetAside            string             `json:"typeOfSetAside"`
	ResponseDeadLine          string             `json:"responseDeadLine"`
	NAICSCode                 string             `json:"naicsCode"`
	NAICSCodes                []string           `json:"naicsCodes"`
	ClassificationCode        string             `json:"classificationCode"`
	Active                    string             `json:"active"`
	Description               string             `json:"description"`
	OrganizationType          string             `json:"organizationType"`
	PlaceOfPerformance        placeOfPerformance `json:"placeOfPerformance"`
	AdditionalInfoLink        string             `json:"additionalInfoLink"`
	UILink                    string             `json:"uiLink"`
	ResourceLinks             json.RawMessage    `json:"resourceLinks,omitempty"`
}

type placeOfPerformance struct {
	StreetAddress string       `json:"streetAddress"`
	City          locationInfo `json:"city"`
	State         locationInfo `json:"state"`
	Zip           string       `json:"zip"`
	Country       locationInfo `json:"country"`
}

type locationInfo struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type resourceLink struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Type string `json:"type"`
	Size int    `json:"size"`
}

// transformOpportunity converts SAM.gov API opportunityData to internal Opportunity struct.
func transformOpportunity(data *opportunityData) (*opportunity.Opportunity, error) {
	now := time.Now().UTC()

	// Parse posted date
	postedDate, err := time.Parse(time.RFC3339, data.PostedDate)
	if err != nil {
		// Try alternative date format (YYYY-MM-DD)
		postedDate, err = time.Parse("2006-01-02", data.PostedDate)
		if err != nil {
			return nil, fmt.Errorf("failed to parse posted date %q: %w", data.PostedDate, err)
		}
	}

	// Parse response deadline
	var responseDeadline time.Time
	if data.ResponseDeadLine != "" {
		responseDeadline, err = time.Parse(time.RFC3339, data.ResponseDeadLine)
		if err != nil {
			// Try alternative format
			responseDeadline, err = time.Parse("2006-01-02T15:04:05-07:00", data.ResponseDeadLine)
			if err != nil {
				return nil, fmt.Errorf("failed to parse response deadline %q: %w", data.ResponseDeadLine, err)
			}
		}
	}

	// Build place of performance string
	placeOfPerformance := formatPlaceOfPerformance(&data.PlaceOfPerformance)

	// Extract attachment URLs - handle both array and string responses
	var attachments []string
	if len(data.ResourceLinks) > 0 {
		// Try to parse as array first
		var links []resourceLink
		if err := json.Unmarshal(data.ResourceLinks, &links); err == nil {
			// Successfully parsed as array
			for _, link := range links {
				if link.URL != "" {
					attachments = append(attachments, link.URL)
				}
			}
		}
		// If it's a string or parsing failed, ignore it and use empty array
		// This prevents the entire opportunity from failing to parse
	}

	// Determine NAICS description (SAM.gov doesn't always provide this, we'd need a lookup table)
	// For now, we'll leave it empty or use a placeholder
	naicsDescription := "" // TODO(phase-1): Add NAICS code lookup table

	opp := &opportunity.Opportunity{
		ID:                 data.NoticeID,
		Title:              data.Title,
		SolicitationNum:    data.SolicitationNumber,
		Agency:             data.Department,
		Office:             data.Office,
		PostedDate:         postedDate,
		ResponseDeadline:   responseDeadline,
		NAICSCode:          data.NAICSCode,
		NAICSDescription:   naicsDescription,
		SetAsideCode:       data.TypeOfSetAside,
		PlaceOfPerformance: placeOfPerformance,
		Description:        data.Description,
		Type:               data.Type,
		ContractType:       "", // SAM.gov doesn't always include this in search results
		URL:                data.UILink,
		Attachments:        attachments,
		Score:              0.0,
		ScoreReasoning:     "",
		Selected:           false,
		ProposalStatus:     "",
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	return opp, nil
}

// formatPlaceOfPerformance builds a human-readable location string.
func formatPlaceOfPerformance(pop *placeOfPerformance) string {
	var parts []string

	if pop.StreetAddress != "" {
		parts = append(parts, pop.StreetAddress)
	}
	if pop.City.Name != "" {
		parts = append(parts, pop.City.Name)
	}
	if pop.State.Code != "" {
		parts = append(parts, pop.State.Code)
	} else if pop.State.Name != "" {
		parts = append(parts, pop.State.Name)
	}
	if pop.Zip != "" && pop.Zip != pop.State.Code {
		parts = append(parts, pop.Zip)
	}

	if len(parts) == 0 && pop.Country.Name != "" {
		return pop.Country.Name
	}

	return strings.Join(parts, ", ")
}

// deduplicateOpportunities removes duplicate opportunities based on ID.
func deduplicateOpportunities(opportunities []*opportunity.Opportunity) []*opportunity.Opportunity {
	seen := make(map[string]bool)
	var unique []*opportunity.Opportunity

	for _, opp := range opportunities {
		if !seen[opp.ID] {
			seen[opp.ID] = true
			unique = append(unique, opp)
		}
	}

	return unique
}

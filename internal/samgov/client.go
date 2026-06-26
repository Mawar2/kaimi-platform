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
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
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

	// LookbackDays is how far back the search window reaches: postedFrom = now -
	// LookbackDays (postedTo = now). SAM requires postedFrom/postedTo and caps the
	// span at one year. Defaults to defaultLookbackDays when <= 0. A short window
	// (e.g. 2) makes a daily incremental hunt cheap — it pulls only newly-posted
	// notices instead of re-paging the whole month every day — while a wider window
	// suits a first/backfill hunt right after a tenant connects their key.
	LookbackDays int

	// MaxSearchRequests caps the total number of SAM /search HTTP requests a single
	// FetchByNAICS call may issue, across all NAICS codes and all pagination. It is a
	// safety net so an unexpectedly broad NAICS code (many pages) can never exhaust the
	// tenant's daily quota in one hunt. Defaults to defaultMaxSearchRequests when <= 0.
	MaxSearchRequests int
}

// Quota-safety defaults. The SAM.gov daily quota for an entity-registered key is 1,000
// requests/day (resets midnight UTC), shared with the per-notice description resolver, so
// the search side stays well under it. A typical profile is ~1 request per NAICS code; the
// cap only ever engages for a pathologically broad code.
const (
	defaultLookbackDays      = 30
	defaultMaxSearchRequests = 250
)

// requestBudget bounds how many SAM /search requests one hunt may issue. take() reports
// whether another request is allowed and records when the cap is hit so the caller can
// surface that results are partial rather than silently truncating.
type requestBudget struct {
	remaining int
	capped    bool
}

func (b *requestBudget) take() bool {
	if b.remaining <= 0 {
		b.capped = true
		return false
	}
	b.remaining--
	return true
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
	apiKey            string
	baseURL           string
	client            *http.Client
	lookbackDays      int
	maxSearchRequests int
}

// lookback returns the configured search window in days, or the default when unset.
func (l *liveClient) lookback() int {
	if l.lookbackDays > 0 {
		return l.lookbackDays
	}
	return defaultLookbackDays
}

// maxRequests returns the configured per-hunt search-request cap, or the default when unset.
func (l *liveClient) maxRequests() int {
	if l.maxSearchRequests > 0 {
		return l.maxSearchRequests
	}
	return defaultMaxSearchRequests
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
		apiKey:            config.APIKey,
		baseURL:           baseURL,
		lookbackDays:      config.LookbackDays,
		maxSearchRequests: config.MaxSearchRequests,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// redactURLSecrets returns a copy of rawURL with the SAM.gov api_key query
// parameter and any userinfo credentials replaced by the literal "REDACTED".
//
// SAM.gov requires the api_key to be passed as a query parameter, so the
// request URL carries a live secret. net/http embeds the full request URL in
// the *url.Error it returns on transport failures; wrapping that error verbatim
// would leak the key into error strings and logs (issue #129). All error paths
// that reference the request URL must route it through this helper first.
//
// If rawURL cannot be parsed, the original string is NOT returned (it may still
// contain the key); instead a fixed placeholder is returned so nothing leaks.
func redactURLSecrets(rawURL string) string {
	const redacted = "REDACTED"

	u, err := url.Parse(rawURL)
	if err != nil {
		// Parsing failed, so we cannot safely locate the secret. Never echo the
		// raw URL back — return a placeholder rather than risk leaking the key.
		return "[unparseable URL: " + redacted + "]"
	}

	// Redact userinfo (e.g. user:password@host) if present.
	if u.User != nil {
		u.User = url.User(redacted)
	}

	// Redact the api_key query parameter while preserving the other params,
	// which are useful, non-secret context (naics, offset, etc.).
	if query := u.Query(); query.Has("api_key") {
		query.Set("api_key", redacted)
		u.RawQuery = query.Encode()
	}

	return u.String()
}

// redactedRequestError wraps an error that may reference a secret-bearing
// request URL, ensuring the api_key never reaches a log or caller. The msg
// argument describes the failing step (e.g. "failed to execute request").
//
// net/http returns a *url.Error from both NewRequestWithContext (on URL parse
// failure) and Client.Do (on transport failure); its Error() string embeds the
// full request URL, which for SAM.gov carries the api_key. We rebuild the
// message from the redacted URL plus the underlying cause so operators still
// get actionable detail (timeout, DNS failure, etc.) without the secret.
func redactedRequestError(msg, rawURL string, err error) error {
	safeURL := redactURLSecrets(rawURL)

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		// Wrap urlErr.Err (the root cause) rather than urlErr itself, because
		// urlErr.Error() would render the secret-bearing URL verbatim.
		return fmt.Errorf("%s %s: %w", msg, safeURL, urlErr.Err)
	}

	// Defensive fallback: not a *url.Error, but the message could still reference
	// the URL. Include only the redacted URL alongside the error.
	return fmt.Errorf("%s %s: %w", msg, safeURL, err)
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

	// One request budget spans ALL NAICS codes + pagination for this hunt, so the cap
	// bounds the whole hunt's quota cost, not each code independently.
	budget := &requestBudget{remaining: l.maxRequests()}

	// Fetch opportunities for each NAICS code
	// SAM.gov API requires separate queries per NAICS code
	for _, naicsCode := range naicsCodes {
		opportunities, err := l.fetchByNAICSCode(ctx, naicsCode, budget)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch opportunities for NAICS %s: %w", naicsCode, err)
		}
		allOpportunities = append(allOpportunities, opportunities...)
		if budget.capped {
			break
		}
	}
	if budget.capped {
		// Surface partial results loudly rather than silently truncating — the hunt
		// still saves what it gathered, and the operator learns the cap was hit.
		fmt.Fprintf(os.Stderr, "Warning: SAM.gov search request cap (%d) reached; this hunt's results may be partial\n", l.maxRequests())
	}

	// Deduplicate opportunities (same opportunity may match multiple NAICS codes)
	return deduplicateOpportunities(allOpportunities), nil
}

// fetchByNAICSCode fetches opportunities for a single NAICS code with pagination, drawing
// each request from the shared budget so the whole hunt stays within the request cap.
func (l *liveClient) fetchByNAICSCode(ctx context.Context, naicsCode string, budget *requestBudget) ([]*opportunity.Opportunity, error) {
	var allOpportunities []*opportunity.Opportunity
	// SAM.gov's documented maximum page size. The previous limit=100 cost 10x
	// the requests against the 1,000 req/day quota for no benefit (issue #268).
	limit := 1000
	offset := 0

	for {
		// Stop before exceeding the hunt's request budget (quota safety net).
		if !budget.take() {
			break
		}
		// Calculate the search window: postedFrom = now - lookback, postedTo = now.
		now := time.Now()
		postedTo := now.Format("01/02/2006")
		postedFrom := now.AddDate(0, 0, -l.lookback()).Format("01/02/2006")

		// Build the query with url.Values so param names and escaping stay
		// honest. The NAICS filter is `ncode` per the Opportunities v2 spec —
		// the previously-sent `naics` param does not exist, and SAM.gov
		// silently ignored it, returning the entire unfiltered 30-day corpus
		// on every Hunter run (issue #268).
		params := url.Values{}
		params.Set("ncode", naicsCode)
		params.Set("limit", strconv.Itoa(limit))
		params.Set("offset", strconv.Itoa(offset))
		params.Set("postedFrom", postedFrom)
		params.Set("postedTo", postedTo)
		params.Set("api_key", l.apiKey)
		reqURL := l.baseURL + "/search?" + params.Encode()

		// Make HTTP request
		req, err := http.NewRequestWithContext(ctx, "GET", reqURL, http.NoBody)
		if err != nil {
			// NewRequestWithContext can return a *url.Error embedding the full URL
			// (and thus the api_key) on a parse failure — redact before wrapping.
			return nil, redactedRequestError("failed to create request for", reqURL, err)
		}

		resp, err := l.client.Do(req)
		if err != nil {
			// Redact the api_key embedded in the URL before it reaches logs/callers.
			return nil, redactedRequestError("failed to execute request", reqURL, err)
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
	reqURL := fmt.Sprintf("%s/search?noticeid=%s&api_key=%s", l.baseURL, noticeID, l.apiKey)

	// Make HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, http.NoBody)
	if err != nil {
		// NewRequestWithContext can return a *url.Error embedding the full URL
		// (and thus the api_key) on a parse failure — redact before wrapping.
		return nil, redactedRequestError("failed to create request for", reqURL, err)
	}

	resp, err := l.client.Do(req)
	if err != nil {
		// Redact the api_key embedded in the URL before it reaches logs/callers.
		return nil, redactedRequestError("failed to execute request", reqURL, err)
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

// parseFlexibleDate attempts to parse dates in multiple formats commonly returned by SAM.gov.
func parseFlexibleDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, nil // Return zero time for empty dates
	}

	// Try formats in order of specificity
	formats := []string{
		time.RFC3339,          // "2006-01-02T15:04:05Z07:00"
		"2006-01-02T15:04:05", // "2026-06-01T18:00:00" (no timezone)
		"2006-01-02",          // "2026-06-01" (date only)
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
}

// transformOpportunity converts SAM.gov API opportunityData to internal Opportunity struct.
func transformOpportunity(data *opportunityData) (*opportunity.Opportunity, error) {
	now := time.Now().UTC()

	// Parse posted date
	postedDate, err := parseFlexibleDate(data.PostedDate)
	if err != nil {
		// Log warning but continue with zero time - don't fail the entire opportunity
		fmt.Fprintf(os.Stderr, "Warning: failed to parse posted date for opportunity %s: %v\n", data.NoticeID, err)
	}

	// Parse response deadline
	responseDeadline, err := parseFlexibleDate(data.ResponseDeadLine)
	if err != nil {
		// Log warning but continue with zero time - don't fail the entire opportunity
		fmt.Fprintf(os.Stderr, "Warning: failed to parse response deadline for opportunity %s: %v\n", data.NoticeID, err)
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

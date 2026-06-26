package samgov

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// maxDescBytes caps a resolved description response. Solicitation descriptions are prose,
// not large files; 2 MiB is generous headroom while bounding memory on a hostile response.
const maxDescBytes = 2 << 20

// samAPIHost is the only host the SAM api_key may ever be sent to. The description URL
// comes from the SAM search response, but a spoofed/MITM'd response (or an off-host
// redirect) must never cause the key to be attached to an attacker-controlled host.
const samAPIHost = "api.sam.gov"

// requireSAMHost rejects any URL that is not https on api.sam.gov, so the SAM api_key is
// only ever attached to the real SAM API. Returns an error (the caller skips the fetch)
// rather than silently proceeding.
func requireSAMHost(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("samgov: unparseable description URL")
	}
	if u.Scheme != "https" || u.Hostname() != samAPIHost {
		return fmt.Errorf("samgov: refusing to resolve description from non-SAM host %q", u.Hostname())
	}
	return nil
}

// DescriptionResolver fetches an opportunity's full description TEXT from the SAM.gov
// `noticedesc` URL that the v2 search API returns in the Description field — that field is
// a URL, not prose, so the Scorer otherwise scores a link. Resolving requires the SAM API
// key and consumes SAM quota (1,000 req/day), so callers MUST bound how many they resolve
// (the eligible set after the gate — never the full corpus).
type DescriptionResolver struct {
	apiKey string
	client *http.Client
}

// NewDescriptionResolver returns a resolver authenticated with the SAM API key.
func NewDescriptionResolver(apiKey string) (*DescriptionResolver, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("samgov: API key is required to resolve descriptions")
	}
	return &DescriptionResolver{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
			// Re-validate the host on every redirect hop so a 30x cannot bounce the
			// key-bearing request off api.sam.gov to an attacker host.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("samgov: stopped after too many redirects")
				}
				if req.URL.Scheme != "https" || req.URL.Hostname() != samAPIHost {
					return fmt.Errorf("samgov: refusing redirect to non-SAM host %q", req.URL.Hostname())
				}
				return nil
			},
		},
	}, nil
}

// WithHTTPClient overrides the HTTP client (tests inject a fake transport). Returns the
// resolver for chaining.
func (r *DescriptionResolver) WithHTTPClient(c *http.Client) *DescriptionResolver {
	r.client = c
	return r
}

// Resolve fetches descURL and returns its plain-text content. The noticedesc endpoint
// returns JSON {"description": "<html>"}; Resolve decodes that (falling back to the raw
// body) and strips HTML to text. The api_key query parameter is appended when absent.
// Error strings never include the URL verbatim — it carries the key — so they route
// through redactURLSecrets.
func (r *DescriptionResolver) Resolve(ctx context.Context, descURL string) (string, error) {
	if strings.TrimSpace(descURL) == "" {
		return "", fmt.Errorf("samgov: empty description URL")
	}
	// Host allowlist: only ever attach the api_key to the real SAM API over HTTPS, so a
	// spoofed description URL can never receive the key. Checked BEFORE withAPIKey.
	if err := requireSAMHost(descURL); err != nil {
		return "", err
	}
	reqURL := withAPIKey(descURL, r.apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		// Do not wrap err: net/http would embed the key-bearing URL in it.
		return "", fmt.Errorf("samgov: build description request failed")
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("samgov: fetch description %s: %w", redactURLSecrets(reqURL), err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check status before reading: a failed request may return a large error page we
	// have no reason to buffer.
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("samgov: description fetch %s returned status %d", redactURLSecrets(reqURL), resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDescBytes))
	if err != nil {
		return "", fmt.Errorf("samgov: read description body: %w", err)
	}

	// noticedesc returns {"description": "<html>"}; fall back to the raw body if it isn't
	// that JSON shape (the endpoint occasionally returns the HTML directly).
	var payload struct {
		Description string `json:"description"`
	}
	text := string(body)
	if json.Unmarshal(body, &payload) == nil && payload.Description != "" {
		text = payload.Description
	}
	return stripHTML(text), nil
}

// withAPIKey returns rawURL with an api_key query parameter, adding it only when absent
// (the noticedesc URLs from the search API do not carry one). On a parse failure the
// original string is returned unchanged so the caller still attempts the request.
func withAPIKey(rawURL, key string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	if q.Get("api_key") == "" {
		q.Set("api_key", key)
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// htmlTagRe matches HTML tags (including across newlines) for stripping.
var htmlTagRe = regexp.MustCompile(`(?s)<[^>]*>`)

// stripHTML removes HTML tags, unescapes entities, and collapses whitespace, yielding
// plain text suitable for scoring. Paragraph structure is not preserved (irrelevant to
// keyword/LLM scoring).
func stripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

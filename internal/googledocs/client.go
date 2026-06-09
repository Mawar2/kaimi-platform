// Package googledocs creates and populates Google Docs inside a Shared Drive.
//
// The client supports three modes:
//   - Live mode (service account): authenticates with a JSON key; use for
//     production and CI where explicit key management is required.
//   - Live mode (ADC): authenticates via Application Default Credentials; use
//     for local development and GCP-hosted environments (Cloud Run, GKE, etc.).
//   - Cached mode: returns deterministic fixture data for fast, offline testing.
//
// Docs are created inside a Shared Drive (rather than a personal Drive) so that
// Docs produced by the service account are owned and visible the way the rest of
// the team expects, instead of being orphaned in the service account's own Drive.
package googledocs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const docURLPrefix = "https://docs.google.com/document/d/"

// Client creates populated Google Docs and returns their location.
type Client interface {
	// CreateDoc creates a new Google Doc in the configured Shared Drive, populates
	// it with the given title and sections, and returns its ID and URL.
	CreateDoc(ctx context.Context, doc Document) (*CreatedDoc, error)
}

// Document is the input to CreateDoc. It is intentionally decoupled from the
// outline package's domain types (Outline, Section, FormattingRules) so this
// package has no dependency on outline — the dependency graph stays one-way
// (outline -> googledocs), and any future doc-producing agent (Writer, Final
// Review) can reuse this client without coupling to Outline's schema.
type Document struct {
	// Title becomes the Doc's file name and document title.
	Title string

	// Sections become the Doc's body content, in order: each section renders
	// as a heading followed by a body paragraph.
	Sections []DocSection
}

// DocSection is one heading-and-body unit of a Document's content.
type DocSection struct {
	Heading string // rendered as a HEADING_1 paragraph
	Body    string // rendered as a normal-style paragraph below the heading
}

// CreatedDoc identifies a Doc that was created by CreateDoc.
type CreatedDoc struct {
	ID  string // Drive file ID
	URL string // https://docs.google.com/document/d/{ID}/edit
}

// Config holds configuration for the Google Docs client.
type Config struct {
	// CredentialsJSON is the raw service-account JSON key content.
	// Required for live mode unless UseADC is true.
	CredentialsJSON []byte

	// SharedDriveID is the ID of the Shared Drive (or folder) that Docs are
	// created in. Required for live mode.
	SharedDriveID string

	// UseCached indicates whether to use deterministic fixture data instead of
	// making real Drive/Docs API calls.
	UseCached bool

	// UseADC instructs the client to authenticate via Application Default
	// Credentials instead of a service-account JSON key. When true,
	// CredentialsJSON is ignored. ADC resolves credentials in order:
	// GOOGLE_APPLICATION_CREDENTIALS env var → gcloud user credentials
	// (gcloud auth application-default login) → GCE/GKE metadata server.
	// Prefer ADC in GCP-hosted environments; use CredentialsJSON only when
	// explicit key management is required.
	UseADC bool
}

// NewClient creates a new Google Docs client based on the provided configuration.
//
// If config.UseCached is true, returns a client that returns deterministic fixture
// data with no network calls. Otherwise, returns a live client that creates and
// populates real Docs via the Drive and Docs APIs, authenticating with either a
// service-account JSON key (CredentialsJSON) or Application Default Credentials
// (UseADC).
//
// NewClient takes a context because building live Drive/Docs services requires one.
func NewClient(ctx context.Context, cfg Config) (Client, error) {
	if cfg.UseCached {
		return newCachedClient()
	}
	return newLiveClient(ctx, cfg)
}

// docURL builds the canonical edit URL for a Doc from its Drive file ID.
//
// The URL is constructed explicitly rather than relying on Drive's returned
// webViewLink, so the format stays deterministic and testable.
func docURL(id string) string {
	return fmt.Sprintf("%s%s/edit", docURLPrefix, id)
}

// validateDocument checks that doc has the minimum content required to create a
// Doc. Both the cached and live clients enforce this so tests exercise the same
// contract live mode would.
func validateDocument(doc Document) error {
	if doc.Title == "" {
		return fmt.Errorf("document title is required")
	}
	return nil
}

// cachedClient implements Client using deterministic fixture data.
type cachedClient struct {
	fixtureID string
}

// cachedFixture is the on-disk shape of test/fixtures/googledocs_response.json.
type cachedFixture struct {
	ID string `json:"id"`
}

// newCachedClient creates a client that returns deterministic data derived from
// test/fixtures/googledocs_response.json, making no network calls.
func newCachedClient() (*cachedClient, error) {
	// Try to find the fixture file - it may be in different locations depending on
	// where the test is run from (package directory vs project root).
	possiblePaths := []string{
		"test/fixtures/googledocs_response.json",
		"../../test/fixtures/googledocs_response.json",
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

	var fixture cachedFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("failed to parse cached fixture: %w", err)
	}
	if fixture.ID == "" {
		return nil, fmt.Errorf("cached fixture is missing an id")
	}

	return &cachedClient{fixtureID: fixture.ID}, nil
}

// CreateDoc returns deterministic fixture data without making any network calls.
func (c *cachedClient) CreateDoc(_ context.Context, doc Document) (*CreatedDoc, error) {
	if err := validateDocument(doc); err != nil {
		return nil, err
	}

	return &CreatedDoc{
		ID:  c.fixtureID,
		URL: docURL(c.fixtureID),
	}, nil
}

// liveClient implements Client using real Drive and Docs API calls.
type liveClient struct {
	driveSvc      *drive.Service
	docsSvc       *docs.Service
	sharedDriveID string
}

// newLiveClient creates a client that creates and populates real Docs via the
// Drive and Docs APIs. It authenticates with a service-account JSON key
// (cfg.CredentialsJSON) or Application Default Credentials (cfg.UseADC).
func newLiveClient(ctx context.Context, cfg Config) (*liveClient, error) {
	if cfg.SharedDriveID == "" {
		return nil, fmt.Errorf("shared drive ID is required for live mode")
	}

	var opts []option.ClientOption
	if !cfg.UseADC {
		if len(cfg.CredentialsJSON) == 0 {
			return nil, fmt.Errorf("credentials are required for live mode (set CredentialsJSON or enable UseADC)")
		}
		opts = append(opts, option.WithCredentialsJSON(cfg.CredentialsJSON)) //nolint:staticcheck // TODO(phase-1): migrate to option.WithCredentials
	}
	// When UseADC is true no credential option is added — the Google client
	// libraries resolve ADC automatically (env var → gcloud → metadata server).

	driveSvc, err := drive.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Drive service: %w", err)
	}

	docsSvc, err := docs.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docs service: %w", err)
	}

	return &liveClient{
		driveSvc:      driveSvc,
		docsSvc:       docsSvc,
		sharedDriveID: cfg.SharedDriveID,
	}, nil
}

// CreateDoc creates a Google Doc inside the configured Shared Drive and populates
// it with the given title and sections.
//
// The Docs API cannot create a document directly inside a folder or Shared Drive,
// so this happens in two steps: create the file via the Drive API with the Shared
// Drive as its parent, then populate its content via the Docs API.
func (l *liveClient) CreateDoc(ctx context.Context, doc Document) (*CreatedDoc, error) {
	if err := validateDocument(doc); err != nil {
		return nil, err
	}

	file := &drive.File{
		Name:     doc.Title,
		MimeType: "application/vnd.google-apps.document",
		Parents:  []string{l.sharedDriveID},
	}

	created, err := l.driveSvc.Files.Create(file).
		SupportsAllDrives(true).
		Fields("id").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("failed to create Doc in Shared Drive: %w", err)
	}

	if reqs := buildRequests(doc); len(reqs) > 0 {
		_, err := l.docsSvc.Documents.
			BatchUpdate(created.Id, &docs.BatchUpdateDocumentRequest{Requests: reqs}).
			Context(ctx).
			Do()
		if err != nil {
			return nil, fmt.Errorf("failed to populate Doc content: %w", err)
		}
	}

	return &CreatedDoc{
		ID:  created.Id,
		URL: docURL(created.Id),
	}, nil
}

// buildRequests converts a Document's sections into an ordered list of Docs API
// batchUpdate requests: each section inserts its heading text, styles that text
// as HEADING_1, then inserts its body text below.
//
// Requests insert text at a running cursor index that starts at 1 (the first
// character position of a Doc's body) and advances by the length of each
// inserted string.
//
// NOTE: Docs API indices are UTF-16 code units, not byte or rune counts. This
// cursor arithmetic assumes ASCII-range text, which holds for the section
// headings and rationale Outline currently generates. If solicitation-derived
// text containing characters outside the Basic Multilingual Plane's single-unit
// range is ever rendered here, this will need to switch to counting UTF-16 code
// units (e.g. via utf16.Encode) instead of len().
func buildRequests(doc Document) []*docs.Request {
	var reqs []*docs.Request
	index := int64(1)

	for _, sec := range doc.Sections {
		headingText := sec.Heading + "\n"
		reqs = append(reqs,
			&docs.Request{
				InsertText: &docs.InsertTextRequest{
					Text:     headingText,
					Location: &docs.Location{Index: index},
				},
			},
			&docs.Request{
				UpdateParagraphStyle: &docs.UpdateParagraphStyleRequest{
					Range: &docs.Range{
						StartIndex: index,
						EndIndex:   index + int64(len(headingText)),
					},
					ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_1"},
					Fields:         "namedStyleType",
				},
			},
		)
		index += int64(len(headingText))

		bodyText := sec.Body + "\n\n"
		reqs = append(reqs, &docs.Request{
			InsertText: &docs.InsertTextRequest{
				Text:     bodyText,
				Location: &docs.Location{Index: index},
			},
		})
		index += int64(len(bodyText))
	}

	return reqs
}

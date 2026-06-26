// Package googledocs creates and populates Google Docs inside a Shared Drive.
//
// The client supports four modes:
//   - Live mode (OAuth TokenSource): authenticates as a specific user via an
//     oauth2.TokenSource. Docs land in THAT user's Drive — this is the WS-C2 seam
//     for writing into a customer's own Google Workspace. Takes precedence over
//     the service-account and ADC modes when set.
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
	"html"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const docURLPrefix = "https://docs.google.com/document/d/"

// folderMimeType is the Drive MIME type that identifies a folder.
const folderMimeType = "application/vnd.google-apps.folder"

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
	// Required for live mode unless UseADC is true or TokenSource is set.
	CredentialsJSON []byte

	// TokenSource authenticates as a SPECIFIC USER (OAuth2) rather than as a
	// service account. When set, it takes precedence over CredentialsJSON and
	// UseADC, and Docs created by this client land in THAT user's Drive — this is
	// the WS-C2 seam that lets a deployment write proposal Docs into the customer's
	// own Google Workspace instead of a BlueMeta service account.
	//
	// The oauth2.TokenSource is responsible for refreshing the access token; the
	// per-tenant Drive token store (internal/drivetoken) builds one from a stored
	// refresh token via oauth2.Config.TokenSource so it auto-refreshes. The token
	// itself is a secret and is NEVER logged anywhere in this package.
	TokenSource oauth2.TokenSource

	// DestinationID is the ID of the parent folder OR Shared Drive that Docs are
	// created in. It is used directly as the created file's Parents[0]. Required for
	// live mode.
	DestinationID string

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
// populates real Docs via the Drive API alone (uploading rendered HTML with
// conversion), authenticating with either a service-account JSON key
// (CredentialsJSON) or Application Default Credentials (UseADC).
//
// NewClient takes a context because building the live Drive service requires one.
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

// liveClient implements Client using real Drive API calls.
type liveClient struct {
	driveSvc      *drive.Service
	destinationID string
}

// newLiveClient creates a client that creates and populates real Docs via the
// Drive API alone (uploading rendered HTML with conversion). It authenticates
// with a service-account JSON key (cfg.CredentialsJSON) or Application Default
// Credentials (cfg.UseADC).
func newLiveClient(ctx context.Context, cfg Config) (*liveClient, error) {
	if cfg.DestinationID == "" {
		return nil, fmt.Errorf("destination folder ID is required for live mode")
	}

	opts := authClientOptions(cfg)
	if len(opts) == 0 && !cfg.UseADC {
		// authClientOptions returns no options only when no credential source was
		// configured (TokenSource/CredentialsJSON absent and UseADC false).
		return nil, fmt.Errorf("credentials are required for live mode (set TokenSource, CredentialsJSON, or enable UseADC)")
	}

	driveSvc, err := drive.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Drive service: %w", err)
	}

	return &liveClient{
		driveSvc:      driveSvc,
		destinationID: cfg.DestinationID,
	}, nil
}

// authClientOptions resolves the Google API client options from a Config's
// credential fields. Authentication precedence: a per-user OAuth TokenSource wins
// over a service-account key, which wins over ADC. The TokenSource path is the
// WS-C2 customer-Drive seam: when set, the Drive service authenticates as the
// customer's own Workspace user, so Docs are created in their Drive.
// option.WithTokenSource
// uses the source's token and lets it auto-refresh.
//
// It returns nil (no options) for the ADC case AND for the no-credential case;
// callers distinguish the two via cfg.UseADC, since ADC adds no explicit option (the
// Google libraries resolve it automatically: env var → gcloud → metadata server).
func authClientOptions(cfg Config) []option.ClientOption {
	switch {
	case cfg.TokenSource != nil:
		return []option.ClientOption{option.WithTokenSource(cfg.TokenSource)}
	case cfg.UseADC:
		return nil
	case len(cfg.CredentialsJSON) > 0:
		return []option.ClientOption{option.WithCredentialsJSON(cfg.CredentialsJSON)} //nolint:staticcheck // TODO(phase-1): migrate to option.WithCredentials
	default:
		return nil
	}
}

// EnsureFolder finds or creates a Drive folder with the given name, authenticating
// as the user behind ts, and returns its folder id. It is idempotent under the
// drive.file scope: a Drive Files.List restricted to that scope returns ONLY files
// the app itself created, so a prior call's "Kaimi Proposals" folder is found and
// reused rather than duplicated. This backs the WS-C5a auto-provision-on-connect
// flow, which calls it once after the customer connects their Drive.
//
// It searches for a non-trashed folder of the given name first; if found, returns
// that id. Otherwise it creates a new folder and returns its id. SupportsAllDrives
// is set so the same call works whether the user's home is My Drive or a Shared
// Drive. The token behind ts is a secret and is never logged here.
func EnsureFolder(ctx context.Context, ts oauth2.TokenSource, name string) (folderID string, err error) {
	if ts == nil {
		return "", fmt.Errorf("a token source is required to ensure a Drive folder")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("folder name is required")
	}
	// Guard the name against query injection. Drive's query language delimits string
	// literals with single quotes; a name containing one would break the query (and is
	// not a value we ever pass for the literal "Kaimi Proposals"), so reject it rather
	// than risk a malformed/injected query.
	if strings.Contains(name, "'") {
		return "", fmt.Errorf("folder name must not contain a single quote: %q", name)
	}

	driveSvc, err := drive.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return "", fmt.Errorf("failed to create Drive service: %w", err)
	}

	// Search first so we reuse an existing app-created folder instead of making a
	// duplicate. Under drive.file the list is already scoped to app-created files.
	query := fmt.Sprintf("mimeType='%s' and name='%s' and trashed=false", folderMimeType, name)
	list, err := driveSvc.Files.List().
		Q(query).
		Spaces("drive").
		Fields("files(id,name)").
		SupportsAllDrives(true).
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("failed to search for existing folder %q: %w", name, err)
	}
	if len(list.Files) > 0 {
		return list.Files[0].Id, nil
	}

	created, err := driveSvc.Files.Create(&drive.File{
		Name:     name,
		MimeType: folderMimeType,
	}).
		SupportsAllDrives(true).
		Fields("id").
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("failed to create folder %q: %w", name, err)
	}
	return created.Id, nil
}

// CreateDoc creates a Google Doc inside the configured Shared Drive and populates
// it with the given title and sections.
//
// It uses the Drive API ALONE: the Document is rendered to HTML and uploaded as
// the file's media with conversion (uploading text/html against the Google Doc
// MIME type makes Drive convert it into a native Doc). This deliberately avoids
// the Docs API and its sensitive `documents` OAuth scope — a single Drive
// Files.Create with the Shared Drive as the parent does both create and populate.
func (l *liveClient) CreateDoc(ctx context.Context, doc Document) (*CreatedDoc, error) {
	if err := validateDocument(doc); err != nil {
		return nil, err
	}

	htmlBody := renderDocumentHTML(doc)

	created, err := l.driveSvc.Files.Create(&drive.File{
		Name:     doc.Title,
		MimeType: "application/vnd.google-apps.document",
		Parents:  []string{l.destinationID},
	}).
		Media(strings.NewReader(htmlBody), googleapi.ContentType("text/html")).
		SupportsAllDrives(true).
		Fields("id").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("failed to create Doc in Shared Drive: %w", err)
	}

	return &CreatedDoc{
		ID:  created.Id,
		URL: docURL(created.Id),
	}, nil
}

// renderDocumentHTML renders a Document to a minimal HTML document that Drive can
// convert into a native Google Doc on upload. The title becomes an <h1>; each
// section's heading (when present) becomes an <h2>, and each of its body
// paragraphs becomes a <p>.
//
// All text is escaped with html.EscapeString to prevent any HTML/script in the
// content from being interpreted. Note html.EscapeString does NOT touch '[',
// ']', or ':', so "[GAP: …]" markers survive verbatim into the converted Doc.
func renderDocumentHTML(doc Document) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"></head><body>`)
	b.WriteString("<h1>")
	b.WriteString(html.EscapeString(doc.Title))
	b.WriteString("</h1>")

	for _, sec := range doc.Sections {
		if sec.Heading != "" {
			b.WriteString("<h2>")
			b.WriteString(html.EscapeString(sec.Heading))
			b.WriteString("</h2>")
		}
		for _, para := range splitParagraphs(sec.Body) {
			b.WriteString("<p>")
			b.WriteString(html.EscapeString(para))
			b.WriteString("</p>")
		}
	}

	b.WriteString("</body></html>")
	return b.String()
}

// splitParagraphs splits a section body into paragraphs. Outline bodies use a
// SINGLE "\n" between lines (not blank-line-separated paragraphs), so it splits
// on every "\n" after normalizing Windows "\r\n" line endings. Each resulting
// line is trimmed, and empty lines are dropped.
func splitParagraphs(body string) []string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	var paras []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paras = append(paras, line)
		}
	}
	return paras
}

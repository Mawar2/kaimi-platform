// Package contextdoc stores the context documents a tester uploads during onboarding
// (capability statements, CPARS, past proposals) and extracts their text, so the
// capability-map builder can ground a deep business understanding in real documents
// rather than just the onboarding form fields.
//
// It is per-tenant (rooted in the deployment's store base path) and reuses the
// internal/ingest Extractor seam (DOCX via stdlib, PDFs/images via Document AI in
// production) so document handling lives in one place. The raw bytes and the extracted
// text are both persisted: the text feeds the map; the raw file is retained so a tester
// can re-download it and so re-extraction is possible without a re-upload.
package contextdoc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Mawar2/Kaimi/internal/ingest"
)

// Doc is one uploaded context document and its extracted text. Text may be empty when a
// format could not be extracted (e.g. a scanned PDF with no Document AI configured); the
// raw file is still retained.
type Doc struct {
	Name        string    `json:"name"`         // original (display) filename
	StoredName  string    `json:"stored_name"`  // sanitized name of the raw file on disk
	ContentType string    `json:"content_type"` // MIME type as uploaded
	Bytes       int64     `json:"bytes"`        // raw size
	Text        string    `json:"text"`         // extracted plain text ("" if unextractable)
	UploadedAt  time.Time `json:"uploaded_at"`
}

// Store persists uploaded context documents per tenant. JSONStore is the file-backed
// implementation; it lives under the deployment's store base path alongside the queue,
// profile, and capability map, so one deployment = one tenant's documents.
type Store interface {
	// Save extracts text from raw and persists both the raw file and the record. A
	// re-upload of the same name replaces the prior record. It returns the stored Doc.
	Save(ctx context.Context, name, contentType string, raw []byte) (Doc, error)
	// List returns all uploaded documents (newest first).
	List() ([]Doc, error)
}

const (
	dirName      = "context_docs"
	filesSubdir  = "files"
	manifestName = "manifest.json"
)

// JSONStore stores raw files under <base>/context_docs/files/ and a manifest of records
// under <base>/context_docs/manifest.json. It is concurrency-safe.
type JSONStore struct {
	mu        sync.Mutex
	base      string
	filesDir  string
	manifest  string
	extractor ingest.Extractor
	now       func() time.Time
}

// NewJSONStore returns a context-doc store rooted at basePath, extracting text with the
// given extractor (nil → a PlainTextExtractor, which handles text uploads and returns ""
// for binary formats). Directories are created as needed.
func NewJSONStore(basePath string, extractor ingest.Extractor) (*JSONStore, error) {
	if basePath == "" {
		return nil, fmt.Errorf("contextdoc: store base path is required")
	}
	base := filepath.Join(basePath, dirName)
	filesDir := filepath.Join(base, filesSubdir)
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		return nil, fmt.Errorf("contextdoc: create store dir: %w", err)
	}
	if extractor == nil {
		extractor = PlainTextExtractor{}
	}
	return &JSONStore{
		base:      base,
		filesDir:  filesDir,
		manifest:  filepath.Join(base, manifestName),
		extractor: extractor,
		now:       time.Now,
	}, nil
}

// Save persists the raw file, extracts its text, and records it in the manifest. A
// document with the same display name replaces the previous one (re-upload = update).
func (s *JSONStore) Save(ctx context.Context, name, contentType string, raw []byte) (Doc, error) {
	display := strings.TrimSpace(name)
	if display == "" {
		return Doc{}, fmt.Errorf("contextdoc: document name is required")
	}
	if len(raw) == 0 {
		return Doc{}, fmt.Errorf("contextdoc: %q is empty", display)
	}
	stored := sanitizeName(display)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure the files directory exists on every save, not just at construction: on a
	// network-backed store (gcsfuse) the directory can be absent if it was never
	// materialized or was removed out-of-band, which would otherwise fail the write.
	if err := os.MkdirAll(s.filesDir, 0o755); err != nil {
		return Doc{}, fmt.Errorf("contextdoc: ensure store dir: %w", err)
	}

	// Persist the raw bytes first so the record never references a missing file.
	if err := os.WriteFile(filepath.Join(s.filesDir, stored), raw, 0o644); err != nil {
		return Doc{}, fmt.Errorf("contextdoc: write raw file: %w", err)
	}

	// Extract text. An extraction error is not fatal — keep the raw file and an empty
	// text so the upload still succeeds and the user can retry/replace it.
	text, err := s.extractor.ExtractText(ctx, raw, contentType)
	if err != nil {
		text = ""
	}

	doc := Doc{
		Name:        display,
		StoredName:  stored,
		ContentType: contentType,
		Bytes:       int64(len(raw)),
		Text:        text,
		UploadedAt:  s.now().UTC(),
	}

	docs, err := s.read()
	if err != nil {
		return Doc{}, err
	}
	// Replace any existing record with the same display name (re-upload = update).
	out := docs[:0]
	for _, d := range docs {
		if d.Name != display {
			out = append(out, d)
		}
	}
	out = append(out, doc)
	if err := s.write(out); err != nil {
		return Doc{}, err
	}
	return doc, nil
}

// List returns the stored documents, newest first.
func (s *JSONStore) List() ([]Doc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	docs, err := s.read()
	if err != nil {
		return nil, err
	}
	// Newest first by upload time.
	for i, j := 0, len(docs)-1; i < j; i, j = i+1, j-1 {
		docs[i], docs[j] = docs[j], docs[i]
	}
	return docs, nil
}

// read loads the manifest (empty slice when none exists yet). Caller holds the lock.
func (s *JSONStore) read() ([]Doc, error) {
	data, err := os.ReadFile(s.manifest)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("contextdoc: read manifest: %w", err)
	}
	var docs []Doc
	if err := json.Unmarshal(data, &docs); err != nil {
		return nil, fmt.Errorf("contextdoc: decode manifest: %w", err)
	}
	return docs, nil
}

// write persists the manifest atomically (temp + rename). Caller holds the lock.
func (s *JSONStore) write(docs []Doc) error {
	data, err := json.MarshalIndent(docs, "", "  ")
	if err != nil {
		return fmt.Errorf("contextdoc: encode manifest: %w", err)
	}
	tmp := s.manifest + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("contextdoc: write manifest: %w", err)
	}
	if err := os.Rename(tmp, s.manifest); err != nil {
		return fmt.Errorf("contextdoc: rename manifest: %w", err)
	}
	return nil
}

// sanitizeName makes an upload filename safe to use as an on-disk name: it strips any
// directory components (defeating path traversal like "../../etc/passwd") and replaces
// any character outside [A-Za-z0-9._-] with "_". An empty result falls back to "file".
func sanitizeName(name string) string {
	base := filepath.Base(filepath.FromSlash(name))
	base = strings.ReplaceAll(base, "\\", "_")
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), ".")
	if out == "" {
		return "file"
	}
	return out
}

// PlainTextExtractor is the offline/default Extractor: it returns the bytes as text for
// plain-text/markdown uploads and an empty string for anything else (binary formats need
// Document AI, which production wires in). It never errors.
type PlainTextExtractor struct{}

// ExtractText returns the text for text/markdown content types, else "".
func (PlainTextExtractor) ExtractText(_ context.Context, raw []byte, contentType string) (string, error) {
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	if strings.HasPrefix(ct, "text/") || ct == "text/markdown" || ct == "application/json" {
		return string(raw), nil
	}
	return "", nil
}

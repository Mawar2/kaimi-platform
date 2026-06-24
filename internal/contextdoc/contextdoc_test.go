package contextdoc

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakeExtractor returns a fixed text so Save's extraction path is deterministic.
type fakeExtractor struct{ text string }

func (f fakeExtractor) ExtractText(_ context.Context, _ []byte, _ string) (string, error) {
	return f.text, nil
}

func TestJSONStoreSaveAndList(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONStore(dir, fakeExtractor{text: "extracted capability statement"})
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}
	ctx := context.Background()

	doc, err := s.Save(ctx, "Capability Statement.pdf", "application/pdf", []byte("%PDF-1.7 ..."))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if doc.Text != "extracted capability statement" || doc.Bytes == 0 {
		t.Errorf("unexpected doc: %+v", doc)
	}
	// Raw file persisted under files/ with a sanitized name.
	if _, err := os.Stat(filepath.Join(dir, dirName, filesSubdir, doc.StoredName)); err != nil {
		t.Errorf("raw file not persisted: %v", err)
	}

	// Second distinct doc, then a re-upload of the first (same name → replace).
	if _, err := s.Save(ctx, "CPARS.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", []byte("PK...")); err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	if _, err := s.Save(ctx, "Capability Statement.pdf", "application/pdf", []byte("%PDF newer")); err != nil {
		t.Fatalf("Save re-upload: %v", err)
	}

	docs, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs after a re-upload, got %d", len(docs))
	}
	// Newest first: the re-uploaded Capability Statement is most recent.
	if docs[0].Name != "Capability Statement.pdf" {
		t.Errorf("expected newest-first ordering, got %q first", docs[0].Name)
	}
}

func TestJSONStoreRejectsEmpty(t *testing.T) {
	s, _ := NewJSONStore(t.TempDir(), fakeExtractor{})
	ctx := context.Background()
	if _, err := s.Save(ctx, "", "text/plain", []byte("x")); err == nil {
		t.Error("empty name should error")
	}
	if _, err := s.Save(ctx, "x.txt", "text/plain", nil); err == nil {
		t.Error("empty bytes should error")
	}
}

// TestSanitizeName: directory components are stripped (no path traversal) and unsafe
// characters are replaced.
func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"normal.pdf":            "normal.pdf",
		"../../etc/passwd":      "passwd",
		`..\..\windows\sys.dll`: "sys.dll",
		"my file (1).docx":      "my_file__1_.docx",
		"...":                   "file",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestPlainTextExtractor: text passes through, binary yields "".
func TestPlainTextExtractor(t *testing.T) {
	e := PlainTextExtractor{}
	if got, _ := e.ExtractText(context.Background(), []byte("hello"), "text/plain"); got != "hello" {
		t.Errorf("text/plain passthrough = %q", got)
	}
	if got, _ := e.ExtractText(context.Background(), []byte("%PDF"), "application/pdf"); got != "" {
		t.Errorf("binary should yield empty, got %q", got)
	}
}

// TestNewStoreDefaultsExtractor: a nil extractor defaults to PlainTextExtractor.
func TestNewStoreDefaultsExtractor(t *testing.T) {
	s, err := NewJSONStore(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}
	doc, err := s.Save(context.Background(), "notes.txt", "text/plain", []byte("plain notes"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if doc.Text != "plain notes" {
		t.Errorf("default extractor should pass text through, got %q", doc.Text)
	}
}

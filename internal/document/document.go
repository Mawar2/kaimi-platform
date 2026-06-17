package document

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Section is one ordered unit of the proposal document. Agents write whole
// section bodies; humans edit within them; the compliance view derives from
// the requirement links.
type Section struct {
	// ID is a stable slug (e.g. "technical_approach") used by editors and
	// agents to address the section.
	ID string `json:"id"`
	// Heading is the display title.
	Heading string `json:"heading"`
	// Body is the section prose (markdown-ish plain text).
	Body string `json:"body"`
	// RequirementIDs links the section to the solicitation requirements it
	// satisfies.
	RequirementIDs []string `json:"requirement_ids,omitempty"`
	// Status is a display hint ("outlined", "drafted", "edited").
	Status string `json:"status,omitempty"`
}

// Revision records one actor handoff. Actor is an agent name ("outline",
// "writer", "final-review") or "human".
type Revision struct {
	Version int       `json:"version"`
	Actor   string    `json:"actor"`
	At      time.Time `json:"at"`
	Note    string    `json:"note,omitempty"`
}

// Flag is a gap or review issue anchored to the document (optionally to a
// specific section). Flags persist until resolved.
type Flag struct {
	SectionID string `json:"section_id,omitempty"`
	Title     string `json:"title"`
	Detail    string `json:"detail,omitempty"`
	Resolved  bool   `json:"resolved"`
}

// Document is the proposal working draft for one opportunity.
type Document struct {
	OpportunityID string     `json:"opportunity_id"`
	Title         string     `json:"title"`
	Sections      []Section  `json:"sections"`
	Flags         []Flag     `json:"flags,omitempty"`
	Version       int        `json:"version"`
	Revisions     []Revision `json:"revisions"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Markdown renders the document as the human-readable draft text — the
// content of the auto-saved draft.md mirror.
func (d *Document) Markdown() string {
	var b strings.Builder
	b.WriteString("# " + d.Title + "\n")
	for i := range d.Sections {
		b.WriteString("\n## " + d.Sections[i].Heading + "\n\n")
		if d.Sections[i].Body != "" {
			b.WriteString(d.Sections[i].Body + "\n")
		}
	}
	return b.String()
}

// OpenFlagTexts returns the text (title + detail) of every unresolved flag on
// the document. The gate's criteria grid uses these to defer to the Final
// Review's findings rather than running an independent matcher that could
// contradict them.
func (d *Document) OpenFlagTexts() []string {
	var out []string
	for i := range d.Flags {
		if !d.Flags[i].Resolved {
			text := d.Flags[i].Title
			if d.Flags[i].Detail != "" {
				text += " " + d.Flags[i].Detail
			}
			out = append(out, text)
		}
	}
	return out
}

// Section returns a pointer to the section with the given id, or nil.
func (d *Document) Section(id string) *Section {
	for i := range d.Sections {
		if d.Sections[i].ID == id {
			return &d.Sections[i]
		}
	}
	return nil
}

// errNotFound marks a missing document; test with IsNotFound.
var errNotFound = errors.New("document not found")

// IsNotFound reports whether err means the document does not exist.
func IsNotFound(err error) bool {
	return errors.Is(err, errNotFound)
}

// idPattern is the same conservative id shape the dashboard enforces; it
// also guarantees the id is safe to use as a directory name.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Store persists proposal documents under <base>/proposals/<oppID>/. Every
// save rewrites both document.json (canonical) and draft.md (mirror)
// atomically and appends an attributed revision.
type Store struct {
	base string
	mu   sync.Mutex
	// Now is injected for deterministic tests; defaults to time.Now.
	Now func() time.Time
}

// NewStore returns a Store rooted at basePath (the same base directory the
// opportunity JSON store uses). The proposals directory is created eagerly.
func NewStore(basePath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(basePath, "proposals"), 0o755); err != nil {
		return nil, fmt.Errorf("create proposals directory: %w", err)
	}
	return &Store{base: basePath, Now: time.Now}, nil
}

// dir returns the directory that holds one opportunity's document files.
func (s *Store) dir(oppID string) string {
	return filepath.Join(s.base, "proposals", oppID)
}

// Create persists a brand-new document at version 1, attributed to actor.
func (s *Store) Create(doc *Document, actor, note string) error {
	if doc == nil {
		return fmt.Errorf("document is nil")
	}
	if !idPattern.MatchString(doc.OpportunityID) {
		return fmt.Errorf("invalid opportunity id %q", doc.OpportunityID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(filepath.Join(s.dir(doc.OpportunityID), "document.json")); err == nil {
		return fmt.Errorf("document for %s already exists", doc.OpportunityID)
	}
	doc.Version = 0
	doc.Revisions = nil
	return s.save(doc, actor, note)
}

// Get loads the document for oppID. Returns an IsNotFound error when no
// document exists yet.
func (s *Store) Get(oppID string) (*Document, error) {
	if !idPattern.MatchString(oppID) {
		return nil, fmt.Errorf("invalid opportunity id %q: %w", oppID, errNotFound)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(oppID)
}

// ReplaceSections sets the bodies of the given sections (keyed by section
// id), attributed to an agent actor. Sections not present in bodies keep
// their content; unknown ids are an error so agent/document drift surfaces.
func (s *Store) ReplaceSections(oppID string, bodies map[string]string, actor, note string) (*Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.load(oppID)
	if err != nil {
		return nil, err
	}
	for id, body := range bodies {
		sec := doc.Section(id)
		if sec == nil {
			return nil, fmt.Errorf("unknown section %q", id)
		}
		sec.Body = body
		sec.Status = "drafted"
	}
	if err := s.save(doc, actor, note); err != nil {
		return nil, err
	}
	return doc, nil
}

// UpdateSection sets one section's body, attributed to actor (normally
// "human" — this is the edit-gate path).
func (s *Store) UpdateSection(oppID, sectionID, body, actor string) (*Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.load(oppID)
	if err != nil {
		return nil, err
	}
	sec := doc.Section(sectionID)
	if sec == nil {
		return nil, fmt.Errorf("unknown section %q", sectionID)
	}
	sec.Body = body
	if actor == "human" {
		sec.Status = "edited"
	}
	if err := s.save(doc, actor, "Edited "+sec.Heading); err != nil {
		return nil, err
	}
	return doc, nil
}

// SetFlags replaces the document's flags, attributed to actor.
func (s *Store) SetFlags(oppID string, flags []Flag, actor, note string) (*Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.load(oppID)
	if err != nil {
		return nil, err
	}
	doc.Flags = flags
	if err := s.save(doc, actor, note); err != nil {
		return nil, err
	}
	return doc, nil
}

// AppendRevisionNote records a note-only revision (e.g. the human's
// request-changes note) without altering content.
func (s *Store) AppendRevisionNote(oppID, actor, note string) (*Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.load(oppID)
	if err != nil {
		return nil, err
	}
	if err := s.save(doc, actor, note); err != nil {
		return nil, err
	}
	return doc, nil
}

// load reads document.json. Callers hold s.mu.
func (s *Store) load(oppID string) (*Document, error) {
	data, err := os.ReadFile(filepath.Join(s.dir(oppID), "document.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("opportunity %s: %w", oppID, errNotFound)
		}
		return nil, fmt.Errorf("read document: %w", err)
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal document: %w", err)
	}
	return &doc, nil
}

// save bumps the version, appends the attributed revision, and atomically
// rewrites document.json plus the draft.md mirror. Callers hold s.mu.
func (s *Store) save(doc *Document, actor, note string) error {
	now := s.Now()
	doc.Version++
	doc.UpdatedAt = now
	doc.Revisions = append(doc.Revisions, Revision{
		Version: doc.Version,
		Actor:   actor,
		At:      now,
		Note:    note,
	})

	dir := s.dir(doc.OpportunityID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create document directory: %w", err)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal document: %w", err)
	}
	if err := writeAtomic(filepath.Join(dir, "document.json"), data); err != nil {
		return err
	}
	// The mirror is best-effort canonical output: same content, readable
	// outside the apps. It is rewritten on every save (autosave contract).
	return writeAtomic(filepath.Join(dir, "draft.md"), []byte(doc.Markdown()))
}

// writeAtomic writes via a temp file + rename so a crash never leaves a
// half-written document.
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename %s: %w", filepath.Base(path), err)
	}
	return nil
}

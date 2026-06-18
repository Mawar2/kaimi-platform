// Package samsecret stores a tenant's SAM.gov API key as a Secret Manager secret
// version, so a tester can enter their own key during onboarding instead of an
// operator baking it into the deployment.
//
// SECURITY: the SAM.gov key is a credential. It is written ONLY to Secret Manager
// (never to the opportunity/profile store, never logged) as a new version of the
// deployment's SAM secret. The pipeline already reads that secret's "latest"
// version, so a freshly-saved key flows through to the next hunt with no plaintext
// at rest in the app's data layer. Each tenant deployment points at its OWN secret,
// which is what makes per-tenant SAM quota isolation possible (one tenant's key can
// never exhaust another's).
package samsecret

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Writer persists a tenant's SAM.gov API key. Implementations: SecretManagerWriter
// (production) and MemoryWriter (tests/dev). Save validates the key format before
// persisting, so a caller can surface a clean error without writing garbage.
type Writer interface {
	// Save validates apiKey and persists it as the new current key. It returns
	// ErrInvalidKey (wrapped) when the key is malformed, or a backend error.
	Save(ctx context.Context, apiKey string) error
}

// ErrInvalidKey is returned (wrapped) by Save/ValidateKey when the supplied string
// does not look like a SAM.gov API key. Callers map it to a 400-class response.
var ErrInvalidKey = fmt.Errorf("invalid SAM.gov API key")

// SAM.gov API keys are fixed-length alphanumeric tokens (observed length 40). We
// accept a small band around that and reject anything with spaces or punctuation,
// so an obviously-wrong paste (a URL, an email, a truncated key) fails fast without
// being brittle to minor format drift.
const (
	minKeyLen = 30
	maxKeyLen = 64
)

// ValidateKey reports whether key is a plausible SAM.gov API key: trimmed, within the
// length band, and limited to the characters real keys use — letters, digits, and the
// separators '-', '_', '.'. (api.data.gov / SAM.gov keys are 40 characters and DO
// contain hyphens, so a pure-alphanumeric check wrongly rejects a valid key.) It does
// NOT call SAM.gov (that would spend the very quota the key unlocks); a wrong-but-well-
// formed key surfaces on the first hunt. It returns ErrInvalidKey (wrapped) on failure.
func ValidateKey(key string) error {
	k := strings.TrimSpace(key)
	if k == "" {
		return fmt.Errorf("%w: the key is empty", ErrInvalidKey)
	}
	if len(k) < minKeyLen || len(k) > maxKeyLen {
		return fmt.Errorf("%w: expected %d–%d characters, got %d", ErrInvalidKey, minKeyLen, maxKeyLen, len(k))
	}
	for _, r := range k {
		allowed := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.'
		if !allowed {
			return fmt.Errorf("%w: the key may contain only letters, digits, hyphen, underscore and dot", ErrInvalidKey)
		}
	}
	return nil
}

// MemoryWriter is an in-memory Writer for tests and offline/dev mode. It records the
// last saved key and a version count so tests can assert a save happened without a
// real Secret Manager. It is concurrency-safe. It is NOT durable.
type MemoryWriter struct {
	mu       sync.Mutex
	last     string
	versions int
}

// NewMemoryWriter returns an empty in-memory writer.
func NewMemoryWriter() *MemoryWriter { return &MemoryWriter{} }

// Save validates the key and records it.
func (w *MemoryWriter) Save(_ context.Context, apiKey string) error {
	if err := ValidateKey(apiKey); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.last = strings.TrimSpace(apiKey)
	w.versions++
	return nil
}

// Versions returns how many keys have been saved (for tests).
func (w *MemoryWriter) Versions() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.versions
}

// Last returns the most recently saved key (for tests only — never expose in prod).
func (w *MemoryWriter) Last() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.last
}

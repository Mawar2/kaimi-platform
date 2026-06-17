// Package productkey issues and validates time-limited Kaimi product keys that
// gate access to a per-tester deployment for a fixed testing window.
//
// A product key (KAIMI-XXXX-XXXX-XXXX) is the access credential: a tester opens
// their magic link (the key in the URL) or types the key, and a valid,
// unexpired, un-revoked key grants a session for the rest of its window. The
// Registry persists issued keys and their expiry/revocation; the httpapi gate
// consults it. This package owns the key FORMAT and VALIDATION rules so the gate
// and the kaimi-key admin CLI agree.
//
// The key is an OPAQUE random credential, not a signed token: validation is a
// Registry lookup, which is what lets an operator revoke a key instantly (set
// Revoked) without waiting for an expiry. (The KMS-signed-JWT model in
// docs/goals/licensing-provisioning.md is the future per-customer evolution; for
// round-1 BlueMeta-hosted pilots a central registry is simpler and revocable.)
package productkey

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
)

// KeyPrefix opens every product key so a key is recognizable on sight and in logs.
const KeyPrefix = "KAIMI"

// keyGroups / groupLen define the KAIMI-XXXX-XXXX-XXXX shape: 3 groups of 4.
const (
	keyGroups = 3
	groupLen  = 4
)

// keyAlphabet is an unambiguous uppercase alphabet: no 0/O, 1/I/L — so a tester
// can read a key off an email or screen and type it without confusion.
const keyAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// ErrNotFound is returned by Registry.Lookup when no key matches. Callers use
// errors.Is to map it to "access denied" without leaking whether a key existed.
var ErrNotFound = errors.New("product key not found")

// Record is a persisted product key and its access window.
type Record struct {
	Key       string    `firestore:"key" json:"key"`               // KAIMI-XXXX-XXXX-XXXX (also the doc id)
	Tester    string    `firestore:"tester" json:"tester"`         // human label, e.g. "Ey3 Technologies"
	IssuedAt  time.Time `firestore:"issued_at" json:"issued_at"`   //
	ExpiresAt time.Time `firestore:"expires_at" json:"expires_at"` // access ends at this instant
	Revoked   bool      `firestore:"revoked" json:"revoked"`       // operator kill-switch (instant, independent of expiry)
}

// Valid reports whether the record grants access at now: not revoked and not yet
// expired. The gate calls this; expiry and revocation are both enforced here so
// the rule lives in one place. Pointer receiver: Record carries two time.Time +
// two strings, heavy enough that copying per call is wasteful.
func (r *Record) Valid(now time.Time) bool {
	return !r.Revoked && now.Before(r.ExpiresAt)
}

// Registry persists product keys. Implementations: FirestoreRegistry (production)
// and MemoryRegistry (tests / offline dev). All methods take a context so the
// Firestore implementation can honor deadlines and cancellation.
type Registry interface {
	// Mint generates a new unique key for tester, valid for ttl from now, and
	// persists it. It returns the stored Record (including the generated Key).
	Mint(ctx context.Context, tester string, ttl time.Duration) (Record, error)
	// Lookup returns the record for key (after Normalize), or ErrNotFound.
	Lookup(ctx context.Context, key string) (Record, error)
	// Revoke marks key revoked. It is idempotent; revoking an unknown key returns
	// ErrNotFound so the caller can report a typo.
	Revoke(ctx context.Context, key string) error
	// List returns all records (for the admin `kaimi-key list`).
	List(ctx context.Context) ([]Record, error)
}

// Normalize canonicalizes a user-supplied key for lookup: upper-cases, trims
// spaces, and strips separators so "kaimi 7f3a 9c2e b1d4", "KAIMI-7F3A-9C2E-B1D4",
// and the magic-link query value all resolve to the same stored key. It does NOT
// validate the alphabet; an invalid key simply won't be found.
func Normalize(key string) string {
	up := strings.ToUpper(strings.TrimSpace(key))
	up = strings.ReplaceAll(up, " ", "")
	up = strings.ReplaceAll(up, "-", "")
	if !strings.HasPrefix(up, KeyPrefix) {
		return up // not a Kaimi key; return as-is so Lookup misses cleanly
	}
	body := up[len(KeyPrefix):]
	// Re-insert the canonical KAIMI-XXXX-XXXX-XXXX dashes when the body is the
	// expected length; otherwise return the prefix+body so an oddly-shaped input
	// still misses rather than panicking.
	if len(body) != keyGroups*groupLen {
		return KeyPrefix + "-" + body
	}
	groups := make([]string, 0, keyGroups+1)
	groups = append(groups, KeyPrefix)
	for i := range keyGroups {
		groups = append(groups, body[i*groupLen:(i+1)*groupLen])
	}
	return strings.Join(groups, "-")
}

// GenerateKey returns a new random KAIMI-XXXX-XXXX-XXXX key drawn from the
// unambiguous alphabet using crypto/rand. It is the single place key strings are
// minted, so format and entropy stay consistent.
//
// It uses rejection sampling rather than a plain byte%len: because 256 is not a
// multiple of len(keyAlphabet), a naive modulo would make the first
// 256%len(keyAlphabet) symbols slightly more likely. These keys are access
// credentials, so each symbol must be equally probable — we discard the few
// random bytes above the largest exact multiple of the alphabet length.
func GenerateKey() (string, error) {
	n := keyGroups * groupLen
	// limit is the largest multiple of the alphabet length that fits in a byte;
	// bytes >= limit are rejected to keep the distribution uniform.
	limit := 256 - (256 % len(keyAlphabet))
	out := make([]byte, 0, n)
	buf := make([]byte, n)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("product key: read random: %w", err)
		}
		for _, b := range buf {
			if int(b) >= limit {
				continue // would bias toward the start of the alphabet; reject
			}
			out = append(out, keyAlphabet[int(b)%len(keyAlphabet)])
			if len(out) == n {
				break
			}
		}
	}
	groups := make([]string, 0, keyGroups+1)
	groups = append(groups, KeyPrefix)
	for i := range keyGroups {
		groups = append(groups, string(out[i*groupLen:(i+1)*groupLen]))
	}
	return strings.Join(groups, "-"), nil
}

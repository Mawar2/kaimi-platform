package httpapi

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

// TestSessionPayloadEncrypted proves the session cookie payload no longer exposes its
// claims: base64-decoding the payload segment must NOT reveal the product key (KeyID) or
// the JSON field names. It also verifies the token still round-trips and that a token
// minted under one secret cannot be decrypted/verified under a different secret.
func TestSessionPayloadEncrypted(t *testing.T) {
	const secret = "a-long-enough-session-secret-1234567890"
	const key = "KAIMI-AAAA-BBBB-CCCC"
	m := newSessionManager([]byte(secret), time.Hour)

	tok := m.sign(Session{KeyID: key, Expiry: time.Now().Add(time.Hour).Unix()})

	payload, _, ok := strings.Cut(tok, ".")
	if !ok {
		t.Fatalf("token not in <payload>.<sig> form: %q", tok)
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("payload not base64url: %v", err)
	}
	if strings.Contains(string(raw), key) {
		t.Error("payload leaks the product key in plaintext")
	}
	if strings.Contains(string(raw), "\"kid\"") || strings.Contains(string(raw), "KAIMI-") {
		t.Errorf("payload exposes plaintext claims: %q", raw)
	}

	// Round-trips under the same manager.
	got, err := m.verify(tok)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if got.KeyID != key {
		t.Errorf("round-trip KeyID = %q, want %q", got.KeyID, key)
	}

	// A different secret cannot decrypt/verify it.
	other := newSessionManager([]byte("a-DIFFERENT-session-secret-0987654321"), time.Hour)
	if _, err := other.verify(tok); err == nil {
		t.Error("token must not verify under a different secret")
	}
}

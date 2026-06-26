package proposal

import (
	"testing"
)

// TestCurrentContextDocsUsesProvider verifies the Writer grounds on the client's
// onboarding context documents resolved FRESH per draft: when a ContextDocsProvider
// is wired its value is used (so a capability statement uploaded after the service
// started still reaches the draft), and when none is wired the result is nil so the
// pipeline degrades to profile + solicitation grounding rather than failing (#134).
func TestCurrentContextDocsUsesProvider(t *testing.T) {
	docs := map[string]string{"capability-statement.txt": "Zero-downtime VA cloud migration, 2024."}

	cases := []struct {
		name string
		deps *Deps
		want map[string]string
	}{
		{"provider value is used", &Deps{ContextDocsProvider: func() map[string]string { return docs }}, docs},
		{"provider returning nil yields nil", &Deps{ContextDocsProvider: func() map[string]string { return nil }}, nil},
		{"no provider yields nil", &Deps{}, nil},
	}
	for _, c := range cases {
		s := NewService(c.deps)
		got := s.currentContextDocs()
		if len(got) != len(c.want) {
			t.Errorf("%s: got %d docs, want %d", c.name, len(got), len(c.want))
			continue
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Errorf("%s: doc %q = %q, want %q", c.name, k, got[k], v)
			}
		}
	}
}

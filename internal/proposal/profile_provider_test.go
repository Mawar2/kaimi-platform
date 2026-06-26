package proposal

import (
	"testing"

	"github.com/Mawar2/Kaimi/internal/scorer"
)

// TestCurrentProfilePrefersProvider verifies the Writer grounds on a FRESH profile: the
// ProfileProvider (re-resolved per draft) takes precedence over the static startup Profile,
// so a tenant who onboards after the service started gets proposals branded with THEIR
// company — falling back to the static profile when no provider is wired or it returns nil.
func TestCurrentProfilePrefersProvider(t *testing.T) {
	static := &scorer.CapabilityProfile{Company: "Static Co"}
	dynamic := &scorer.CapabilityProfile{Company: "Dynamic Co"}

	cases := []struct {
		name string
		deps *Deps
		want string
	}{
		{"provider value wins over static", &Deps{Profile: static, ProfileProvider: func() *scorer.CapabilityProfile { return dynamic }}, "Dynamic Co"},
		{"provider returning nil falls back to static", &Deps{Profile: static, ProfileProvider: func() *scorer.CapabilityProfile { return nil }}, "Static Co"},
		{"no provider uses static", &Deps{Profile: static}, "Static Co"},
	}
	for _, c := range cases {
		s := NewService(c.deps)
		got := s.currentProfile()
		if got == nil || got.Company != c.want {
			t.Errorf("%s: currentProfile().Company = %v, want %q", c.name, got, c.want)
		}
	}
}

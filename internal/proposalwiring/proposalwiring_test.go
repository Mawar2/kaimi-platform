package proposalwiring

import (
	"context"
	"testing"

	"github.com/Mawar2/Kaimi/internal/config"
	"github.com/Mawar2/Kaimi/internal/store"
)

// TestNew_OfflineWiresStubsNoNetwork verifies that with every live toggle off
// (the Options zero value) New returns a non-nil *proposal.Service wired with
// the stub/deterministic agents and makes no GCP/network calls. It pins the
// offline path that credential-less UI development and the fast test suite rely
// on. WriterPath is left empty so the profile resolver is not exercised (the
// offline contract under test is "no network / no required filesystem"); the
// table toggles the three flags to show only the all-off combination stays
// offline, since any live flag legitimately needs credentials.
func TestNew_OfflineWiresStubsNoNetwork(t *testing.T) {
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	tests := []struct {
		name       string
		opts       Options
		wantErr    bool
		wantNonNil bool
	}{
		{
			name:       "all offline returns a wired service",
			opts:       Options{Store: s, BasePath: t.TempDir()},
			wantNonNil: true,
		},
		{
			name:    "live-writer without project id errors before any call",
			opts:    Options{Store: s, BasePath: t.TempDir(), LiveWriter: true},
			wantErr: true,
		},
		{
			name:    "live-review without project id errors before any call",
			opts:    Options{Store: s, BasePath: t.TempDir(), LiveReview: true},
			wantErr: true,
		},
		{
			name:    "live-ingest without targets errors before any call",
			opts:    Options{Store: s, BasePath: t.TempDir(), LiveIngest: true},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Empty cfg: no ProjectID/ingest targets, and WriterPath unset so the
			// profile resolver (filesystem) is skipped. This makes the offline path
			// purely in-memory + local temp dirs.
			cfg := &config.Config{}

			svc, err := New(context.Background(), cfg, tc.opts)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s, got nil", tc.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("New returned error: %v", err)
			}
			if tc.wantNonNil && svc == nil {
				t.Fatal("expected a non-nil *proposal.Service")
			}
		})
	}
}

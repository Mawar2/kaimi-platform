//go:build live

package outline

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/googledocs"
	"github.com/Mawar2/Kaimi/internal/opportunity"
)

// TestLive_OutlineAgent_CreatesGoogleDoc runs the full Outline agent against real
// Drive and Docs APIs. Run with:
//
//	go test ./internal/outline/ -tags=live -run TestLive_OutlineAgent_CreatesGoogleDoc -v
//
// Requires GOOGLE_DRIVE_SHARED_DRIVE_ID. Authenticates via:
//   - GOOGLE_DRIVE_CREDENTIALS_JSON (service-account key), if set; otherwise
//   - Application Default Credentials (run: gcloud auth application-default login)
func TestLive_OutlineAgent_CreatesGoogleDoc(t *testing.T) {
	driveID := os.Getenv("GOOGLE_DRIVE_SHARED_DRIVE_ID")
	if driveID == "" {
		t.Skip("GOOGLE_DRIVE_SHARED_DRIVE_ID not set — skipping live test")
	}

	cfg := googledocs.Config{SharedDriveID: driveID}

	credsRaw := os.Getenv("GOOGLE_DRIVE_CREDENTIALS_JSON")
	if credsRaw != "" {
		if !json.Valid([]byte(credsRaw)) {
			t.Fatal("GOOGLE_DRIVE_CREDENTIALS_JSON is not valid JSON")
		}
		cfg.CredentialsJSON = []byte(credsRaw)
		t.Log("Auth: service-account JSON key")
	} else {
		cfg.UseADC = true
		t.Log("Auth: Application Default Credentials (gcloud auth application-default login)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := googledocs.NewClient(ctx, cfg)
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	opp := &opportunity.Opportunity{
		ID:           "LIVE-TEST-001",
		Title:        "Live Test: IT Systems Integration Services",
		Description:  "Provide key personnel for IT integration. Proposals shall not to exceed 25 pages. Submit in PDF format. Personnel must hold an active Secret clearance. SF-330 required.",
		SetAsideCode: "SBA",
		Type:         "Solicitation",
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	agent := New(client)
	outline, result, err := agent.Run(ctx, opp)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	t.Logf("Doc URL:       %s", result.OutputRef)
	t.Logf("Sections:      %d", len(outline.Sections))
	t.Logf("Doc ID:        %s", result.Flags["doc_id"])

	if result.OutputRef == "" {
		t.Error("OutputRef (Doc URL) must not be empty")
	}
	if !strings.HasPrefix(result.OutputRef, "https://docs.google.com/document/d/") {
		t.Errorf("OutputRef = %q, want Google Docs URL", result.OutputRef)
	}
	if len(outline.Sections) < 5 {
		t.Errorf("expected at least 5 sections, got %d", len(outline.Sections))
	}

	t.Logf("\nOpen the doc to verify content: %s", result.OutputRef)
}

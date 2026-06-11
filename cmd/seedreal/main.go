// Command seedreal builds real Opportunity records from a curated solicitations
// folder (each subdirectory holds a manifest.json plus the source documents) and
// scores them with the real Gemini scorer, writing the result to a JSON store.
//
// Unlike cmd/pipeline's live mode, this path needs NO SAM.gov API call (the
// solicitations were already sourced), so it is not subject to SAM.gov's daily
// quota. It still exercises the real scoring agent (Vertex AI via ADC).
//
// Usage:
//
//	go run ./cmd/seedreal \
//	  --solicitations "<path>/hackathon/solicitations" \
//	  --textdir /tmp/soltext \
//	  --store ./real-store \
//	  --profile config/bluemeta_scorer_profile.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
)

// manifest mirrors the curated solicitation manifest.json shape.
type manifest struct {
	NoticeID  string `json:"notice_id"`
	Title     string `json:"title"`
	Agency    string `json:"agency"`
	Type      string `json:"type"`
	NAICS     string `json:"naics"`
	SetAside  string `json:"set_aside"`
	OffersDue string `json:"offers_due"`
	URL       string `json:"url"`
	SolNumber string `json:"solicitation_number"`
	PulledAt  string `json:"pulled_at"`
}

const maxDescriptionChars = 12000 // enough real text to ground scoring; bounds tokens/cost

func main() {
	solicitations := flag.String("solicitations", "", "path to the curated solicitations folder (required)")
	textDir := flag.String("textdir", "", "path to a folder of <notice_id>.txt extracted-text files (optional grounding)")
	storePath := flag.String("store", "./real-store", "JSON store output directory")
	profilePath := flag.String("profile", "config/bluemeta_scorer_profile.json", "scorer.CapabilityProfile JSON path")
	project := flag.String("project", "kaimi-seeker", "GCP project ID for Vertex AI")
	region := flag.String("region", "us-east4", "GCP region")
	model := flag.String("model", "gemini-2.5-pro", "Gemini model name")
	flag.Parse()

	if *solicitations == "" {
		log.Fatal("seedreal: --solicitations is required")
	}
	if err := run(*solicitations, *textDir, *storePath, *profilePath, *project, *region, *model); err != nil {
		log.Fatalf("seedreal: %v", err)
	}
}

func run(solDir, textDir, storePath, profilePath, project, region, model string) error {
	ctx := context.Background()

	profile, err := loadProfile(profilePath)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}

	st, err := store.NewJSONStore(storePath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}

	sc, err := scorer.NewGeminiScorer(ctx, project, region, model)
	if err != nil {
		return fmt.Errorf("create Gemini scorer: %w", err)
	}

	manifests, err := filepath.Glob(filepath.Join(solDir, "*", "manifest.json"))
	if err != nil {
		return fmt.Errorf("glob manifests: %w", err)
	}
	if len(manifests) == 0 {
		return fmt.Errorf("no manifest.json found under %s", solDir)
	}

	fmt.Printf("Seeding %d real solicitations into %s (scoring with %s)\n\n", len(manifests), storePath, model)
	scored := 0
	for _, mPath := range manifests {
		m, err := readManifest(mPath)
		if err != nil {
			log.Printf("  skip %s: %v", mPath, err)
			continue
		}
		opp := toOpportunity(m, textDir)
		fmt.Printf("- %s (%s)\n  agency: %s | NAICS %s | due %s\n",
			opp.Title, opp.ID, opp.Agency, opp.NAICSCode, opp.ResponseDeadline.Format("2006-01-02"))
		if err := scorer.ScoreAndSave(ctx, sc, st, opp, profile); err != nil {
			log.Printf("  scoring failed: %v", err)
			continue
		}
		fmt.Printf("  => score %.0f%% | %s\n\n", opp.Score*100, opp.Recommendation)
		scored++
	}
	fmt.Printf("Done: %d/%d solicitations scored and saved to %s\n", scored, len(manifests), storePath)
	return nil
}

func loadProfile(path string) (*scorer.CapabilityProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = stripBOM(data)
	var cp scorer.CapabilityProfile
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &cp, nil
}

func readManifest(path string) (*manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(stripBOM(data), &m); err != nil {
		return nil, err
	}
	if m.NoticeID == "" || m.Title == "" {
		return nil, fmt.Errorf("manifest missing notice_id/title")
	}
	return &m, nil
}

// toOpportunity maps a curated manifest to the Hunter-stage Opportunity fields
// the scorer reads, grounding Description on the real extracted text when present.
func toOpportunity(m *manifest, textDir string) *opportunity.Opportunity {
	now := time.Now().UTC()
	code, desc := splitNAICS(m.NAICS)

	deadline := time.Time{}
	if m.OffersDue != "" {
		if t, err := time.Parse(time.RFC3339, m.OffersDue); err == nil {
			deadline = t.UTC()
		}
	}

	body := grounding(textDir, m.NoticeID)
	header := fmt.Sprintf("%s — %s. Issued by %s. NAICS %s (%s). Set-aside: %s. Responses due %s.\n\n",
		m.Title, strDefault(m.Type, "Solicitation"), m.Agency, code, desc, strDefault(m.SetAside, "unspecified"),
		strDefault(m.OffersDue, "TBD"))
	description := header + body
	if len(description) > maxDescriptionChars {
		description = description[:maxDescriptionChars]
	}

	return &opportunity.Opportunity{
		ID:               m.NoticeID,
		Title:            m.Title,
		SolicitationNum:  m.SolNumber,
		Agency:           m.Agency,
		ResponseDeadline: deadline,
		PostedDate:       now,
		NAICSCode:        code,
		NAICSDescription: desc,
		SetAsideCode:     m.SetAside,
		Description:      description,
		Type:             strDefault(m.Type, "Solicitation"),
		URL:              m.URL,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// grounding reads textDir/<noticeID>.txt if available; otherwise returns "".
func grounding(textDir, noticeID string) string {
	if textDir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(textDir, noticeID+".txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// splitNAICS turns "541511 - Custom Computer Programming Services" into
// ("541511", "Custom Computer Programming Services").
func splitNAICS(s string) (code, desc string) {
	s = strings.TrimSpace(s)
	for _, sep := range []string{" - ", " – ", "-"} {
		if i := strings.Index(s, sep); i > 0 {
			return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+len(sep):])
		}
	}
	return s, ""
}

func strDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

func stripBOM(b []byte) []byte {
	return []byte(strings.TrimPrefix(string(b), "\ufeff"))
}

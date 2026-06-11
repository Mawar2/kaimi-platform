// Command demo-seed ports a manually-sourced, real SAM.gov opportunity into the
// store exactly the way the Zone-1 pipeline (Hunter eligibility + Scorer) would,
// so the live Zone-2 dashboard flow can be exercised on genuine data without
// burning the SAM.gov API quota.
//
// It builds the Opportunity from verified SAM.gov fields, then runs the REAL
// offline DeterministicScorer via scorer.ScoreAndSave — the same code path and
// the same field mapping (Score, ScoreReasoning, Recommendation, Requirements,
// ScoredAt) that cmd/pipeline uses in cached mode. The only thing skipped is the
// live SAM fetch, which the quota currently blocks.
//
// Usage:
//
//	go run ./cmd/demo-seed --store-path=./live-store/queue
//
// The opportunity below is the Selective Service System Website Modernization
// solicitation (90MC26R0004, NAICS 541519, Total Small Business Set-Aside,
// response due 2026-06-30), verified against the SAM.gov public opportunity API.
// It is an eligible, on-profile match for the bundled demo capability profile
// (primary NAICS 541519; small business + SDB qualifies for the SBA set-aside).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
)

// demoScorerProfile is a self-contained capability profile in the scorer's
// CapabilityProfile shape, used only to seed the demo store. Competency tags are
// realistic federal IT technical areas; the DeterministicScorer matches them
// case-insensitively against the opportunity title and description. Production
// deployments load their profile from config instead of hardcoding it here.
func demoScorerProfile() *scorer.CapabilityProfile {
	return &scorer.CapabilityProfile{
		PrimaryNAICS:   []string{"541519", "541512", "541511"},
		SecondaryNAICS: []string{"518210", "513210", "541715"},
		CompetencyTags: []string{
			"website", "web application", "software development", "modernization",
			"cloud", "GCP", "API development", "data visualization", "user experience",
			"Agile", "AI/ML", "federal systems", "automation", "federal compliance",
		},
		PastPerformance: []string{
			"U.S. Census Bureau", "Census Bureau", "Justice40", "Bullard Center",
			"CEEJH", "Selective Service", "federal web platform",
		},
		SDBStatus:           true,
		QualifyingSetAsides: []string{"SDB", "SBA", "SBP"},
	}
}

// selectiveServiceOpportunity is the verified SAM.gov record (notice
// e89891bf686b4a95a8df4befcd6d296e). Posted/deadline are real; the description
// is condensed from the notice. Scoring fields are intentionally left zero —
// scorer.ScoreAndSave fills them.
func selectiveServiceOpportunity(now time.Time) *opportunity.Opportunity {
	posted := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	// 2026-06-30 14:00 ET (EDT, UTC-4).
	deadline := time.Date(2026, 6, 30, 14, 0, 0, 0, time.FixedZone("EDT", -4*3600))

	return &opportunity.Opportunity{
		ID:               "e89891bf686b4a95a8df4befcd6d296e",
		Title:            "Selective Service System Website Modernization",
		SolicitationNum:  "90MC26R0004",
		Agency:           "SELECTIVE SERVICE SYSTEM",
		Office:           "SELECTIVE SERVICE SYSTEM",
		PostedDate:       posted,
		ResponseDeadline: deadline,

		NAICSCode:          "541519",
		NAICSDescription:   "Other Computer Related Services",
		SetAsideCode:       "SBA", // Total Small Business Set-Aside (FAR 19.5)
		PlaceOfPerformance: "Arlington, VA 22209, USA",

		Description: "The Selective Service System (SSS) is soliciting for its Website " +
			"Modernization Project. This solicitation is issued as a commercial product and " +
			"commercial service acquisition pursuant to FAR Part 12. The Government intends to " +
			"award a single Firm-Fixed-Price contract using negotiated procurement procedures in " +
			"accordance with FAR Part 15. Offerors submit proposals per the addenda to FAR " +
			"52.212-1 and are evaluated per the addenda to FAR 52.212-2. The Government intends to " +
			"evaluate proposals and make award without discussions, but reserves the right to " +
			"conduct discussions if in the Government's best interest. The requirement was " +
			"developed following market research previously conducted under SAM.gov Notice ID " +
			"RFI 90MC00_0002. Two amendments have been issued: Amendment 0001 (initial addendum) " +
			"and Amendment 0002 (which begins the question-and-response process).",
		Type:         "Solicitation",
		ContractType: "Firm Fixed Price",

		URL: "https://sam.gov/opp/e89891bf686b4a95a8df4befcd6d296e/view",
		Attachments: []string{
			"https://sam.gov/opp/e89891bf686b4a95a8df4befcd6d296e/view",
		},

		CreatedAt: now,
		UpdatedAt: now,
	}
}

func main() {
	storePath := flag.String("store-path", "./live-store/queue", "Store directory path")
	flag.Parse()

	ctx := context.Background()
	now := time.Now().UTC()

	s, err := store.NewJSONStore(*storePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "demo-seed: create store: %v\n", err)
		os.Exit(1)
	}

	opp := selectiveServiceOpportunity(now)
	profile := demoScorerProfile()

	// Same code path Zone-1 uses in cached mode: score with the real
	// DeterministicScorer and persist via the Store.
	if err := scorer.ScoreAndSave(ctx, scorer.NewDeterministicScorer(), s, opp, profile); err != nil {
		fmt.Fprintf(os.Stderr, "demo-seed: score and save: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Seeded %s into %s\n", opp.ID, *storePath)
	fmt.Printf("  Title:          %s\n", opp.Title)
	fmt.Printf("  NAICS:          %s (%s)\n", opp.NAICSCode, opp.NAICSDescription)
	fmt.Printf("  Set-aside:      %s\n", opp.SetAsideCode)
	fmt.Printf("  Score:          %.2f\n", opp.Score)
	fmt.Printf("  Recommendation: %s\n", opp.Recommendation)
	fmt.Printf("  Reasoning:      %s\n", opp.ScoreReasoning)
}

// Package pipeline wires Kaimi's Zone-1 agents into a single runnable flow.
//
// Zone 1 is the scheduled half of the architecture: the Hunter discovers and
// eligibility-gates SAM.gov opportunities, and the Scorer assigns each a bid-fit
// score. KAI-M6 glues them together so an operator (and, later, a scheduler) can
// run Hunter → Scorer in one call and land scored opportunities in the Store.
//
// RunZone1 is deliberately a plain function over injected collaborators so it can
// be driven from a cmd entrypoint now and from a scheduler later without change.
package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/Mawar2/Kaimi/internal/profile"
	"github.com/Mawar2/Kaimi/internal/samgov"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
)

// DescriptionResolver fetches an opportunity's full description text from the SAM
// noticedesc URL its Description field carries. samgov.DescriptionResolver satisfies it;
// the interface keeps RunZone1 testable without live SAM.
type DescriptionResolver interface {
	Resolve(ctx context.Context, descURL string) (string, error)
}

// Zone1Deps are the collaborators RunZone1 needs.
//
// Sam, Scorer, Store, Profile, and Eligibility are all required. NAICSCodes is
// optional and defaults to the eligibility profile's full code list when empty.
type Zone1Deps struct {
	// Sam fetches opportunities (cached fixtures or live SAM.gov).
	Sam samgov.Client

	// Scorer assigns a bid-fit score. Use scorer.DeterministicScorer for cached/
	// offline runs and scorer.GeminiScorer for live runs.
	Scorer scorer.Scorer

	// Store is where scored opportunities are persisted.
	Store store.Store

	// Profile is the weighted scoring profile passed to the Scorer.
	Profile *scorer.CapabilityProfile

	// Eligibility is the binary eligibility gate applied before scoring.
	// Required — load via profile.LoadProfile before constructing Zone1Deps.
	Eligibility *profile.CapabilityProfile

	// NAICSCodes are the codes to fetch. Defaults to the eligibility profile's
	// full code list (AllNAICSCodes) when empty.
	NAICSCodes []string

	// Resolver, when set, fetches each ELIGIBLE opportunity's full description text from
	// its SAM noticedesc URL before scoring, so the Scorer scores real prose rather than
	// the URL the search API returns in Description. Optional; nil skips resolution (the
	// Scorer falls back to the raw Description). Resolution is bounded to the eligible set
	// — never the full fetch — to respect the SAM daily quota, and is non-fatal per opp.
	Resolver DescriptionResolver

	// TenantID is the owning deployment/org stamped onto every opportunity this
	// run persists, making each record self-describing. Empty leaves the field
	// unset (omitted from JSON), matching legacy records. Sourced from the
	// pipeline's config (config.Tenant.ID).
	TenantID string
}

// Zone1Report summarizes a single Zone-1 run.
type Zone1Report struct {
	Fetched  int      // opportunities returned by the Sam client
	Eligible int      // opportunities that passed the eligibility gate
	Dropped  int      // opportunities dropped by the eligibility gate
	Scored   int      // eligible opportunities scored and saved successfully
	Failed   int      // eligible opportunities that failed to score or save
	Resolved int      // eligible opportunities whose description text was resolved
	SavedIDs []string // IDs of opportunities persisted to the Store
	Errors   []string // per-opportunity failures, formatted "<id>: <error>"
}

// RunZone1 runs the Hunter eligibility gate + Scorer flow exactly once and
// persists each scored opportunity to the Store.
//
// It fetches opportunities for the configured NAICS codes, drops those reserved
// for set-asides the company does not hold, and scores+saves the rest. A scoring or
// save failure on one opportunity is recorded in the report and does not abort
// the run — only a fetch failure (which yields no work at all) is returned as an
// error. This single-shot shape is intentional; a scheduler can call RunZone1 on
// an interval later.
//
// TODO(phase-N): a Zone-1 scheduler (Agent Engine / cron) will call RunZone1
// on a schedule. That infrastructure is out of scope for KAI-M6.
func RunZone1(ctx context.Context, deps *Zone1Deps) (*Zone1Report, error) {
	if deps == nil {
		return nil, fmt.Errorf("pipeline: deps is required")
	}
	if deps.Sam == nil {
		return nil, fmt.Errorf("pipeline: Sam client is required")
	}
	if deps.Scorer == nil {
		return nil, fmt.Errorf("pipeline: Scorer is required")
	}
	if deps.Store == nil {
		return nil, fmt.Errorf("pipeline: Store is required")
	}
	if deps.Profile == nil {
		return nil, fmt.Errorf("pipeline: scoring Profile is required")
	}

	if deps.Eligibility == nil {
		return nil, fmt.Errorf("pipeline: Eligibility profile is required")
	}
	eligibility := deps.Eligibility
	naics := deps.NAICSCodes
	if len(naics) == 0 {
		naics = eligibility.AllNAICSCodes()
	}

	opps, err := deps.Sam.FetchByNAICS(ctx, naics)
	if err != nil {
		return nil, fmt.Errorf("pipeline: fetch opportunities: %w", err)
	}

	report := &Zone1Report{Fetched: len(opps)}
	// TODO(phase-N): scoring runs sequentially. When batch sizes grow, a bounded
	// worker pool / errgroup could parallelize live scoring within GCP rate limits.
	for _, opp := range opps {
		// Stop promptly if the caller cancels — live scoring can run for minutes.
		if err := ctx.Err(); err != nil {
			return report, err
		}

		if !eligibility.IsEligible(opp) {
			report.Dropped++
			continue
		}
		report.Eligible++

		// Stamp the owning tenant onto the record before it is scored and
		// persisted, so every saved opportunity is self-describing. Empty
		// TenantID leaves the field unset (omitempty), matching legacy records.
		opp.TenantID = deps.TenantID

		// Resolve the full description text (eligible set only — SAM-quota-bounded) so the
		// Scorer scores real prose, not the noticedesc URL the search API returns. Skip if
		// already resolved or if Description is not a URL. Non-fatal: on failure the Scorer
		// falls back to the raw Description via EffectiveDescription.
		if deps.Resolver != nil && opp.ResolvedDescription == "" && strings.HasPrefix(opp.Description, "http") {
			if text, rerr := deps.Resolver.Resolve(ctx, opp.Description); rerr == nil && text != "" {
				opp.ResolvedDescription = text
				report.Resolved++
			}
		}

		if err := scorer.ScoreAndSave(ctx, deps.Scorer, deps.Store, opp, deps.Profile); err != nil {
			report.Failed++
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", opp.ID, err))
			continue
		}
		report.Scored++
		report.SavedIDs = append(report.SavedIDs, opp.ID)
	}

	return report, nil
}

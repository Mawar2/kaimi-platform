package dashboard

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/store"
)

// SortKey selects the sort field for List results.
type SortKey string

const (
	// SortByDeadline sorts by ResponseDeadline ascending (earliest deadline first).
	SortByDeadline SortKey = "deadline"
	// SortByScore sorts by Score descending (highest score first).
	SortByScore SortKey = "score"
)

// deadlineSoonWindow is the duration within which an upcoming deadline is flagged
// as "soon" on the OpportunityRow.
const deadlineSoonWindow = 7 * 24 * time.Hour

// ListOptions configures the behavior of Service.List.
type ListOptions struct {
	// Stage filters to a specific pipeline stage. nil returns all stages.
	Stage *Stage
	// MinScore filters to opportunities with Score >= MinScore. 0 means no filter.
	MinScore float64
	// Recommendation filters to a specific scorer recommendation ("BID",
	// "REVIEW", "NO_BID") for the Triage segmented filter (issue #150).
	// Empty means no filter.
	Recommendation string
	// SortBy selects the sort field. Defaults to SortByDeadline when zero.
	SortBy SortKey
	// Now is injected for DeadlineSoon computation. Zero value disables the flag.
	Now time.Time
	// ExcludeSelected drops opportunities a human has already pursued
	// (opp.Selected). The Triage queue is self-cleaning: an opportunity
	// disappears from the Opportunities tab the moment it is pursued
	// (PIPELINE §1, issue #224).
	ExcludeSelected bool
	// ExcludeExpired drops opportunities whose response deadline has already passed
	// (relative to Now), so the Opportunities board never shows dead solicitations a
	// tester can no longer bid. Opportunities with no deadline are kept (we can't know
	// they're closed). It requires a non-zero Now; with the zero value it is a no-op.
	ExcludeExpired bool
}

// OpportunityRow is the view-model for a single row in the dashboard list view.
// It carries only the fields the list view needs; the full Opportunity is
// available via Service.Get for the detail page.
type OpportunityRow struct {
	// ID is the SAM.gov notice ID (store key).
	ID string
	// Title is the opportunity title.
	Title string
	// Agency is the issuing agency.
	Agency string
	// NAICSCode is the primary NAICS code.
	NAICSCode string
	// SolicitationNum is the SAM.gov solicitation number (shown as "SOL#").
	SolicitationNum string
	// Score is the bid/no-bid score (0.0–1.0), zero if not yet scored.
	Score float64
	// ReasoningSnippet is the Scorer's reasoning text.
	ReasoningSnippet string
	// Recommendation is the scorer recommendation ("BID", "REVIEW", "NO_BID"),
	// empty when not yet scored.
	Recommendation string
	// Stage is the derived pipeline stage.
	Stage Stage
	// ProposalStatus is the raw Opportunity.ProposalStatus. It is the single
	// source of truth for the proposal's exact pipeline position; the command
	// view derives its card state from this so it can never contradict the
	// workspace (issue #246 B2). Empty until the opportunity is selected.
	ProposalStatus string
	// ResponseDeadline is the proposal due date (zero if not set).
	ResponseDeadline time.Time
	// LastUpdated is the last store update timestamp.
	LastUpdated time.Time
	// DeadlineSoon is true when ResponseDeadline is upcoming and within 7 days of Now.
	DeadlineSoon bool
	// CreatedAt is when the opportunity was first saved (drives the
	// "NEW TODAY" day grouping on the Triage screen).
	CreatedAt time.Time
}

// Service provides read-only dashboard views over a store.Store.
// It never calls Store.Save or Store.Delete; it is safe to use with a read-only
// store proxy.
type Service struct {
	store store.Store
}

// NewService returns a Service backed by the given store.
func NewService(s store.Store) *Service {
	return &Service{store: s}
}

// List loads opportunities from the store, applies stage and score filters,
// and returns sorted OpportunityRows.
//
// Score filtering is delegated to store.Filter (store-side, efficient).
// Stage filtering is applied in Go after retrieval because Stage is derived
// from field values and is not a stored field.
func (svc *Service) List(ctx context.Context, opts ListOptions) ([]OpportunityRow, error) { //nolint:gocritic // ListOptions is a small read-only options struct; value semantics are intentional
	opps, err := svc.store.List(ctx, &store.Filter{MinScore: opts.MinScore})
	if err != nil {
		return nil, fmt.Errorf("dashboard list: %w", err)
	}

	rows := make([]OpportunityRow, 0, len(opps))
	for _, opp := range opps {
		if opts.ExcludeSelected && opp.Selected {
			continue
		}
		if opts.ExcludeExpired && isExpired(opp, opts.Now) {
			continue
		}
		stage := DeriveStage(opp)
		if opts.Stage != nil && stage != *opts.Stage {
			continue
		}
		if opts.Recommendation != "" && opp.Recommendation != opts.Recommendation {
			continue
		}
		rows = append(rows, toRow(opp, stage, opts.Now))
	}

	switch opts.SortBy {
	case SortByScore:
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].Score > rows[j].Score
		})
	default: // SortByDeadline and zero value
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].ResponseDeadline.Before(rows[j].ResponseDeadline)
		})
	}

	return rows, nil
}

// isExpired reports whether an opportunity's response deadline has passed relative to
// now. An opportunity with no deadline (zero time) is never treated as expired — we
// can't know it's closed. A zero now disables the check (callers gate on ExcludeExpired
// + a real Now), so this never silently drops everything when Now is unset.
func isExpired(opp *opportunity.Opportunity, now time.Time) bool {
	if now.IsZero() || opp.ResponseDeadline.IsZero() {
		return false
	}
	return opp.ResponseDeadline.Before(now)
}

// CountsByStage returns a map of stage counts across all opportunities in the store.
func (svc *Service) CountsByStage(ctx context.Context) (map[Stage]int, error) {
	opps, err := svc.store.List(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("dashboard counts: %w", err)
	}
	return CountByStage(opps), nil
}

// Get returns the full Opportunity for the detail page.
// It reads through the store interface without mutation.
func (svc *Service) Get(ctx context.Context, id string) (*opportunity.Opportunity, error) {
	opp, err := svc.store.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("dashboard get %s: %w", id, err)
	}
	return opp, nil
}

// CountStages returns the count of opportunities per derived Stage for all stored
// opportunities. Used by the list handler to build stage summary cards.
func (svc *Service) CountStages(ctx context.Context) (map[Stage]int, error) {
	ptrs, err := svc.store.List(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("dashboard count stages: %w", err)
	}
	return CountByStage(ptrs), nil
}

// toRow converts an Opportunity and its derived Stage to an OpportunityRow.
func toRow(opp *opportunity.Opportunity, stage Stage, now time.Time) OpportunityRow {
	return OpportunityRow{
		ID:               opp.ID,
		Title:            opp.Title,
		Agency:           opp.Agency,
		NAICSCode:        opp.NAICSCode,
		SolicitationNum:  opp.SolicitationNum,
		Score:            opp.Score,
		ReasoningSnippet: opp.ScoreReasoning,
		Recommendation:   opp.Recommendation,
		Stage:            stage,
		ProposalStatus:   opp.ProposalStatus,
		ResponseDeadline: opp.ResponseDeadline,
		LastUpdated:      opp.UpdatedAt,
		CreatedAt:        opp.CreatedAt,
		DeadlineSoon:     isDeadlineSoon(opp.ResponseDeadline, now),
	}
}

// isDeadlineSoon returns true when the deadline is upcoming and within
// deadlineSoonWindow (7 days) of now. Returns false when either time is zero,
// or when the deadline has already passed.
func isDeadlineSoon(deadline, now time.Time) bool {
	if now.IsZero() || deadline.IsZero() {
		return false
	}
	if deadline.Before(now) {
		return false
	}
	return deadline.Sub(now) <= deadlineSoonWindow
}

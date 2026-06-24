package dashboard_test

import (
	"context"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/store"
)

// newTestStore creates a JSON-backed store in a temp dir for testing.
func newTestStore(t *testing.T) store.Store {
	t.Helper()
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}
	return s
}

// stagePtr is a convenience helper for building stage filter pointers.
func stagePtr(s dashboard.Stage) *dashboard.Stage { return &s }

// baseOpp builds an Opportunity with only the fields the test needs set.
func baseOpp(id string, score float64, scoredAt *time.Time) *opportunity.Opportunity {
	return &opportunity.Opportunity{
		ID:        id,
		Title:     "Title " + id,
		Agency:    "Agency " + id,
		NAICSCode: "541511",
		Score:     score,
		ScoredAt:  scoredAt,
	}
}

func TestService_List_NoFilters_ReturnsAll(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)

	if err := s.Save(ctx, baseOpp("a", 0.8, &now)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save(ctx, baseOpp("b", 0, nil)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	svc := dashboard.NewService(s)
	rows, err := svc.List(ctx, dashboard.ListOptions{Now: now})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

// TestService_List_ExcludeSelected proves the self-cleaning Triage queue
// (issue #224, PIPELINE §1): an opportunity disappears from the Opportunities
// tab the moment it is pursued. With ExcludeSelected set, a pursued
// (opp.Selected) opportunity is dropped; the un-pursued one remains. Without
// the option, both are returned (the pipeline-count path still sees pursued).
func TestService_List_ExcludeSelected(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)

	queued := baseOpp("queued", 0.8, &now) // un-pursued, stays in the queue
	pursued := baseOpp("pursued", 0.9, &now)
	pursued.Selected = true // a human pursued it -> must leave the queue

	for _, opp := range []*opportunity.Opportunity{queued, pursued} {
		if err := s.Save(ctx, opp); err != nil {
			t.Fatalf("Save %s: %v", opp.ID, err)
		}
	}

	svc := dashboard.NewService(s)

	// ExcludeSelected drops the pursued opportunity.
	rows, err := svc.List(ctx, dashboard.ListOptions{Now: now, ExcludeSelected: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ExcludeSelected: expected 1 row, got %d", len(rows))
	}
	if rows[0].ID != "queued" {
		t.Errorf("ExcludeSelected kept %q, want the un-pursued \"queued\"", rows[0].ID)
	}

	// Without it, both are returned (pipeline counts still need pursued opps).
	all, err := svc.List(ctx, dashboard.ListOptions{Now: now})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("no ExcludeSelected: expected 2 rows, got %d", len(all))
	}
}

// TestService_List_ExcludeExpired proves the Opportunities board drops solicitations
// whose response deadline has passed (relative to Now), keeps future ones, and keeps
// opportunities with no deadline (we can't know they're closed). Without the option, all
// are returned.
func TestService_List_ExcludeExpired(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)

	past := baseOpp("past", 0.8, &now)
	past.ResponseDeadline = now.Add(-24 * time.Hour) // deadline already passed
	future := baseOpp("future", 0.8, &now)
	future.ResponseDeadline = now.Add(72 * time.Hour) // still biddable
	noDeadline := baseOpp("nodeadline", 0.8, &now)    // zero deadline → never expired

	for _, opp := range []*opportunity.Opportunity{past, future, noDeadline} {
		if err := s.Save(ctx, opp); err != nil {
			t.Fatalf("Save %s: %v", opp.ID, err)
		}
	}
	svc := dashboard.NewService(s)

	rows, err := svc.List(ctx, dashboard.ListOptions{Now: now, ExcludeExpired: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.ID] = true
	}
	if got["past"] {
		t.Error("ExcludeExpired kept a past-deadline opportunity")
	}
	if !got["future"] || !got["nodeadline"] {
		t.Errorf("ExcludeExpired dropped a biddable/undated opportunity: got %v", got)
	}

	// Without the option, all three are returned.
	all, err := svc.List(ctx, dashboard.ListOptions{Now: now})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("no ExcludeExpired: expected 3 rows, got %d", len(all))
	}
}

func TestService_List_StageFilter(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		filter  dashboard.Stage
		wantIDs []string
	}{
		{
			name:    "hunted: ScoredAt nil",
			filter:  dashboard.StageHunted,
			wantIDs: []string{"hunted"},
		},
		{
			name:    "scored: ScoredAt set",
			filter:  dashboard.StageScored,
			wantIDs: []string{"scored"},
		},
		// TODO(phase-1): test StageSelected when that stage is introduced.
		// TODO(phase-2): test StageInProposal, StageAwaitingHumanReview.
		// TODO(phase-3): test StageFinalized.
	}

	// Seed one opportunity for each Phase 0 stage.
	s := newTestStore(t)
	seed := []*opportunity.Opportunity{
		{ID: "hunted", Score: 0, ScoredAt: nil},
		{ID: "scored", Score: 0.7, ScoredAt: &now},
	}
	for _, opp := range seed {
		if err := s.Save(ctx, opp); err != nil {
			t.Fatalf("Save %s: %v", opp.ID, err)
		}
	}

	svc := dashboard.NewService(s)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := svc.List(ctx, dashboard.ListOptions{Stage: stagePtr(tc.filter), Now: now})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(rows) != len(tc.wantIDs) {
				t.Fatalf("expected %d rows, got %d", len(tc.wantIDs), len(rows))
			}
			for i, want := range tc.wantIDs {
				if rows[i].ID != want {
					t.Errorf("rows[%d].ID = %s, want %s", i, rows[i].ID, want)
				}
			}
		})
	}
}

func TestService_List_MinScoreFilter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)

	if err := s.Save(ctx, baseOpp("hi", 0.9, &now)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save(ctx, baseOpp("mid", 0.5, &now)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save(ctx, baseOpp("lo", 0.2, &now)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	svc := dashboard.NewService(s)
	rows, err := svc.List(ctx, dashboard.ListOptions{MinScore: 0.5, Now: now})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows with score >= 0.5, got %d", len(rows))
	}
	for _, r := range rows {
		if r.Score < 0.5 {
			t.Errorf("row %s has score %f below MinScore 0.5", r.ID, r.Score)
		}
	}
}

func TestService_List_SortByDeadline(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	base := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)

	opps := []*opportunity.Opportunity{
		{ID: "late", Score: 0.5, ScoredAt: &base, ResponseDeadline: base.AddDate(0, 0, 30)},
		{ID: "early", Score: 0.7, ScoredAt: &base, ResponseDeadline: base.AddDate(0, 0, 5)},
		{ID: "mid", Score: 0.4, ScoredAt: &base, ResponseDeadline: base.AddDate(0, 0, 15)},
	}
	for _, o := range opps {
		if err := s.Save(ctx, o); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	svc := dashboard.NewService(s)
	rows, err := svc.List(ctx, dashboard.ListOptions{SortBy: dashboard.SortByDeadline, Now: base})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	order := []string{"early", "mid", "late"}
	for i, want := range order {
		if rows[i].ID != want {
			t.Errorf("rows[%d].ID = %s, want %s (deadline sort ascending)", i, rows[i].ID, want)
		}
	}
}

func TestService_List_SortByScore(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	base := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)

	if err := s.Save(ctx, baseOpp("mid", 0.5, &base)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save(ctx, baseOpp("high", 0.9, &base)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Save(ctx, baseOpp("low", 0.2, &base)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	svc := dashboard.NewService(s)
	rows, err := svc.List(ctx, dashboard.ListOptions{SortBy: dashboard.SortByScore, Now: base})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	order := []string{"high", "mid", "low"}
	for i, want := range order {
		if rows[i].ID != want {
			t.Errorf("rows[%d].ID = %s, want %s (score sort descending)", i, rows[i].ID, want)
		}
	}
}

func TestService_List_DeadlineSoon_Boundaries(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	scored := now

	tests := []struct {
		name     string
		deadline time.Time
		want     bool
	}{
		{
			name:     "exactly 7 days is soon",
			deadline: now.Add(7 * 24 * time.Hour),
			want:     true,
		},
		{
			name:     "8 days is not soon",
			deadline: now.Add(8 * 24 * time.Hour),
			want:     false,
		},
		{
			name:     "past due is not soon",
			deadline: now.Add(-24 * time.Hour),
			want:     false,
		},
		{
			name:     "1 day is soon",
			deadline: now.Add(24 * time.Hour),
			want:     true,
		},
		{
			name:     "zero deadline is not soon",
			deadline: time.Time{},
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			opp := baseOpp("x", 0.5, &scored)
			opp.ResponseDeadline = tc.deadline
			if err := s.Save(ctx, opp); err != nil {
				t.Fatalf("Save: %v", err)
			}

			svc := dashboard.NewService(s)
			rows, err := svc.List(ctx, dashboard.ListOptions{Now: now})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(rows) != 1 {
				t.Fatalf("expected 1 row, got %d", len(rows))
			}
			if rows[0].DeadlineSoon != tc.want {
				t.Errorf("DeadlineSoon = %v, want %v (deadline=%v, now=%v)",
					rows[0].DeadlineSoon, tc.want, tc.deadline, now)
			}
		})
	}
}

func TestService_List_RowFieldsPopulated(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	deadline := now.AddDate(0, 0, 3)

	opp := &opportunity.Opportunity{
		ID:               "row1",
		Title:            "Important Contract",
		Agency:           "Department of Defense",
		NAICSCode:        "541330",
		SolicitationNum:  "W912-24-R-0042",
		Score:            0.85,
		ScoreReasoning:   "Strong past performance and relevant experience.",
		Recommendation:   "BID",
		ScoredAt:         &now,
		ResponseDeadline: deadline,
		UpdatedAt:        now,
	}
	if err := s.Save(ctx, opp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	svc := dashboard.NewService(s)
	rows, err := svc.List(ctx, dashboard.ListOptions{Now: now})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	r := rows[0]
	if r.ID != "row1" {
		t.Errorf("ID: got %s, want row1", r.ID)
	}
	if r.Title != "Important Contract" {
		t.Errorf("Title: got %s, want Important Contract", r.Title)
	}
	if r.Agency != "Department of Defense" {
		t.Errorf("Agency: got %s", r.Agency)
	}
	if r.NAICSCode != "541330" {
		t.Errorf("NAICSCode: got %s", r.NAICSCode)
	}
	if r.SolicitationNum != "W912-24-R-0042" {
		t.Errorf("SolicitationNum: got %s", r.SolicitationNum)
	}
	if r.Recommendation != "BID" {
		t.Errorf("Recommendation: got %s, want BID", r.Recommendation)
	}
	if r.Score != 0.85 {
		t.Errorf("Score: got %f, want 0.85", r.Score)
	}
	if r.ReasoningSnippet != "Strong past performance and relevant experience." {
		t.Errorf("ReasoningSnippet: got %q", r.ReasoningSnippet)
	}
	if r.Stage != dashboard.StageScored {
		t.Errorf("Stage: got %v, want StageScored", r.Stage)
	}
	if !r.ResponseDeadline.Equal(deadline) {
		t.Errorf("ResponseDeadline: got %v, want %v", r.ResponseDeadline, deadline)
	}
	if !r.LastUpdated.Equal(now) {
		t.Errorf("LastUpdated: got %v, want %v", r.LastUpdated, now)
	}
	if !r.DeadlineSoon {
		t.Error("DeadlineSoon should be true for 3-day deadline")
	}
}

func TestService_Get(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)

	if err := s.Save(ctx, baseOpp("abc", 0.8, &now)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	svc := dashboard.NewService(s)

	t.Run("found returns opportunity", func(t *testing.T) {
		got, err := svc.Get(ctx, "abc")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ID != "abc" {
			t.Errorf("ID: got %s, want abc", got.ID)
		}
	})

	t.Run("not found returns error", func(t *testing.T) {
		_, err := svc.Get(ctx, "no-such-id")
		if err == nil {
			t.Fatal("expected error for nonexistent ID, got nil")
		}
	})
}

// TestListFilterByRecommendation covers the segmented filter contract from
// issue #150: All / To pursue (BID) / Needs review (REVIEW).
func TestListFilterByRecommendation(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	seed := []opportunity.Opportunity{
		{ID: "r-bid", Title: "Bid Opp", Recommendation: "BID", Score: 0.9, ScoredAt: &now, UpdatedAt: now},
		{ID: "r-rev", Title: "Review Opp", Recommendation: "REVIEW", Score: 0.5, ScoredAt: &now, UpdatedAt: now},
		{ID: "r-no", Title: "NoBid Opp", Recommendation: "NO_BID", Score: 0.1, ScoredAt: &now, UpdatedAt: now},
		{ID: "r-raw", Title: "Unscored Opp", UpdatedAt: now},
	}
	for i := range seed {
		if err := s.Save(ctx, &seed[i]); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	svc := dashboard.NewService(s)

	bid, err := svc.List(ctx, dashboard.ListOptions{Recommendation: "BID"})
	if err != nil {
		t.Fatalf("List(BID): %v", err)
	}
	if len(bid) != 1 || bid[0].ID != "r-bid" {
		t.Errorf("List(BID) = %v, want only r-bid", bid)
	}

	review, err := svc.List(ctx, dashboard.ListOptions{Recommendation: "REVIEW"})
	if err != nil {
		t.Fatalf("List(REVIEW): %v", err)
	}
	if len(review) != 1 || review[0].ID != "r-rev" {
		t.Errorf("List(REVIEW) = %v, want only r-rev", review)
	}

	all, err := svc.List(ctx, dashboard.ListOptions{})
	if err != nil {
		t.Fatalf("List(all): %v", err)
	}
	if len(all) != 4 {
		t.Errorf("List(all) returned %d rows, want 4 (empty filter is no filter)", len(all))
	}
}

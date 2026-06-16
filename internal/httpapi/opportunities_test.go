package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/store"
)

// seedTestServer builds a Server backed by a real JSON store seeded with three
// opportunities at known scores, deadlines, and stages. It returns the wired
// handler plus the fixed "now" the deterministic deadline/age math uses.
func seedTestServer(t *testing.T) (http.Handler, time.Time) {
	t.Helper()
	ctx := context.Background()
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	// Seed dates are RELATIVE to the real clock because the server computes derived
	// fields like DeadlineSoon from time.Now() (no clock is injected on this path). A
	// hardcoded date rots: once the real date passes the seeded "soon" deadline,
	// deadline_soon flips to false and TestListRowFields fails for no real reason.
	now := time.Now().UTC()
	scored := now.Add(-24 * time.Hour)

	opps := []*opportunity.Opportunity{
		{
			ID:               "soon-1",
			Title:            "Deadline Soon",
			Agency:           "Agency A",
			NAICSCode:        "541512",
			Score:            0.9,
			ScoredAt:         &scored,
			Recommendation:   "BID",
			EstimatedValue:   2_000_000,
			PostedDate:       now.Add(-48 * time.Hour),
			ResponseDeadline: now.Add(2 * 24 * time.Hour),
			CreatedAt:        scored,
			UpdatedAt:        scored,
		},
		{
			ID:               "late-2",
			Title:            "Deadline Late",
			Agency:           "Agency B",
			NAICSCode:        "541330",
			Score:            0.5,
			ScoredAt:         &scored,
			Recommendation:   "REVIEW",
			PostedDate:       now.Add(-72 * time.Hour),
			ResponseDeadline: now.Add(20 * 24 * time.Hour),
			CreatedAt:        scored,
			UpdatedAt:        scored,
		},
		{
			ID:        "hunted-3",
			Title:     "Not Scored",
			Agency:    "Agency C",
			NAICSCode: "236220",
			Score:     0,
			ScoredAt:  nil, // stays in the Hunted stage
			CreatedAt: scored,
			UpdatedAt: scored,
		},
	}
	for _, opp := range opps {
		if err := s.Save(ctx, opp); err != nil {
			t.Fatalf("seed %s: %v", opp.ID, err)
		}
	}

	// Read-endpoint tests run without OAuth; opt in to the insecure no-auth path so
	// Routes() builds instead of failing closed (the production default).
	srv := New(Deps{Dashboard: dashboard.NewService(s), AllowInsecureNoAuth: true})
	return srv.Routes(), now
}

// decodeList decodes a list response body, failing the test on a non-200 status
// or undecodable body.
func decodeList(t *testing.T, rec *httptest.ResponseRecorder) listResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	var got listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode list body %q: %v", rec.Body.String(), err)
	}
	return got
}

func doGet(h http.Handler, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestListReturnsSeededRows confirms the unfiltered list returns every seeded
// opportunity with a matching count, defaulting to deadline-ascending order.
func TestListReturnsSeededRows(t *testing.T) {
	h, _ := seedTestServer(t)

	got := decodeList(t, doGet(h, "/api/v1/opportunities"))

	if got.Count != 3 {
		t.Fatalf("count = %d, want 3", got.Count)
	}
	if len(got.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(got.Rows))
	}
	// Default sort is by deadline ascending: the Hunted row (zero deadline)
	// sorts first, then soon, then late.
	wantOrder := []string{"hunted-3", "soon-1", "late-2"}
	for i, id := range wantOrder {
		if got.Rows[i].ID != id {
			t.Errorf("default order[%d] = %q, want %q (full: %v)", i, got.Rows[i].ID, id, idsOf(got.Rows))
		}
	}
}

// TestListSortByScore confirms sort=score orders rows by descending fit score.
func TestListSortByScore(t *testing.T) {
	h, _ := seedTestServer(t)

	got := decodeList(t, doGet(h, "/api/v1/opportunities?sort=score"))

	wantOrder := []string{"soon-1", "late-2", "hunted-3"}
	for i, id := range wantOrder {
		if got.Rows[i].ID != id {
			t.Errorf("score order[%d] = %q, want %q (full: %v)", i, got.Rows[i].ID, id, idsOf(got.Rows))
		}
	}
}

// TestListFilterByStage confirms stage=Scored drops the unscored (Hunted) row.
func TestListFilterByStage(t *testing.T) {
	h, _ := seedTestServer(t)

	got := decodeList(t, doGet(h, "/api/v1/opportunities?stage=Scored"))

	if got.Count != 2 {
		t.Fatalf("count = %d, want 2 (%v)", got.Count, idsOf(got.Rows))
	}
	for _, r := range got.Rows {
		if r.Stage != string(dashboard.StageScored) {
			t.Errorf("row %s stage = %q, want Scored", r.ID, r.Stage)
		}
		if r.ID == "hunted-3" {
			t.Errorf("hunted-3 should be filtered out by stage=Scored")
		}
	}
}

// TestListFilterByMinScore confirms minScore drops rows below the threshold.
func TestListFilterByMinScore(t *testing.T) {
	h, _ := seedTestServer(t)

	got := decodeList(t, doGet(h, "/api/v1/opportunities?minScore=0.8"))

	if got.Count != 1 {
		t.Fatalf("count = %d, want 1 (%v)", got.Count, idsOf(got.Rows))
	}
	if got.Rows[0].ID != "soon-1" {
		t.Errorf("row = %q, want soon-1", got.Rows[0].ID)
	}
}

// TestListFilterByRecommendation confirms rec=BID keeps only BID rows, matching
// the dashboard handler's recommendation segment semantics.
func TestListFilterByRecommendation(t *testing.T) {
	h, _ := seedTestServer(t)

	got := decodeList(t, doGet(h, "/api/v1/opportunities?rec=BID"))

	if got.Count != 1 || got.Rows[0].ID != "soon-1" {
		t.Fatalf("rec=BID rows = %v, want [soon-1]", idsOf(got.Rows))
	}
}

// TestListRowFields confirms a list row carries the flattened, derived fields the
// DTO promises (score_pct, deadline_soon, recommendation, stage string).
func TestListRowFields(t *testing.T) {
	h, _ := seedTestServer(t)

	got := decodeList(t, doGet(h, "/api/v1/opportunities?sort=score"))
	soon := got.Rows[0]
	if soon.ID != "soon-1" {
		t.Fatalf("first row = %q, want soon-1", soon.ID)
	}
	if soon.ScorePct != 90 {
		t.Errorf("score_pct = %d, want 90", soon.ScorePct)
	}
	if !soon.DeadlineSoon {
		t.Errorf("deadline_soon = false, want true for a 2-day deadline")
	}
	if soon.Stage != string(dashboard.StageScored) {
		t.Errorf("stage = %q, want Scored", soon.Stage)
	}
	if soon.Recommendation != "BID" {
		t.Errorf("recommendation = %q, want BID", soon.Recommendation)
	}
}

// TestListZeroDeadlineOmitted confirms the `omitzero` tag actually drops a zero
// ResponseDeadline from the JSON. With plain `omitempty` a zero time.Time would
// serialize as "0001-01-01T00:00:00Z" (a struct value is never "empty"), which
// clients would parse as a real ancient date; `omitzero` removes the key entirely.
// The unscored "hunted-3" row has no deadline, so its row object must not carry a
// response_deadline key at all.
func TestListZeroDeadlineOmitted(t *testing.T) {
	h, _ := seedTestServer(t)

	rec := doGet(h, "/api/v1/opportunities")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	// Decode into a generic shape so we can assert on key presence, not just the
	// zero value a typed decode would silently produce.
	var envelope struct {
		Rows []map[string]json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode list body %q: %v", rec.Body.String(), err)
	}

	var hunted map[string]json.RawMessage
	for _, row := range envelope.Rows {
		var id string
		if err := json.Unmarshal(row["id"], &id); err == nil && id == "hunted-3" {
			hunted = row
			break
		}
	}
	if hunted == nil {
		t.Fatalf("hunted-3 row not found in list response %s", rec.Body.String())
	}
	if raw, ok := hunted["response_deadline"]; ok {
		t.Errorf("response_deadline present for zero deadline (= %s), want key omitted via omitzero", raw)
	}
}

// TestGetReturnsOpportunity confirms the detail endpoint returns the full record
// plus the derived stage and score percentage.
func TestGetReturnsOpportunity(t *testing.T) {
	h, _ := seedTestServer(t)

	rec := doGet(h, "/api/v1/opportunities/soon-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var got OpportunityDetailDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode detail %q: %v", rec.Body.String(), err)
	}
	if got.Opportunity == nil || got.ID != "soon-1" {
		t.Fatalf("opportunity = %+v, want id soon-1", got.Opportunity)
	}
	if got.DerivedStage != string(dashboard.StageScored) {
		t.Errorf("derived_stage = %q, want Scored", got.DerivedStage)
	}
	if got.ScorePct != 90 {
		t.Errorf("score_pct = %d, want 90", got.ScorePct)
	}
}

// TestGetUnknownIDReturns404 confirms a well-formed but absent id returns a JSON
// 404 (mapped from store.ErrNotFound), not a 500.
func TestGetUnknownIDReturns404(t *testing.T) {
	h, _ := seedTestServer(t)

	rec := doGet(h, "/api/v1/opportunities/does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
	assertJSONError(t, rec)
}

// TestGetMalformedIDReturns400 confirms an id that fails ValidOpportunityID is
// rejected with a JSON 400 before the store is ever consulted.
func TestGetMalformedIDReturns400(t *testing.T) {
	h, _ := seedTestServer(t)

	// A space is rejected by the shared validator. (URL-encoded so the request
	// line is well-formed but the decoded path value is malformed.)
	rec := doGet(h, "/api/v1/opportunities/bad%20id")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	assertJSONError(t, rec)
}

// TestStageCounts confirms the counts endpoint returns the per-stage tally.
func TestStageCounts(t *testing.T) {
	h, _ := seedTestServer(t)

	rec := doGet(h, "/api/v1/stages/counts")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var got stageCountsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode counts %q: %v", rec.Body.String(), err)
	}
	if got.Counts[string(dashboard.StageScored)] != 2 {
		t.Errorf("Scored count = %d, want 2 (%v)", got.Counts[string(dashboard.StageScored)], got.Counts)
	}
	if got.Counts[string(dashboard.StageHunted)] != 1 {
		t.Errorf("Hunted count = %d, want 1 (%v)", got.Counts[string(dashboard.StageHunted)], got.Counts)
	}
}

// assertJSONError fails unless the response carries the API's JSON error envelope
// with a non-empty message and the JSON content type.
func assertJSONError(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	var got ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode error envelope %q: %v", rec.Body.String(), err)
	}
	if got.Error == "" {
		t.Errorf("error envelope = %+v, want non-empty message", got)
	}
}

// idsOf extracts the IDs from a list of rows for readable failure messages.
func idsOf(rows []OpportunityDTO) []string {
	ids := make([]string, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	return ids
}

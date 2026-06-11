package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Mawar2/Kaimi/internal/dashboard"
	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/proposal"
	"github.com/Mawar2/Kaimi/internal/store"
)

// fakeProposals is a hand-written double for the ProposalService interface so the
// handler tests stay fast and deterministic (no agents, no background work). It
// records calls and returns scripted results, letting each test drive the exact
// Select/Document outcome the handler must translate.
type fakeProposals struct {
	selectErr   error
	statusAfter string // status persisted on a successful Select
	store       store.Store
	docs        map[string]*document.Document
	docErrs     map[string]error
	selectedIDs []string
}

func (f *fakeProposals) Select(ctx context.Context, oppID string) error {
	if f.selectErr != nil {
		return f.selectErr
	}
	f.selectedIDs = append(f.selectedIDs, oppID)
	// Mirror the real service: persist Selected + the resulting ProposalStatus so
	// the handler's read-back returns a meaningful status.
	if f.store != nil {
		opp, err := f.store.Get(ctx, oppID)
		if err != nil {
			return err
		}
		opp.Selected = true
		opp.ProposalStatus = f.statusAfter
		if err := f.store.Save(ctx, opp); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeProposals) Document(oppID string) (*document.Document, error) {
	if err := f.docErrs[oppID]; err != nil {
		return nil, err
	}
	return f.docs[oppID], nil
}

// seedProposalServer builds a Server with a real JSON store (so the opportunity
// existence checks exercise store.ErrNotFound exactly as B2 does) plus an injected
// fake proposal service. A nil fp yields a read-only API (Deps.Proposals nil path).
func seedProposalServer(t *testing.T, fp ProposalService) http.Handler {
	t.Helper()
	ctx := context.Background()
	s, err := store.NewJSONStore(t.TempDir())
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	opps := []*opportunity.Opportunity{
		{ID: "opp-1", Title: "Unselected", Agency: "Agency A", NAICSCode: "541512", CreatedAt: now, UpdatedAt: now},
		{ID: "opp-2", Title: "Selected", Agency: "Agency B", NAICSCode: "541330", Selected: true, ProposalStatus: proposal.StatusGate, CreatedAt: now, UpdatedAt: now},
	}
	for _, opp := range opps {
		if err := s.Save(ctx, opp); err != nil {
			t.Fatalf("seed %s: %v", opp.ID, err)
		}
	}
	// Give the fake the store so its Select persists status like the real service.
	if f, ok := fp.(*fakeProposals); ok {
		f.store = s
	}
	srv := New(Deps{Dashboard: dashboard.NewService(s), Proposals: fp})
	return srv.Routes()
}

func doPost(h http.Handler, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestSelectUnselectedReturns202 confirms selecting an unselected opportunity
// returns 202 Accepted with the resulting proposal status in the JSON body.
func TestSelectUnselectedReturns202(t *testing.T) {
	fp := &fakeProposals{statusAfter: proposal.StatusOutlineRunning}
	h := seedProposalServer(t, fp)

	rec := doPost(h, "/api/v1/opportunities/opp-1/select")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body = %s", rec.Code, rec.Body.String())
	}
	var got SelectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode select body %q: %v", rec.Body.String(), err)
	}
	if got.ID != "opp-1" {
		t.Errorf("id = %q, want opp-1", got.ID)
	}
	if got.Status != proposal.StatusOutlineRunning {
		t.Errorf("status = %q, want %q", got.Status, proposal.StatusOutlineRunning)
	}
	if len(fp.selectedIDs) != 1 || fp.selectedIDs[0] != "opp-1" {
		t.Errorf("Select calls = %v, want [opp-1]", fp.selectedIDs)
	}
}

// TestSelectAlreadySelectedReturns409 confirms a second select (mapped from
// proposal.ErrAlreadySelected) returns 409 Conflict.
func TestSelectAlreadySelectedReturns409(t *testing.T) {
	fp := &fakeProposals{selectErr: proposal.ErrAlreadySelected}
	h := seedProposalServer(t, fp)

	rec := doPost(h, "/api/v1/opportunities/opp-2/select")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
	assertJSONError(t, rec)
}

// TestSelectStageRunningReturns409 confirms an in-flight stage (mapped from
// proposal.ErrStageRunning) also returns 409 Conflict.
func TestSelectStageRunningReturns409(t *testing.T) {
	fp := &fakeProposals{selectErr: proposal.ErrStageRunning}
	h := seedProposalServer(t, fp)

	rec := doPost(h, "/api/v1/opportunities/opp-1/select")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
	assertJSONError(t, rec)
}

// TestSelectUnknownIDReturns404 confirms selecting a well-formed but absent id
// returns 404 (the existence check runs before the proposal service is called).
func TestSelectUnknownIDReturns404(t *testing.T) {
	fp := &fakeProposals{}
	h := seedProposalServer(t, fp)

	rec := doPost(h, "/api/v1/opportunities/does-not-exist/select")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
	assertJSONError(t, rec)
	if len(fp.selectedIDs) != 0 {
		t.Errorf("Select should not be called for an unknown id; got %v", fp.selectedIDs)
	}
}

// TestSelectMalformedIDReturns400 confirms an id failing ValidOpportunityID is
// rejected with 400 before any store or service call.
func TestSelectMalformedIDReturns400(t *testing.T) {
	fp := &fakeProposals{}
	h := seedProposalServer(t, fp)

	rec := doPost(h, "/api/v1/opportunities/bad%20id/select")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	assertJSONError(t, rec)
}

// TestSelectNilProposalsReturns503 confirms a read-only deployment (no proposal
// service wired) returns 503 Service Unavailable.
func TestSelectNilProposalsReturns503(t *testing.T) {
	h := seedProposalServer(t, nil)

	rec := doPost(h, "/api/v1/opportunities/opp-1/select")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body = %s", rec.Code, rec.Body.String())
	}
	assertJSONError(t, rec)
}

// TestGetProposalComposes confirms the proposal-status endpoint composes the
// opportunity stage/status with the drafted document.
func TestGetProposalComposes(t *testing.T) {
	doc := &document.Document{
		OpportunityID: "opp-2",
		Title:         "Draft",
		Sections:      []document.Section{{ID: "s1", Heading: "Intro", Body: "Hello"}},
		Version:       3,
	}
	fp := &fakeProposals{docs: map[string]*document.Document{"opp-2": doc}}
	h := seedProposalServer(t, fp)

	rec := doGet(h, "/api/v1/proposals/opp-2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var got ProposalStatusDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode proposal status %q: %v", rec.Body.String(), err)
	}
	if got.Status != proposal.StatusGate {
		t.Errorf("status = %q, want %q", got.Status, proposal.StatusGate)
	}
	if got.State != "human" {
		t.Errorf("state = %q, want human (the gate)", got.State)
	}
	if got.Stage != string(dashboard.StageAwaitingHumanReview) {
		t.Errorf("stage = %q, want %q", got.Stage, dashboard.StageAwaitingHumanReview)
	}
	if got.Document == nil || got.Document.Version != 3 {
		t.Fatalf("document = %+v, want version 3", got.Document)
	}
}

// TestGetProposalNoDocument confirms the endpoint succeeds with a nil document
// when the draft does not exist yet (an opportunity selected but not yet drafted,
// or one never selected): the document field is simply absent.
func TestGetProposalNoDocument(t *testing.T) {
	fp := &fakeProposals{docErrs: map[string]error{"opp-1": errors.New("document not found")}}
	h := seedProposalServer(t, fp)

	rec := doGet(h, "/api/v1/proposals/opp-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if _, ok := raw["document"]; ok {
		t.Errorf("document key present for an undrafted proposal, want omitted")
	}
}

// TestGetProposalUnknownIDReturns404 confirms an absent opportunity returns 404.
func TestGetProposalUnknownIDReturns404(t *testing.T) {
	fp := &fakeProposals{}
	h := seedProposalServer(t, fp)

	rec := doGet(h, "/api/v1/proposals/does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
	assertJSONError(t, rec)
}

// TestGetProposalMalformedIDReturns400 confirms a malformed id is rejected 400.
func TestGetProposalMalformedIDReturns400(t *testing.T) {
	fp := &fakeProposals{}
	h := seedProposalServer(t, fp)

	rec := doGet(h, "/api/v1/proposals/bad%20id")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
	assertJSONError(t, rec)
}

// TestGetProposalNilProposalsReturns503 confirms a read-only deployment returns
// 503 for the proposal-status endpoint.
func TestGetProposalNilProposalsReturns503(t *testing.T) {
	h := seedProposalServer(t, nil)

	rec := doGet(h, "/api/v1/proposals/opp-2")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body = %s", rec.Code, rec.Body.String())
	}
	assertJSONError(t, rec)
}

// TestRealServiceSatisfiesInterface is a compile-time guard that the real
// *proposal.Service satisfies the small interface the handlers depend on. The
// package-level `var _ ProposalService = (*proposal.Service)(nil)` in proposals.go
// asserts the same thing; this keeps the assertion visible alongside the suite.
func TestRealServiceSatisfiesInterface(t *testing.T) {
	var _ ProposalService = (*proposal.Service)(nil)
}

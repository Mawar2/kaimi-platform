package kobs_test

// This end-to-end test is the Definition-of-Done evidence for wiring the
// telemetry pipeline into a live proposal lifecycle (ticket T2.2). It proves
// that REAL proposal.Service events flow through the kobs bridge, the async
// emitter, and a fan-out sink all the way to BOTH durable JSONL on disk and the
// in-memory live stream rendered as Server-Sent Events.
//
// It lives in package kobs_test (external) so it may import internal/proposal,
// which itself imports internal/kobs — an external test package breaks what
// would otherwise be an import cycle.

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Mawar2/kaimi-telemetry/emit"
	"github.com/Mawar2/kaimi-telemetry/event"
	"github.com/Mawar2/kaimi-telemetry/httpstream"
	"github.com/Mawar2/kaimi-telemetry/sink"

	"github.com/Mawar2/Kaimi/internal/agent"
	"github.com/Mawar2/Kaimi/internal/document"
	"github.com/Mawar2/Kaimi/internal/finalreview"
	"github.com/Mawar2/Kaimi/internal/kobs"
	"github.com/Mawar2/Kaimi/internal/opportunity"
	"github.com/Mawar2/Kaimi/internal/outline"
	"github.com/Mawar2/Kaimi/internal/proposal"
	"github.com/Mawar2/Kaimi/internal/scorer"
	"github.com/Mawar2/Kaimi/internal/store"
	"github.com/Mawar2/Kaimi/internal/writer"
)

// fakeOutline returns a fixed two-section outline, so the e2e test exercises the
// real proposal.Service drafting path without depending on the live Google Docs
// client (which the real outline.Agent writes to). Adapted from the unexported
// fakes in proposal_test.go, which a separate test package cannot reach.
type fakeOutline struct{}

func (fakeOutline) Run(_ context.Context, opp *opportunity.Opportunity, _ map[string]string) (*outline.Outline, *agent.Result, error) {
	out := &outline.Outline{
		OpportunityID: opp.ID,
		Title:         opp.Title,
		Sections: []outline.Section{
			{ID: "technical_approach", Title: "Technical Approach", Required: true},
			{ID: "management_approach", Title: "Management Approach", Required: true},
		},
	}
	return out, &agent.Result{AgentName: "outline", Status: agent.StatusSuccess, CompletedAt: time.Now()}, nil
}

// fakeWriter drafts each single-section outline the Service hands it in the
// "\n## Title\n body" shape the Service's splitDraft expects, so the drafted
// body lands on the document and a proposal.section.updated event is emitted.
type fakeWriter struct{}

func (fakeWriter) Run(_ context.Context, in writer.Input) (string, *agent.Result, error) {
	title := ""
	if in.Outline != nil && len(in.Outline.Sections) > 0 {
		title = in.Outline.Sections[0].Title
	}
	draft := "\n## " + title + "\nDrafted body for " + title + "\n"
	return draft, &agent.Result{AgentName: "writer", Status: agent.StatusSuccess, CompletedAt: time.Now()}, nil
}

// readyReviewer always returns ready_to_submit, so the gated lifecycle reaches a
// clean Final Review and a human Submit is valid.
type readyReviewer struct{}

func (readyReviewer) Review(_ context.Context, _ finalreview.Input) (*agent.Result, error) {
	return &agent.Result{AgentName: "final-review", Status: agent.StatusReadyToSubmit, CompletedAt: time.Now()}, nil
}

// TestEndToEndProposalTelemetry_JSONLAndSSE wires the real telemetry pipeline —
// kobs.Init(emit.New(sink.Multi{JSONL, Live}, …)) — drives a real
// proposal.Service through its full gated lifecycle (Select → draft → gate →
// Approve → Final Review → Submit), then proves the same ordered lifecycle
// events landed BOTH in the durable JSONL log and on the live SSE stream, each
// carrying the proposal's trace_id (the opportunity ID) and tenant_id.
func TestEndToEndProposalTelemetry_JSONLAndSSE(t *testing.T) {
	const (
		oppID  = "zta-1"
		tenant = "bluemeta"
	)

	// ---- Wire the real telemetry pipeline: emit -> Multi{JSONL, Live}. ----
	dir := t.TempDir()
	telDir := filepath.Join(dir, "telemetry")
	if err := os.MkdirAll(telDir, 0o755); err != nil {
		t.Fatalf("create telemetry dir: %v", err)
	}
	jsonlPath := filepath.Join(telDir, "events.jsonl")
	jsonl, err := sink.NewJSONLSink(jsonlPath)
	if err != nil {
		t.Fatalf("jsonl sink: %v", err)
	}
	live := sink.NewLiveSink(500, 512)

	em := emit.New(sink.Multi{jsonl, live}, emit.Config{})
	kobs.Init(em)
	t.Cleanup(func() {
		// Clear the process-wide handle so this test never leaks its emitter into
		// another, then drain+close the sinks.
		kobs.Init(nil)
		_ = em.Shutdown(context.Background())
	})

	// ---- Build a REAL proposal.Service with deterministic fakes. ----
	opps, err := store.NewJSONStore(dir)
	if err != nil {
		t.Fatalf("opportunity store: %v", err)
	}
	docs, err := document.NewStore(dir)
	if err != nil {
		t.Fatalf("document store: %v", err)
	}
	svc := proposal.NewService(&proposal.Deps{
		Opportunities: opps,
		Documents:     docs,
		Outline:       fakeOutline{},
		Writer:        fakeWriter{},
		Review:        readyReviewer{},
		Profile:       &scorer.CapabilityProfile{},
	})

	now := time.Now()
	opp := &opportunity.Opportunity{
		ID:               oppID,
		Title:            "Zero Trust Architecture Modernization",
		Agency:           "DHS CISA",
		NAICSCode:        "541512",
		Description:      "Modernize zero trust architecture.",
		ResponseDeadline: now.Add(30 * 24 * time.Hour),
		Requirements:     []string{"FedRAMP High"},
		TenantID:         tenant,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := opps.Save(context.Background(), opp); err != nil {
		t.Fatalf("seed opportunity: %v", err)
	}

	// ---- Drive the full lifecycle. Each background stage is awaited with Wait
	// so the emit order is the lifecycle order. ----
	ctx := context.Background()
	if err := svc.Select(ctx, oppID); err != nil {
		t.Fatalf("Select: %v", err)
	}
	svc.Wait()
	if err := svc.Approve(ctx, oppID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	svc.Wait()
	if err := svc.Submit(ctx, oppID); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Flush the emitter so every buffered event has reached BOTH sinks (the JSONL
	// buffer is also fsynced to disk) before we read them back.
	if err := em.Flush(context.Background()); err != nil {
		t.Fatalf("flush emitter: %v", err)
	}

	want := []string{
		"proposal.selected",
		"proposal.outline.started",
		"proposal.outline.completed",
		"proposal.writer.started",
		"proposal.section.updated",
		"proposal.writer.completed",
		"proposal.gate.reached",
		"proposal.approved",
		"proposal.finalreview.started",
		"proposal.finalreview.completed",
		"proposal.submitted",
	}

	// ---- (1) Assert the durable JSONL log. ----
	rawLines, jsonlEvents := readJSONL(t, jsonlPath)
	if len(jsonlEvents) == 0 {
		t.Fatal("no events in the JSONL log")
	}
	assertTraceAndTenant(t, "jsonl", jsonlEvents, oppID, tenant)
	if got := collapseLifecycle(names(jsonlEvents)); !reflect.DeepEqual(got, want) {
		t.Fatalf("JSONL lifecycle order:\n got  %v\n want %v", got, want)
	}

	// Demonstration dump: the actual JSONL lines that flowed end to end.
	t.Logf("=== %d telemetry events written to %s ===", len(rawLines), jsonlPath)
	for _, line := range rawLines {
		t.Log(line)
	}

	// ---- (2) Assert the SAME events arrive live over SSE. ----
	// Render the LiveSink through the real SSE handler. A pre-cancelled request
	// context makes the replay-then-tail handler return as soon as it has written
	// the retained frames, so the assertion is deterministic without sleeps.
	handler := httpstream.NewHandler(live)
	streamCtx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/v1/events/stream", http.NoBody).WithContext(streamCtx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("SSE Content-Type = %q, want text/event-stream", ct)
	}
	sseEvents := parseSSE(t, rec.Body.String())
	if len(sseEvents) == 0 {
		t.Fatal("no events streamed over SSE")
	}
	assertTraceAndTenant(t, "sse", sseEvents, oppID, tenant)
	if got := collapseLifecycle(names(sseEvents)); !reflect.DeepEqual(got, want) {
		t.Fatalf("SSE lifecycle order:\n got  %v\n want %v", got, want)
	}
}

// readJSONL reads the JSONL event log, returning both the raw lines (for the
// demonstration dump) and the decoded events.
func readJSONL(t *testing.T, path string) ([]string, []event.Event) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read jsonl log: %v", err)
	}
	var raw []string
	var events []event.Event
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev event.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("unmarshal jsonl line %q: %v", line, err)
		}
		raw = append(raw, line)
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan jsonl log: %v", err)
	}
	return raw, events
}

// parseSSE reconstructs the events from an SSE response body by decoding each
// frame's data lines back into an event.Event.
func parseSSE(t *testing.T, body string) []event.Event {
	t.Helper()
	var events []event.Event
	for _, frame := range strings.Split(body, "\n\n") {
		var dataLines []string
		for _, line := range strings.Split(frame, "\n") {
			if d, ok := strings.CutPrefix(line, "data: "); ok {
				dataLines = append(dataLines, d)
			}
		}
		if len(dataLines) == 0 {
			continue
		}
		var ev event.Event
		if err := json.Unmarshal([]byte(strings.Join(dataLines, "\n")), &ev); err != nil {
			t.Fatalf("unmarshal SSE frame data %q: %v", strings.Join(dataLines, "\n"), err)
		}
		events = append(events, ev)
	}
	return events
}

// names returns the ordered event names.
func names(events []event.Event) []string {
	out := make([]string, len(events))
	for i := range events {
		out[i] = events[i].Name
	}
	return out
}

// collapseLifecycle collapses consecutive proposal.section.updated repeats into a
// single entry, so the ordered backbone is stable regardless of how many sections
// the outline produced.
func collapseLifecycle(in []string) []string {
	var out []string
	for _, n := range in {
		if n == "proposal.section.updated" && len(out) > 0 && out[len(out)-1] == n {
			continue
		}
		out = append(out, n)
	}
	return out
}

// assertTraceAndTenant checks every event rides the proposal's trace_id (the
// opportunity ID) and tenant_id.
func assertTraceAndTenant(t *testing.T, label string, events []event.Event, oppID, tenant string) {
	t.Helper()
	for _, e := range events {
		if e.TraceID != oppID {
			t.Errorf("%s: event %q TraceID = %q, want %q", label, e.Name, e.TraceID, oppID)
		}
		if e.TenantID != tenant {
			t.Errorf("%s: event %q TenantID = %q, want %q", label, e.Name, e.TenantID, tenant)
		}
	}
}

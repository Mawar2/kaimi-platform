package event

import (
	"encoding/json"
	"reflect"
	"regexp"
	"testing"
	"time"
)

func TestSchemaVersionIsOne(t *testing.T) {
	if SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", SchemaVersion)
	}
}

func TestNewEventPopulatesEnvelope(t *testing.T) {
	before := time.Now().UTC()
	ev := NewEvent(CategoryLLM, "gemini.generate",
		Usage("tokens", 1280),
		Content("prompt", "draft the executive summary"),
	)
	after := time.Now().UTC()

	if ev.Category != CategoryLLM {
		t.Errorf("Category = %q, want %q", ev.Category, CategoryLLM)
	}
	if ev.Name != "gemini.generate" {
		t.Errorf("Name = %q, want %q", ev.Name, "gemini.generate")
	}
	if ev.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", ev.SchemaVersion, SchemaVersion)
	}

	// EventID must be 32 lowercase hex chars (16 random bytes).
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(ev.EventID) {
		t.Errorf("EventID = %q, want 32 hex chars", ev.EventID)
	}

	// OccurredAt must be UTC and inside the call window.
	if ev.OccurredAt.Location() != time.UTC {
		t.Errorf("OccurredAt location = %v, want UTC", ev.OccurredAt.Location())
	}
	if ev.OccurredAt.Before(before) || ev.OccurredAt.After(after) {
		t.Errorf("OccurredAt = %v, want within [%v, %v]", ev.OccurredAt, before, after)
	}

	if len(ev.Attributes) != 2 {
		t.Fatalf("len(Attributes) = %d, want 2", len(ev.Attributes))
	}
}

func TestNewEventUniqueIDs(t *testing.T) {
	a := NewEvent(CategorySystem, "boot")
	b := NewEvent(CategorySystem, "boot")
	if a.EventID == b.EventID {
		t.Fatalf("EventID collision: %q", a.EventID)
	}
}

func TestUsageAndContentClass(t *testing.T) {
	u := Usage("tokens", 42)
	if u.Class != ClassUsage {
		t.Errorf("Usage class = %d, want %d (ClassUsage)", u.Class, ClassUsage)
	}
	if u.Key != "tokens" || u.Value != 42 {
		t.Errorf("Usage = %+v, want key=tokens value=42", u)
	}

	c := Content("prompt", "secret text")
	if c.Class != ClassContent {
		t.Errorf("Content class = %d, want %d (ClassContent)", c.Class, ClassContent)
	}
	if c.Key != "prompt" || c.Value != "secret text" {
		t.Errorf("Content = %+v, want key=prompt value=secret text", c)
	}
}

func TestClassConstants(t *testing.T) {
	if ClassUsage != 0 {
		t.Errorf("ClassUsage = %d, want 0", ClassUsage)
	}
	if ClassContent != 1 {
		t.Errorf("ClassContent = %d, want 1", ClassContent)
	}
}

func TestAttrsGet(t *testing.T) {
	attrs := Attrs{
		Usage("tokens", 100),
		Content("prompt", "hello"),
	}

	v, ok := attrs.Get("tokens")
	if !ok {
		t.Fatal("Get(tokens) ok = false, want true")
	}
	if v != 100 {
		t.Errorf("Get(tokens) = %v, want 100", v)
	}

	v, ok = attrs.Get("prompt")
	if !ok || v != "hello" {
		t.Errorf("Get(prompt) = (%v, %v), want (hello, true)", v, ok)
	}

	_, ok = attrs.Get("missing")
	if ok {
		t.Error("Get(missing) ok = true, want false")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	original := Event{
		EventID:       "0123456789abcdef0123456789abcdef",
		SchemaVersion: SchemaVersion,
		OccurredAt:    time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC),
		TenantID:      "bluemeta",
		Category:      CategoryProposal,
		Name:          "section.drafted",
		Level:         LevelInfo,
		Actor:         Actor{Kind: "agent", ID: "writer-1", Name: "Writer"},
		TraceID:       "trace-abc",
		SpanID:        "span-1",
		ParentSpanID:  "span-0",
		DurationMS:    1500,
		Attributes: Attrs{
			Usage("tokens", float64(1280)),
			Content("prompt", "draft it"),
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !reflect.DeepEqual(original, got) {
		t.Errorf("round-trip mismatch:\n original = %+v\n got      = %+v", original, got)
	}
}

func TestJSONOmitsEmptyOptionalFields(t *testing.T) {
	ev := Event{
		EventID:       "id",
		SchemaVersion: SchemaVersion,
		OccurredAt:    time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC),
		Category:      CategorySystem,
		Name:          "boot",
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	for _, omitted := range []string{"tenant_id", "level", "trace_id", "span_id", "parent_span_id", "duration_ms", "attributes", "actor"} {
		if regexp.MustCompile(`"` + omitted + `"`).MatchString(s) {
			t.Errorf("expected %q to be omitted from JSON, got: %s", omitted, s)
		}
	}
}

func TestCategoryConstants(t *testing.T) {
	cases := map[Category]string{
		CategoryJourney:  "journey",
		CategorySystem:   "system",
		CategoryProposal: "proposal",
		CategoryLLM:      "llm",
	}
	for c, want := range cases {
		if string(c) != want {
			t.Errorf("category %v = %q, want %q", c, string(c), want)
		}
	}
}

package kobs

import (
	"testing"
	"time"

	"google.golang.org/genai"

	"github.com/Mawar2/kaimi-telemetry/event"
)

// attrValue returns the value of the named attribute on ev, failing the test if
// it is absent. It keeps the assertions below terse.
func attrValue(t *testing.T, ev event.Event, key string) any { //nolint:gocritic // Event passed by value; test helper mirrors the by-value sink contract
	t.Helper()
	v, ok := event.Attrs(ev.Attributes).Get(key)
	if !ok {
		t.Fatalf("event %q missing attribute %q", ev.Name, key)
	}
	return v
}

// TestBuildLLMCompletedEvent feeds fake responses with known UsageMetadata and
// FinishReason and asserts the emitted usage attributes, including the derived
// truncated flag.
func TestBuildLLMCompletedEvent(t *testing.T) {
	tests := []struct {
		name           string
		model          string
		resp           *genai.GenerateContentResponse
		wantInput      int
		wantOutput     int
		wantThinking   int
		wantTotal      int
		wantFinish     string
		wantTruncated  bool
		wantResponse   string
		wantCostNonNil bool // true when the model is priced, so cost must be > 0
	}{
		{
			name:  "normal stop",
			model: "gemini-2.5-pro",
			resp: &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						FinishReason: genai.FinishReasonStop,
						Content: &genai.Content{
							Parts: []*genai.Part{{Text: "the drafted section"}},
						},
					},
				},
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:     1000,
					CandidatesTokenCount: 500,
					ThoughtsTokenCount:   200,
					TotalTokenCount:      1700,
				},
			},
			wantInput:      1000,
			wantOutput:     500,
			wantThinking:   200,
			wantTotal:      1700,
			wantFinish:     "STOP",
			wantTruncated:  false,
			wantResponse:   "the drafted section",
			wantCostNonNil: true,
		},
		{
			name:  "truncated at max tokens",
			model: "gemini-3.5-flash",
			resp: &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						FinishReason: genai.FinishReasonMaxTokens,
						Content: &genai.Content{
							Parts: []*genai.Part{{Text: "partial"}},
						},
					},
				},
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:     2048,
					CandidatesTokenCount: 8192,
					ThoughtsTokenCount:   4096,
					TotalTokenCount:      10240,
				},
			},
			wantInput:      2048,
			wantOutput:     8192,
			wantThinking:   4096,
			wantTotal:      10240,
			wantFinish:     "MAX_TOKENS",
			wantTruncated:  true,
			wantResponse:   "partial",
			wantCostNonNil: true,
		},
		{
			name:  "blocked response with nil usage metadata does not panic",
			model: "gemini-2.5-pro",
			resp: &genai.GenerateContentResponse{
				// No candidates, no usage metadata: the safety-blocked shape.
				Candidates:    nil,
				UsageMetadata: nil,
			},
			wantInput:      0,
			wantOutput:     0,
			wantThinking:   0,
			wantTotal:      0,
			wantFinish:     "",
			wantTruncated:  false,
			wantResponse:   "",
			wantCostNonNil: false,
		},
		{
			name:           "nil response does not panic",
			model:          "unknown-model",
			resp:           nil,
			wantInput:      0,
			wantOutput:     0,
			wantThinking:   0,
			wantTotal:      0,
			wantFinish:     "",
			wantTruncated:  false,
			wantResponse:   "",
			wantCostNonNil: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev := buildLLMCompletedEvent(tc.model, tc.resp, 1234*time.Millisecond)

			if ev.Name != EventLLMRequestCompleted {
				t.Errorf("Name = %q, want %q", ev.Name, EventLLMRequestCompleted)
			}
			if ev.Category != event.CategoryLLM {
				t.Errorf("Category = %q, want %q", ev.Category, event.CategoryLLM)
			}
			if got := attrValue(t, ev, AttrLLMModel); got != tc.model {
				t.Errorf("model = %v, want %v", got, tc.model)
			}
			if got := attrValue(t, ev, AttrLLMInputTokens); got != tc.wantInput {
				t.Errorf("input_tokens = %v, want %v", got, tc.wantInput)
			}
			if got := attrValue(t, ev, AttrLLMOutputTokens); got != tc.wantOutput {
				t.Errorf("output_tokens = %v, want %v", got, tc.wantOutput)
			}
			if got := attrValue(t, ev, AttrLLMThinkingTokens); got != tc.wantThinking {
				t.Errorf("thinking_tokens = %v, want %v", got, tc.wantThinking)
			}
			if got := attrValue(t, ev, AttrLLMTotalTokens); got != tc.wantTotal {
				t.Errorf("total_tokens = %v, want %v", got, tc.wantTotal)
			}
			if got := attrValue(t, ev, AttrLLMFinishReason); got != tc.wantFinish {
				t.Errorf("finish_reason = %v, want %v", got, tc.wantFinish)
			}
			if got := attrValue(t, ev, AttrLLMTruncated); got != tc.wantTruncated {
				t.Errorf("truncated = %v, want %v", got, tc.wantTruncated)
			}
			if got := attrValue(t, ev, AttrLLMResponse); got != tc.wantResponse {
				t.Errorf("response = %v, want %v", got, tc.wantResponse)
			}
			if got := attrValue(t, ev, AttrLLMLatencyMS); got != int64(1234) {
				t.Errorf("latency_ms = %v, want 1234", got)
			}

			// cost_usd is always present; it is > 0 only for priced models.
			cost, ok := attrValue(t, ev, AttrLLMCostUSD).(float64)
			if !ok {
				t.Fatalf("cost_usd is not a float64: %T", attrValue(t, ev, AttrLLMCostUSD))
			}
			if tc.wantCostNonNil && cost <= 0 {
				t.Errorf("cost_usd = %v, want > 0 for a priced model", cost)
			}
			if !tc.wantCostNonNil && cost != 0 {
				t.Errorf("cost_usd = %v, want 0 for an unpriced/empty call", cost)
			}

			// Every usage attribute must carry the forwardable usage class and the
			// response payload must be content-class.
			for _, key := range []string{
				AttrLLMModel, AttrLLMInputTokens, AttrLLMOutputTokens,
				AttrLLMThinkingTokens, AttrLLMTotalTokens, AttrLLMFinishReason,
				AttrLLMTruncated, AttrLLMLatencyMS, AttrLLMCostUSD,
			} {
				if class := attrClass(t, ev, key); class != event.ClassUsage {
					t.Errorf("attr %q class = %d, want ClassUsage", key, class)
				}
			}
			if class := attrClass(t, ev, AttrLLMResponse); class != event.ClassContent {
				t.Errorf("attr %q class = %d, want ClassContent", AttrLLMResponse, class)
			}
		})
	}
}

// attrClass returns the redaction class of the named attribute on ev.
func attrClass(t *testing.T, ev event.Event, key string) event.Class { //nolint:gocritic // Event passed by value; test helper mirrors the by-value sink contract
	t.Helper()
	for _, a := range ev.Attributes {
		if a.Key == key {
			return a.Class
		}
	}
	t.Fatalf("event %q missing attribute %q", ev.Name, key)
	return 0
}

// TestBuildLLMStartedEvent asserts the started event carries the model, config,
// and the concatenated prompt (contents + system instruction) as content.
func TestBuildLLMStartedEvent(t *testing.T) {
	temp := float32(0.2)
	budget := int32(2048)
	cfg := &genai.GenerateContentConfig{
		Temperature:       &temp,
		MaxOutputTokens:   8192,
		ThinkingConfig:    &genai.ThinkingConfig{ThinkingBudget: &budget},
		SystemInstruction: genai.NewContentFromText("you are a planner", genai.RoleUser),
	}
	contents := []*genai.Content{
		genai.NewContentFromText("plan the sections", genai.RoleUser),
	}

	ev := buildLLMStartedEvent("gemini-2.5-pro", contents, cfg)

	if ev.Name != EventLLMRequestStarted {
		t.Errorf("Name = %q, want %q", ev.Name, EventLLMRequestStarted)
	}
	if got := attrValue(t, ev, AttrLLMModel); got != "gemini-2.5-pro" {
		t.Errorf("model = %v, want gemini-2.5-pro", got)
	}
	if got := attrValue(t, ev, AttrLLMMaxOutputTokens); got != int32(8192) {
		t.Errorf("max_output_tokens = %v, want 8192", got)
	}
	if got := attrValue(t, ev, AttrLLMThinkingBudget); got != int32(2048) {
		t.Errorf("thinking_budget = %v, want 2048", got)
	}
	if got := attrValue(t, ev, AttrLLMTemperature); got != float32(0.2) {
		t.Errorf("temperature = %v, want 0.2", got)
	}

	wantPrompt := "plan the sections\nyou are a planner"
	if got := attrValue(t, ev, AttrLLMPrompt); got != wantPrompt {
		t.Errorf("prompt = %q, want %q", got, wantPrompt)
	}
	if class := attrClass(t, ev, AttrLLMPrompt); class != event.ClassContent {
		t.Errorf("prompt class = %d, want ClassContent", class)
	}
}

// TestBuildLLMStartedEventNilConfig confirms a nil config and nil parts are
// tolerated: the model and an empty prompt are emitted, and the optional config
// attributes are simply omitted, with no panic.
func TestBuildLLMStartedEventNilConfig(t *testing.T) {
	ev := buildLLMStartedEvent("gemini-2.5-pro", nil, nil)

	if got := attrValue(t, ev, AttrLLMModel); got != "gemini-2.5-pro" {
		t.Errorf("model = %v, want gemini-2.5-pro", got)
	}
	if got := attrValue(t, ev, AttrLLMPrompt); got != "" {
		t.Errorf("prompt = %q, want empty", got)
	}
	if _, ok := event.Attrs(ev.Attributes).Get(AttrLLMMaxOutputTokens); ok {
		t.Errorf("max_output_tokens should be omitted when config is nil")
	}
	if _, ok := event.Attrs(ev.Attributes).Get(AttrLLMTemperature); ok {
		t.Errorf("temperature should be omitted when config is nil")
	}
}

// TestEstimateCostUSD covers the price-map lookup, prefix matching for versioned
// IDs, and the zero fallback for unknown models.
func TestEstimateCostUSD(t *testing.T) {
	// gemini-2.5-pro: 1000/1e6*1.25 + 500/1e6*10 = 0.00125 + 0.005 = 0.00625
	if got := estimateCostUSD("gemini-2.5-pro", 1000, 500); got != 0.00625 {
		t.Errorf("estimateCostUSD(2.5-pro) = %v, want 0.00625", got)
	}
	// Prefix match: a versioned ID resolves to the family price.
	if got := estimateCostUSD("gemini-2.5-pro-preview-06-05", 1000, 500); got != 0.00625 {
		t.Errorf("estimateCostUSD(versioned 2.5-pro) = %v, want 0.00625", got)
	}
	// Unknown model: zero estimate, never a wrong one.
	if got := estimateCostUSD("some-other-model", 1000, 500); got != 0 {
		t.Errorf("estimateCostUSD(unknown) = %v, want 0", got)
	}
}

// TestGenerateContentEmitsTwoEventsThroughCapture confirms the emit path: the
// started/completed pair flows to an installed emitter. The underlying call is
// not exercised here (it needs a live client); the event builders are unit-tested
// above. This guards that both emits reach the sink with the right names.
func TestLLMEventPairThroughCapture(t *testing.T) {
	capture, restore := NewCapture()
	defer restore()

	Emit(buildLLMStartedEvent("gemini-2.5-pro", nil, nil))
	Emit(buildLLMCompletedEvent("gemini-2.5-pro", nil, 5*time.Millisecond))

	events := capture.Drain()
	if len(events) != 2 {
		t.Fatalf("captured %d events, want 2", len(events))
	}
	if events[0].Name != EventLLMRequestStarted {
		t.Errorf("first event = %q, want %q", events[0].Name, EventLLMRequestStarted)
	}
	if events[1].Name != EventLLMRequestCompleted {
		t.Errorf("second event = %q, want %q", events[1].Name, EventLLMRequestCompleted)
	}
}

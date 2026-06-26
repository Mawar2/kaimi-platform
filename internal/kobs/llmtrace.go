package kobs

import (
	"context"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/Mawar2/kaimi-telemetry/event"
)

// LLM telemetry event names. They are the opaque, host-supplied names stamped on
// the CategoryLLM events this wrapper emits, kept here as constants so producers
// and any downstream renderer agree on spelling.
const (
	// EventLLMRequestStarted is emitted just before a generation call is issued.
	EventLLMRequestStarted = "llm.request.started"
	// EventLLMRequestCompleted is emitted after the call returns, whether it
	// succeeded or failed.
	EventLLMRequestCompleted = "llm.request.completed"
)

// GenerateContent wraps client.Models.GenerateContent with telemetry. It is the
// "what the model is thinking" signal: it emits an llm.request.started event with
// the prompt and call configuration, then the underlying call, then an
// llm.request.completed event with token usage, finish reason, latency, and a
// best-effort cost estimate.
//
// The wrapper is strictly additive: it returns the SAME response and error as the
// underlying call, never altering behavior or control flow. Emitting is a no-op
// until an emitter is installed (see Emit), so call sites can adopt it freely. A
// nil response (the call errored) or nil usage metadata (a blocked response) is
// tolerated without panicking.
func GenerateContent(ctx context.Context, client *genai.Client, model string, contents []*genai.Content, cfg *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	Emit(buildLLMStartedEvent(model, contents, cfg))
	// Failover detection: if a prior call on this same operation (context) failed
	// against a different model, this call is the fallover and emits one
	// llm.fallback.triggered. Purely observational — it never alters the call.
	noteFallbackStart(ctx, model)

	start := time.Now()
	resp, err := client.Models.GenerateContent(ctx, model, contents, cfg)
	latency := time.Since(start)

	Emit(buildLLMCompletedEvent(model, resp, latency))
	// Record this call's outcome so a following different-model call can recognize
	// a failover; a success clears any pending failure for this operation.
	noteFallbackResult(ctx, model, err)

	// Return the underlying values unchanged so downstream Candidates/Text logic
	// at the call site is untouched.
	return resp, err
}

// buildLLMStartedEvent builds the llm.request.started event from the call inputs.
// The model and configuration are usage-class; the assembled prompt is
// content-class so the redaction gate keeps it inside the deployment. Temperature
// and thinking budget are nil-guarded and only attached when configured.
func buildLLMStartedEvent(model string, contents []*genai.Content, cfg *genai.GenerateContentConfig) event.Event {
	attrs := []event.Attr{
		LLMModel(model),
		LLMPrompt(promptText(contents, cfg)),
	}

	if cfg != nil {
		attrs = append(attrs, LLMMaxOutputTokens(cfg.MaxOutputTokens))
		if cfg.ThinkingConfig != nil && cfg.ThinkingConfig.ThinkingBudget != nil {
			attrs = append(attrs, LLMThinkingBudget(*cfg.ThinkingConfig.ThinkingBudget))
		}
		if cfg.Temperature != nil {
			attrs = append(attrs, LLMTemperature(*cfg.Temperature))
		}
	}

	return LLMEvent(EventLLMRequestStarted, "", attrs...)
}

// buildLLMCompletedEvent builds the llm.request.completed event from the response
// and measured latency. Usage metadata is nil-guarded to zeros (a safety-blocked
// response carries none), Candidates is guarded before reading the finish reason,
// and the response text is read through the nil-safe Text helper. cost_usd is a
// DERIVED best-effort estimate from a static price map.
func buildLLMCompletedEvent(model string, resp *genai.GenerateContentResponse, latency time.Duration) event.Event {
	var inputTokens, outputTokens, thinkingTokens, totalTokens int
	var finishReason string
	var responseText string

	if resp != nil {
		if um := resp.UsageMetadata; um != nil {
			inputTokens = int(um.PromptTokenCount)
			outputTokens = int(um.CandidatesTokenCount)
			thinkingTokens = int(um.ThoughtsTokenCount)
			totalTokens = int(um.TotalTokenCount)
		}
		if len(resp.Candidates) > 0 {
			finishReason = string(resp.Candidates[0].FinishReason)
		}
		responseText = resp.Text()
	}

	truncated := finishReason == string(genai.FinishReasonMaxTokens)
	costUSD := estimateCostUSD(model, inputTokens, outputTokens)

	return LLMEvent(EventLLMRequestCompleted, "",
		LLMModel(model),
		LLMInputTokens(inputTokens),
		LLMOutputTokens(outputTokens),
		LLMThinkingTokens(thinkingTokens),
		LLMTotalTokens(totalTokens),
		LLMFinishReason(finishReason),
		LLMTruncated(truncated),
		LLMLatencyMS(latency.Milliseconds()),
		LLMCostUSD(costUSD),
		LLMResponse(responseText),
	)
}

// promptText concatenates the text parts of contents and the configured system
// instruction into a single prompt string for the content-class prompt attribute.
// Non-text parts (inline data, function calls) are skipped; the system
// instruction is appended last so it mirrors how the model receives it.
func promptText(contents []*genai.Content, cfg *genai.GenerateContentConfig) string {
	var sb strings.Builder
	writeContentText(&sb, contents)
	if cfg != nil && cfg.SystemInstruction != nil {
		writeContentText(&sb, []*genai.Content{cfg.SystemInstruction})
	}
	return sb.String()
}

// writeContentText appends every non-empty text part of contents to sb, one per
// line, guarding nil contents and nil parts.
func writeContentText(sb *strings.Builder, contents []*genai.Content) {
	for _, c := range contents {
		if c == nil {
			continue
		}
		for _, p := range c.Parts {
			if p == nil || p.Text == "" {
				continue
			}
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(p.Text)
		}
	}
}

// modelPrice is the per-million-token price, in USD, for one model's input and
// output tokens. Values are approximate published Vertex AI list prices used only
// to DERIVE a best-effort cost estimate — they are not billed figures and may lag
// price changes.
type modelPrice struct {
	inputPerM  float64
	outputPerM float64
}

// modelPrices maps a model-name prefix to its approximate price. A prefix match
// is used so versioned IDs (e.g. gemini-2.5-pro-preview) resolve to the family
// price. Unknown models yield a zero estimate rather than a wrong one.
//
// DERIVED, best-effort: keep this list short and clearly approximate. It exists
// so a cost signal is present, not to be an authoritative ledger.
var modelPrices = map[string]modelPrice{
	"gemini-2.5-pro":   {inputPerM: 1.25, outputPerM: 10.00},
	"gemini-2.5-flash": {inputPerM: 0.30, outputPerM: 2.50},
	"gemini-3.5-flash": {inputPerM: 0.30, outputPerM: 2.50},
	"gemini-3-pro":     {inputPerM: 1.25, outputPerM: 10.00},
}

// estimateCostUSD returns a DERIVED, best-effort USD cost for the call from the
// static price map, or 0 when the model is unknown. The estimate is
// input_tokens × input price + output_tokens × output price (output includes any
// thinking tokens, which the API already counts in CandidatesTokenCount).
func estimateCostUSD(model string, inputTokens, outputTokens int) float64 {
	price, ok := modelPrices[model]
	if !ok {
		for prefix, p := range modelPrices {
			if strings.HasPrefix(model, prefix) {
				price = p
				ok = true
				break
			}
		}
	}
	if !ok {
		return 0
	}
	const perMillion = 1_000_000.0
	return float64(inputTokens)/perMillion*price.inputPerM +
		float64(outputTokens)/perMillion*price.outputPerM
}

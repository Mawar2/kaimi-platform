package kobs

import "github.com/Mawar2/kaimi-telemetry/event"

// Attribute-key catalog.
//
// These constants are the single source of truth for the attribute keys Kaimi
// emits, so producers and any downstream renderer/query agree on spelling. Keys
// are namespaced by concern (proposal.* for Zone-2 work-unit events, llm.* for
// model-interaction events). The matching constructor helpers below pin each key
// to its correct redaction class — content keys (prompts, responses) can only be
// built through Content* helpers, so a prompt can never leak out as usage
// metadata by mistake.
const (
	// proposal.* — Zone-2 per-proposal work-unit attributes. Identifiers,
	// stage/status/revision labels, and durations are usage-class metadata; the
	// section title is content-class because a heading can echo solicitation
	// wording, and the failure detail is content-class because it can quote
	// draft or solicitation text.
	AttrProposalID            = "proposal.id"             // store/opportunity ID for the proposal (usage)
	AttrProposalOpportunityID = "proposal.opportunity_id" // originating SAM.gov notice ID (usage)
	AttrProposalStage         = "proposal.stage"          // pipeline stage: outline | writer | finalreview (usage)
	AttrProposalStatus        = "proposal.status"         // AgentResult status: success | failed | needs_human | ready_to_submit (usage)
	AttrProposalRevision      = "proposal.revision"       // true when the writer span is a re-run after a change request (usage)
	AttrProposalSection       = "proposal.section"        // section title the stage operated on (CONTENT — never forwarded)
	AttrProposalError         = "proposal.error"          // failure detail (CONTENT — never forwarded)

	// llm.* — model-interaction attributes. The token/model/latency keys are
	// usage-class; the prompt and response payloads are content-class and must
	// never leave the deployment.
	AttrLLMModel           = "llm.model"             // model ID, e.g. gemini-2.5-pro (usage)
	AttrLLMProvider        = "llm.provider"          // backend, e.g. vertex | anthropic (usage)
	AttrLLMInputTokens     = "llm.input_tokens"      // prompt token count (usage)
	AttrLLMOutputTokens    = "llm.output_tokens"     // completion token count (usage)
	AttrLLMThinkingTokens  = "llm.thinking_tokens"   // tokens spent on hidden reasoning (usage)
	AttrLLMTotalTokens     = "llm.total_tokens"      // total token count (usage)
	AttrLLMFinishReason    = "llm.finish_reason"     // why generation stopped, e.g. stop | max_tokens (usage)
	AttrLLMTruncated       = "llm.truncated"         // true when generation hit the output cap (usage)
	AttrLLMLatencyMS       = "llm.latency_ms"        // wall-clock latency of the call in ms (usage)
	AttrLLMCostUSD         = "llm.cost_usd"          // DERIVED best-effort USD cost estimate (usage)
	AttrLLMMaxOutputTokens = "llm.max_output_tokens" // configured output-token cap (usage)
	AttrLLMThinkingBudget  = "llm.thinking_budget"   // configured thinking-token budget (usage)
	AttrLLMTemperature     = "llm.temperature"       // configured sampling temperature (usage)
	AttrLLMPrompt          = "llm.prompt"            // prompt text (CONTENT — never forwarded)
	AttrLLMResponse        = "llm.response"          // response text (CONTENT — never forwarded)

	// llm.fallback.* — model failover attributes, stamped on the
	// llm.fallback.triggered event when the fallback chain fails over from one
	// model backend to a different one. The model IDs and the trigger reason are
	// usage-class: the reason is the transient backend error (throttling, 5xx,
	// timeout) that forced the failover, not draft or solicitation content.
	AttrLLMPrimaryModel   = "llm.primary_model"   // model whose call failed, forcing failover (usage)
	AttrLLMFallbackModel  = "llm.fallback_model"  // model the failover routed the next call to (usage)
	AttrLLMFallbackReason = "llm.fallback_reason" // error text that triggered the failover (usage)
)

// Agent names recorded as the Actor on proposal lifecycle spans, so a renderer
// can attribute each phase to the responsible Zone-2 agent.
const (
	// AgentOutline is Noa, who builds the document skeleton.
	AgentOutline = "Noa"
	// AgentWriter is Tomás, who drafts and revises sections.
	AgentWriter = "Tomás"
	// AgentReview is Vera, who runs the final compliance pass.
	AgentReview = "Vera"
)

// ProposalEvent builds a CategoryProposal event named name, scoped to tenantID,
// carrying attrs. Use the proposal.* helpers below to populate attrs so the keys
// and classes stay consistent.
func ProposalEvent(name, tenantID string, attrs ...event.Attr) event.Event {
	ev := event.NewEvent(event.CategoryProposal, name, attrs...)
	ev.TenantID = tenantID
	return ev
}

// LLMEvent builds a CategoryLLM event named name, scoped to tenantID, carrying
// attrs. Use the llm.* helpers below so prompt/response payloads are correctly
// classified as content and never escape the deployment.
func LLMEvent(name, tenantID string, attrs ...event.Attr) event.Event {
	ev := event.NewEvent(event.CategoryLLM, name, attrs...)
	ev.TenantID = tenantID
	return ev
}

// ProposalID returns the proposal identifier as a usage attribute.
func ProposalID(id string) event.Attr { return event.Usage(AttrProposalID, id) }

// ProposalOpportunityID returns the originating opportunity ID as a usage attribute.
func ProposalOpportunityID(id string) event.Attr {
	return event.Usage(AttrProposalOpportunityID, id)
}

// ProposalStage returns the pipeline stage as a usage attribute.
func ProposalStage(stage string) event.Attr { return event.Usage(AttrProposalStage, stage) }

// ProposalStatus returns the AgentResult status as a usage attribute.
func ProposalStatus(status string) event.Attr { return event.Usage(AttrProposalStatus, status) }

// ProposalRevision marks a writer span as a revision — a re-run after a human
// change request — as a usage attribute.
func ProposalRevision(revision bool) event.Attr {
	return event.Usage(AttrProposalRevision, revision)
}

// ProposalSection returns the section title as a CONTENT attribute, since a
// heading can echo solicitation wording, so the redaction gate keeps it inside
// the deployment.
func ProposalSection(section string) event.Attr {
	return event.Content(AttrProposalSection, section)
}

// ProposalErrorOf returns err's message as a CONTENT attribute, or an
// empty-string content attribute when err is nil. Returning an attribute either
// way lets a failed-span emit stay a single statement regardless of whether a
// concrete error value is in scope at the failure branch.
func ProposalErrorOf(err error) event.Attr {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return event.Content(AttrProposalError, msg)
}

// EmitProposal constructs and emits one CategoryProposal lifecycle event.
//
// Because a background proposal stage runs on a context deliberately severed
// from the request that spawned it, trace propagation cannot be inherited:
// traceID is therefore set explicitly to the opportunity ID so every event for
// one proposal groups under a single trace. span pairs a phase's *.started
// event with its matching *.completed/*.failed event (one span per phase) and
// is empty for point events such as proposal.selected. agent records the
// responsible Zone-2 agent (AgentOutline/AgentWriter/AgentReview) as the Actor
// and is empty for non-agent events. durationMS is the elapsed milliseconds of
// a finished span and 0 for started or point events. attrs carry the usage and
// content attributes; build them with the helpers in this package.
//
// EmitProposal is purely additive: like Emit it is a no-op until an emitter is
// installed, and it never blocks, errors, or alters the caller's control flow.
func EmitProposal(name, tenantID, traceID, span, agent string, durationMS int64, attrs ...event.Attr) {
	ev := ProposalEvent(name, tenantID, attrs...)
	ev.TraceID = traceID
	if span != "" {
		ev.SpanID = span
	}
	if agent != "" {
		ev.Actor = event.Actor{Kind: "agent", Name: agent}
	}
	if durationMS > 0 {
		ev.DurationMS = durationMS
	}
	Emit(ev)
}

// LLMModel returns the model ID as a usage attribute.
func LLMModel(model string) event.Attr { return event.Usage(AttrLLMModel, model) }

// LLMProvider returns the backend provider as a usage attribute.
func LLMProvider(provider string) event.Attr { return event.Usage(AttrLLMProvider, provider) }

// LLMInputTokens returns the prompt token count as a usage attribute.
func LLMInputTokens(n int) event.Attr { return event.Usage(AttrLLMInputTokens, n) }

// LLMOutputTokens returns the completion token count as a usage attribute.
func LLMOutputTokens(n int) event.Attr { return event.Usage(AttrLLMOutputTokens, n) }

// LLMTotalTokens returns the total token count as a usage attribute.
func LLMTotalTokens(n int) event.Attr { return event.Usage(AttrLLMTotalTokens, n) }

// LLMFinishReason returns the generation finish reason as a usage attribute.
func LLMFinishReason(reason string) event.Attr {
	return event.Usage(AttrLLMFinishReason, reason)
}

// LLMPrompt returns the prompt text as a CONTENT attribute, so the redaction
// gate keeps it inside the deployment.
func LLMPrompt(prompt string) event.Attr { return event.Content(AttrLLMPrompt, prompt) }

// LLMResponse returns the response text as a CONTENT attribute, so the redaction
// gate keeps it inside the deployment.
func LLMResponse(response string) event.Attr {
	return event.Content(AttrLLMResponse, response)
}

// LLMThinkingTokens returns the count of tokens spent on the model's hidden
// reasoning as a usage attribute.
func LLMThinkingTokens(n int) event.Attr { return event.Usage(AttrLLMThinkingTokens, n) }

// LLMTruncated returns whether the generation was cut off at the output cap
// (finish reason max_tokens) as a usage attribute.
func LLMTruncated(truncated bool) event.Attr { return event.Usage(AttrLLMTruncated, truncated) }

// LLMLatencyMS returns the wall-clock latency of the call in milliseconds as a
// usage attribute.
func LLMLatencyMS(ms int64) event.Attr { return event.Usage(AttrLLMLatencyMS, ms) }

// LLMCostUSD returns the best-effort, DERIVED USD cost estimate as a usage
// attribute. The value is computed from a static per-model price map (see
// llmtrace.go) and is approximate — it is not a billed figure.
func LLMCostUSD(usd float64) event.Attr { return event.Usage(AttrLLMCostUSD, usd) }

// LLMMaxOutputTokens returns the configured output-token cap as a usage attribute.
func LLMMaxOutputTokens(n int32) event.Attr { return event.Usage(AttrLLMMaxOutputTokens, n) }

// LLMThinkingBudget returns the configured thinking-token budget as a usage attribute.
func LLMThinkingBudget(n int32) event.Attr { return event.Usage(AttrLLMThinkingBudget, n) }

// LLMTemperature returns the configured sampling temperature as a usage attribute.
func LLMTemperature(t float32) event.Attr { return event.Usage(AttrLLMTemperature, t) }

// LLMPrimaryModel returns the failed-over-from model ID as a usage attribute.
func LLMPrimaryModel(model string) event.Attr { return event.Usage(AttrLLMPrimaryModel, model) }

// LLMFallbackModel returns the failed-over-to model ID as a usage attribute.
func LLMFallbackModel(model string) event.Attr { return event.Usage(AttrLLMFallbackModel, model) }

// LLMFallbackReason returns the failover trigger reason (the transient backend
// error text) as a usage attribute.
func LLMFallbackReason(reason string) event.Attr {
	return event.Usage(AttrLLMFallbackReason, reason)
}

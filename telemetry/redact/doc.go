// Package redact is the privacy seam of the telemetry core: it guarantees that
// sensitive content never leaves the deployment while still letting safe usage
// metadata flow to a central sink.
//
// Every attribute on an event.Event carries an event.Class. ClassContent marks
// protected payloads — prompts, responses, user text — that must stay local;
// ClassUsage marks metadata — counts, durations, model names — that is safe to
// forward. Strip turns a full event into a forwardable one by dropping every
// content attribute while preserving the envelope and all usage attributes,
// without mutating its input.
//
// Gate composes two sinks around that rule: it writes the full event to a Local
// sink and the stripped event to an optional Central sink. A nil Central sink
// means there is no egress path at all — content cannot leave because nothing
// is sent. This is the product's core guarantee: usage may travel, content
// never does.
package redact

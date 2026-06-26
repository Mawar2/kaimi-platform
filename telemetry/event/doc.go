// Package event defines the Event envelope — the single, domain-agnostic data
// structure every part of the telemetry core produces, transports, redacts,
// stores, and renders.
//
// An Event carries an opaque Name supplied by the host (the core never
// enumerates names), a coarse Category, an optional Actor, distributed-tracing
// identifiers, and a set of Attrs. Each Attr is tagged with a Class — either
// ClassUsage (counts, durations, model names; safe to forward to a central
// sink) or ClassContent (prompts, responses, user text; must never leave the
// deployment). That per-attribute class is the seam the redaction gate acts on,
// so it lives on the data itself rather than in a separate policy.
//
// The envelope is versioned by SchemaVersion so stored events remain readable
// as the shape evolves. JSON tags are stable wire names; optional fields are
// omitempty so minimal events stay small.
package event

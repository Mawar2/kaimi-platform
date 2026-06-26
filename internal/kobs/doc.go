// Package kobs is the Kaimi-specific bridge to the domain-agnostic
// kaimi-telemetry observability core.
//
// kobs is the ONLY package permitted to import both Kaimi domain types (such as
// opportunity.Opportunity) AND the core (event/emit). Every other Kaimi package
// instruments itself by calling kobs helpers, never by importing the core
// directly. This keeps the core's zero-domain-import contract intact while
// giving the rest of the codebase a small, typed vocabulary for telemetry.
//
// Two responsibilities live here:
//
//   - A process-wide emitter handle (Init / Emit) that is a safe no-op until an
//     entrypoint installs an emitter. Instrumentation sprinkled through the
//     codebase therefore never panics in tests or in binaries that have not
//     wired telemetry, and emitting is always additive — it never changes
//     behavior or control flow.
//
//   - A typed attribute catalog (the proposal.* and llm.* keys) plus
//     constructor helpers that bake in the correct event.Usage / event.Content
//     redaction classification, so callers cannot accidentally forward a prompt
//     or response as usage metadata.
package kobs

// Package proposalwiring assembles a *proposal.Service from a resolved
// *config.Config plus per-run mode flags (live writer / live review / live
// ingest). It is the single place that wires the Zone 2 agents — Outline,
// Writer, Final Review, and the optional document ingestor — in either their
// live (Gemini / Vertex AI / Document AI) or offline stub form.
//
// The construction logic was extracted out of cmd/dashboard so that a future
// cmd/api can reuse it verbatim: both entry points resolve a config.Config and
// then call proposalwiring.New to obtain a ready-to-serve proposal.Service. The
// package is deliberately a pure assembler — it makes no behavioral decisions of
// its own beyond translating the flags and config it is handed.
package proposalwiring

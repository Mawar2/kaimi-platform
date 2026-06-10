// Package proposal implements the gated Zone 2 proposal lifecycle (GitHub
// issue #155, epic #153) — the shared service both the web dashboard and
// the desktop app call to wire the REAL agents into the product:
//
//	Select        → real Outline agent builds the document skeleton,
//	                real Technical Writer drafts every section, and the
//	                pipeline PAUSES at the single human review gate
//	UpdateSection → human edits at the gate become attributed revisions
//	Approve       → real Final Review agent runs on the document exactly
//	                as the human left it (human edits are first-class
//	                content, per INTENT.md); issues land as document flags
//	RequestChanges→ the draft returns to the Writer with the human's note
//	Submit        → always a human act; agents stand down
//
// Select also runs the optional document-ingestion stage first (fetch, store,
// and extract the solicitation attachments), threading their text into the
// Outline, Writer, and Final Review stages so those agents ground on the real
// documents rather than the SAM.gov summary alone.
//
// Single source of truth (issue #174): an earlier parallel orchestrator,
// internal/manager.Manager, ran the same Outline → Writer → Final Review chain
// straight through with no human gate and was only ever used by the e2e tests.
// It was retired in favor of this gated service so the ingestion + document
// threading logic lives in exactly one place and the dashboard and the e2e tests
// exercise the same orchestration code path. The Zone 2 agent interfaces the
// stages implement (Ingestor, OutlineRunner, WriterRunner, Reviewer) now live in
// agents.go, alongside the orchestrator that consumes them.
//
// The service composes the existing agents through those interfaces, persists
// ProposalStatus on every transition so polling UIs stay truthful, and stores
// all artifacts through internal/document.
package proposal

# Goal: Finish proposal export — (a) drop `documents` scope, (b) compliance matrix

**Last updated:** 2026-06-26
**Owner loop:** autonomous (`/loop` until done) · sub-agent teams
**Done when:** both workstreams merged to `main`, deployed to pilot, and verified (gate green + browser/curl check); instance left pristine; PushNotification sent.

## Why
- **(a)** Kaimi's Drive save requests the **sensitive `documents` scope**, which forces Google OAuth verification (weeks) before the consent screen can be published. Dropping it → only `drive.file` (non-sensitive) → **publishable with zero verification**, killing the "unverified app" screen for good.
- **(b)** Federal proposals live or die on a **compliance matrix** (requirement → coverage). It's the highest-value remaining export and a real differentiator.

## Workstream A — drop the `documents` scope (refactor)
Rewrite `googledocs.liveClient.CreateDoc` to create + populate the Doc with the **Drive API only**: render `Document`→HTML, `Files.Create` with `mimeType=application/vnd.google-apps.document` + `.Media(htmlReader, text/html)` (Drive converts on import). Then:
- Delete the Docs API path (`buildRequests`, `docsSvc`, the `docs/v1` import).
- `drivetoken`: `Scopes = []string{ScopeDriveFile}` only; drop `ScopeDocuments`.
- Delete dead `internal/gdocs/` (the only other Docs-API importer).
- `splitParagraphs` must split on **single `\n`** (Outline bodies rely on it) — NOT `export.bodyParagraphs` (`\n\n`).
- Keep `CreateDoc` signature, the `Client` interface, the cached client, and `CreatedDoc.URL = docURL(id)` UNCHANGED.
- Fidelity: h1/h2→Doc headings, p→paragraphs; `[GAP: …]` markers survive (HTML-escape body, brackets are safe). Existing testers' tokens still work (narrowing scope is safe).

**AC:** no `documents`/`auth/documents` anywhere in scopes; `go build ./...` + tests pass; cached-client tests unchanged; `renderDocumentHTML` golden test (headings/escaping/[GAP]/single-\n); `tokensource_test` asserts `documents` ABSENT + `drive.file` present.

## Workstream B — compliance matrix CSV
`Section.RequirementIDs` is **never populated** in prod — do NOT use it. Build from real data:
- `export.RenderComplianceCSV(doc *document.Document, requirements []string, opts Options) ([]byte, error)` (stdlib `encoding/csv`, no new dep; keep `export` decoupled from `opportunity`).
- Rows from `opp.Requirements`; Status = **GAP** if an unresolved `doc.OpenFlagTexts()` mentions it; else **Addressed** if a section body mentions its keywords (list the heading in "Addressed in"); else **Review**. Empty requirements → header + one "fill manually from Section L/M" row (usable template).
- Columns: `# | Requirement | Source | Status | Addressed in (section) | Notes`. Title + Generated-date header rows first.
- Handler `GET /workspace/{id}/compliance.csv` (reuse `loadExportDoc`/`exportOptions`/`proposalFilename`; fetch `opp.Requirements` via `h.svc.Get`); route in handler.go; "Compliance matrix (CSV)" button in the workspace art-row.

**AC:** golden CSV tests (addressed/GAP/review/empty/well-formed/deterministic); handler test (200, text/csv, filename; empty reqs → valid CSV not 500); gate green.

## Checklist
- [x] A implemented + tested (sub-agent, `export/drop-docs-scope`)
- [x] B implemented + tested (sub-agent, `export/compliance-matrix`)
- [x] Merge both → `feat/export-finish`; full gate (build/test/lint) green
- [x] Deploy api to pilot (rev 00029); verify: compliance.csv downloads + valid; Drive connect requests ONLY drive.file
- [x] Merge `feat/export-finish` → main; clean instance to pristine; PushNotification; end loop

## Progress log
- 2026-06-26: Research done (2 agents) — both plans verified against code. Launched implementation team.
- 2026-06-26: Both implemented by parallel sub-agents, integrated on `feat/export-finish`. Gate green (build/tests/lint 0 issues); `documents` scope fully removed from production. Deployed rev 00029-5vx. Verified live: /connect requests `drive.file` only (no documents); compliance.csv downloads valid (3 real requirements → 3-state coverage); all 5 export buttons render, console clean. DONE.

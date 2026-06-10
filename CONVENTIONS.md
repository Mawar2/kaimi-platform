# CONVENTIONS.md — Kaimi

**Last updated:** 2026-06-09

How code is organized, named, tested, and shipped in this repo. These conventions
exist to prevent the 47-file chaos failure mode. Honor them. If you need to introduce
a new pattern, update this file in the same ticket (see "Patterns" below).

---

## Folder structure — where new code goes

```
.
├── cmd/                     # Entrypoints — one binary/probe per subdirectory
│   ├── pipeline/            # Zone-1 pipeline runner (Hunter → Scorer → Queue)
│   ├── hunter/              # Hunter agent entry point
│   ├── scorer/              # Scorer agent entry point
│   ├── outline-probe/       # Outline developer probe tool
│   └── spike/               # Throwaway/experiment spike
├── internal/                # Packages (not importable outside this module)
│   ├── agent/               # AgentResult contract and agent interfaces
│   ├── capability/          # CapabilityProfile (company capabilities/profile)
│   ├── opportunity/         # Opportunity schema (the shared, enriched data object)
│   ├── store/               # Store interface for persistence (JSON-backed)
│   ├── samgov/              # SAM.gov API client
│   ├── pipeline/            # Zone-1 orchestration (Hunter + Scorer)
│   ├── scorer/              # Scoring logic and Gemini integration
│   ├── outline/             # Outline generation and formatting rules
│   ├── writer/              # Technical Writer agent (draft generation)
│   ├── manager/             # Zone-2 per-proposal orchestrator
│   ├── finalreview/         # Final Review agent with validation
│   ├── gdocs/               # Google Docs/Drive integration
│   ├── googledocs/          # Google Docs support
│   ├── dashboard/           # Dashboard data layer (stage derivation + views)
│   ├── e2e/                 # End-to-end pipeline tests
│   ├── profile/             # Profile support
│   └── github/              # GitHub API caching layer
├── config/                  # Profiles and configuration (e.g. bluemeta_profile.yaml)
├── test/
│   └── fixtures/            # Cached fixtures (e.g. cached SAM.gov responses)
└── docs/                    # Documentation
```

**Rules of thumb:**
- A runnable binary or developer probe goes in its own `cmd/<name>/` directory.
- Reusable logic goes in a focused package under `internal/`. Pick the package whose
  single responsibility already fits; create a new package only when none does.
- Company profiles and config live in `config/`. Cached test data lives in
  `test/fixtures/`. Documentation lives in `docs/` (or the repo-root foundational docs).

---

## Anti-junk-drawer rules

**Extend before creating.** Before adding any new file:
1. Search the codebase for an existing file you can extend instead.
2. If extending is reasonable, extend it.
3. If you must create a new file, justify it on the ticket: "Created new file `[path]`
   because [reason existing files don't fit]."

**FORBIDDEN filenames** — these are junk-drawer magnets and are not allowed:
`utils.go`, `helpers.go`, `common.go`, `misc.go`, `lib.go`.

- Every file must have a **specific, descriptive name** indicating its single
  responsibility.
- Every package has a **`doc.go`** (or a package comment) explaining its purpose. The
  linter enforces this via the revive `package-comments` rule. See
  `internal/dashboard/doc.go`, `internal/scorer/doc.go`, and
  `internal/finalreview/doc.go` for examples.

---

## Go naming and style

Legibility is a **hard requirement** — two people review and learn from this code, one
newer to Go. Favor clear, conventional, well-commented Go over clever concurrency.

- **Exported functions and types MUST have doc comments starting with the name**
  (e.g. `// Save persists the Opportunity ...`). Enforced by the revive `exported` rule.
- **Wrap errors with context:** `fmt.Errorf("context: %w", err)`.
- **Never silently ignore a returned error.** If you must ignore one, comment why:
  `// Ignore error: [reason]`.
- **Inline comments explain WHY, not WHAT** for any non-obvious logic.

---

## Dependencies

- **Prefer stdlib.** Only add an external dependency when stdlib and existing tools
  are insufficient.
- **Justify on the ticket FIRST**, before adding: "Adding [dependency] for [purpose]
  because [reason stdlib/existing tools don't suffice]."
- **Pin exact versions:** `go get [package]@[version]`, then `go mod tidy`.
- No silent additions — the justification must live on the ticket.

---

## Branching and commits

- **Branch name:** `feature/KAI-XXX-short-summary` (or `fix/`, `chore/`, etc.).
- **Commit message:** `<ticket_number>_<feature_completed>` in snake_case —
  e.g. `12_hunter_cached_mode`. The ticket number is the GitHub Issue number; the
  description is a short snake_case summary of what was done.
- One logical feature per commit where practical; keep commits reviewable.

---

## Testing

Kaimi calls LLMs and external APIs, so tests have **two layers**, and **both must
exist and pass** before a feature is tested:

- **Unit + contract tests** — fast, deterministic, run on **every commit** and in CI.
  Use mocks and cached fixtures from `test/fixtures/` (e.g. SAM.gov `cached` mode).
  These must **never** depend on a live SAM.gov or live Gemini call.
- **End-to-end tests** — exercise the real flow (real SAM.gov API, real Gemini 2.5 Pro).
  Slower, costlier, non-deterministic, so they run in a **separate, less frequent job**,
  not on every commit. Assert **structure and behavior** (did it return a valid scored
  `Opportunity`?), never exact output strings. E2E coverage lives in `internal/e2e`.

**TDD is required:** write the failing test first, watch it fail, then write the code
to make it pass. This is the default for all feature work.

**Before every PR, run `make all`** (build + test + lint). Specific targets:

| Target | Does |
|--------|------|
| `make all` | Build, test, and lint — run before every PR |
| `make build` | Build all binaries |
| `make test` | `go test -v -race -cover ./...` |
| `make lint` | `golangci-lint run` |
| `make clean` | Remove build artifacts |

- **Formatter:** `gofmt` must be clean (`gofmt` + `goimports` are configured as
  formatters in `.golangci.yml`, with local import prefix `github.com/Mawar2/Kaimi`).
- **Linter:** `golangci-lint` per `.golangci.yml` (enabled: `gocritic`, `misspell`,
  `revive`; revive enforces `exported` and `package-comments`). Lint must pass with no
  errors.

---

## Patterns

Before introducing any new pattern (error handling, logging, config, etc.):
1. Check this file for an existing pattern and use it if it fits.
2. If you must introduce a new pattern, **update CONVENTIONS.md in the same ticket**,
   documenting why it is needed and how it differs from existing patterns.

No new convention is introduced without updating this file.

---

## Read next

- **WORKFLOW.md** — the full engineering workflow contract (ticket gate, TDD, PR/merge).
- **ARCHITECTURE.md** — the two-zone design, tech stack, and data model.
- **PROJECT.md** — what Kaimi is, who it's for, and success criteria.
- **CLAUDE.md** — how AI agents operate in this repo (anti-bloat rules, AI review).

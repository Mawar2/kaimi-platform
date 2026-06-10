# Architecture — Kaimi

**Last updated:** 2026-06-09
**Status:** Living

> **Read this before building anything.** This document gives you the full system
> context so your choices stay forward-compatible.
>
> **⚠️ Scope update (2026-06-09): the "Phase 0 only" lock is RETIRED.** Kaimi's full
> end-to-end pipeline is built and deployed, and we are completing the product (web +
> desktop dashboards, Zone-2 chain polish) for the June 11, 2026 Google AI Agents
> Challenge submission. Build across all zones and phases as approved tickets direct.
> The phase table and "Scope discipline" section below are kept for historical context
> and forward-compatibility guidance — **not** as a ceiling on what you may build.
> Forward-compatible schema design and "provision lazily, design eagerly" still apply.

---

## What this is

**Product name: Kaimi** — Hawaiian for "the seeker." An autonomous agent that
seeks and qualifies federal opportunities, tirelessly hunting for the right contracts.

An autonomous business-development pipeline for federal government contracting. It
hunts live federal opportunities on SAM.gov, scores them bid/no-bid against a
company's capabilities, and drafts tailored proposals — with a human reviewing
before anything is submitted.

This is **real production infrastructure** for BlueMeta Technologies' day-to-day BD
pipeline. It is being built under a hackathon timeline, but it is not a throwaway
demo. Optimize for a system that will be operated for years, not a one-off.

---

## Tech Stack

| Layer | Choice | Justification |
|-------|--------|---------------|
| **Language** | Go | Concurrency model required for end-state parallel proposal lifecycles; Google-native fit for ADK/GCP; single-binary deployment; strong readability for team learning. Python explicitly rejected. |
| **Agent Framework** | Google ADK (Agent Development Kit) v1.0+ Go SDK | Required for Google's Gemini Enterprise Agent Platform. Use current Agent Platform SDK, NOT deprecated Vertex AI SDK modules. |
| **LLM** | Gemini 2.5 Pro via Vertex AI | Google-native integration with ADK; enterprise platform support; reasoning capability required for bid/no-bid scoring and proposal generation. |
| **Cloud** | Google Cloud Platform (project: `kaimi-seeker`) | Required by ADK/Gemini integration; federal-compatible (FedRAMP Moderate available if needed later). |
| **Data Layer (Phase 0)** | JSON-backed Store interface | Provision lazily: JSON file sufficient for Phase 0. Interface designed for Firestore swap in Phase 1+ without touching Hunter code. |
| **Data Layer (Phase 1+)** | Firestore | Native GCP integration; document model fits Opportunity enrichment pattern; serverless scaling. |
| **CI/CD** | GitHub Actions | Already in use (see .github/ directory); free for private repos; sufficient for current team size. |
| **Linting** | golangci-lint | Go ecosystem standard; catches common issues; configured in .golangci.yml. |
| **Desktop client** | Wails v2 (Go) | See ADR-001 (`docs/desktop/adr-001-stack.md`). Go backend imports `internal/store` / `internal/dashboard` / `internal/opportunity` directly — zero logic duplication; single Go binary per OS; offline-first client. Chosen over Tauri (Rust backend can't reuse Go) and Electron (second runtime + bundled Chromium). |

**Code style constraint:** Favor clear, conventional, well-commented Go over clever
concurrency. Two people will review and learn from this code, one of them newer to
the language. Legibility is a hard requirement, not a nice-to-have.

---

## The architecture: two zones

The system has two distinct zones with different coordination styles. Understanding
this split is essential — it determines where every component belongs.

### Zone 1 — Scheduled pipeline (no orchestrator)
Runs daily as a batch job. No "Manager." State passes through a shared store.

```
Hunter  →  Scorer  →  Opportunity Queue (dashboard)
```

- **Hunter** — pulls + filters opportunities from the SAM.gov API by NAICS code.
- **Scorer** — scores each opportunity for bid/no-bid fit, with reasoning.
- **Queue** — shared store of scored opportunities awaiting human selection.

### Zone 2 — Per-proposal lifecycle (orchestrated)
Triggered when an opportunity is *selected*. A **Manager** agent spins up **per
proposal** and coordinates a sequence of specialist agents, pausing for one human
review gate.

```
Manager  →  Outline  →  Technical Writer  →  [HUMAN GATE]  →  Final Review
```

### The bridge between zones
The Hunter does **not** report to the Manager. The two zones are connected by a
single event: a human (or a rule) **selects** an opportunity from the queue, which
spins up a Manager for that one proposal. At scale, many Managers run concurrently —
one per active proposal.

---

## Client surfaces (web dashboard + desktop)

The two zones are the backend. Humans interact with them through **client surfaces**
that read the shared `Store` and present the pipeline. Clients are *not* a third zone:
they observe Zone 1's queue and Zone 2's proposal state, and the only write they
originate is the human's decisions (selection, approve/request-changes).

- **Web dashboard** — server-rendered Go HTML (`internal/dashboard`, `cmd/dashboard`).
  Reads the `Store` and derives pipeline stage (`DeriveStage`). The Kaimi design
  system (tokens + components, issues #126/#132) exists in Go (`StyleTag()`,
  `components.go`, `brand.go`) but is **not yet wired into the live render path**
  (placeholder templates remain — the #125 follow-up); adopting it is pending Go work.
- **Desktop dashboard** (Windows + macOS) — a **Wails v2 (Go)** app that reuses the
  *same* `internal/store`, `internal/opportunity`, and `internal/dashboard` data/view
  logic inside an OS-webview window, and shares the Go design system once it is wired
  in (the same work the web dashboard owes). Its reason to exist is
  **offline-first proposal work**: the human can read the already-synced queue, open
  proposals, edit the working draft, and make the one review-gate decision while
  temporarily offline; those actions are queued locally and replayed on reconnect.
  See ADR-001 (`docs/desktop/adr-001-stack.md`) for the stack decision and
  DESKTOP.md (design handoff) for the product intent.

**Offline-first is a client property, not a pipeline property.** The nightly hunt and
the live agent runs are server-side and remain online-only; "Select to pursue" needs
the agent runtime and is disabled offline. Provision lazily, design eagerly: the
device's local `Store` is the offline source of truth now; the local↔cloud **sync
layer is design-only** (backlog #140) — define the interface, don't build it yet.

---

## High-Level Components

```
┌─────────────────────────────────────────────────────────────────┐
│                         ZONE 1 (Daily Batch)                     │
│                                                                   │
│  ┌─────────┐      ┌─────────┐      ┌──────────────────────┐    │
│  │ Hunter  │─────▶│ Scorer  │─────▶│ Opportunity Queue    │    │
│  │ Agent   │      │ Agent   │      │ (Store interface)    │    │
│  └─────────┘      └─────────┘      └──────────────────────┘    │
│       │                │                       │                 │
│       ▼                ▼                       │                 │
│  SAM.gov API    Gemini 2.5 Pro            JSON/Firestore          │
└─────────────────────────────────────────────┬───────────────────┘
                                               │
                                    [HUMAN SELECTS OPPORTUNITY]
                                               │
┌──────────────────────────────────────────────▼──────────────────┐
│                    ZONE 2 (Per-Proposal Orchestration)           │
│                                                                   │
│  ┌─────────┐   ┌─────────┐   ┌──────────┐   ┌──────────────┐  │
│  │ Manager │──▶│ Outline │──▶│ Technical│──▶│ Final Review │  │
│  │ Agent   │   │ Agent   │   │ Writer   │   │ Agent        │  │
│  └─────────┘   └─────────┘   └──────────┘   └──────────────┘  │
│                                     │                │           │
│                                     ▼                ▼           │
│                              [HUMAN REVIEW GATE]  Google Docs    │
└───────────────────────────────────────────────────────────────────┘
```

### Hunter Agent
Pulls live federal opportunities from SAM.gov API, filters by NAICS code against BlueMeta's capability profile, populates the `Opportunity` schema. Respects SAM.gov rate limits (1,000 req/day) through intelligent caching. Runs daily as Zone 1 batch job. No dependencies on other agents.

### Scorer Agent
Takes opportunities from Hunter, scores bid/no-bid fit against BlueMeta's structured capability profile using Gemini reasoning. Produces explainable scores (not just numeric values). Writes scored opportunities to Queue. **Built and running** (`internal/scorer`).

### Opportunity Queue
Store interface (JSON-backed in Phase 0, Firestore in Phase 1+) holding scored opportunities awaiting human selection. The bridge between Zone 1 and Zone 2.

### Manager Agent
Orchestrator that spins up per-proposal when an opportunity is selected. Coordinates Zone 2 specialist agents in sequence. **Built** (`internal/manager`).

### Outline, Technical Writer, Final Review Agents
Zone 2 specialists for proposal generation. **Built** (`internal/outline`, `internal/writer`, `internal/finalreview`). See the ticket queues for remaining polish assignments.

---

## Data Model Sketch

- **Opportunity** — The shared data object enriched by every agent. Hunter creates it; downstream agents add fields. Designed in Phase 0 to hold all downstream fields even though Phase 0 only populates Hunter's portion. Fields include: SAM.gov ID, title, NAICS codes, eligibility filters, posting/response dates, solicitation text, agency info, score (Phase 1+), outline (Phase 3+), draft reference (Phase 3+). Schema lives in `internal/opportunity/`. Changing this later is highest integration risk — design eagerly.

- **Store interface** — Abstraction for persistence. Actual signature (`internal/store/store.go`): `Save(ctx, *Opportunity)`, `Get(ctx, id)`, `List(ctx, *Filter)`, `Delete(ctx, id)`. JSON file implementation (`internal/store/json.go`, `NewJSONStore(basePath)`), designed to swap to Firestore later without touching the agents. This is the "provision lazily, design eagerly" pattern in action.

- **CapabilityProfile** — Structured representation of BlueMeta Technologies' capabilities, certifications, and past performance for federal contracting. Loaded from YAML config file (`config/bluemeta_profile.yaml`). Used by Hunter agent for hard eligibility gates and Scorer agent for fit reasoning. Schema lives in `internal/capability/profile.go`. Contains:
  - Company identifiers (UEI: XVUEA59LY579, CAGE: 9RY40)
  - NAICS codes organized by tier (Primary/Secondary/Tertiary) for weighted matching
  - Set-aside eligibility status (small business, SDB, minority-owned)
  - Security clearance level (Public Trust)
  - Core competencies (16 technical and domain capabilities)
  - Past performance entries (9 projects with client, scope, value, and what each proves)

  The profile is designed to be forward-compatible with Phase 3 knowledge base enhancements (full narratives, embeddings, RAG). Current implementation provides lightweight facts sufficient for Phase 1 hard gates and scoring logic.

- **AgentResult** — Return type for all agents (Zone 1 and Zone 2). Every agent returns an `AgentResult` to communicate outcome and output location. Defined in `internal/agent/result.go`. Fields:
  - `agent_name` (string) - identifies which agent produced the result
  - `status` (enum) - outcome: `success`, `failed`, `needs_human`, or `ready_to_submit`
  - `notice_id` (string) - SAM.gov opportunity ID this result relates to
  - `summary` (string) - human-readable description of what happened (1-2 sentences)
  - `output_ref` (string) - pointer to output artifact (file path, URL, etc.)
  - `flags` (map[string]string) - extensible key-value metadata for agent-specific info
  - `error` (string) - error message if status is `failed`
  - `completed_at` (timestamp) - when the agent finished execution

  **Contract guarantees:**
  - Every agent must return an AgentResult, even on failure
  - Status enum is the source of truth for outcome - never just check error field
  - `ready_to_submit` status used only by Final Review agent to signal proposal is ready for human approval
  - Flags enable agents to communicate structured data without schema changes (e.g., scorer returns `{"score": "87", "recommendation": "BID"}`)

  See `internal/agent/stub.go` for a reference implementation.

---

## External Dependencies

| Dependency | Purpose | Justification | Failure mode if unavailable |
|------------|---------|---------------|------------------------------|
| **SAM.gov Opportunities API** | Source of federal contract opportunities | Only authoritative source for federal opportunities; registered account grants 1,000 req/day. | Hunter cannot run; system falls back to cached opportunities from previous successful runs. Must cache aggressively. |
| **Gemini 2.5 Pro (Vertex AI)** | LLM reasoning for scoring and proposal generation | Required by ADK framework; enterprise platform with federal compliance path (FedRAMP Moderate). | Scorer and Zone 2 agents cannot run; fall back to human-only BD workflow. |
| **Google Cloud Platform** | Hosting, Firestore (Phase 1+), ADK runtime | Required by Gemini/ADK integration; federal-compatible infrastructure. | Phase 0 unaffected (local only). Phase 1+ deployment fails; fall back to local dev environment. |

---

## Trust Boundaries

- **SAM.gov API → Hunter:** External untrusted data enters system here. Hunter MUST validate all SAM.gov responses (schema conformance, NAICS code format, date parsing). Do not assume SAM.gov API is well-formed. See `internal/samgov/client.go` and test fixtures in `test/fixtures/` for validation patterns.

- **Human selection → Manager (Phase 2+):** Human selects opportunity from Queue, triggering Manager agent. No authentication/authorization in Phase 0-1 (single-user internal tool). If system scales to multi-user, add auth here.

- **Agent outputs → Human review gate (Phase 3+):** Zone 2 agents produce draft proposals that humans review before submission. Human is final authority; agents never auto-submit to government portals. This is a core design principle.

---

## Known Constraints

- **SAM.gov rate limits (1,000 req/day registered):** Hunter must cache responses and avoid re-fetching on every run. Test fixtures in `test/fixtures/` enable development without burning API quota. Cache strategy: daily Hunter runs fetch new opportunities only; Scorer re-scores from cache.

- **Hackathon timeline (June 5, 2026 milestone):** Phase 0-1 slice must be judge-ready (working code + architecture diagram + video + test build). This is a milestone the production system passes through, not the end date. Do not take demo shortcuts that compromise production quality.

- **Two-person learning team:** Malik + Timm (ramping on Go). Code must prioritize legibility over cleverness. Comments required for non-obvious logic. This is why "clear, conventional, well-commented Go" is a hard requirement in the tech stack.

- **Single-binary deployment preference:** Go chosen partially for this. Simplifies Phase 1+ GCP deployment — single binary to Cloud Run, not multi-service orchestration.

---

## Guiding principle: provision lazily, design eagerly

- **Provision lazily:** stand up a GCP service only in the phase that needs it.
  Do not deploy databases, Agent Engine, vector search, etc. ahead of need.
- **Design eagerly:** design data layers (schemas, interfaces) to be
  forward-compatible from the start, so later agents plug in without retrofits.

Concretely for Phase 0: the queue is an **interface** (a `Store` with save/load),
backed by a simple JSON file. Do not reach for a real database yet — but define the
interface so the implementation can be swapped for Firestore later without touching
the Hunter.

---

## Build phases (roadmap — the "Phase 0 only" lock is retired, 2026-06-09)

| Phase | Scope | Status |
|-------|-------|--------|
| **0** | Foundation + Hunter agent + Opportunity schema + queue interface | ✅ Done |
| **1** | Scorer agent + queue + daily scheduling + GCP deploy | ✅ Done (Cloud Run Job + Scheduler; Store still JSON-backed, Firestore optional later) |
| **2** | Manager + Zone 2 orchestration + selection event | ✅ Built (`internal/manager`) |
| **3** | Outline + Writer + Final Review + dashboards | 🔧 In flight — agents built; web + desktop dashboards in active development for the submission |
| 4 | Past-performance knowledge base (RAG) + cross-proposal memory + scale hardening + observability | ⏳ Not yet — design eagerly, provision lazily; needs an approved ticket before build (see KAI-M8) |

**For the June 11 submission, build whatever an approved ticket asks across Phases 0–3.**
Phase 4 items (RAG knowledge base, cross-proposal memory, multi-tenancy) remain
genuinely future work — leave a `// TODO(phase-4):` marker rather than building them
ahead of an approved ticket.

---

## Architectural Decisions Record (ADR) Pointer

Significant architectural decisions made AFTER initial genesis are recorded as ADRs in `docs/adr/`. Each ADR uses the [Michael Nygard format](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions): Context, Decision, Status, Consequences. ADRs are append-only — superseded decisions stay in the record with status updated to "Superseded by ADR-XXX."

**Current ADRs:**
- **ADR-001 — Desktop dashboard stack: Wails v2 (Go)** (`docs/desktop/adr-001-stack.md`, accepted 2026-06-09, issue #137 / PR #144). Desktop ADRs are co-located with the desktop docs under `docs/desktop/` rather than `docs/adr/`, cross-referenced from here.

---

## What This Architecture Explicitly Does Not Do

The following are NOT supported by this architecture and would require fundamental redesign:

- **Auto-submission to government portals without human approval** — The human review gate in Zone 2 is a core principle, not a temporary limitation. Removing it would require rethinking trust boundaries and legal liability.

- **Multi-tenancy for multiple contracting firms** — Designed for BlueMeta's single-user/small-team use. Supporting multiple isolated tenants would require adding authentication, authorization, data isolation, and separate capability profiles.

- **Real-time streaming of SAM.gov updates** — Designed for daily batch runs. SAM.gov API doesn't support webhooks; polling every minute would burn rate limits. Real-time would require SAM.gov to change their API model.

- **Offline/air-gapped operation of the *pipeline*** — The backend pipeline (Hunter, Scorer, and the Zone 2 agent runs) requires live SAM.gov API and Gemini API access and cannot operate in classified/air-gapped environments without fundamental rearchitecture. **This does not forbid the offline-first desktop *client*:** the desktop app (ADR-001) lets a human read the already-synced queue and edit/review drafts offline against the local `Store`, queuing decisions for replay on reconnect. The pipeline still runs online; the client degrades gracefully offline. These are different things — do not read this bullet as prohibiting the desktop client.

- **Retroactive learning from past proposals across opportunities** — Phase 4+ feature. v1 architecture treats each proposal independently. Cross-proposal memory requires Memory Bank infrastructure not in current design.

---

## Scope discipline (updated 2026-06-09 — the phase lock is lifted)

The original version of this section restricted all work to Phase 0. **That
restriction is retired.** Hunter, Scorer, Manager, Outline, Writer, Final Review, the
`AgentResult` contract, scheduling, and GCP deploy are all built. We are completing
the product (dashboards + Zone-2 polish) for the June 11 submission.

What "scope discipline" means now:

- **DO** build across all zones and phases as approved tickets direct — no more refusing work as "later-phase."
- **DO** keep each ticket tightly scoped to its acceptance criteria. Building the full product is not license to gold-plate; build what the ticket asks, well.
- **DO** keep the `Opportunity` schema and `Store` interface forward-compatible — they remain the highest integration risk.
- **DO** keep code simple, conventional, and well-commented (legibility is a hard requirement).
- **Provision lazily**: stand up a new GCP service or dependency only when an approved ticket needs it — not speculatively.
- For genuinely future (Phase 4) work — RAG knowledge base, cross-proposal memory, multi-tenancy — leave a clear `// TODO(phase-4):` comment rather than building ahead of an approved ticket.

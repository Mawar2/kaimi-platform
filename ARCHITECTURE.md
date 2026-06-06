# Architecture — Kaimi

**Last updated:** 2026-06-03
**Status:** Living

> **Read this before building anything.** This document gives you the full system
> context so your choices stay forward-compatible. **You are only building Phase 0
> right now.** Do not build agents or infrastructure from later phases. See
> "Scope discipline" at the bottom.

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
| **LLM** | Gemini 3 Pro via Vertex AI | Google-native integration with ADK; enterprise platform support; reasoning capability required for bid/no-bid scoring and proposal generation. |
| **Cloud** | Google Cloud Platform (project: `kaimi-seeker`) | Required by ADK/Gemini integration; federal-compatible (FedRAMP Moderate available if needed later). |
| **Data Layer (Phase 0)** | JSON-backed Store interface | Provision lazily: JSON file sufficient for Phase 0. Interface designed for Firestore swap in Phase 1+ without touching Hunter code. |
| **Data Layer (Phase 1+)** | Firestore | Native GCP integration; document model fits Opportunity enrichment pattern; serverless scaling. |
| **CI/CD** | GitHub Actions | Already in use (see .github/ directory); free for private repos; sufficient for current team size. |
| **Linting** | golangci-lint | Go ecosystem standard; catches common issues; configured in .golangci.yml. |

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
│  SAM.gov API    Gemini 3 Pro            JSON/Firestore          │
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
Takes opportunities from Hunter, scores bid/no-bid fit against BlueMeta's structured capability profile using Gemini 3 Pro reasoning. Produces explainable scores (not just numeric values). Writes scored opportunities to Queue. Phase 1+.

### Opportunity Queue
Store interface (JSON-backed in Phase 0, Firestore in Phase 1+) holding scored opportunities awaiting human selection. The bridge between Zone 1 and Zone 2.

### Manager Agent
Orchestrator that spins up per-proposal when an opportunity is selected. Coordinates Zone 2 specialist agents in sequence. Does not exist until Phase 2.

### Outline, Technical Writer, Final Review Agents
Zone 2 specialists for proposal generation. Phase 3+. See kaimi_timm_tickets.md for Timm's agent build assignments.

---

## Data Model Sketch

- **Opportunity** — The shared data object enriched by every agent. Hunter creates it; downstream agents add fields. Designed in Phase 0 to hold all downstream fields even though Phase 0 only populates Hunter's portion. Fields include: SAM.gov ID, title, NAICS codes, eligibility filters, posting/response dates, solicitation text, agency info, score (Phase 1+), outline (Phase 3+), draft reference (Phase 3+). Schema lives in `internal/opportunity/`. Changing this later is highest integration risk — design eagerly.

- **Store interface** — Abstraction for persistence (`Save(opp)`, `Load(id)`, `List()`). JSON file implementation in Phase 0 (`internal/store/json.go`), swaps to Firestore in Phase 1+ without touching Hunter code. This is the "provision lazily, design eagerly" pattern in action.

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
| **Gemini 3 Pro (Vertex AI)** | LLM reasoning for scoring and proposal generation | Required by ADK framework; enterprise platform with federal compliance path (FedRAMP Moderate). | Scorer and Zone 2 agents cannot run; fall back to human-only BD workflow. |
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

## Build phases (context only — build Phase 0 only)

| Phase | Scope | Do you build it now? |
|-------|-------|----------------------|
| **0** | Foundation + Hunter agent + Opportunity schema + queue interface | **YES — this is the current task** |
| 1 | Scorer agent + real queue (Firestore) + daily scheduling | No |
| 2 | Manager + Zone 2 orchestration + selection event | No |
| 3 | Outline + Writer + Final Review + past-performance knowledge base (RAG) | No |
| 4 | Cross-proposal memory + scale hardening + observability | No |

---

## Architectural Decisions Record (ADR) Pointer

Significant architectural decisions made AFTER initial genesis are recorded as ADRs in `docs/adr/`. Each ADR uses the [Michael Nygard format](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions): Context, Decision, Status, Consequences. ADRs are append-only — superseded decisions stay in the record with status updated to "Superseded by ADR-XXX."

**Current ADRs:** None yet (genesis phase). First ADR will document the "two zones" architecture split if it's ever questioned.

---

## What This Architecture Explicitly Does Not Do

The following are NOT supported by this architecture and would require fundamental redesign:

- **Auto-submission to government portals without human approval** — The human review gate in Zone 2 is a core principle, not a temporary limitation. Removing it would require rethinking trust boundaries and legal liability.

- **Multi-tenancy for multiple contracting firms** — Designed for BlueMeta's single-user/small-team use. Supporting multiple isolated tenants would require adding authentication, authorization, data isolation, and separate capability profiles.

- **Real-time streaming of SAM.gov updates** — Designed for daily batch runs. SAM.gov API doesn't support webhooks; polling every minute would burn rate limits. Real-time would require SAM.gov to change their API model.

- **Offline/air-gapped operation** — Requires live SAM.gov API and Gemini API access. Cannot operate in classified/air-gapped environments without fundamental rearchitecture.

- **Retroactive learning from past proposals across opportunities** — Phase 4+ feature. v1 architecture treats each proposal independently. Cross-proposal memory requires Memory Bank infrastructure not in current design.

---

## Scope discipline (read this twice)

You are building **Phase 0 only**: project foundation, the Hunter agent, the
`Opportunity` schema, and the queue interface. A separate build brief specifies the
exact Phase 0 work.

- **Do NOT** build the Scorer, Manager, Outline, Writer, or Final Review agents yet (Phase 1+).
- **Do NOT** deploy databases, Agent Engine, vector search, or scheduling yet.
- **DO** implement the `AgentResult` contract now (KAI-M1) - it's foundational and unblocks other agent work.
- **DO** make the `Opportunity` schema and the `Store` interface forward-compatible.
- **DO** keep the code simple, conventional, and well-commented.

When in doubt, build less. The foundation others build on matters more than
features. If a decision seems to require knowledge of a later phase, leave a clear
`// TODO(phase-N):` comment rather than building ahead.

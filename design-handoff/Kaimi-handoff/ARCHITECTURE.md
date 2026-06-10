# ARCHITECTURE.md — Kaimi

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

## Core stack (decided — do not substitute)

- **Language: Go.** Chosen for its concurrency model (the end state runs many
  proposal lifecycles in parallel), Google-native fit, single-binary deployment,
  and readability. Do not suggest or scaffold Python.
- **Agent framework: Google ADK (Agent Development Kit), Go SDK, v1.0+.**
- **LLM: Gemini 3 Pro** via Vertex AI, where an agent needs reasoning.
- **Cloud: Google Cloud Platform / Vertex AI.**

**Code style:** Favor clear, conventional, well-commented Go over clever
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

## The interface contract (the spine of the system)

Every agent is a **black box**: it takes a typed input, does one job, returns a
typed result. **Agents never call each other directly.** A coordinator (the Manager
in Zone 2; the scheduler in Zone 1) reads one agent's output and feeds the next.
This is what lets agents be built and tested independently.

The shared data object is the `Opportunity`. The Hunter creates it; every
downstream agent enriches it. **Design this struct now to hold all downstream fields
even though Phase 0 only populates the Hunter's portion** — changing this schema
later is the highest integration risk in the project.

A later phase will add an `AgentResult` return type that every Zone 2 agent conforms
to (fields: agent name, status of `success`/`failed`/`needs_human`, a summary, an
output reference, and flags). You do not build this in Phase 0, but know it's coming
so your foundations don't preclude it.

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

## Scope discipline (read this twice)

You are building **Phase 0 only**: project foundation, the Hunter agent, the
`Opportunity` schema, and the queue interface. A separate build brief specifies the
exact Phase 0 work.

- **Do NOT** build the Scorer, Manager, Outline, Writer, or Final Review agents.
- **Do NOT** deploy databases, Agent Engine, vector search, or scheduling yet.
- **Do NOT** implement the `AgentResult` contract yet (just don't preclude it).
- **DO** make the `Opportunity` schema and the `Store` interface forward-compatible.
- **DO** keep the code simple, conventional, and well-commented.

When in doubt, build less. The foundation others build on matters more than
features. If a decision seems to require knowledge of a later phase, leave a clear
`// TODO(phase-N):` comment rather than building ahead.

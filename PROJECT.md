# PROJECT.md — Kaimi

**Last updated:** 2026-06-09

## What Kaimi is

**Kaimi** (Hawaiian for "the seeker") is an autonomous business-development pipeline
for federal government contracting. It hunts live federal opportunities on SAM.gov,
scores them bid/no-bid against a company's capabilities with explainable reasoning,
and drafts tailored proposals — **with a human reviewing before anything is submitted.**

This is **real production infrastructure** for BlueMeta Technologies' day-to-day BD
operations, built to be operated for years. It is being built under a hackathon
timeline, but it is **not a throwaway demo.** Optimize accordingly — do not take demo
shortcuts that compromise production quality.

## Who it's for

- **Primary user: Malik** — technical capture lead running a solo / two-person BD
  operation at BlueMeta Technologies.
- **Timm** — ramping on Go. Code must be clear enough for Timm to learn from, which is
  why legibility is a hard requirement, not a nice-to-have.

## The problem it solves

Federal BD is slow and manual: finding relevant opportunities on SAM.gov, judging
fit against a company's real capabilities, and drafting compliant proposals all
consume scarce capture-team time. Kaimi automates the repetitive hunting, scoring,
and drafting so the human spends time on judgment and approval, not on grunt work.

## The two-zone solution at a glance

Kaimi operates in two distinct zones with different coordination styles:

### Zone 1 — Scheduled pipeline (daily batch, no orchestrator)
```
Hunter → Scorer → Opportunity Queue
```
- **Hunter** — pulls and filters opportunities from the SAM.gov API by NAICS code
  against BlueMeta's capability profile.
- **Scorer** — scores each opportunity for bid/no-bid fit using Gemini reasoning,
  producing explainable scores (not just numeric values).
- **Queue** — shared `Store` of scored opportunities awaiting human selection. This
  is the bridge between Zone 1 and Zone 2.

Built and deployed (Cloud Run Job on Cloud Scheduler).

### Zone 2 — Per-proposal lifecycle (orchestrated)
```
Manager → Outline → Technical Writer → [HUMAN GATE] → Final Review
```
Triggered when a human **selects** an opportunity from the queue. A **Manager** agent
spins up per proposal and coordinates the specialist agents in sequence, pausing for
one human review gate before finalization. Built, with Google Docs/Drive integration.

The LLM throughout is **Gemini 2.5 Pro via Vertex AI**. The language is **Go**.

## Success criteria

1. **Google AI Agents Challenge (Track 1) submission** — due **June 11, 2026,
   5:00 PM PST.** A complete, shippable, judge-ready product: working end-to-end
   pipeline, architecture, and the dashboards over the scored queue.
2. **A real production system operated for years** — Kaimi is production
   infrastructure for BlueMeta's BD pipeline. The hackathon is a milestone it passes
   through, not the end date. Quality, legibility, and forward-compatibility are
   judged against the multi-year operating horizon, not the demo.

## Core design principles

- **Human always approves before submission.** Agents never auto-submit to government
  portals. The Zone 2 human review gate is a core principle, not a temporary limitation.
- **Agents never merge their own code.** A human (Malik or Timm) approves and performs
  every merge.
- **Respect SAM.gov rate limits → cache aggressively.** A registered account grants
  1,000 req/day. Hunter caches responses and avoids re-fetching; Scorer re-scores from
  cache. Cached fixtures enable development without burning quota.
- **Forward-compatible `Opportunity` schema.** The `Opportunity` holds fields for
  every agent across both zones; Hunter creates it and downstream agents enrich it.
  Changing it later is the highest integration risk, so design eagerly.
- **Provision lazily, design eagerly.** Stand up a GCP service only when an approved
  ticket needs it; design schemas and interfaces (the `Store`) to be
  forward-compatible from the start. The `Store` is JSON-backed today and can swap to
  Firestore later without touching agent code.

## Out of scope

The following are **not** supported and would require fundamental redesign:

- **Auto-submission to government portals without human approval** — the human review
  gate is a core principle; removing it changes trust boundaries and legal liability.
- **Multi-tenancy for multiple contracting firms** — Kaimi is designed for BlueMeta's
  single-user / small-team use. Multiple isolated tenants would require auth, data
  isolation, and separate capability profiles.
- **Real-time streaming of SAM.gov updates** — Kaimi runs as a daily batch. SAM.gov
  has no webhooks; minute-by-minute polling would burn rate limits.
- **Offline / air-gapped operation** — requires live SAM.gov and Gemini access; cannot
  run in classified/air-gapped environments without rearchitecture.
- **Cross-proposal RAG / past-performance knowledge base** — a Phase-4 future feature.
  The current architecture treats each proposal independently. Leave a
  `// TODO(phase-4):` marker rather than building it ahead of an approved ticket.

## Read next

- **ARCHITECTURE.md** — the two-zone design, tech stack, data model, build phases.
- **CONVENTIONS.md** — folder structure, anti-bloat rules, Go style, testing, branching.
- **WORKFLOW.md** — the engineering workflow contract (ticket gate, TDD, PR/merge).
- **CLAUDE.md** — how AI agents operate in this repo.

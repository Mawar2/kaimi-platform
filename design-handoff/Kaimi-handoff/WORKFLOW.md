# WORKFLOW.md — BlueMeta Pulse · Engineering Workflow Contract

> **Every agent that writes code for this project follows this workflow on every
> task. No exceptions.** This document defines *how* work gets done. `ARCHITECTURE.md`
> defines *what* gets built. Read both before starting.

---

## Core principle: no work without an approved ticket

You do not write code for anything that is not a ticket on the project board with
**approved acceptance criteria** and a **clear definition of done**. If a task has no
ticket, or the ticket's criteria are not yet approved by a human, **stop and ask** —
do not start building.

- **Source of truth: GitHub Issues.** Every unit of work is a GitHub Issue.
- Each Issue must have, before work begins:
  - A clear description of the feature.
  - **Acceptance criteria** (a checklist of what must be true to be done).
  - A **definition of done** (tests pass, linter passes, reviewed, merged).
  - A ticket number (the Issue number) used in the commit message.
- If acceptance criteria are missing or unapproved, propose them on the Issue and
  wait for human approval before coding.

---

## Phase 0.0 — Scaffolding comes first

This workflow requires infrastructure that must exist *before* any feature code.
The very first work on this project is to set that up, in this order:

1. Initialize the Git repository and push to GitHub.
2. Commit `ARCHITECTURE.md` and `WORKFLOW.md` to the repo root.
3. Set up the GitHub project board and create the initial Phase 0 Issues with
   acceptance criteria (for human approval before any are worked).
4. Set up the CI/CD pipeline (see below) so it runs from the first feature PR.
5. Only then begin the first feature ticket.

Do not write feature code (the Hunter, etc.) until the board and CI pipeline exist.

---

## Test-Driven Development (required)

Write the test first, watch it fail, then write the code to make it pass. This is
the default for all feature work.

**Test layers** (because this system calls LLMs and external APIs):

- **Unit + contract tests** — fast, deterministic, run on every commit and in CI.
  Test against mocks and the cached fixtures (e.g. the SAM.gov `cached` mode).
  These must never depend on a live LLM response or live network call.
- **End-to-end tests** — exercise the real flow (real API, real model). These are
  slower, costlier, and non-deterministic, so they run in a **separate, less
  frequent job**, not on every commit. LLM-dependent assertions must check
  *structure and behavior* (did it return a valid scored Opportunity?), never exact
  output strings.

A feature is not "tested end to end" until both layers exist and pass.

---

## Definition of "feature complete"

A feature is complete only when ALL of the following are true:

1. The ticket's acceptance criteria are all met.
2. Tests are written (TDD) and **all tests pass** (unit/contract + the relevant
   end-to-end coverage).
3. **The linter passes** with no errors.
4. An AI **sub-agent code-review team** has reviewed the change (see below).
5. The change is on a pull request that references the ticket.

---

## AI sub-agent code review

When a feature is considered complete, before opening it for human merge approval,
spin up a **sub-agent review team** to review the change. The review must check, at
minimum:

- Acceptance criteria are actually met (not just claimed).
- Test coverage is real (TDD followed; tests would fail if the code broke).
- Code matches `ARCHITECTURE.md` — correct zone, conforms to the interface contract,
  forward-compatible schema, no building ahead of the current phase.
- Code is clear, conventional, well-commented Go (legibility is a hard requirement).
- No secrets in code; security-sensitive changes are flagged explicitly.

The AI review **does not approve the merge.** It produces a review report on the PR.
Its job is to catch problems before a human looks, not to replace the human.

---

## Pull requests, approval, and merge

- Every feature lands via a **pull request** that references its ticket.
- The CI/CD pipeline runs on every PR: it must pass **all tests and the linter** for
  the PR to be mergeable. A PR that fails any gate cannot be merged.
- The CI pipeline also verifies the PR meets the criteria tied to its ticket.
- **A human approves and performs every merge.** No agent merges code. The AI review
  and CI gates run first and must pass, but the final approve-and-merge is always a
  human decision (you or Timm).
- Security-sensitive or contract-level changes (IAM, Secret Manager, the
  `Opportunity` schema, the interface contract) get extra human scrutiny — call them
  out prominently in the PR description.

---

## Commit format

Commits use the format:

```
<ticket_number>_<feature_completed>
```

Example: `12_hunter_samgov_cached_mode`

- `ticket_number` is the GitHub Issue number.
- `feature_completed` is a short snake_case description of what was done.
- One logical feature per commit where practical; keep commits reviewable.

---

## The loop, end to end

For every ticket:

1. Confirm the Issue exists with **approved** acceptance criteria + definition of
   done. If not, propose them and wait for human approval.
2. Write the failing test (TDD).
3. Write the code to pass it.
4. Run all tests + linter locally until green.
5. Spin up the AI sub-agent review team; address its report.
6. Open a PR referencing the ticket; CI runs all gates.
7. A **human** reviews and merges if everything passes.
8. Commit message follows `<ticket_number>_<feature_completed>`.

If any gate fails, the feature is not done. Fix it or stop and ask.

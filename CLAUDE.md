# CLAUDE.md — Agent Operating System for Kaimi

**Last updated:** 2026-06-03

This document defines how Claude Code (and any other AI agent) operates in this repository. Read this file in full at the start of every session. Re-read after any major changes are committed.

## Document Pointers — Read These First

Before working on any ticket, ensure you have read:
- **PROJECT.md** — what Kaimi is, who it's for (Malik/BlueMeta BD), success criteria (hackathon/30d/90d), what's out of scope
- **ARCHITECTURE.md** — two-zone architecture, tech stack (Go/ADK/Gemini), provision lazily/design eagerly, Phase 0 scope discipline
- **CONVENTIONS.md** — folder structure, where new code goes, anti-junk-drawer rules, Go naming conventions, testing requirements
- **WORKFLOW.md** — engineering workflow contract, TDD requirement, AI sub-agent review, PR protocol

If any of these documents conflict with this CLAUDE.md, flag the conflict and STOP. Do not silently choose a winner.

## Repository Information

- **GitHub Repository:** `Mawar2/Kaimi`
- **Main Branch:** `main`
- **Remote URL:** https://github.com/Mawar2/Kaimi.git
- **Project:** Production infrastructure for BlueMeta Technologies' federal BD pipeline
- **Current Phase:** Phase 0 (Foundation + Hunter agent)

## The Hard Gate — Ticket Discipline

**No code is written without an approved GitHub Issue.** Period.

From WORKFLOW.md:
> You do not write code for anything that is not a ticket on the project board with **approved acceptance criteria** and a **clear definition of done**. If a task has no ticket, or the ticket's criteria are not yet approved by a human, **stop and ask** — do not start building.

If asked to "just quickly fix X" or "make a small change Y," respond:
> "Per WORKFLOW.md, this work needs an approved GitHub Issue with acceptance criteria. Want me to create one and propose acceptance criteria for your approval?"

**The only exception:** Genesis scaffold work, committed under messages without ticket numbers during initial setup.

**Ticket sources:**
- **GitHub Issues** (source of truth) — use GitHub MCP server or `gh` CLI to access
- `docs/tickets/malik-tickets.md` — Malik's ticket queue (KAI-M1, KAI-M2, etc.)
- `docs/tickets/timm-tickets.md` — Timm's ticket queue (KAI-1 through KAI-7)

**Issue organization:**
- **Phase labels:** `phase-0`, `phase-1`, `phase-2`, `phase-3`
- **Zone labels:** `zone-1` (scheduled pipeline), `zone-2` (orchestrated per-proposal)
- **Agent labels:** `agent:hunter`, `agent:scorer`, `agent:outline`, `agent:final-review`
- **Team labels:** `malik`, `timm`

## Definition of Done (Universal)

From WORKFLOW.md, a feature is complete ONLY when ALL of the following are true:

- [ ] All acceptance criteria from the ticket are met with evidence (file:line or test name)
- [ ] **Tests are written (TDD)** — test written first, watched it fail, then code written to pass
- [ ] **All tests pass** locally — both unit/contract tests (mocked, fast) and relevant E2E coverage (live APIs, slower)
- [ ] **Linter passes** (`make lint` or `golangci-lint run`) with no errors
- [ ] **Formatter passes** (`gofmt` clean)
- [ ] No new dependencies added without justification written on the ticket first
- [ ] No new files created outside CONVENTIONS.md patterns without justification
- [ ] No new conventions introduced without updating CONVENTIONS.md in the same ticket
- [ ] Branch named according to CONVENTIONS.md: `feature/KAI-XXX-short-summary` (or `fix/`, `chore/`, etc.)
- [ ] All commits use the format from CONVENTIONS.md: `XXX_feature_description` (e.g., `12_hunter_cached_mode`)
- [ ] No secrets committed (API keys, credentials, tokens)
- [ ] **AI code review** completed automatically in CI (see below) and feedback considered
- [ ] **Human approval** received on the PR (Malik or Timm)
- [ ] CI pipeline passes (tests + linter + AI review, enforced by GitHub Actions)

## Two-Layer Testing Requirement (from WORKFLOW.md)

Kaimi calls LLMs and external APIs, so tests have two layers:

**Unit + contract tests (run on every commit):**
- Fast, deterministic, run in CI on every PR
- Use mocks and cached fixtures from `test/fixtures/`
- NEVER depend on live SAM.gov API or live Gemini calls
- These prove the code works against known inputs

**End-to-end tests (run separately, less frequently):**
- Real SAM.gov API, real Gemini 3 Pro calls
- Slower, costlier, non-deterministic
- Assert structure and behavior (did it return a valid scored Opportunity?), not exact output strings
- Run separately from unit tests, not on every commit

**A feature is NOT tested until both layers exist and pass.**

**TDD is required:**
> Write the test first, watch it fail, then write the code to make it pass. This is the default for all feature work.

## AI Code Review System (Automated in CI/CD)

The project has an **automated AI code review** integrated into the CI/CD pipeline.

### How It Works

Every pull request triggers an AI code review job (`.github/workflows/ci.yml` - `ai-code-review` job) that:
1. Authenticates to GCP and retrieves the Gemini API key from Secret Manager
2. Gets the PR diff (limited to 50KB)
3. Sends the diff to **Gemini 2.5 Pro** (with thinking capability) for review
4. AI reviews for:
   - Bugs and logic errors
   - Security vulnerabilities (OWASP Top 10)
   - Performance issues
   - Go best practices and idioms
   - Test coverage gaps
   - Alignment with ARCHITECTURE.md (correct zone, forward-compatible design, phase scope)
5. Posts review feedback as a PR comment
6. **Required gate** - must complete before merge

### How to Use AI Review Feedback

- The AI review is **required** but not **blocking** based on findings
- You MUST see the feedback (CI gate ensures review completes)
- You and the human reviewer decide what to fix
- Not all AI suggestions must be addressed, but they must be **considered**
- If the review finds critical security issues, **prioritize fixing them**
- If you disagree with a suggestion, document why in a PR comment

### IMPORTANT: When to Create PRs

**Only create a PR when your code is ready for human review.**

- Work on your feature branch as long as needed
- Every push to an open PR triggers an AI review (~$0.01 per review)
- Opening a PR early and pushing many commits adds unnecessary cost
- **Best practice**: Complete your work, run tests locally, then open the PR
- If you need early feedback, use a **draft PR** (AI review is skipped on drafts)
- When you're ready, mark the draft as "Ready for review" to trigger AI review

**Cost example**:
- Good: 1 commit → open PR → 1 AI review = $0.01
- Bad: Open PR → 10 commits → 10 AI reviews = $0.10

**Technical details:**
- Platform: **Vertex AI** (Google Cloud's enterprise AI platform)
- Model: Gemini 2.5 Pro (`gemini-2.5-pro`) - June 2025 stable release
- Why this model: Best for code review - has **thinking capability** for deep reasoning, catches subtle bugs, enforces rules strictly
- Authentication: GCP service account (same as other GCP services)
- Region: us-east4 (same as other Kaimi infrastructure)
- Cost: Pay-as-you-go (first 50 requests/day free)
- Runs on: Every PR from non-fork branches

## AI Sub-Agent Review Protocol (from WORKFLOW.md)

In addition to the automated CI review, when a feature is considered complete, BEFORE opening it for human review, you may spin up an **AI sub-agent review team** for deeper analysis. This is separate from the CI review and checks:

- Acceptance criteria are actually met (not just claimed) with evidence
- Test coverage is real (TDD followed; tests would fail if the code broke)
- Code matches ARCHITECTURE.md:
  - Correct zone (Zone 1 scheduled vs. Zone 2 orchestrated)
  - Conforms to interface contract (Opportunity enrichment, Store interface)
  - Forward-compatible schema design
  - **No building ahead of current phase** (Phase 0 only right now)
- Code is clear, conventional, well-commented Go (legibility is a hard requirement)
- No secrets in code
- Security-sensitive changes flagged explicitly (Opportunity schema, Store interface, IAM, Secret Manager)

**The sub-agent review produces a detailed report. It does NOT approve the merge.** Its job is to catch problems before a human looks.

## Pull Request & Merge Protocol (from WORKFLOW.md)

- Every feature lands via a **pull request** that references its GitHub Issue
- The **CI/CD pipeline runs** on every PR: tests + linter + AI review must pass for PR to be mergeable
- **Human approves and performs every merge** — Malik or Timm. No agent merges code.
- **Security-sensitive changes** (IAM, Secret Manager, Opportunity schema, Store interface) get extra scrutiny — call out prominently in PR description

**Blocked PRs:** If CI fails or review finds issues, the feature is NOT done. Fix it or escalate to human.

## Anti-Bloat Rules

These rules prevent the 47-file chaos failure mode.

### File Creation — Extend Before Creating

Before creating any new file:
1. Search the codebase for existing files that could be extended instead
2. If extending is reasonable, extend the existing file
3. If creating new file, justify in the ticket: "Created new file `[path]` because [reason existing files don't fit]"

**FORBIDDEN filenames:** `utils.go`, `helpers.go`, `common.go`, `misc.go`, `lib.go`. Every file must have a specific, descriptive name indicating its single responsibility.

### Dependency Addition — Justify First

Before adding any dependency to `go.mod`:
1. **Justify on the ticket FIRST** (before adding): "Adding [dependency] for [purpose] because [reason stdlib/existing tools don't suffice]"
2. Pin exact version: `go get [package]@[version]` then `go mod tidy`
3. No silent additions — the ticket must document the justification

**Minimal dependency philosophy:** Prefer stdlib. Only add external deps when stdlib is insufficient.

### Pattern Introduction — Update CONVENTIONS.md

Before introducing any new pattern (error handling, logging, config, etc.):
1. Check CONVENTIONS.md for existing pattern — use it if it fits
2. If introducing new pattern, **update CONVENTIONS.md in the same ticket**
3. Document why the new pattern is needed and how it differs from existing patterns

### Documentation Bloat — Post-Merge Evaluation

After every ticket is merged, evaluate each new documentation artifact created during the work:

- **KEEP in `docs/`** if: describes architecture, public interface, API contract, onboarding-relevant behavior, or operational guide
- **MOVE to CLAUDE.md** if: describes a convention or pattern future agents must follow on subsequent tickets
- **REMOVE** if: implementation detail better captured by code itself, superseded by later work, or duplicates existing docs

Write the justification citing which rubric criterion applied. No documentation lives in the repo without a current justification.

**Date every doc artifact** with `**Last updated:** YYYY-MM-DD` at the top. Docs older than 90 days without updates get re-evaluated.

## Architecture Discipline — Phase 0 Scope Constraints

From ARCHITECTURE.md:
> You are building **Phase 0 only**: project foundation, the Hunter agent, the `Opportunity` schema, and the queue interface.

**DO in Phase 0:**
- ✅ Build the Hunter agent (pulls SAM.gov opportunities, filters by NAICS)
- ✅ Design the `Opportunity` schema to hold all downstream fields (even though Phase 0 only populates Hunter's portion)
- ✅ Create the `Store` interface (JSON-backed in Phase 0)
- ✅ Keep code simple, conventional, well-commented Go

**DO NOT in Phase 0:**
- ❌ Build Scorer, Manager, Outline, Writer, or Final Review agents
- ❌ Deploy databases, Agent Engine, vector search, or scheduling infrastructure
- ❌ Implement the `AgentResult` contract yet (just don't preclude it)
- ❌ Build ahead of the current phase

**When in doubt, build less.** If a decision requires knowledge of a later phase, leave a `// TODO(phase-N):` comment rather than building ahead.

**Provision lazily, design eagerly:**
- Stand up GCP services only when the phase needs them
- Design data layers (schemas, interfaces) to be forward-compatible from the start

## Enforcement Mechanisms (Not Just Documentation)

CLAUDE.md is context, not a fence. Real enforcement lives in:

| Mechanism | Enforces | Status |
|-----------|----------|--------|
| **Pre-commit hook** | Ticket prefix on commits, no secrets, formatter clean | Phase 1+ (not implemented in Phase 0) |
| **Pre-push hook** | Branch naming pattern | Phase 1+ (not implemented in Phase 0) |
| **CI pipeline (GitHub Actions)** | Tests pass, lint clean, builds succeed, AI review completes | Active (see `.github/workflows/ci.yml`) |
| **AI code review (automated)** | Security, bugs, Go best practices, architecture alignment | Active (Gemini 2.5 Pro in CI) |
| **AI sub-agent review (manual)** | AC + DoD verification with evidence, deep architecture check | Active (WORKFLOW.md protocol, optional) |
| **Human approval** | Final merge gate | Active (Malik or Timm approves every PR) |

**Phase 0 note:** Pre-commit/pre-push hooks are not yet implemented. Rely on CI pipeline and human review. Hooks will be added in Phase 1.

## Go-Specific Conventions

From CONVENTIONS.md, enforced for all Go code:

**Code style:**
- Linter: `golangci-lint` configured in `.golangci.yml`
- Formatter: `gofmt` (standard Go formatter)
- Run `make all` before every PR (builds, tests, lints)

**Legibility requirement (from ARCHITECTURE.md):**
> Favor clear, conventional, well-commented Go over clever concurrency. Two people will review and learn from this code, one of them newer to the language. Legibility is a hard requirement, not a nice-to-have.

**Specific rules:**
- Exported functions/types MUST have doc comments starting with the name
- Complex logic MUST have inline comments explaining WHY, not WHAT
- Error handling: wrap errors with context using `fmt.Errorf("context: %w", err)`
- Never ignore returned errors without comment: `// Ignore error: [reason]`
- Package doc.go files required for every package explaining purpose

## What To Do When Stuck

If blocked, do NOT improvise. Follow this protocol:

1. **Document the blocker** on the GitHub Issue: what you tried, what failed, what you need
2. **Re-read CONVENTIONS.md and ARCHITECTURE.md** for guidance you may have missed
3. **Search the codebase** for existing solutions to similar problems (use `grep`, check `internal/` packages)
4. **Check test fixtures** in `test/fixtures/` for examples of expected data formats
5. **Escalate to Malik** if blocked after multiple attempts or if the blocker requires architectural decision

**Never invent a solution that violates CONVENTIONS.md or ARCHITECTURE.md to "unblock" yourself.** The conventions exist to prevent 47-file chaos. Honor them.

## Critical Project Context

**This is a real production system, not a demo:**
- Kaimi is production infrastructure for BlueMeta Technologies' daily BD operations
- Hackathon (June 5, 2026) is a milestone it passes through, not the end date
- Do not take demo shortcuts that compromise production quality
- Optimize for a system operated for years, not a throwaway one-off

**Primary user:**
- Malik (technical capture lead) running solo/two-person BD operations
- Code must be clear enough for Timm (ramping on Go) to learn from

**Core design principles:**
- Human always approves proposals before submission (never auto-submit)
- Agents never merge their own code (Malik or Timm merges)
- SAM.gov rate limits (1,000 req/day) — must cache aggressively
- Forward-compatible schema design (Opportunity holds all downstream fields even in Phase 0)

## Session Start Checklist

At the start of every Claude Code session in this repo:

- [ ] Read this CLAUDE.md
- [ ] Skim PROJECT.md for current phase and success criteria
- [ ] Check ARCHITECTURE.md for phase scope ("Phase 0 only" reminder)
- [ ] Review CONVENTIONS.md if creating new files or packages
- [ ] Check GitHub Issues for any active work assigned (use GitHub MCP server or `gh` CLI)
- [ ] Check `docs/tickets/malik-tickets.md` and `docs/tickets/timm-tickets.md` for context
- [ ] Confirm `git status` is clean before starting new work
- [ ] Run `make test` and `make lint` to verify environment is working

## GitHub MCP Server (Optional)

The project can use a GitHub MCP (Model Context Protocol) server for direct GitHub access:

```bash
claude mcp add --transport http github https://api.githubcopilot.com/mcp/ \
  --header "Authorization: Bearer YOUR_GITHUB_PAT"
```

This enables fetching GitHub issues, PRs, and repository data without `gh` CLI. Run `/mcp` to verify connection.

## The Closing Reminder

This document is the contract. Following it produces clean, maintainable, shippable code that Kaimi operates on for years. Skipping it produces 47-file chaos.

The genesis ritual built this contract. Now we honor it.

**When in doubt:**
- Read the docs (PROJECT.md, ARCHITECTURE.md, CONVENTIONS.md, WORKFLOW.md)
- Build less, not more
- Extend existing files before creating new ones
- Ask before deviating from conventions
- Remember: this is production infrastructure, not a throwaway demo

Slow is smooth. Smooth is fast. Genesis is the smooth.

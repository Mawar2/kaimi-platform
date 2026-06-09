# Dashboard Architecture — Kaimi

**Last updated:** 2026-06-09
**Status:** Design (pre-implementation)
**Issue:** #105 — Design doc: dashboard architecture & opportunity-field → stage mapping

---

## Purpose

This document defines the architecture for the Kaimi dashboard: a read-only HTTP
server that visualises where every opportunity sits in the six-stage pipeline.
It is the shared map that all subsequent dashboard implementation waves use as a
reference. No implementation code is written in this ticket; later tickets will
implement each wave.

---

## New packages

### `cmd/dashboard`

A standalone binary entry point, following the same pattern as `cmd/hunter` and
`cmd/pipeline`. Responsibilities:

- Parse configuration (store path, listen address) from environment / flags
- Construct a `store.Store` (JSON implementation in Phase 0; swappable to Firestore
  in Phase 1+)
- Construct and start the `internal/dashboard` HTTP server
- Block on `os.Signal` for graceful shutdown

### `internal/dashboard`

The HTTP server and all dashboard logic. Responsibilities:

- Expose read-only HTTP endpoints over the Store
- Derive each opportunity's pipeline stage from its current field values
- Render the results (JSON API in Phase 1; HTML templates possible in Phase 2)

**This package does not own any state.** It holds no data structures of its own
beyond the request lifetime. The Store is its only data source.

---

## Read-only, single-data-path constraint

> **The dashboard server reads exclusively through the `store.Store` interface
> (`internal/store/store.go:22`). It does not mutate the store, does not maintain
> a parallel cache, and does not open the underlying JSON files directly.**

Rationale:

1. **No parallel data path.** All agents (Hunter, Scorer, Manager) write to and
   read from the Store through the same interface. Bypassing the interface
   (e.g., reading JSON files directly) would diverge from the canonical view and
   break when the backing implementation changes to Firestore in Phase 1+.

2. **No store mutation.** The dashboard is a read-only observability layer in
   Phase 1. Selection (setting `Selected = true`) happens via a separate CLI
   action or, in a later phase, a separate write-capable endpoint that explicitly
   calls `store.Store.Save`. Keeping Phase 1 dashboard read-only minimises risk
   and clarifies the trust boundary: the dashboard cannot corrupt pipeline state.

3. **Interface contract.** All dashboard code depends only on `store.Store`:
   ```go
   type Store interface {
       Save(ctx context.Context, opp *opportunity.Opportunity) error   // NOT called by dashboard
       Get(ctx context.Context, id string) (*opportunity.Opportunity, error)
       List(ctx context.Context, filter *Filter) ([]*opportunity.Opportunity, error)
       Delete(ctx context.Context, id string) error                   // NOT called by dashboard
   }
   ```
   (`internal/store/store.go:22–40`)

---

## Request flow

```
HTTP Client
    │
    ▼
cmd/dashboard/main.go
    │  (constructs store.Store, constructs server, calls server.Start)
    ▼
internal/dashboard/server.go   — mux, middleware, graceful shutdown
    │
    ▼
internal/dashboard/handler.go  — one handler per endpoint
    │
    │  store.List(ctx, filter)   [read-only]
    │  store.Get(ctx, id)        [read-only]
    ▼
store.JSONStore (or future FirestoreStore)
    │
    ▼
internal/dashboard/stage.go    — DeriveStage(*opportunity.Opportunity) Stage
    │                            (runs in-process, no I/O)
    ▼
JSON response / HTML template  → HTTP Client
```

Stage derivation runs entirely in process on the retrieved `*opportunity.Opportunity`
values. It requires no additional store calls.

**Filter usage note.** `store.Filter` (`internal/store/store.go:48–59`) currently
exposes `Selected *bool`, `MinScore float64`, and `MaxScore float64`. It has no
`ProposalStatus` field. For Phase 1 the server calls `store.List(ctx, nil)` to
retrieve all opportunities and applies stage derivation in Go. At scale (Phase 2+),
consider extending `store.Filter` with a `ProposalStatus` prefix field or moving
to Firestore query predicates to avoid reading all records for each request.

---

## The six pipeline stages

### Field reference

All stage derivation is based on the following fields of
`internal/opportunity/opportunity.Opportunity`:

| Field | Type | JSON key | Source line |
|-------|------|----------|-------------|
| `ScoredAt` | `*time.Time` | `"scored_at"` | `opportunity.go:53` |
| `Score` | `float64` | `"score"` | `opportunity.go:49` |
| `Recommendation` | `string` | `"recommendation"` | `opportunity.go:51` |
| `Selected` | `bool` | `"selected"` | `opportunity.go:56` |
| `SelectedAt` | `*time.Time` | `"selected_at"` | `opportunity.go:57` |
| `ProposalStatus` | `string` | `"proposal_status"` | `opportunity.go:58` |

`ProposalStatus` is a composite `"<stage>:<status>"` string written by
`internal/manager/manager.go:188`:

```go
opp.ProposalStatus = fmt.Sprintf("%s:%s", stage, status)
```

where `stage` is one of the constants at `manager.go:32–34`:

| Constant | Value |
|----------|-------|
| `stageOutline` | `"outline"` |
| `stageWriter` | `"writer"` |
| `stageReview` | `"final-review"` |

and `status` is one of the `agent.Status` constants at
`internal/agent/result.go:19–33`:

| Constant | Value |
|----------|-------|
| `StatusSuccess` | `"success"` |
| `StatusFailed` | `"failed"` |
| `StatusNeedsHuman` | `"needs_human"` |
| `StatusReadyToSubmit` | `"ready_to_submit"` |

`Recommendation` is set by `internal/scorer/scorer.go:ScoreAndSave` to one of the
`scorer.Recommendation` constants at `scorer.go:19–29`:

| Constant | Value | Score range |
|----------|-------|-------------|
| `RecommendationBID` | `"BID"` | `Score >= 0.60` |
| `RecommendationReview` | `"REVIEW"` | `0.40 <= Score < 0.60` |
| `RecommendationNoBid` | `"NO_BID"` | `Score < 0.40` |

(`scorer.go:281` defines the rubric boundaries.)

---

### Stage definitions and derivation rules

Stages are derived deterministically. Apply the rules **in order**; the first match
is authoritative.

| Priority | Stage | Condition |
|----------|-------|-----------|
| 1 | **Finalized** | `Selected == true` AND `ProposalStatus == "final-review:ready_to_submit"` |
| 2 | **Awaiting Human Review** | `Selected == true` AND `strings.HasSuffix(ProposalStatus, ":needs_human")` |
| 3 | **In Proposal** | `Selected == true` AND `ProposalStatus != ""` |
| 4 | **Selected** | `Selected == true` AND `ProposalStatus == ""` |
| 5 | **Scored** | `Selected == false` AND `ScoredAt != nil` |
| 6 | **Hunted** | *(default)* `ScoredAt == nil` |

Expressed as Go pseudocode (canonical reference for implementers):

```go
func DeriveStage(opp *opportunity.Opportunity) Stage {
    if opp.Selected {
        switch {
        case opp.ProposalStatus == "final-review:ready_to_submit":
            return StageFinalized
        case strings.HasSuffix(opp.ProposalStatus, ":needs_human"):
            return StageAwaitingHumanReview
        case opp.ProposalStatus != "":
            return StageInProposal
        default:
            return StageSelected
        }
    }
    if opp.ScoredAt != nil {
        return StageScored
    }
    return StageHunted
}
```

#### Stage: Hunted

**When:** `ScoredAt == nil`

The Hunter (`cmd/hunter`) has discovered the opportunity and saved it to the Store,
but the Scorer has not yet run. Core, classification, and detail fields are populated
(`opportunity.go:24–46`). Scoring fields (`Score`, `Recommendation`, `ScoredAt`) are
zero/nil. `Selected == false`. `ProposalStatus == ""`.

#### Stage: Scored

**When:** `Selected == false` AND `ScoredAt != nil`

The Scorer has run. `Score` (`opportunity.go:49`), `ScoreReasoning`,
`Recommendation` (`opportunity.go:51`), `Requirements`, and `ScoredAt`
(`opportunity.go:53`) are all populated. `Selected == false`.

This stage carries a **Recommendation sub-state**:

| Sub-state | Condition | Meaning |
|-----------|-----------|---------|
| Bid candidate | `Recommendation == "BID"` | Score ≥ 0.60; proceed to selection |
| No-bid | `Recommendation == "NO_BID"` | Score < 0.40; terminal, skip |
| Needs decision | `Recommendation == "REVIEW"` | 0.40 ≤ Score < 0.60; uncertain, human decides |

The dashboard should surface the sub-state as a visual indicator within the Scored
column. **The "REVIEW" sub-state is NOT the same as the "Awaiting Human Review"
stage** — see ambiguity note #1 below.

#### Stage: Selected

**When:** `Selected == true` AND `ProposalStatus == ""`

A human has set `Selected = true` (`opportunity.go:56`) on this opportunity, but the
Manager (`internal/manager`) has not yet started processing it. `SelectedAt`
(`opportunity.go:57`) is populated.

#### Stage: In Proposal

**When:** `Selected == true` AND `ProposalStatus != ""`
AND the stage is NOT Finalized and NOT Awaiting Human Review
(i.e., `ProposalStatus` does not equal `"final-review:ready_to_submit"`
and does not have suffix `":needs_human"`)

The Manager is processing this opportunity through the Zone 2 chain
(Outline → Writer → Final Review). Common `ProposalStatus` values at this stage:

| `ProposalStatus` value | Meaning |
|------------------------|---------|
| `"outline:success"` | Outline complete; Writer queued |
| `"writer:success"` | Draft complete; Final Review queued |
| `"outline:failed"` | Outline stage failed (system error) |
| `"writer:failed"` | Writer stage failed (system error) |
| `"final-review:failed"` | Final Review stage failed (system error) |

The `":failed"` values remain in "In Proposal" (not "Awaiting Human Review") because
`StatusFailed` indicates an agent error, not a blocker requiring human input. The
dashboard should display a failed badge within the In Proposal column.

#### Stage: Awaiting Human Review

**When:** `Selected == true` AND `strings.HasSuffix(ProposalStatus, ":needs_human")`

A Zone 2 agent returned `agent.StatusNeedsHuman` (`result.go:28`). The Manager
persisted `ProposalStatus = "<stage>:needs_human"` and halted the chain. A human
must intervene before the pipeline can continue.

Common `ProposalStatus` values at this stage:

| `ProposalStatus` value | Who blocked | Typical reason |
|------------------------|-------------|----------------|
| `"outline:needs_human"` | Outline agent | Ambiguous requirements |
| `"writer:needs_human"` | Writer agent | Conflicting inputs |
| `"final-review:needs_human"` | Final Review agent | Failed quality checks (deadline, missing sections, page limit) |

#### Stage: Finalized

**When:** `Selected == true` AND `ProposalStatus == "final-review:ready_to_submit"`

The Final Review agent returned `agent.StatusReadyToSubmit` (`result.go:32`). The
proposal has passed all automated quality checks and is ready for the human to
review and submit. The Manager never auto-submits (`manager.go:124–125`). This is
the terminal pre-submission state; no "submitted" field exists in the current schema
(Phase 3+ consideration).

---

### Ambiguous and overlapping cases

#### Case 1 — `Recommendation == "REVIEW"` with `Selected == false`

**Condition:** `ScoredAt != nil` AND `Recommendation == "REVIEW"` AND
`Selected == false`

**Resolution:** Map to **Scored** (sub-state "Needs decision"), NOT to "Awaiting
Human Review".

"Awaiting Human Review" is reserved for in-flight proposals that have been selected
and then hit a `needs_human` blocker. The bid/no-bid REVIEW uncertainty is a
scoring-phase concern. The dashboard shows a distinct "Needs decision" badge within
the Scored column, making the required human action clear without conflating it with
the proposal-pipeline blocker.

#### Case 2 — `ProposalStatus` ends with `":failed"`

**Condition:** `Selected == true` AND `ProposalStatus` ends with `":failed"` (e.g.,
`"outline:failed"`, `"writer:failed"`, `"final-review:failed"`)

**Resolution:** Map to **In Proposal** with a "failed" badge. These are system
errors, not human-intervention requests. They are operationally distinct from
`":needs_human"` (which requires human domain judgment) and require a developer or
operator to diagnose and potentially re-trigger the stage.

#### Case 3 — `Selected == false` with `ProposalStatus != ""`

**Condition:** `ScoredAt != nil` AND `Selected == false` AND `ProposalStatus != ""`

This should never occur: the Manager only runs after `Selected` is set to `true`,
and neither Hunter nor Scorer sets `ProposalStatus`. If observed, it indicates a
data-integrity anomaly (e.g., manual store edit, migration bug).

**Resolution:** Treat `Selected == false` as authoritative. Apply the normal rules:
if `ScoredAt != nil` → **Scored**; otherwise → **Hunted**. `ProposalStatus` is
ignored. The dashboard logs a warning at startup when it encounters this condition.

#### Case 4 — `ScoredAt != nil` with `Recommendation == ""`

**Condition:** `ScoredAt != nil` AND `Selected == false` AND `Recommendation == ""`

The Scorer ran but did not populate `Recommendation` (e.g., an older record written
before the field existed, or a partial write).

**Resolution:** Map to **Scored** with sub-state "Unknown". The dashboard renders
this as an anomaly badge within the Scored column. The `Score` float64 value remains
usable; the empty recommendation is flagged for operator attention.

---

## Planned files

The following file layout defines the full dashboard implementation across waves.
Each wave is a separate GitHub Issue. This table is the shared map.

### Wave 1 — Server skeleton (Phase 1)

| File | Purpose |
|------|---------|
| `cmd/dashboard/main.go` | Binary entry point: parse config, wire store, start server |
| `internal/dashboard/doc.go` | Package documentation |
| `internal/dashboard/stage.go` | `Stage` enum, `DeriveStage()`, `SubState()` |
| `internal/dashboard/stage_test.go` | Unit tests for all derivation rules and ambiguity cases |
| `internal/dashboard/server.go` | HTTP mux, middleware, graceful shutdown |
| `internal/dashboard/server_test.go` | Integration tests with in-memory store stub |

### Wave 2 — Core endpoints (Phase 1)

| File | Purpose |
|------|---------|
| `internal/dashboard/handler.go` | `handleList`, `handleDetail` HTTP handlers |
| `internal/dashboard/handler_test.go` | Handler-level tests (JSON response shape, filter params) |

### Wave 3 — UI layer (Phase 2)

| File | Purpose |
|------|---------|
| `internal/dashboard/templates/layout.html` | Base HTML layout |
| `internal/dashboard/templates/list.html` | Stage-grouped opportunity list |
| `internal/dashboard/templates/detail.html` | Single opportunity detail view |

### Wave 4 — Write actions (Phase 2)

| File | Purpose |
|------|---------|
| `internal/dashboard/action.go` | `handleSelect` — sets `Selected = true` via `store.Save` |
| `internal/dashboard/action_test.go` | Tests for write actions |

**Note:** Wave 4 is the ONLY wave that calls `store.Save`. All prior waves are
strictly read-only. Wave 4 introduces a new constraint: the handler must validate
that the opportunity is in the **Scored/BID** sub-state before allowing selection,
to prevent selecting unscored or NO_BID opportunities.

---

## API endpoints (Wave 1–2 target)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/opportunities` | List all opportunities; supports `?stage=` filter |
| `GET` | `/api/v1/opportunities/{id}` | Single opportunity detail with derived stage |
| `GET` | `/healthz` | Liveness probe; returns `{"ok":true}` |

The `?stage=` query parameter maps to the six stage names:
`hunted`, `scored`, `selected`, `in_proposal`, `awaiting_human_review`, `finalized`.
Filtering is applied server-side after `store.List(ctx, nil)` in Phase 1.

---

## Security and trust model

- The dashboard binary is an **internal operations tool**, not a public endpoint.
  No authentication is added in Phase 1 (consistent with the single-user trust
  model noted in `ARCHITECTURE.md:178`).
- The server never proxies or forwards SAM.gov data to external services.
- No secrets are needed: the dashboard only reads from the local JSON store.
- If deployed to GCP in Phase 2+, authentication via Identity-Aware Proxy (IAP)
  is the preferred approach (consistent with the GCP-native stack).

---

## Relationship to ARCHITECTURE.md

This document supplements `ARCHITECTURE.md`. The two-zone diagram in
`ARCHITECTURE.md:57–108` shows the dashboard as the "Opportunity Queue" consumer at
the end of Zone 1. The `cmd/dashboard` binary is the concrete implementation of
that queue consumer. It does not alter the two-zone architecture; it adds a
read-only window into it.

The constraint "Human selects opportunity from Queue, triggering Manager agent"
(`ARCHITECTURE.md:178`) remains unchanged. The Wave 1–3 dashboard is a read-only
viewer; the selection mechanism is a separate write action added in Wave 4.

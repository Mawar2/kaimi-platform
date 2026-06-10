# PIPELINE.md — Kaimi · Queue rules, the Submitted archive & BD reporting

> Read AFTER `INTENT.md`. This documents the pipeline-lifecycle features added
> after the original handoff: how opportunities leave the queue, the
> **Submitted** tab (the third nav destination), award outcomes, and the
> BD-report export. Reference implementation: `kaimi/app-submitted.jsx` +
> the app shells (`Kaimi App.html`, `Kaimi Desktop.html`).

---

## The full lifecycle (mental model)

```
SAM.gov hunt → Opportunities (queue) → [human: Select to pursue]
            → Proposals (active work) → [human review gate] → Submit
            → Submitted (archive)     → [award outcome: pending → won/lost]
```

A record lives in **exactly one** of the three tabs at a time. That's the
core rule — the tabs are *stages*, not *views over the same list*.

---

## 1. Opportunities queue — self-cleaning

- An opportunity **disappears from the Opportunities tab** the moment it is
  pursued: either the user clicked "Select to pursue" this session, **or** an
  active proposal already references it (`proposal.oppId === opp.id`).
- The sidebar count is the count of the **visible** (still-actionable) queue.
- Intent: the queue is a to-triage inbox. Anything being worked is no longer
  a decision waiting to happen, so showing it there is noise.
- In the prototype this is a derived filter in the app shell
  (`visibleOpps = KAIMI_OPPS.filter(o => !pursued.has(o.id) && !proposals.some(p => p.oppId === o.id))`).
  In production this should be a server-side queue state (`new | pursued |
  dismissed`), not client filtering — but the UX contract is the same.

## 2. Proposals — active work only

- A proposal that has been **submitted leaves the Proposals tab** and appears
  in Submitted (in the prototype: status `submitted` is filtered out of the
  active list and mapped into the archive as a "Just now · Pending award" row).
- Proposals now carry `oppId`, `sol`, `value`, `valueNum` so the link back to
  the originating opportunity and the dollar value survive the whole lifecycle.

## 3. Submitted — the archive + pipeline value

The third nav item ("Submitted", archive-box icon, count badge). Purpose:
**what went out the door, what it's worth, and everything we produced for it.**

### Header stats (live, recomputed from data)
- **$ awaiting award** — sum of `valueNum` over `status === "pending"`.
- **$ won** — sum over `status === "won"`.
- **Total submitted** — all time count.
- Dollar formatting: `valueNum` is in $M; display `$3.2M` / `$850K`.

### Rows
- Search (title / agency / SOL#) + status filter (All / Pending award / Won /
  Not awarded). Search box: `.searchbox`; segmented filter: `.seg`.
- Each row expands (accordion, one open at a time) to show:
  - the award note ("Award expected Jul 2026", "Awarded Jan 9, 2026", …)
  - **Reference documents** — final technical volume, compliance matrix,
    price volume, diagrams, debrief notes, the solicitation PDF. These are
    the artifacts the agent team produced; in production they link to real
    stored files. This is deliberate: past proposals are reusable assets
    (past performance, boilerplate, diagrams), so the archive is a library,
    not a graveyard.
  - the **Outcome** control (below).

### Award outcomes — recorded in the expanded row
- Segmented control: **Pending / Won / Not awarded**. Flipping it updates the
  badge, the header stats, the filters, and the export — immediately.
- Attribution note flips to "Marked won/not awarded by you · just now".
- Product intent: Kaimi watches SAM.gov for award notices and may set the
  outcome automatically, but **the human can always record or override it
  here**. Outcome changes should be auditable (who, when) in production.
- Prototype state: an `outcomes` override map layered over the synced data;
  production: a real field on the submission record.

## 4. Export BD report (CSV)

Toolbar button on Submitted → dialog (`ExportDialog`). Purpose: a business
owner planning around BD metrics can pull, for any period, **how many
proposals went out, total pipeline value, amount won, win rate**.

- **Periods are federal fiscal quarters** (FY starts Oct 1 — FY26 Q1 =
  Oct–Dec 2025). BD planning runs on the government's calendar, not the
  calendar year. Helper: `fyQuarter(date)`.
- **Presets:** This quarter · FY-to-date · All time.
- **Custom range:** a strip of quarter chips; click a start quarter, then an
  end quarter → the span highlights (e.g. FY26 Q1–Q3, or across fiscal years).
  One click = single quarter. State: `[startQI, endQI]` where
  `QI = fy*4 + (q-1)` makes ranges integer comparisons.
- **Only quarters that actually contain submissions are offered** — empty
  quarters show no chip, presets whose span would be empty are hidden, and if
  the current quarter has no data the dialog defaults to the latest quarter
  that does. Never present a period that exports an empty report.
- Dialog previews the 4 headline metrics for the selected range before export.
- **CSV shape:** summary block (report label, generated date, submitted count,
  total value $M, won value $M, win rate over *decided* outcomes only) +
  blank line + one row per proposal: Title, Agency, Solicitation, Submitted,
  FY quarter, Value ($M), Status. Filename: `kaimi-bd-report-<range>.csv`.
- Win rate denominator is won + lost (decided), **not** total submitted —
  pending awards don't count against you.

## 5. Draft-editor parity on web

"Edit the draft" at the review gate opens the same section-structured
working-draft editor on web (`kaimi/desktop-editor.jsx` + `kaimi/editor.css`,
full-page route `editor`) that desktop has. Back returns to the review
workspace. All INTENT.md / DESKTOP.md editor rules apply unchanged.

---

## Data model additions (mock → production mapping)

| Mock field | Meaning |
|---|---|
| `opp.valueNum`, `proposal.valueNum` | dollar value in $M, used for all sums |
| `proposal.oppId` | link to originating opportunity (drives queue removal) |
| `proposal.sol` | solicitation number, carried through to the archive |
| `KAIMI_SUBMITTED[]` | archive records: `status: pending\|won\|lost`, `submitted` date, `award` note, `docs[]` |

## Errors to avoid

1. **Showing pursued/in-flight opportunities in the queue** — the queue is
   only undecided items.
2. **Counting pending awards in the win rate** — denominator is decided only.
3. **Calendar-year quarters** — federal FY quarters, always labeled `FY26 Q3`.
4. **Offering empty periods in the export picker** — only quarters with data.
5. **Losing documents at submission** — the archive must retain every
   artifact (volumes, matrices, diagrams, debriefs, solicitation).
6. **Making outcome changes silent** — attribute and timestamp them; allow
   human override of any auto-detected award status.
7. **Two sources of truth for stats** — header stats, filters, and the export
   must all derive from the same records, including outcome overrides.

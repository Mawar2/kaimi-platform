# ADR-001 — Desktop dashboard stack: Wails v2 (Go) over Tauri / Electron

**Status:** Accepted — approved by Malik 2026-06-09 (on [issue #137](https://github.com/Mawar2/Kaimi/issues/137) / PR #144). Unblocks #138.
**Date:** 2026-06-10
**Deciders:** Malik (approver), implementing session (author)
**Supersedes:** none
**Related:** Epic [#136](https://github.com/Mawar2/Kaimi/issues/136); tickets [#138](https://github.com/Mawar2/Kaimi/issues/138) (scaffold, blocked by this), [#139](https://github.com/Mawar2/Kaimi/issues/139) (parity), [#140](https://github.com/Mawar2/Kaimi/issues/140) (sync, design-only); design handoff `DESKTOP.md`.

> ADR format: [Michael Nygard](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions) (Context · Decision · Consequences). Per ARCHITECTURE.md, ADRs are append-only. This file is placed under `docs/desktop/` because #137's acceptance criteria name that path; ARCHITECTURE.md's ADR pointer says `docs/adr/`. That divergence is the ticket's call, not this ADR's to self-authorize — if you'd rather consolidate ADRs under `docs/adr/`, say so and I'll move it.
>
> **This revision (rev 2) corrects a material error in rev 1** flagged by an independent adversarial review: rev 1 claimed the web dashboard "already renders the Go design system." It does not (see Context §1). The data/logic reuse is real; the design-system *rendering* is net-new Go work. The recommendation is unchanged; its justification is now accurate.

---

## Context

Malik decided (2026-06-09) that Kaimi gets a **desktop dashboard** for Windows and macOS, usable **offline** while working on proposals, alongside the existing web dashboard. Epic #136 is the handoff brief; this ADR is the design-eagerly gate it requires before any scaffold code: choosing the wrong shell here is expensive to unwind once views and build/signing pipelines sit on top of it.

**Phase note:** CLAUDE.md's current directive banner (2026-06-09) **retires the "Phase 0 only" scope lock** — the full pipeline is built and deployed, and "web + desktop dashboards in active development" is explicitly in scope, driving to the **Google AI Agents Challenge submission (June 11, 2026)**. So this desktop work is in-scope by directive; ARCHITECTURE.md's lingering "Phase 0 only" language is superseded by that banner. The June 11 horizon also shapes the recommendation: the realistic near-term deliverable is **read-only dashboard parity in a branded offline-capable shell**, not the full onboarding/editor product the design depicts.

### What is actually reusable today (verified against the code)

1. **Real, shipping reuse — data and view logic (Go):**
   - `internal/store` — the `Store` interface (`Save`/`Get`/`List`/`Delete`, all `ctx`-aware) with a file-backed `NewJSONStore(basePath)`. Offline reads fall out of this: the store *is* a local JSON directory.
   - `internal/opportunity` — the shared `Opportunity` schema (single source of truth; must not be forked for desktop).
   - `internal/dashboard` — `DeriveStage` (deterministic stage derivation) and `NewService(store)` → `List`/`Get`. **The live web dashboard already uses these**: `cmd/dashboard/main.go`'s `handleOverview` calls `svc.List(...)` and groups by derived `Stage`. This is genuine, exercised reuse a desktop Go backend gets by importing the same package.

2. **Exists in Go but NOT yet wired into the live dashboard — the design system:**
   - `StyleTag()` (tokens + `ui.css`, #132), `StatusBadge`/`RecommendationPill`/`DeadlinePill`/`FitRing`/`MetaTag`, and `brand.go`'s Kai-wave assets (#126) are implemented **and tested**, but **nothing renders them**: the live dashboard serves placeholder templates (`cmd/dashboard/templates/layout.html`/`overview.html`) and a second, inline `sans-serif` template in `internal/dashboard/handler.go` (mounted at `/opportunities`). A grep of `cmd/` and `handler.go` for `StyleTag`/the components returns **zero** uses. So adopting the design system is **pending Go work for the web dashboard too** (the known #125 placeholder-styling follow-up). The desktop does not "inherit" a styled UI for free — it would do that wiring in Go, and that work is **shared with**, not duplicated against, the web dashboard.

3. **Two render paths exist** (`handleOverview` at `/` via embedded templates; `dashboard.NewHandler` at `/opportunities` via its own inline template). "Parity" must target the `Service`-backed data path (item 1), and ideally the two web renderers get consolidated as the Go design system is adopted. Desktop should not pick up the placeholder markup.

4. **No write path / offline queue exists yet.** `internal/dashboard.Service` is read-only by design (it never calls `Save`/`Delete`). The offline *writes* the product needs — draft edits, approve/request-changes queued for replay (DESKTOP.md) — have **no home in the current code** and are unbuilt. Offline-first is therefore "nearly free" **for reads only**; the durable action queue is load-bearing, net-new, and lands later (#140 is its design gate).

### Team and distribution constraints

5. **The team is Go-centric; "legible Go" is a hard requirement** (CLAUDE.md/ARCHITECTURE.md, twice): two people review and learn from this code, one newer to Go. A desktop backend in a *different* language (Node/Rust) splits the review surface and means the reusable Go logic (item 1) must be reached across a process/language boundary rather than imported.

6. **Single-binary distribution is an established repo principle** (a stated reason Go was chosen).

7. **The design handoff recommends "Electron or Tauri"** (`DESKTOP.md`, README). That guidance assumed a **greenfield JavaScript/React frontend** (the prototypes are React-via-Babel) and did **not** have the Go reuse context. The design's actual architectural intent is narrower and is what binds: *"the same three-surface app wrapped in a desktop shell"* — an OS-webview window with branded chrome, keychain, and offline behavior. **Wails satisfies that intent**, using the same OS-webview model as Tauri but with a Go backend.

8. **Offline-first is a client property, not a pipeline property.** The nightly SAM.gov hunt and live agent runs stay online-only; "Select to pursue" needs the agent runtime and is disabled offline. ARCHITECTURE.md's "Offline/air-gapped operation" not-supported bullet is about the *pipeline* and is reconciled in the companion edit so it isn't read as forbidding this client.

---

## Decision

**Adopt Wails v2 (Go) as the desktop shell.** The desktop app is a Go binary that imports `internal/store`, `internal/opportunity`, and `internal/dashboard` directly, wrapped in a frameless OS-webview window (WebView2 on Windows, WKWebView on macOS) with a custom branded title bar. Use the **stable v2 line**, not v3 (still pre-release — revisit in a later ADR; nothing here precludes a v2→v3 migration, though see the longevity risk below).

### Frontend strategy (the consequential sub-decision — decided now, not deferred)

The honest tension: the design's **showcase surfaces are unavoidably interactive** — a frameless draggable title bar with a live sync pill, a six-step stateful onboarding wizard, and a section-structured `contenteditable` draft editor with autosave (`desktop-onboarding.jsx`, `desktop-editor.jsx`). Server-rendered Go HTML with a meta-refresh **cannot** carry those. So the JS-frontend question is not deferrable in principle — what *is* legitimately deferrable is **building** those surfaces, because they are not in #138/#139 (read-only parity) and their backends (review gate, editor data model) are themselves later tickets.

Decision, in two tiers:

- **Now (#138/#139 — read-only parity, the June-11 deliverable):** render the parity views from `internal/dashboard` (`Service` + `DeriveStage`) **and wire in the Go design system** (`StyleTag()` + components) — the same wiring the web dashboard still owes (#125). For static, read-only views this is straightforward server-rendered HTML inside the Wails webview, with **Figtree + IBM Plex Mono bundled locally** (embedded, not fetched from Google Fonts — fetching breaks offline; ux-spec.md explicitly anticipates desktop as the "surface that may ship external assets"). Minimal new tech, maximal Go reuse, demoable fast.
- **Later (onboarding, the offline editor — separate tickets, post-submission):** these get a **Wails Vite frontend** (a small JS framework) that calls the Go backend via Wails bindings and **shares `kaimi/tokens.css` (= the same values as `tokens.go`) as the single token source.** Recorded in a follow-up ADR when those tickets exist. Choosing Wails does **not** lock us out of a JS frontend — Wails' normal mode *is* a JS frontend; we simply aren't standing one up before it's needed.

### Out of scope for this ADR

Scaffold code, UI work, the durable offline queue, keychain/OAuth integration, installers, code-signing/notarization, auto-update wiring, and the local↔GCS sync design (#140). This ADR fixes only the shell and the two-tier frontend strategy, and surfaces the risks the shell must be able to discharge (below).

---

## Options considered

Weights reflect Kaimi's constraints (reuse + team language dominate).

| Criterion (weight) | **Wails v2 (Go)** | Tauri (Rust) | Electron (Node) | Any shell + local Go HTTP server |
|---|---|---|---|---|
| Reuse `store`/`opportunity`/`dashboard` **logic** (high) | ✅ direct in-process import | ❌ Rust can't import Go | ❌ Node can't import Go | ✅ via localhost to the existing Go server |
| Team language / "legible Go" (high) | ✅ Go backend | ❌ adds Rust | ⚠️ adds Node | ⚠️ shell still Rust/Node + a Go server to supervise |
| Offline JSON-store **reads** (high) | ✅ in-process | ⚠️ sidecar/re-impl | ⚠️ sidecar/re-impl | ✅ server reads it |
| Single binary (repo principle) | ✅ one Go binary | ✅ small native binary | ❌ bundled Chromium (~100–150 MB) | ❌ ships shell **+** a second Go process |
| Interactive surfaces (onboarding/editor) | ⚠️ Wails Vite frontend (later) | ✅ JS frontend native | ✅ JS frontend native, biggest ecosystem | depends on shell |
| Ecosystem / auto-update / keychain plugins (medium→high for 2-person team) | ⚠️ thinner; more hand-rolled | ⚠️ maturing; built-in updater | ✅ most mature (electron-updater, etc.) | n/a (shell-dependent) |
| Process model / ops simplicity | ✅ single process | ✅ single process | ✅ single process | ❌ two processes, port mgmt, lifecycle/IPC |

### Why not Tauri
Closest technical analogue (OS webview, small binary) and a fine pick for a JS-frontend team. But its **Rust** backend can't import `internal/*`, so the reusable Go logic must be re-implemented in Rust (a second source of truth — the drift ARCHITECTURE.md warns against) or reached via a Go sidecar (extra process). Either way the "legible Go" advantage is lost. Tauri's built-in updater is a genuine point in its favor (see risks).

### Why not Electron
Most mature ecosystem and the strongest auto-update/keychain story, and Node is more approachable than Rust. But it can't reuse the Go packages, adds a second runtime to a Go-learning team, and ships a full Chromium per app — against the single-binary principle.

### Why not "any shell + local Go HTTP server" (the strongest counter to the reuse thesis)
This is the honest rebuttal to "only Wails reuses Go": the dashboard **already is** an HTTP server, so an Electron/Tauri shell could point a webview at a localhost Go process and reuse *all* the Go logic too. True — and it shows the reuse advantage is about **how**, not **whether**. Wails' edge over this is operational: **one process, one binary, no port allocation/collision handling, no second-process lifecycle/crash-supervision, no IPC/CORS surface** — and the shell glue is Go, the team's language. For a two-person team on a tight timeline, collapsing the sidecar into the shell is the meaningful win. (Note: even Wails runs an internal asset/bindings bridge — but we don't own or operate it as a separate process.) This option stays on the table as the fallback if Wails' interactive-frontend or distribution story disappoints.

---

## Risks the shell must be able to discharge (raised by adversarial review)

These are **not** built in #138/#139, but the shell choice is only valid if it can carry them. Wails can, with more hand-rolling than Electron:

- **OS keychain for secrets (security-sensitive — flagged per CLAUDE.md).** `DESKTOP.md` hard-requires storing OAuth tokens **and** the Kaimi license key in Keychain/Credential Manager, never plaintext. Wails has **no built-in keychain API**; this needs a CGo-based library (e.g. `github.com/zalando/go-keyring`) added per the dependency rule, with notarization/entitlement implications on macOS. Must be its own security-reviewed ticket.
- **Durable offline action queue.** Must survive quit/reopen (DESKTOP.md). No write/queue primitive exists today (`Service` is read-only). This is the load-bearing offline component; #140 designs it.
- **OAuth via the system browser** (loopback redirect). Wails supports it but it's hand-rolled (local listener + redirect handling). Deferred with onboarding.
- **Auto-update.** Wails v2 has **no first-class updater** (unlike Electron's electron-updater or Tauri's built-in updater). For a licensed app shipped to a customer org this is not optional long-term; budget a dedicated solution (e.g. a self-hosted update feed) or treat it as a known gap. **This is the strongest single argument for Electron/Tauri** and is accepted with eyes open.
- **WebView2 ↔ WKWebView engine divergence** for the *interactive* surfaces (drawers, `contenteditable`, `-webkit-app-region`). Low exposure for the read-only parity views; real for the later editor — test both engines.

---

## Dependency justification (per CLAUDE.md dependency rule)

Adopting Wails v2 introduces `github.com/wailsapp/wails/v2` (Go module + build CLI).

- **Why a dependency at all / why not stdlib:** there is no stdlib path to a native OS window hosting a webview with frameless chrome, drag regions, and native menus. The realistic options are all third-party desktop frameworks; this ADR selects among them.
- **Why this one:** it is the only mainstream option whose backend is **Go**, which is why it wins on reuse-in-process, single binary, and team language. It avoids a Node/Rust toolchain and a supervised sidecar process.
- **Pinning:** added in **#138**, not here — this ticket is code-free. #138 runs `go get github.com/wailsapp/wails/v2@<latest-stable-v2>` then `go mod tidy`, records the exact version on that ticket, and reports the `go.mod`/`go.sum` delta. **Anticipated companion deps** (later tickets, justified then): a keychain library (`go-keyring` or equivalent) and, eventually, an update mechanism.
- **CONVENTIONS.md gap:** CLAUDE.md references a `CONVENTIONS.md` that is **absent from the tree**. The desktop scaffold (#138) introduces a new top-level layout (`cmd/desktop` + a Wails project); per CLAUDE.md's "new pattern → update CONVENTIONS.md" rule, #138 should restore/extend CONVENTIONS.md or document the layout in `docs/desktop/` until it exists.

---

## Consequences

**Positive**
- The **logic** reuse is proven and exercised: desktop imports the same `Store`, `Opportunity`, `Service`, and `DeriveStage` the live web dashboard runs on. No schema fork, no second stage-derivation, no duplicated store access.
- Adopting the Go design system on desktop is the **same work the web dashboard still owes** (#125) — done once in Go, shared, not duplicated.
- Offline **reads** are nearly free (local JSON store + read-only `Service`).
- One language for the whole system; a single Go binary per OS.
- Fastest credible path to a branded, offline-capable read-only desktop dashboard for the June 11 submission.

**Negative / accepted**
- The design system is **not yet rendered anywhere**; desktop parity includes writing that Go rendering layer (shared with web, but real work — don't bill it as free).
- The interactive showcase surfaces (onboarding, offline editor) require a **Wails Vite frontend later** (follow-up ADR) and are not in this window.
- Wails carries a thinner ecosystem, a hand-rolled **keychain** path, a **WebView2 runtime** dependency on Windows, cross-engine testing, the macOS **notarization** tax (framework-independent), and — most pointedly — **no first-class auto-update**. We accept these; auto-update and keychain get dedicated tickets.
- **Longevity:** Wails v2 maintenance will taper as v3 matures; starting a multi-year app on a soon-legacy major version is a real (accepted) risk, mitigated by v3 being a migration, not a rewrite, and by the fallback "local Go server + any shell" option.

**Follow-ups this unblocks / requires**
- ARCHITECTURE.md updated in the same change set: desktop client added to the component map; client-offline-vs-pipeline-online distinction recorded; stale `Store` signature corrected; "Offline/air-gapped" bullet clarified; ADR-001 referenced.
- #138 proceeds **only after Malik approves this ADR on #137**: adds the Wails dep (pinned, on the ticket), `cmd/desktop` + `doc.go`, boots a window on Windows, reads a local store via `internal/store`, lists opportunities via `internal/dashboard.DeriveStage`. **Scope it to read-only parity of existing views.**
- A future ADR decides the Wails Vite frontend when the onboarding/editor tickets land. Keychain, durable offline queue (#140), and auto-update are their own tickets.

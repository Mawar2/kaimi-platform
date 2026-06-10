# Dashboard Design-Consistency Log

**Last updated:** 2026-06-10 · **Owner:** Malik (malik@bluemetatech.com)

Running checklist for driving the Kaimi web dashboard (`internal/dashboard`) to full
visual consistency with the locked design handoff. One surface at a time, audited and
verified **in a real browser** (gstack-browse) against
`design-handoff/Kaimi-handoff/kaimi/project/design_handoff_kaimi/screenshots/`.

> Note: each surface lands as its own PR off `main`, so this shared log is created/edited
> by more than one open PR at a time. Each PR's version is a strict superset of the prior,
> so the land-time resolution is simply "take the most complete version" (keep every
> surface entry). The independent code diffs do not conflict.

Hard rules (from `hackathon/design-consistency-agent.txt`): define design values once and
reuse (`StyleTag()` tokens + `components.go` helpers — never re-hardcode per page); no
external assets/fonts (inline SVG + base64 OK); amber `#E8870E`/`#FFF3E0` is reserved
app-wide for "a human is needed"; any layout change updates `docs/dashboard/ux-spec.md` in
the same change; ticketed + TDD + `make all` green + one PR per surface + a human merges.

## Verification harness (how to reproduce)

```
# in the isolated worktree C:\Users\Owner\Kaimi-design
go run ./cmd/pipeline --mode=cached --store-path=./design-store   # seed render data
go build -o bin/dashboard.exe ./cmd/dashboard
bin\dashboard.exe --store=.\design-store --port=8901             # NB: kill stale PID first
# browser: gstack-browse goto http://127.0.0.1:8901/ → screenshot → diff vs screenshots/
```

## Surface status

| Surface | Route | Reference shot | Audited | Fixed | Browser-verified | 2× clean design-review |
|---|---|---|---|---|---|---|
| Triage | `/` | `01-opportunities.png` | ✅ 06-10 | typography only | ✅ fonts | ☐ |
| Opportunity detail | `/opportunity/{id}` | `02-opportunity-drawer.png` | ✅ 06-10 | tokens (h2 + .kv) | ✅ no regression | ☐ |
| Proposals command | `/proposals` | `03-proposals-command.png` | ✅ 06-10 | card link reset | ✅ navy/no-underline | ☐ |
| Workspace | `/workspace/{id}` | `04/06/07-workspace*.png` | ☐ | ☐ | ☐ | ☐ |
| Shared chrome (header/nav/states/responsive) | all | — | partial | typography | ✅ fonts | ☐ |
| Component pass (all states) | — | `08-design-system*.png` | ☐ | ☐ | ☐ | ☐ |

## Iteration log

### 2026-06-10 — Global typography: self-host Figtree + Geist Mono (PR #203, issue #202)

StyleTag() declared `--font-sans: "Figtree"` / `--font-mono: "Geist Mono"` but embedded
**no `@font-face`**, so the served UI fell back to system fonts and drifted from the comps.
(`document.fonts.check('…Figtree')` lies — returns `true` when no matching `@font-face`
exists, meaning "nothing pending," not "installed.") Fix: self-host both as inline base64
`@font-face` data-URIs (self-hosting, not an external fetch). Mono = **Geist Mono** per the
design-system token order + Malik's call. Variable builds (non-standard token weights
420/430/550/650). Verified: `Figtree:loaded | Geist Mono:loaded`, 0 external font requests.

### 2026-06-10 — Detail surface: route re-hardcodes through tokens (PR #206, issue #205)

`/opportunity/{id}` re-hardcoded the title `<h2 style="font:700 21px/1.2 …">` (duplicating
`.dr-top h2`, sidestepping the designed `max-width:22ch`) and used `.kv`/`.detail-pre`
magic numbers. Fix: title styled solely by `.dr-top h2`; table → `--t-small` / `--s-2`/
`--s-3`. Corrected a stale ux-spec Non-Goals note (Select-to-pursue is implemented, #156).
Verified no regression: title 21px/700/22ch, no inline style; `.kv` cells 8px 12px / 13px.

### 2026-06-10 — Fix agent-avatar gradient ZgotmplZ (PR #218 / PR #219, issue #218)

Follow-up from the Workspace surface. The progress-state avatar
(`proposals_templates.go:290`) interpolates `{{.Agent.HueBG}}` (a `linear-gradient`) into a
style attribute; with `HueBG` typed `string`, html/template sanitized it to `ZgotmplZ`,
blanking the avatar background in the live-writer flow. Fix: type `agentIdentity.HueBG` as
`template.CSS` (static map constants — safe). That also let the gate handoff avatar (`:226`)
dedup through the `agents` map (define-once) instead of repeating the literal. Browser-verified:
gate avatar renders `linear-gradient(155deg,#67E0F4,#0EA5C4)`, **no `ZgotmplZ`**. TDD
`TestAgentGradientIsStyleSafe`; `make all`-green; lint clean.

### 2026-06-10 — Proposals command view: card link reset (PR pending, issue #207)

The whole proposal card is `<a class="pcard">`, but the shared shell link reset listed only
`a.nav-item, a.orow, a.artifact2 { text-decoration: none; color: inherit; }` — `a.pcard`
was missing, so cards rendered as default **underlined link-blue** (`.pc-ttl` computed
`rgb(0,0,238)`) instead of the designed navy non-underlined cards. Fix: add `a.pcard` to the
one shared reset (define once; covers every surface). Verified in a real browser: `.pc-ttl`
computes navy `--ink` (`rgb(10,27,61)`), `text-decoration: none`; populated `/proposals`
(Waiting on you / Agents working / Ready to submit) matches `03-proposals-command.png`.
TDD `TestProposalCardsResetLinkStyling`; `make all`-green; `golangci-lint` clean.

## Audit backlog (found while auditing; not yet ticketed/fixed)

- **Workspace** (`proposals_templates.go`) re-hardcodes agent-identity gradients + `#fff`
  surfaces (`:226,299-300,314,348,356`). Note: the agent gradients are **defined once** in
  the `agents` map (`proposals.go:30-33`) and mirror the handoff (which also hardcodes them
  inline; the `.kava` class defines only avatar shape). Vera's purple has no token, so route
  only the clearly-tokenizable ones (`#fff` → `--surface`; the success-green gradient) and
  the inline duplication at `:226`, in the Workspace iteration.
- **Shared chrome:** `sidebarMarkSVG` (inline brand SVG, `handler.go:100`) duplicates the
  brand mark; consider sourcing it from `brand.go` (`HeaderLockup`) in the shared-chrome pass.
- **Component coverage:** dedicated component pass — span every RecommendationPill, DeadlinePill
  urgency band, FitRing fit band, StatusBadge ProposalStatus (seed JSONs under design-store/queue/).
- **Design-review:** run `gstack-design-review` to two consecutive clean passes per surface
  (the END gate) once surfaces are individually fixed.

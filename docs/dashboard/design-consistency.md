# Dashboard Design-Consistency Log

**Last updated:** 2026-06-10 · **Owner:** Malik (malik@bluemetatech.com)

Running checklist for driving the Kaimi web dashboard (`internal/dashboard`) to full
visual consistency with the locked design handoff. One surface at a time, audited and
verified **in a real browser** (gstack-browse) against
`design-handoff/Kaimi-handoff/kaimi/project/design_handoff_kaimi/screenshots/`.

> Note: each surface lands as its own PR off `main`, so this shared log is touched by
> more than one open PR at a time — expect a trivial append-merge when they land in
> sequence (keep every surface's entry).

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

Gotcha logged: Windows locks a running `.exe`, so `go build` cannot overwrite a live
`dashboard.exe`. Always stop every listener on the port first (a zombie process will keep
serving the **old** bytes and silently mask your change). Confirm with
`curl -s http://127.0.0.1:8901/ | grep -c @font-face` and a byte-count delta.

## Surface status

| Surface | Route | Reference shot | Audited | Fixed | Browser-verified | 2× clean design-review |
|---|---|---|---|---|---|---|
| Triage | `/` | `01-opportunities.png` | ✅ 06-10 | typography only | ✅ fonts | ☐ |
| Opportunity detail | `/opportunity/{id}` | `02-opportunity-drawer.png` | ✅ 06-10 | tokens (h2 + .kv) | ✅ no regression | ☐ |
| Proposals command | `/proposals` | `03-proposals-command.png` | ☐ | ☐ | ☐ | ☐ |
| Workspace | `/workspace/{id}` | `04/06/07-workspace*.png` | ☐ | ☐ | ☐ | ☐ |
| Shared chrome (header/nav/states/responsive) | all | — | partial | typography | ✅ fonts | ☐ |
| Component pass (all states) | — | `08-design-system*.png` | ☐ | ☐ | ☐ | ☐ |

## Iteration log

### 2026-06-10 — Global typography: self-host Figtree + Geist Mono (PR #203, issue #202)

**Divergence (the headline one).** `StyleTag()` declared `--font-sans: "Figtree"` /
`--font-mono: "Geist Mono"` but embedded **no `@font-face`**, so the served UI fell back
to system fonts (Segoe UI on Windows) and drifted from the comps. Confirmed in a real
browser: `document.fonts.size === 0`, no `@font-face` in served HTML, fonts not in the
machine's font dirs. (Note: `document.fonts.check('…Figtree')` returns a misleading `true`
when no matching `@font-face` exists — it means "nothing pending," not "installed." Do not
trust it; check `document.fonts` membership and computed render instead.)

**Decision.** Self-host both faces as inline base64 `@font-face` data-URIs (self-hosting,
not an external fetch — honors the no-external-assets rule). Mono face = **Geist Mono**,
per the design-system token order and Malik's "match the design system as a requirement"
call; the handoff screenshots show IBM Plex Mono only because the comps loaded that
fallback. Variable builds chosen because the type tokens use non-standard weights
(420/430/550/650). SIL OFL, license files shipped.

**Verified (real browser, screenshot-diff verdict: PASS).** `document.fonts` →
`Figtree:loaded | Geist Mono:loaded`; H1 renders Figtree, NAICS mono renders Geist Mono;
**0** external font requests; no console errors; served page `+62.5KB`. `make all`-green.

### 2026-06-10 — Detail surface: route re-hardcodes through tokens (PR pending, issue #205)

**Divergence.** `/opportunity/{id}` is structurally faithful to ux-spec §View 2 (drawer
header rendered inline + `<h2>` full-record sections), but it re-hardcoded design values
per page: the title `<h2 style="font:700 21px/1.2 …">` **duplicated** the design system's
`.dr-top h2` rule (and the inline override sidestepped the designed `max-width:22ch`); the
page-local `.kv` / `.detail-pre` styles used magic numbers (`font-size:13.5px`,
`padding:0.4rem 0.7rem`, `padding:0.75rem`) instead of `--t-*` / `--s-*` tokens.

**Fix.** Title typography now comes from `.dr-top h2` (inline style removed). `.kv` table
text → `font: var(--t-small)`, padding → `var(--s-2) var(--s-3)`; `.detail-pre` padding →
`var(--s-3)`. The full-record table has no comp to match, so the token scale is its only
correct reference. Also corrected the stale ux-spec "Non-Goals" note (Select-to-pursue is
implemented, #156).

**Verified (real browser, no regression).** `.dr-top h2` computes to **21px / 700**,
maxWidth **22ch (265.74px)**, **no inline style attr**; `.kv td` padding **8px 12px**,
size **13px**, line-height **18.85px** (the designed 1.45 rhythm); no console errors.
TDD `TestDetailRoutesThroughDesignTokens` added; `make all`-green; `golangci-lint` clean.

### 2026-06-10 — Fix agent-avatar gradient ZgotmplZ (PR pending, issue #218)

Follow-up from the Workspace surface. The progress-state avatar
(`proposals_templates.go:290`) interpolates `{{.Agent.HueBG}}` (a `linear-gradient`) into a
style attribute; with `HueBG` typed `string`, html/template sanitized it to `ZgotmplZ`,
blanking the avatar background in the live-writer flow. Fix: type `agentIdentity.HueBG` as
`template.CSS` (static map constants — safe). That also let the gate handoff avatar (`:226`)
dedup through the `agents` map (define-once) instead of repeating the literal. Browser-verified:
gate avatar renders `linear-gradient(155deg,#67E0F4,#0EA5C4)`, **no `ZgotmplZ`**. TDD
`TestAgentGradientIsStyleSafe`; `make all`-green; lint clean.

## Audit backlog (found while auditing; not yet ticketed/fixed)

- **Proposals/Workspace** re-hardcode agent-identity gradients and `#fff` surfaces
  (`proposals.go:30-32`, `proposals_templates.go:226,299-300,314,348,356`). Route agent
  avatar colors through tokens/`components.go` in the Proposals/Workspace iteration.
- **Shared chrome:** `sidebarMarkSVG` (inline brand SVG in `handler.go:100`) duplicates the
  brand mark; consider sourcing it from `brand.go` (`HeaderLockup`) in the shared-chrome pass.
- **Component coverage:** the cached seed only spans BID rows; hand-add opportunity JSONs
  under `./design-store/queue/` covering every RecommendationPill (BID/NO_BID/REVIEW),
  DeadlinePill urgency band, FitRing fit band, and StatusBadge ProposalStatus for the
  dedicated component pass.
- **Design-review:** run `gstack-design-review` to two consecutive clean passes per surface
  (the END gate) once surfaces are individually fixed.

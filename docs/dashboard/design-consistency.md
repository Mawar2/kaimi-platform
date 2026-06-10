# Dashboard Design-Consistency Log

**Last updated:** 2026-06-10 · **Owner:** Malik (malik@bluemetatech.com)

Running checklist for driving the Kaimi web dashboard (`internal/dashboard`) to full
visual consistency with the locked design handoff. One surface at a time, audited and
verified **in a real browser** (gstack-browse) against
`design-handoff/Kaimi-handoff/kaimi/project/design_handoff_kaimi/screenshots/`.

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
| Opportunity detail | `/opportunity/{id}` | `02-opportunity-drawer*.png` | ☐ | ☐ | ☐ | ☐ |
| Proposals command | `/proposals` | `03-proposals-command.png` | ☐ | ☐ | ☐ | ☐ |
| Workspace | `/workspace/{id}` | `04/06/07-workspace*.png` | ☐ | ☐ | ☐ | ☐ |
| Shared chrome (header/nav/states/responsive) | all | — | partial | typography | ✅ fonts | ☐ |
| Component pass (all states) | — | `08-design-system*.png` | ☐ | ☐ | ☐ | ☐ |

## Iteration log

### 2026-06-10 — Global typography: self-host Figtree + Geist Mono

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

**Changes.** `internal/dashboard/fonts.go` (new — `//go:embed` + base64 `@font-face`),
`internal/dashboard/fonts/{figtree-variable.woff2,geist-mono-variable.woff2,*-OFL.txt}`,
`StyleTag()` prepends `fontFaceCSS`, stale "falls back to system fonts" comment corrected,
`tokens_test.go` extended (`TestStyleTagSelfHostsDesignedFonts`), `ux-spec.md` updated.

**Verified (real browser, screenshot-diff verdict: PASS).** `document.fonts` →
`Figtree:loaded | Geist Mono:loaded`; H1 renders Figtree, NAICS mono renders Geist Mono;
**0** external font requests; no console errors; served page `+62.5KB` (the embedded
faces). `make all`-equivalent green (module build + all package tests + `golangci-lint`
clean on `internal/dashboard`). Typography now matches the comps.

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

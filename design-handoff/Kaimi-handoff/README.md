# Handoff: Kaimi — Federal BD Pipeline App

## Read order
1. **`INTENT.md`** — how the product is supposed to work (zones, the editable working draft, the human gate). Read first.
2. **`README.md`** (this file) — visual/interaction spec for the web app, design tokens, components.
3. **`PIPELINE.md`** — queue rules, the **Submitted** archive tab, award outcomes, and the BD-report export (FY quarters).
4. **`DESKTOP.md`** — the cross-platform desktop product: branded window, onboarding, SSO + license key, offline-first sync, and the in-app draft editor.
5. **`ARCHITECTURE.md`, `WORKFLOW.md`** — the client's engineering contracts. Follow them.
6. **`screenshots/`** — visual checksums (01–09 web app & design system; 10–18 desktop & onboarding; 19+ Submitted archive & export).

## Overview
Kaimi ("the seeker") is BlueMeta's autonomous federal business-development product. Agents hunt SAM.gov opportunities nightly, score them against the company's capabilities, and draft proposals through a staged agent pipeline that **pauses exactly once for human review**. This handoff covers the production app design: a minimal, light-theme web app with three connected surfaces — the **Opportunities** queue (triage), the **Proposals** command view (supervision), and the **Workspace** (single-proposal human review).

It also includes the underlying design system (tokens + status vocabulary), the brand mark, and an earlier dark-theme "Focus" concept page for reference.

## About the Design Files
The files in this bundle are **design references created in HTML** — interactive prototypes showing the intended look and behavior. They are NOT production code to copy directly. The HTML uses React 18 via in-browser Babel with mock data; your task is to **recreate these designs in the target codebase's existing environment** (its framework, component library, routing, and data layer), following its established patterns. If no frontend environment exists yet, choose the stack that best fits the team (the prototypes map naturally to React + CSS custom properties). For the **desktop** target, use Electron or Tauri — see `DESKTOP.md`.

Engineering context from the client lives in `ARCHITECTURE.md` and `WORKFLOW.md` (included). Follow those contracts where they apply.

## Fidelity
**High-fidelity.** Colors, typography, spacing, radii, shadows, copy, and interactions are final design intent. Recreate the UI pixel-faithfully using the codebase's conventions. All visual values come from the token files (`kaimi/tokens.css`, `kaimi/ui.css`, `kaimi/app.css`) — treat those as the source of truth over this document if they ever disagree.

## App Structure
```
┌─────────────┬──────────────────────────────────────┐
│  Sidebar    │  Main (max-width 1080px, centered)   │
│  248px      │                                      │
│  fixed/     │  Route: opportunities | proposals    │
│  sticky     │         | workspace/:proposalId      │
└─────────────┴──────────────────────────────────────┘
```
- App shell: CSS grid `248px 1fr`, min-height 100vh. Page bg `#FBFCFE`; sidebar bg `#fff` with 1px right border `rgba(16,30,60,0.08)`.
- Under 860px: single column, sidebar collapses to a horizontal bar (user block and section headers hidden).
- Font: **Figtree** (Google Fonts; weights 400–800). Monospace for technical data (NAICS, SOL#, counts): **IBM Plex Mono**.

## Screens / Views

### 1. Sidebar
- **Logo**: 34px rounded-square (radius 10px) navy gradient tile (`linear-gradient(150deg, #1D4ED8 → #0A1B3D)`) containing the Kai-wave mark (see Brand). Wordmark "Kaimi" 18px/800, sub-label "THE SEEKER" 11px/500 uppercase, color `#94A3BE`.
- **Nav section label**: "PIPELINE" — 11px/600 uppercase, letter-spacing 0.1em, color `#94A3BE`.
- **Nav items** (buttons, full-width): 14px/550 text, 19px icons, padding 10px 11px, radius 11px. Rest: text `#475569`, icon `#94A3BE`. Hover: bg `#F4F7FC`. Active: bg `#EEF4FF`, text `#1A3FAE`, icon `#2563EB`, weight 600.
  - "Opportunities" — trailing count (mono 12px, `#94A3BE`) of queue size.
  - "Proposals" — trailing badge: if any proposal needs human review, an **amber pill** (`#E8870E` bg, white 11px/700 text, min-width 20px, pill radius) with the count; otherwise plain count.
- **User block** (bottom): 1px-bordered card, 32px initials avatar (bg `#DCE6FF`, text `#1A3FAE`), name 13px/600 + role 11.5px `#94A3BE`.

### 2. Opportunities (Triage queue)
Purpose: business owner scans the scored SAM.gov queue and decides what to pursue.
- **Page head**: eyebrow "TRIAGE" (12px/600 uppercase, letter-spacing 0.1em, cyan `#0EA5C4`); H1 "Opportunities" 30px/700, letter-spacing −0.02em; lead paragraph 15px `#5A6B86`.
- **Stat strip** (flex, gap 40px): big number 30px/700 + small unit label 15px `#94A3BE`; caption 13px `#5A6B86`. Stats: `N in queue` (from last night's run), `N new` (added today), `top fit score`.
- **Toolbar**: segmented filter (All / To pursue / Needs review) — pill container bg `#ECF1F9`, active segment white bg + shadow `0 1px 2px rgba(15,27,48,0.06)`; right-aligned sort button (white, 1px border, 34px tall): "Sort: Fit score".
- **Day group headers**: "NEW TODAY" / "EARLIER THIS WEEK" — 12px/600 uppercase `#94A3BE` with a hairline rule filling the remaining width.
- **Opportunity row** (white card, 1px border `rgba(16,30,60,0.08)`, radius 16px, padding 18px, 10px between rows; hover: border `#B9CEFF` + shadow + no transform; keyboard: `role="button"`, tabIndex 0, Enter/Space opens):
  - New-dot: 7px cyan `#22D3EE` dot with 3px halo `#E7FBFF` (hidden, not removed, when not new — keeps alignment).
  - **Fit ring** 46px (see Components).
  - Title 16px/600; meta line: agency 13px `#5A6B86` · NAICS in mono 12px `#94A3BE`.
  - Right cluster (flex, gap 18px): recommendation word ("BID" green `#15A06B` / "REVIEW" amber `#E8870E` / "NO BID" rose `#C2354A` — 12px/700 uppercase, nowrap), deadline pill, chevron `#94A3BE`.
- **Empty state**: centered 60px glyph tile (bg `#ECF1F9`), heading 18px, caption ≤38ch, "next hunt runs tonight at 02:00".

### 3. Opportunity Drawer (pursue decision)
Right-side overlay drawer, width 486px, full height, white; scrim `rgba(10,27,61,0.32)` + 2px blur. Slides in 28px (transform only, 360ms `cubic-bezier(0.16,1,0.3,1)`). **Esc closes; scrim click closes.**
- Header row: 34px close button (bordered square, back-arrow icon), recommendation label, deadline pill right-aligned.
- Top block: 92px fit ring with "FIT" sublabel; title 21px/700; sub-line `agency · contract value` (value 600 weight, ink); tag chips for `NAICS xxxxx` and `SOL# xxxxxxxx` (mono 12px on `#ECF1F9`).
- Section "WHY KAIMI SCORED THIS N" — bullet list, 6px cyan dot markers, 14px text `#475569`.
- Section "MUST-HAVE REQUIREMENTS" — checklist rows (bg `#FAFCFF`, 1px border, radius 11px): 20px icon tile, green check (`#15A06B` on `#E2F6EE`) or amber warn (`#E8870E` on `#FFF3E0`).
- Footer (top border): ghost link-button "View solicitation" (link icon); primary CTA **"Select to pursue"** — the cyan high-stakes button (see Buttons). If already pursued: disabled secondary button "✓ In your proposals".

### 4. Proposals (command view)
Purpose: "across everything, what needs me?"
- Head: eyebrow "FOCUS", H1 "Active proposals". Stats: `N in flight` (excludes submitted), `N agents` (sum of active agent counts), `N need you` (amber `#E8870E` number when > 0).
- **Sections** in fixed order, each with an uppercase label + count + hairline rule: **WAITING ON YOU** (amber label) → **AGENTS WORKING** → **READY TO SUBMIT** → **SUBMITTED**. Sections render only when non-empty.
- **Proposal card** (white, radius 16px, padding 20px 22px, gap 22px; keyboard accessible like rows):
  - Needs-review variant: border `rgba(232,135,14,~0.4)` and a left-fading amber wash `linear-gradient(90deg, #FFF3E0, #fff 38%)`.
  - Body: title 16px/600; sub 13px `#5A6B86`: `agency · status phrase` ("Tomás drafting now", "Paused 6 min ago", "Submitted just now").
  - **Mini pipeline** (186px column): 5 nodes (9px dots) joined by 30px × 3px segments. Done: green `#15A06B` (segments fill green up to current). Active: blue `#2563EB` dot with 3px `#E7EEFF` halo. Human: amber dot with `#FFF3E0` halo. Below: stage label 12.5px/600 ("Technical Writer · 3 agents", "Human Review" in amber, "Submitted to SAM.gov").
  - Right cluster: status chip — amber **"Needs you"** pill with hand icon · blue "● Working" with blinking dot (1.3s opacity pulse) · green "Submitted"/"Ready" badge — then deadline pill, chevron.

### 5. Workspace (single-proposal review)
Purpose: calm focus on one proposal; max-width 920px.
- Back link "← All proposals" (13px, `#5A6B86`).
- Head: 64px fit ring + title 27px/700 (max 26ch) + meta line: agency · deadline pill · status phrase.
- **Stage pipeline** (horizontal): 5 nodes, 44px circles (1.5px border `#DCE4F0`, white bg), 120px columns, connected by 2px lines (green when passed). Node states — done: green border/bg/icon; progress: blue + 4px halo, spinner icon rotating 2.4s linear; human: solid amber circle, white hand icon, `scale(1.05)`, amber glow shadow; pending: gray dot. Labels: name 12.5px/600 + state caption 10px uppercase ("DONE", "WORKING", "NEEDS YOU", "PENDING"). Stage names: Outline, Technical Writer, Human Review, Final Review, Submit.
- **Review card** (status = human; the product's centerpiece):
  - Container: radius 22px, border `rgba(232,135,14,~0.35)`, shadow, enters with a 12px translate-up spring (450ms; **transform only — never animate opacity**, and respect `prefers-reduced-motion`).
  - Header (amber→white gradient wash): amber pill "NEEDS YOU" with hand icon; H2 "Tomás is handing you the draft" 19px/700; caption with the prompt. Right: the **handoff motif** — agent avatar (42px, cyan gradient, initial "T") → amber arrow → amber "you" tile (42px, hand icon).
  - Body: "WHAT TOMÁS PRODUCED" section label (11px uppercase `#94A3BE`); summary 15.5px/1.6; artifact chips (doc icon blue, filename, mono meta like "18 pp"); **gap flag** — amber-tinted callout (bg `#FFF3E0`, 32px warn icon tile, title 14px/650 dark amber, detail 13px `#5A6B86`); "CHECK AGAINST CRITERIA" — 2-column grid (1 column < 720px) of check items, green check or amber warn icon tiles, label 13.5px/600 + optional note 12px.
  - Footer (bg `#FAFCFF`, top border): **"Approve & resume"** (green gradient CTA) + **"Request changes"** (white, amber border/text) + right-aligned note ≤30ch: "Approving resumes Vera's final pass. Requesting changes sends it back to Tomás."
- **Working state** (status = progress): calm card with the agent's 48px avatar (gradient by agent, animated working ring), "Noa/Tomás/Vera is working", role caption, one descriptive sentence.
- **Ready state** (status = done): green-tinted card, check tile, "Package ready to submit", compliance summary, **"Submit to SAM.gov"** cyan CTA.
- **Submitted state**: quiet card — "Submitted to SAM.gov", "Confirmation logged · the agents stand down on this one", note that Kaimi watches for amendments/Q&A.

## Interactions & Behavior
- **Routing**: three routes; current route persisted (prototype uses localStorage — use the codebase's router; deep-link workspace by proposal id). Guard: workspace route with a missing/unknown proposal id falls back to the proposals list.
- **Select to pursue**: creates a new proposal at stage 0 (Outline, status progress, "Noa outlining now"), closes drawer, navigates to Proposals. Opportunity becomes non-selectable ("In your proposals").
- **Approve & resume**: gate → done; Final Review (Vera) becomes progress; after the final pass completes (mock: 2.6s) status becomes done ("Ready to submit"). Sidebar amber badge decrements immediately.
- **Request changes**: back to Technical Writer (progress, "Tomás revising"); after mock 2.6s returns to the human gate ("Paused just now").
- **Submit to SAM.gov**: status → submitted; proposal moves to the SUBMITTED section; workspace shows the submitted confirmation; "in flight" stat excludes it.
- **Ambient autonomy** (prototype simulation of the real event stream): ~14s after load one drafting proposal completes and arrives at Human Review (amber badge ticks up); ~26s another advances Outline → Technical Writer. In production these are server-pushed agent events — preserve the *feeling*: proposals advance without user action, and arrival at the gate is visible but not alarming.
- **Keyboard**: rows/cards are buttons (Tab + Enter/Space). Esc closes the drawer. Focus ring: `0 0 0 3px rgba(37,99,235,0.35)`.
- **Route transition**: content slides up 6px on route change (280ms; transform only).
- **Animation rule (important)**: never animate opacity from 0 as an entrance base-state, and gate decorative motion behind `prefers-reduced-motion: no-preference`. Pulses (amber "needs human", working dots) are gentle, ~1.3–2.2s cycles.

## State Management
- `opportunities: Opportunity[]` — `{ id, title, agency, naics, sol, fit (0–100), rec ('bid'|'review'|'nobid'), deadlineLabel, deadlineLevel ('calm'|'soon'|'near'|'crit'), isNew, day, value }`
- `proposals: Proposal[]` — `{ id, title, agency, fit, deadlineLabel, deadlineLevel, stageIndex (0–4), status ('progress'|'human'|'done'|'submitted'), agents (count), when (status phrase) }`
- `pursuedOpportunityIds: Set<id>`; current route + selected proposal id; drawer open/closed + selected opportunity.
- Derived: needsCount (status==='human'), inFlight (status!=='submitted'), agentsTotal.
- Data fetching: opportunity queue from the nightly hunt; proposal/agent status from the agent orchestration layer (event-driven updates per ARCHITECTURE.md).

## Design Tokens
Authoritative files: `kaimi/tokens.css` (full ramps + dark "Focus" theme), `kaimi/ui.css` (status components), `kaimi/app.css` (app shell). Key values:

**Brand** — navy ink `#0A1B3D`; BlueMeta blue `#2563EB` (ramp 50–900 in tokens.css); Kaimi cyan accent `#22D3EE` (dark variant `#0EA5C4`); neutrals are navy-tinted (`#FAFCFF` → `#0F1B30`).

**Status vocabulary** (used everywhere; color always means the same thing):
- Pending `#64748B` on `#EEF1F6` · In Progress `#2563EB` on `#E7EEFF` · Done `#15A06B` on `#E2F6EE` · Failed `#DC2626` on `#FCE8E8`
- **Needs Human `#E8870E`** (tint `#F6A938`, bg `#FFF3E0`) — the loudest signal; solid fill + glow + gentle pulse. Recommendation REVIEW shares this family.
- Recommendations: Bid `#15A06B` · No Bid `#C2354A` · Review `#E8870E`.
- Fit bands: ≥80 `#15A06B` · 60–79 `#0EA5C4` · 40–59 `#E8870E` · <40 `#C2354A`; ring track `#E4EAF3`.
- Deadline escalation: >30d slate · 14–30d blue · 7–14d amber · <7d red `#DC2626` (solid fill, pulsing).

**Type** — Figtree: 30px/700 page titles, 27px workspace title, 21/19px card headings, 16px row titles, 15px body, 13px meta, 11–12px uppercase labels (letter-spacing 0.08–0.1em). IBM Plex Mono for NAICS/SOL/counts/scores.

**Spacing** 4px base (4/8/12/16/20/24/32/40/48/64). **Radii** 5/8/11/16/22/pill. **Shadows** e-1 `0 1px 2px rgba(15,27,48,0.06)` → e-4 modal `0 18px 48px rgba(10,27,61,0.18)`. **Motion** 120/220/360ms, ease `cubic-bezier(0.22,0.8,0.28,1)`, spring `cubic-bezier(0.34,1.56,0.64,1)`.

## Components (reusable)
- **FitRing** — SVG ring, stroke ≈11% of diameter, round caps, fills clockwise from 12 o'clock to `score%`; mono centered number; band color by score; sizes used: 46/64/92px (+ "FIT" sublabel ≥92px).
- **StatusBadge** — pill, 24px tall, leading dot; variants per status vocabulary; "Needs Human" is solid amber gradient with white dot + pulse halo.
- **DeadlinePill** — 24px pill, clock icon; tint by escalation level; critical = solid red, pulsing.
- **Buttons** — ghost / secondary (white, bordered) / primary (blue); high-stakes: **Select** (cyan gradient `#22D3EE→#0EA5C4`, dark text `#042530`, 52px lg) and **Approve** (green gradient, white text); **Request changes** (white, amber border). Active press: translateY(1px); focus ring as above.
- **Avatar** — agent initial tile, radius ≈30% of size; gradients: Noa blue `#5B9BFF→#2563EB`, Tomás cyan `#67E0F4→#0EA5C4` (dark text), Vera violet `#A99BFF→#7C6BF5`; "working" = animated cyan ring + small conic spinner badge.
- **MiniPipe / stage pipeline** — as described per screen.

## Agents (named teammates)
Noa — Outline · Tomás — Technical Writer · Vera — Final Review. Status phrases use their names ("Tomás drafting now"). Keep this warmth; the human gate is framed as a teammate handing work over, never an alarm.

## Assets
- **Kai wave mark** (chosen logo): cyan sun (circle) over two waves; full system in `Kaimi Brand.html` (primary/reversed/mono/stacked lockups, clear space = sun diameter, min 20px, single-wave fallback < 24px). Favicon: rounded navy square, cyan sun, single white wave (inline SVG data-URI in each HTML head).
- All icons are inline SVG (stroke-based, 1.8–2.5 stroke width) — recreate with the codebase's icon system or copy the paths.
- Fonts via Google Fonts: Figtree, IBM Plex Mono.

## Files
| File | What it is |
|---|---|
| `INTENT.md` | **Read first** — how the product works: architecture mapping, the editable proposal document, the gate, common errors to avoid |
| `screenshots/` | Captures of each screen in its key states — visual checksum |
| `Kaimi App.html` | **The product** — app shell, routing, state, ambient simulation |
| `kaimi/app-data.js` | Mock data model (opportunities, proposals, agents, review detail) |
| `kaimi/app-screens.jsx` | Sidebar, Opportunities screen, opportunity drawer, MiniPipe |
| `kaimi/app-proposals.jsx` | Proposals command view |
| `kaimi/app-workspace.jsx` | Workspace: pipeline, review card, working/ready/submitted states |
| `kaimi/app.css` | App-specific styles (shell, rows, cards, drawer, workspace) |
| `kaimi/tokens.css` | **Design tokens** — both light Triage and dark Focus themes |
| `kaimi/ui.css` | Status vocabulary components (badges, pills, fit ring, buttons, avatars) |
| `kaimi/lifecycle-components.jsx` | Shared React components (FitRing, StatusBadge, Avatar, Btn, icons) |
| `Kaimi Design System.html` | Living style guide — tokens, status vocabulary, core components |
| `Kaimi Brand.html` | Kai wave logo system |
| `Kaimi Proposal Lifecycle.html` + `kaimi/lifecycle-*`, `take-*.jsx` | Earlier dark "Focus" concept (3 takes) — reference only, not the shipping design |
| **`Kaimi Desktop.html`** | **Desktop product** — branded window, onboarding, offline mode, draft editor (see `DESKTOP.md`) |
| `kaimi/desktop.css`, `kaimi/desktop-onboarding.jsx`, `kaimi/desktop-editor.jsx` | Desktop chrome, onboarding flow, working-draft editor |
| `kaimi/app-submitted.jsx` | **Submitted archive** — pipeline stats, search/filter, award outcomes, BD-report export (see `PIPELINE.md`) |
| `kaimi/editor.css` | Working-draft editor styles for the web app (mirror of the editor section in desktop.css) |
| `ARCHITECTURE.md`, `WORKFLOW.md` | Client engineering contracts — follow these |

## Implementation Notes
- One prototype-only workaround to NOT carry over: active nav/filter buttons are remounted via React `key` changes because the preview webview failed to restyle in-place class toggles. In a real browser build, normal class toggling is fine.
- The in-browser Babel setup, `window.*` component sharing, and localStorage routing are prototype scaffolding — replace with proper modules, imports, and a router.
- Status colors are semantic and load-bearing: amber **always** means "a human is needed." Don't introduce amber for anything else.

# GOAL: RELEASE 002 — Port upstream Kaimi improvements into the commercial product

**Owner:** autonomous /loop · **Created:** 2026-06-16 · **Repo:** Mawar2/kaimi-platform (`platform` remote)
**Revert point:** `release-001` tag (RELEASE 001). **Upstream:** `origin` = Mawar2/Kaimi.

## Objective
Bring the valuable core improvements from public Kaimi (`origin/main`, 18 commits ahead at fork `f21a4f3`) into the commercial product **without regressing RELEASE 001**, ending at a tagged **RELEASE 002** that is **pilot-ready for Ey3 Technologies** (full flow: SAM search → scoring → proposal drafting into the customer's Drive).

## The hard testing rule (applies to EVERY phase — no exceptions)
A change is **NOT complete** until ALL of the following pass and are cited as evidence:
- `go build ./...` (only acceptable failure: `cmd/desktop` Wails embed).
- `go test ./...` green except that one `cmd/desktop` failure.
- `golangci-lint run ./...` = 0 issues on touched packages.
- `gofmt -l` clean.
- **Independent Gemini review** (talk-to-gemini, `-Model pro`) → APPROVE (REQUEST_CHANGES → fix → re-review).
- **For ANY change that touches the UI** (`internal/dashboard`, editor, onboarding, proposals, gate): **run the app and verify in a real browser** —
  - boot `cmd/api` locally with `-insecure-no-auth` (or test the deployed revision), open the affected screen in a headless browser (gstack/gstack-qa),
  - **click through the actual flow** the change affects, confirm it renders and behaves, and
  - confirm the **UI/UX is streamlined** (no broken layout, no dead controls, no AI-slop spacing, sensible empty/error states). Capture a screenshot or DOM assertion as evidence.
- Re-verify the **committed tree** with go tooling every time (IDE diagnostics + builder reports are stale).
- Each item lands as its **own PR**; agents may merge in this repo (rule relaxed) but only after the gate above is green.

## Execution model
- Per item: branch → cherry-pick (clean) OR manual port (conflicting) → verify (go gate) → browser-verify (if UI) → Gemini review → PR → squash-merge → sync `platform-main`.
- **Sub-agent teams** where useful: a builder agent per port; an `Explore` agent to map conflicts before a careful port; Gemini for independent review; gstack-qa for browser UX. Dispatch in parallel only for independent items.
- Never break the deployed pipeline. Never weaken auth/isolation. Keep `Opportunity`/`Store` forward-compatible.

## Phases

### Phase 0 — Setup ✅ DONE
- RELEASE 001 tagged + pushed + GitHub Release (revert: `git checkout release-001`).

### Phase 1 — SAM search fix (pilot-critical, clean) 🔴 PRIORITY
- **#270** `samgov_ncode_filter_and_max_page` (`f430362`) — NAICS filter actually filters; quota ~3%. Touches only `internal/samgov`.
- DoD: go gate + Gemini + a **live SAM hunt smoke test** (verify it returns NAICS-filtered results and stays within quota). No UI.

### Phase 2 — Clean core batch (packages we never modified)
- **#267** `outline_planner_fallback_chain` (`6aebb70`) — outline robustness.
- **#265** `vera_llm_pass_without_documents` (`fc5cf74`) — final-review without docs.
- **#263** `finalreview_term_overlap_musthave` (`36dac25`) — must-have/compliance check.
- **#245** `fallback: real-model failover (Writer + Final Review)` (`7bedf78`) — new `internal/fallback` pkg (clean) + **port the wiring into `proposalwiring`** (NOT `cmd/dashboard`).
- DoD: go gate + Gemini each. #245 also: a drafting smoke test proving failover wiring is reachable.

### Phase 3 — Careful ports (touch files we changed) — UI testing REQUIRED
- **#261** gate bugs from tester (`40580e4`) — criteria-vs-final-review, request-changes note. `proposal.go` + dashboard.
- **#271** `vera_gap_flagging_and_highlighting` (`8fa8b9b`) — Final-review gap detection + editor highlighting. Pull logic; port editor UI.
- **#255** `eval_writer_facts_in_prompt` (`5fb7c1d`) — writer grounding (reconcile with A4 de-BlueMeta).
- **#248** `dashboard_zone2_qa_fixes` (`d41dffd`) — Zone-2 dashboard bugfixes.
- **#257** `detail_eligibility_honest_copy` (`b998af3`) — detail-page copy.
- DoD: go gate + Gemini + **browser walkthrough** of each affected screen (gate, editor gaps, Zone-2 workspace, detail page) confirming streamlined UX.

### Phase 4 — Optional UX / tooling (decide after Phase 3)
- **#275 / #276 / #280** — gap-bar UX, gate simplification, template tweak (only with #271; strip "demo" framing).
- **#273** `demo_seed_from_csv` — optional pilot pre-seed tool.
- DoD: same gate incl. browser UX.

### Phase 5 — Integration, full UX pass, RELEASE 002
- Full `go test ./...` + lint clean on merged `platform-main`.
- **End-to-end browser walkthrough**: sign-in → onboarding (profile + Drive destination) → run hunt → opportunities list → detail → select → Zone-2 draft → gate → Doc in Drive. Confirm streamlined throughout.
- Rebuild + redeploy `cmd/api` image; live smoke test.
- Tag + push **`release-002`** + GitHub Release. Update memory + this goal doc to DONE.

### SKIP (out of scope) — do NOT port
- #251 / #253 / #260 desktop (epic #136). #277 hackathon submission docs. *(Note: #251 introduces `internal/zone2view`; revisit only if desktop is resumed.)*

## Progress log (append per iteration)
- 2026-06-16: Goal created. RELEASE 001 tagged. Phase 1 starting.

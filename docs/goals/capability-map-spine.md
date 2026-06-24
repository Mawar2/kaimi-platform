# GOAL: Capability Map spine — deep per-tenant business understanding

**Owner:** Malik + agent · **Created:** 2026-06-18 · **Repo:** Mawar2/kaimi-platform
**Builds on:** the onboarding wizard + product-key pilot (feat/onboarding-wizard).

## Objective
Make Kaimi genuinely understand each tester's business — from onboarding **and** the
context documents they upload (and, later, a Drive folder they maintain) — and use that
understanding to **qualify and score** opportunities far better than today's shallow
keyword-substring matching. The understanding lives in one shared artifact, the
**Capability Map**, that every agent (qualification, scorer, Zone-2 drafting) references.

## Current state (verified 2026-06-18)
- **Scoring** (`internal/scorer`): Gemini 2.5 Pro + pre-computed signals, but the
  capability signals are **substring matches** of competency *keywords* and past-perf
  *sentences* against the opportunity text. Shallow. No semantic understanding.
- **Gate** (`internal/pipeline/zone1.go` + `internal/profile.IsEligible`): NAICS fetch
  filter + a **set-aside eligibility table**. NOT capability-based. NOT hardcoded
  BlueMeta in code — config-driven; an onboarded tenant's profile already comes from
  onboarding via `ResolveProfileWithStore` (baked config is fallback only).
- **Drive**: write-only (`drive.file`) — **cannot read** the tester's existing Drive.
- **Ingest** (`internal/ingest`): SAM solicitation attachments only, Zone-2 only.
- **No capability map / shared-context object exists.** Agents get the Opportunity +
  a thin `scorer.CapabilityProfile` (name, tags, past-perf).
- **Opportunity detail**: SAM attachment URLs captured but never shown; `ContractType`,
  `NAICSDescription`, `EstimatedValue` shown but never populated.

## Locked decisions (Malik, 2026-06-18)
- **Context intake = upload docs in onboarding** (capability statement, CPARS, past
  proposals), **plus guidance** telling testers they can build a Drive folder of context
  for their BD team (full Drive-read via `drive.readonly` is a later follow-on — bigger
  scope + Google verification).
- **Build the capability-map spine first** (context intake → capability map → capability-
  aware qualification + scoring), before the quick wins.

## Phases
### C1 — Capability Map core (THIS slice)
`internal/capabilitymap`: the `CapabilityMap` schema (summary, core competencies with
evidence, differentiators, domains, past performance, certifications, NAICS, expanded
keywords, source provenance), a per-tenant JSON `Store`, a `Builder` interface, an
offline deterministic builder (profile → map, no LLM), and a Gemini builder (profile +
context-doc texts → map, mirroring `internal/scorer`'s Vertex client). TDD.

### A/B — Context intake (upload + extract)
Onboarding doc upload (multipart) → store → extract text (reuse the `internal/ingest`
extractor: Document AI + DOCX/stdlib). Onboarding copy guiding testers to also maintain a
Drive context folder. Trigger a map (re)build after onboarding/upload.

### D — Capability-aware qualification
Use the Capability Map to qualify/score: match solicitation requirements ↔ capabilities
semantically, beyond NAICS/set-aside.

### E — Scoring upgrade
Replace substring signals with map-driven matching; re-evaluate rubric/thresholds; A/B
against the current scorer. **Also resolve the SAM `description` field**: the v2 search
API returns it as a `noticedesc` URL, not text, so the Scorer currently scores against a
URL string. Resolve the URL → real solicitation text (for the eligible set, after the
gate, to bound SAM-quota cost) so scoring + drafting use real content.

### Later (independent, fast)
- **F** — opportunity-detail completeness (surface attachments; NAICS-description lookup;
  pull ContractType/EstimatedValue from SAM detail endpoint).
- **G** — remove expired (past-deadline) opportunities from the board.
- **Drive-read** — `drive.readonly` + picker to ingest a referenced Drive folder.

## Discipline
TDD; go gate (build/test/lint/fmt); independent review; deploy to the pilot + browser-
validate; never break the deployed gate/onboarding/drafting happy paths. Capability data
is per-tenant — no cross-tenant leakage.

## Progress log
- 2026-06-18: Goal created; decisions locked. Starting C1 (capability map core).
- 2026-06-18: **C1 done** — `internal/capabilitymap` (schema + Deterministic + Gemini builders + JSON store; TDD). Committed on `feat/onboarding-wizard`.
- 2026-06-18: **Quick wins (F + G) done + deployed** (pilot rev 00006-v69): expired opps dropped from the board (`ExcludeExpired`; 131→59 live), SAM `description` URL rendered as a link + attachments surfaced. Next: loop the spine (A/B → wire → D/E).
- 2026-06-18: **A/B backend done** — `internal/contextdoc` (per-tenant uploaded-doc store + text extraction reusing `ingest.Extractor`; path-safe; TDD; Gemma review clean). Not yet user-visible. Loop next: onboarding upload endpoint + UI, then WIRE the map (re)build.
- 2026-06-18: **A/B done + deployed** (pilot rev 00007-dt5) — onboarding context-doc UPLOAD on the Connect step (multipart -> contextdoc store; CSRF + MaxBytesReader; lists uploads), wired in cmd/api; verified live (upload -> stored -> "text extracted"). Loop next: WIRE the capability-map (re)build after onboarding.
- 2026-06-18: **WIRE done + deployed** (pilot rev 00008-2md) — profile-save/doc-upload fire a best-effort capability-map rebuild; cmd/api builds via Gemini (GCP set) else deterministic, persists capabilitymap.JSONStore. Verified LIVE: saving a profile synthesized a real map from profile + uploaded cap.txt (competencies incl. DevSecOps/Continuous Monitoring mined from the doc; model gemini-2.5-pro). NOTE: build is synchronous (~14.5s save) — make async (follow-on). Loop next: UI to surface the map.
- 2026-06-18: **UI done + deployed** (pilot rev 00009-vv2) — /capability-map view (summary, competencies+evidence, differentiators, domains, certs, NAICS, past perf, keywords, sources, model+time); linked from onboarding Done. Verified LIVE: renders the Ey3 map with evidence cited to "Company profile"/"cap.txt" + ISO 27001 mined from the doc. TDD (built/not-built/unavailable); Gemma review clean. The "understand the business" half (A/B+WIRE+UI) is COMPLETE + visible. Loop next: D (capability-aware qualification), then E (map-driven scoring — STOPS for Malik before live cutover).
- 2026-06-18: **D done + deployed** (pilot rev 00011-sx8) — capability-aware qualification: opportunity detail shows "Why this fits your capabilities" (deterministic whole-word match of competencies/domains/keywords vs the listing; min-len 3 so gov acronyms DHS/CDM/ZTA count; word-boundary so no "cloud"-in-"cloudy" noise). ADDITIVE — does not change the score. Gemma review → switched substring→word-boundary. Verified LIVE: DHS NOSC opp shows "Matched: DHS"; unrelated opps show the assess-fuller-text note. Loop next (FINAL): E — map-driven scoring + resolve description URL→text; A/B evaluate; STOP + checkpoint Malik before any live cutover.
- 2026-06-18: **E part 1 done** (committed, NOT deployed — no live cutover) — samgov.DescriptionResolver (noticedesc URL→text; key-safe; bounded; TDD/fake client) + Opportunity.ResolvedDescription/EffectiveDescription() + scorer computeSignals/prompt now read EffectiveDescription (identical until resolved → no live change). Gemma review clean. Next: map-driven signal + A/B eval (resolve ~10 pilot opps, score current vs map+resolved-text via Vertex), then STOP + checkpoint Malik on cutover.
- 2026-06-18: **E COMPLETE (Part 2 + A/B eval) — AWAITING MALIK CUTOVER DECISION.** Part 2: scorer gains an opt-in capability-map signal (computeSignals+prompt; nil in live path → no change); cmd/capeval A/B harness. Ran live A/B on 8 pilot opps (Ey3 TEST profile, gemini-2.5-pro) → docs/goals/capability-scoring-ab.md. FINDINGS: description resolver works (7/8; 1 stale v1 URL 404); map surfaces explainable matches (DHS/Homeland Security/541519); BUT absolute scores floored (0-10, all NO_BID both arms) on the thin test profile, and run-to-run LLM variance (±5) exceeds the map effect (net delta +0 then -10, 0 rec changes) — validates the MECHANISM, not a scoring WIN. RECOMMENDATION: (1) ship the resolver+EffectiveDescription into live (clearly correct — stop scoring a URL); (2) do NOT enable the map scoring signal yet — re-run A/B on a REAL onboarded profile + review the rubric first. Nothing wired to the live scorer. **SPINE BUILD COMPLETE; loop ENDED.**

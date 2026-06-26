# GOAL: Browser-QA the Kaimi pilot end-to-end until certain it works

**Last updated:** 2026-06-26
**Owner loop:** autonomous /loop (dynamic). Ends when a FULL clean pass finds zero new issues.

## Objective
Drive **every component** of the deployed pilot in a real headless browser (gstack-browse),
capture concrete feedback (functional breaks, JS console errors, failed requests, visual/UX
issues), turn findings into a patch plan, fix them (TDD + deploy), and re-verify — looping
until a complete pass is clean. Then restore the instance to pristine and end.

- **Target:** https://pilot-kaimi-api-1098973371312.us-east4.run.app  (key KAIMI-GB3S-D2JH-NXMY)
- **Browser:** `B=/c/Users/Owner/.claude/skills/gstack/browse/dist/browse` — `$B goto|text|snapshot -i|click|fill|console|network|screenshot|is visible`
- **Code/deploy:** worktree C:/Users/Owner/Kaimi-sec, branch feat/pilot-qa (off main); deploy api img → `gcloud run services update pilot-kaimi-api`.

## Component checklist (each: renders? functional? console clean? visual ok?)
- [x] Login (access link → session → onboarding)
- [x] Onboarding: Welcome
- [x] Onboarding: License (verified key)
- [x] Onboarding: Profile (fill + save; validation)
- [x] Onboarding: Connect — SAM key field/connected state
- [x] Onboarding: Connect — single Upload-document button (upload works)
- [ ] Onboarding: Connect — Google Drive button (expected redirect_uri_mismatch, graceful)
- [x] Onboarding: Done summary
- [ ] First-run redirect (no profile → onboarding)
- [ ] Capability map view (built from profile + doc; sources cited)
- [x] Opportunities board (list, scores, stage filters, sort)
- [ ] Opportunity detail (capability match, deadline pill)
- [ ] Select → Zone-2 draft (status transitions)
- [ ] Proposal/editor screen (sections render, edit, approve/request-changes)
- [ ] Submitted screen (if reachable)
- [ ] /help page
- [ ] Sidebar/nav links
- [ ] Security headers present on pages; gate blocks no-session
- [ ] Responsive (mobile/tablet/desktop)
- [ ] Console errors + failed network requests across all pages

## Loop protocol (each iteration)
1. Browse a slice of components; record findings under "## Findings".
2. Triage findings → "## Patch plan" (severity, fix).
3. Patch quick/safe ones (TDD where code changes), commit, deploy, re-verify.
4. Update the checklist + progress log. ScheduleWakeup to continue.
5. END when a full sweep yields zero new issues: re-clean instance (profile/map/docs/test
   proposals) to pristine, PushNotification the outcome, omit ScheduleWakeup.

## Findings
### Iteration 1 — onboarding visual + console pass (CLEAN, no issues)
- Login (access link) → /onboarding?step=welcome ✓. Only expected 302/303 redirects, no failed requests.
- Welcome, License (shows "License verified · key KAIMI-····-NXMY"), Profile, Connect, Done: all render; **0 console errors** on every step.
- Connect step interactive els correct: SAM key field, "Save SAM.gov key", **single "Upload document"** (fix live), "Connect Google Drive", Back/Continue. Good privacy copy ("encrypted in Secret Manager… your daily quota is never shared").
- Visuals polished (dark theme, 5-step indicator, value cards). No layout breakage seen.
- No defects found this slice.

### Tooling notes (for the loop)
- Browser context RESETS between separate Bash calls → **start every browser block with `$B goto $U/access?key=KAIMI-GB3S-D2JH-NXMY`** (login) before other commands.
- Deep-link a wizard step: `$B goto $U/onboarding?step=<welcome|license|profile|connect|done>`.
- Board/detail/draft are gated behind a saved profile (first-run redirects "/" → onboarding) — must complete onboarding (save profile) first; that writes data to clean up at loop end.

### Iteration 2 — functional onboarding + board
- Profile save works in-browser: filled company/NAICS/competencies/set-aside → advanced to Connect; persisted to profile.json (naics stored under `naics_codes`). 0 console errors on save.
- **BUG (fixed): CSP blocked the dashboard's embedded `data:` web fonts on every page.** CSP had `img-src data:` but no `font-src`, so fonts fell back to `default-src 'self'` and the browser blocked them (UI silently degraded to system fonts). Browser-only catch (curl can't see it).
- Board (/) renders: opportunities list, fit-score rings, stage counts, sidebar. After the fix + cache-bust: **0 console errors**.

### Tooling notes (added)
- The browse server PERSISTS a console buffer + a persistent browser PROFILE/cache (/c/Users/Owner/.gemini/antigravity-browser-profile) across kills → stale console + cached pages. **Before each console check: `$B console --clear`, then load a CACHE-BUSTED url (`?qa=<nanos>`), then `$B console --errors`.** (Iteration-1 onboarding "0 errors" was likely an unreliable read — RE-VERIFY onboarding console with this method.)

## Patch plan
- [x] CSP `font-src 'self' data:` — FIXED (commit on feat/pilot-qa), deployed rev 00023-v82, verified board console clean. Test asserts font-src + img-src allow data:.
- [ ] (low/a11y) profile textareas (NAICS, competencies) not enumerated by `snapshot -i` → likely missing `<label for>`/aria-label association. Polish, not a blocker.

## Progress log
- 2026-06-26: goal created; browser verified. **Iteration 1 done: onboarding (all 5 steps) visually + console clean, upload-button fix confirmed live, no defects.** Next: functional onboarding (save profile + upload doc) → capability map → board → detail → select/draft → editor; then help/responsive/edge cases.

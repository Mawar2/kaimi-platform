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
- [ ] Login (access link → session → onboarding)
- [ ] Onboarding: Welcome
- [ ] Onboarding: License (verified key)
- [ ] Onboarding: Profile (fill + save; validation)
- [ ] Onboarding: Connect — SAM key field/connected state
- [ ] Onboarding: Connect — single Upload-document button (upload works)
- [ ] Onboarding: Connect — Google Drive button (expected redirect_uri_mismatch, graceful)
- [ ] Onboarding: Done summary
- [ ] First-run redirect (no profile → onboarding)
- [ ] Capability map view (built from profile + doc; sources cited)
- [ ] Opportunities board (list, scores, stage filters, sort)
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

## Patch plan
- None yet — iteration 1 clean.

## Progress log
- 2026-06-26: goal created; browser verified. **Iteration 1 done: onboarding (all 5 steps) visually + console clean, upload-button fix confirmed live, no defects.** Next: functional onboarding (save profile + upload doc) → capability map → board → detail → select/draft → editor; then help/responsive/edge cases.

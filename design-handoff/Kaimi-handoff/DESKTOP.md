# DESKTOP.md — Kaimi Desktop (onboarding, licensing, offline)

> Read AFTER `INTENT.md` and `README.md`. This covers what the **desktop
> product** adds on top of the web app: a branded window shell, an end-to-end
> onboarding flow, license-key auth, and offline-first behavior. The reference
> prototype is `Kaimi Desktop.html` (+ `kaimi/desktop*.{css,jsx}`).

---

## What the desktop product is

A cross-platform desktop app (target: **Electron or Tauri** — same UI on macOS
and Windows) so users can work on proposals **even when temporarily offline**.
It is the same three-surface app (Opportunities / Proposals / Workspace) wrapped
in a desktop shell, plus first-run onboarding and an offline sync queue, plus
the **editable working-draft editor** (the offline payoff).

Everything that was true in INTENT.md still holds. This document only adds the
desktop-specific intent.

---

## Window shell

- **Custom branded title bar**, identical on both OSes (46px, navy `#0A1B3D`):
  Kai-wave mark + "Kaimi Desktop" on the left, a **network/sync pill** and
  window controls on the right. In production use a frameless window and draw
  this yourself (`-webkit-app-region: drag` on the bar, `no-drag` on the
  controls) so Mac and Windows look the same. The prototype's min/max/close
  buttons are cosmetic — wire them to the real window API.
- The **network pill** is the live sync indicator: "Online · synced" (green dot)
  vs "Offline · working locally" (slate dot) with a "N queued" counter. In the
  prototype it's clickable to *simulate* connectivity — in production it
  reflects real `navigator.onLine` / reachability to the Kaimi backend, and is
  not user-toggled.
- When offline, a **slate** info bar drops below the title bar. **Offline is
  always slate, never amber** — amber remains reserved exclusively for "a human
  is needed." Do not signal connectivity with the human-attention color.

---

## Onboarding (first run)

Six steps, left brand panel + right step panel, with a progress bar. Tone is
warm and confident — the user meets their agent teammates during setup. Persist
completion (the prototype uses a localStorage flag; production: per-device +
per-account setup state). Steps:

1. **Welcome** — what Kaimi does (hunts nightly, drafts what you pick, pauses for
   you). Sets expectations that it works offline once set up.
2. **Sign in — SSO.** Google Workspace or Microsoft Entra (work accounts). No
   passwords. The desktop app should use the OS browser / system webview for the
   OAuth handshake and store tokens in the OS keychain (Keychain / Credential
   Manager), not in app storage. After sign-in, show the resolved identity.
3. **License key.** This is a **Kaimi-issued license** tied to the org's
   subscription — *not* a model/provider API key. Format `KAIMI-XXXX-XXXX-XXXX`
   (the prototype auto-formats and validates to "BlueMeta Technologies · Team
   plan · N seats"). Validation is a real server call: it binds this device to
   the subscription and authorizes the agent runtime. One key per org; seats are
   tracked per signed-in user. Store the key securely (keychain), never in
   plaintext config.
4. **Company profile.** The data Kaimi scores opportunities against:
   - **NAICS codes** (multi-select chips) — required; at least one.
   - **Capabilities statement** (free text).
   - **Past-performance uploads** (CPARS, references, past proposals) — used as
     evidence by the Technical Writer agent. In production these upload to the
     org profile and are indexed; the prototype just lists them.
   - This is skippable ("Finish later") but strongly encouraged — fit scores are
     only as good as this profile.
5. **Meet your team.** Noa (Outline), Tomás (Technical Writer), Vera (Final
   Review), and the one-gate promise. This is brand/onboarding, not config —
   but the names and roles must match the rest of the product exactly.
6. **First sync.** Animated setup: link license → pull tonight's queue → index
   past performance → wake the agents → prepare the offline cache. Ends on
   "your queue is ready," button into the app. In production this is the real
   initial sync; show genuine progress, and make it resumable if it fails.

After onboarding, the user lands in the app (Opportunities). Provide a way to
re-run/review setup from Settings (the prototype exposes a replay button in the
title bar for demoing — in production this lives in Settings, not the chrome).

---

## Offline-first behavior

**Principle:** everything except the nightly hunt and live agent runs works
offline. The human can keep reading, editing drafts, and making review decisions
on a plane; the system reconciles on reconnect.

| Capability | Offline? |
|---|---|
| Browse the already-synced opportunity queue | ✅ read-only |
| Open a proposal, read its pipeline/status | ✅ |
| **Edit the working draft** (the point of offline) | ✅ saves to device |
| Approve / Request changes at the gate | ✅ **queued**, applied on reconnect |
| Submit to SAM.gov | ⚠️ requires connection (queue or block — see below) |
| Select a new opportunity to pursue | ❌ needs the agent runtime ("Reconnect to pursue") |
| Nightly hunt + agent drafting/review runs | ❌ server-side; **paused** locally, shown as "Paused — resume online" |

### The sync queue (intent)

- A human decision made offline (approve / request-changes, plus any draft edits)
  is **recorded locally as a queued action**, attributed and timestamped, and the
  proposal shows a calm "Queued for sync / Approved · syncs online" state — using
  **slate/pending**, not amber.
- On reconnect, queued actions **replay in order** against the server: an approve
  resumes Vera's final pass; a request-changes returns the draft (with the human's
  edits/notes) to Tomás. The UI transitions those proposals from "queued" into
  their live working state automatically.
- Draft edits made offline are the **canonical content** when the action syncs —
  Final Review runs on what the human left, exactly as in INTENT.md. Edits carry
  a local revision ("v4 · edited by you") and merge as a new server revision on
  sync.
- **Conflict policy** (decide with the team; prototype assumes the simple case):
  if the server advanced the same proposal while the user was offline (rare —
  agents pause, but amendments can land), surface a conflict rather than silently
  overwriting. Last-writer-wins is acceptable for v1 *only* if the human is shown
  what changed.
- Persist the queue durably (the prototype holds it in memory; production needs a
  local store — SQLite / IndexedDB / file — that survives app restart, since
  "temporarily offline" includes "quit and reopened on the train").

---

## The working-draft editor (desktop)

This realizes the INTENT.md requirement that the draft be **editable in-app at
the gate**. Reached via "Edit the draft" in the review card.

- **Section-structured**, matching Noa's outline: a left rail lists the 7
  sections with their compliance state (green = ok, amber dot = flagged), and the
  document shows each section with its linked-requirement tag.
- **Click-to-edit** paragraphs (the prototype uses `contenteditable`; production
  should use the codebase's structured rich-text editor — sections are data, not
  one blob; see INTENT.md "What this means for the data model").
- The **gap flag is anchored inside the affected section** (Past Performance),
  not just shown as a banner — the human edits exactly where the problem is.
- **Attribution + versioning:** the moment the human edits, the version flips to
  "v4 · edited by you" and autosaves. Save copy is connectivity-aware: "Saved"
  online, **"Saved to this device"** offline.
- Approve uses the current (human-edited) revision. Back returns to the review
  gate, where the decision is made.

---

## Files (desktop additions)

| File | What it is |
|---|---|
| `Kaimi Desktop.html` | Desktop shell — title bar, onboarding gate, offline sim, app + editor routing |
| `kaimi/desktop.css` | Window chrome, onboarding, offline bar, editor styles |
| `kaimi/desktop-onboarding.jsx` | The six-step onboarding flow |
| `kaimi/desktop-editor.jsx` | The section-structured working-draft editor |
| (reuses) `kaimi/app-*.jsx`, `app.css`, `tokens.css`, `ui.css` | Same app screens, now offline-aware (`offline` / `queued` states added) |

Screenshots of the desktop flow are in `screenshots/` (prefixed `10-`+).

## Desktop-specific errors to avoid

1. **Using amber for offline/sync state.** Offline is slate. Amber = human needed,
   only.
2. **Storing the license key or OAuth tokens in plaintext** — use the OS keychain.
3. **Treating the license key as a model API key** — it's a Kaimi subscription
   license; model credentials (if any) are a separate, server-side concern.
4. **Losing the offline action queue on restart** — it must be durable.
5. **Letting "Select to pursue" or agent runs proceed offline** — those need the
   server; pause and label them clearly.
6. **Silently overwriting server state on sync** — show conflicts.
7. **Making onboarding skippable past sign-in/license** — those two gate access;
   profile can be deferred.
8. **Re-skinning the title bar per-OS** — the product intent is one identical
   branded bar on both platforms.

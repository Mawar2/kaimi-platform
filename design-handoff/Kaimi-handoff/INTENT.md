# INTENT.md — How Kaimi is supposed to work

> Read this WITH README.md. The README specifies *what the screens look like*;
> this document explains *what the product is actually doing* so implementation
> decisions don't drift from intent. When a detail is ambiguous, resolve it in
> favor of the intent described here.

---

## The one-sentence model

**Agents do the work; one human makes the calls.** Kaimi hunts and scores federal
opportunities overnight, drafts proposals through a staged agent pipeline, and
pauses at exactly one human review gate per proposal — where the human can read,
**edit**, and approve the working draft before the final pass runs.

---

## How the UI maps to the architecture (ARCHITECTURE.md)

The system has two zones. Each app surface is a window onto one of them:

| App surface | Architecture zone | What it shows |
|---|---|---|
| **Opportunities** screen | Zone 1 (scheduled pipeline: Hunter → Scorer → Queue) | The scored opportunity queue awaiting human selection |
| "Select to pursue" button | **The bridge event** | Emits the selection that spins up a Manager for that one proposal |
| **Proposals** screen | Zone 2 (all running Managers) | Every active proposal lifecycle, grouped by what needs the human |
| **Workspace** screen | Zone 2 (one Manager) | A single proposal's pipeline, its working draft, and the human gate |

Implementation consequences:

- **Opportunities data is read-mostly.** The queue is produced by the nightly
  batch run. The UI never mutates an opportunity except to mark it selected.
  New rows appear once per day ("New today"), not in real time.
- **Proposals data is event-driven.** Each card mirrors a live Manager. Agents
  report status (`success` / `failed` / `needs_human` — the coming `AgentResult`
  contract); the UI reflects those events as they arrive. The prototype's
  "ambient" 14s/26s timers simulate this — in production it's server push
  (polling is an acceptable first implementation), not client-side timers.
- **Selection is the only bridge.** Nothing in Zone 1 talks to Zone 2 directly.
  Don't wire the Opportunities screen to Manager state, and don't let the
  Proposals screen reach back into the queue.

---

## The proposal document — the central artifact (read carefully)

This is the most important intent in this file. The proposal is not a status
record with attachments; **it is a living, structured document that agents and
the human both work on.**

### Document lifecycle

1. **Outline (Noa)** reads the solicitation and produces the document's
   *skeleton*: an ordered list of sections, each mapped to specific RFP
   requirements and evaluation factors (the prototype shows "7 sections,
   24 requirements mapped"). **The outline IS the document being born** — not a
   separate artifact. It creates the section structure that everything
   downstream fills in.
2. **Technical Writer (Tomás)** fills the skeleton section by section, producing
   the **working draft** (e.g. "technical-volume-draft-v3"). The draft carries a
   compliance matrix linking each section to the requirements it satisfies, and
   the writer may attach **flags** (e.g. "no past-performance at this scale").
3. **Human gate.** The pipeline pauses. The human reviews the working draft —
   and this is the key requirement: **the working draft must be editable
   in-app, by the human, at this gate.** Not download-edit-reupload. Not
   comment-only. The reviewer opens the draft inside the application, edits
   section content directly, and then decides.
4. **Final Review (Vera)** runs on the draft *as the human left it* — human
   edits are first-class content, not annotations to be merged later. It
   validates compliance, formatting, and cross-references, and produces the
   submission-ready package.
5. **Submit** is always a human act. Kaimi prepares; a person submits to
   SAM.gov. After submission the agents stand down (watching for amendments).

### What this means for the data model

- **Sections are data, not a blob.** Model the document as ordered sections
  (id, heading, body content, linked requirement ids, status), not one opaque
  rich-text field. Agents write whole sections; humans edit within them; the
  compliance matrix derives from section↔requirement links.
- **Edits are attributed.** Every revision records *who* (which agent, or the
  human) and *when*. The review gate's credibility depends on the human being
  able to see what the agent produced vs. what they changed.
- **Version on every actor handoff.** Writer finishes → v(n). Human edits at
  the gate → v(n+1). "Request changes" → writer produces v(n+2). Keep the
  history; the artifact chips in the UI ("draft-v3 · 18 pp") expose it.
- **Flags belong to the document.** A gap flag (like the past-performance
  warning) points at the section(s) it concerns and persists until resolved —
  resolution is visible in the UI ("✓ resolved" after the final pass).

### Editor expectations (Workspace, at the gate)

The prototype shows the draft as a read-only preview card — production replaces
that preview with an **embedded editor for the working draft**:

- Opens in the Workspace (the review card's "what Tomás produced" area expands
  into / links to the editor view). The human stays in the app the whole time.
- Section-structured editing: navigate by outline section; edit heading/body
  text; see each section's linked requirements and its compliance status.
- The gap flag is shown in context — anchored to the affected section — not
  only as a banner.
- Save is non-destructive (new revision, attributed to the human).
- **Approve & resume** uses the *current* (possibly human-edited) revision.
- **Request changes** should let the human attach a note (and optionally their
  partial edits) so the writer agent has actionable direction. The prototype
  omits the note input — add it; it materially reduces wasted agent cycles.
- Use whatever rich-text/structured editor the codebase already standardizes
  on. A clean, calm reading/writing surface matters more than toolbar breadth —
  this is a focused review room, not a word processor.

---

## The human review gate — product soul, not a modal

- There is **exactly one gate per proposal**, between Technical Writer and
  Final Review. Don't add approval steps elsewhere; don't make the gate
  skippable.
- The gate is framed as a **warm handoff from a named teammate** ("Tomás is
  handing you the draft"), never an alarm or an error state. Amber = "a human
  is needed," and amber is reserved exclusively for that meaning everywhere in
  the product.
- The two gate actions are real state transitions:
  - **Approve & resume** → gate closes, Final Review starts immediately.
  - **Request changes** → draft returns to the writer with the human's note;
    the proposal will arrive at the gate again with a new revision.
- While a proposal waits at the gate, it's the loudest thing in the app: the
  amber sidebar badge, the "Waiting on you" section pinned first, the pulsing
  status. Everything else stays calm — the design's job is to make the one
  decision unmissable without making the whole app noisy.

## The agents are named teammates

Noa (Outline), Tomás (Technical Writer), Vera (Final Review). Status copy uses
their names and present-tense verbs ("Tomás drafting now", "Vera finalizing").
Keep this voice — it's how the product makes autonomy feel supervised rather
than spooky. Agent identity is also load-bearing in attribution (see edits
above) and in the activity history.

## Status vocabulary is semantic, app-wide

| Signal | Meaning | Never use for |
|---|---|---|
| Slate | pending / not started | — |
| Blue | an agent is working | links/branding accents inside status UI |
| Green | complete / approved / bid | generic success toasts unrelated to pipeline |
| **Amber** | **a human is needed** (gate, review rec, flags, 7–14d deadlines) | anything decorative |
| Red | failed / critical deadline (<7d) | emphasis |

Fit score bands (≥80 / 60–79 / 40–59 / <40) and deadline escalation
(>30d calm → <7d red pulsing) are defined in the README and `tokens.css`.

## Phase discipline (WORKFLOW.md applies)

The client builds in phases (currently Phase 0: Hunter + queue). The UI you are
integrating spans later phases. Respect their workflow contract:

- Build UI features as **tickets with approved acceptance criteria** — propose,
  wait for approval, then code (TDD, lint, AI review, human merge).
- It is correct to ship the UI **bound to the queue store first** (Opportunities
  screen against the Phase 0/1 `Store` interface, fixtures for the rest) and
  bind Proposals/Workspace to real Manager events in later phases. Keep mock
  boundaries clean so swapping in real data never touches view code.
- Do not invent backend capabilities ahead of the phase plan; leave
  `// TODO(phase-N)` seams exactly as ARCHITECTURE.md prescribes.

## Common implementation errors to avoid

1. **Flattening the document into a blob** — breaks compliance mapping, edit
   attribution, and the request-changes loop. Sections are data.
2. **Making the draft read-only at the gate** — the human must edit in-app;
   approve-only review is explicitly not the product.
3. **Running Final Review on the agent's draft instead of the human-edited
   revision** — human edits are the content, not comments.
4. **Using amber for anything but "human needed."**
5. **Multiple approval gates** — there is one.
6. **Client-side timers as the real event mechanism** — the prototype's ambient
   timers are simulation only.
7. **Letting Zone 1 and Zone 2 share state beyond the selection event.**
8. **Renaming/de-personifying the agents in copy** — keep names + present tense.
9. **Treating "Submit to SAM.gov" as an agent action** — it is always human.
10. **Carrying over prototype scaffolding** (in-browser Babel, `window.*`
    sharing, localStorage routing, the React-`key` remount workaround).

## Screenshots

The `screenshots/` folder contains captures of each screen in its key states —
use them as a visual checksum against your implementation.

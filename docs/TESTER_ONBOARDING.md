# Kaimi — Tester Onboarding (Pilot, Round 1)

**Last updated:** 2026-06-17

Welcome, and thanks for helping us put Kaimi through its paces. This guide tells you
exactly what you need, how to get in, and what to try. It should take ~20 minutes to
get set up and seeing real opportunities.

---

## What Kaimi does
Kaimi is an automated federal BD analyst. It:
1. **Hunts** SAM.gov every day for opportunities that fit *your* business (by NAICS, agency, set-aside).
2. **Scores** each one for fit and shows the reasoning — so you triage in minutes, not hours.
3. **Drafts** a first-pass proposal (Outline → Writer → compliance review) into **your own Google Drive** when you choose to pursue one.

You are always in control. Kaimi **never** submits anything — every proposal stops at a human-review gate.

---

## Your testing window
Your access is controlled by a **Product Key** (format `KAIMI-XXXX-XXXX-XXXX`) that we issue you.
It is valid for **your agreed testing window (7 or 14 days)** and then expires automatically.
If you need more time, just ask and we'll extend or reissue it.

---

## Before you start — have these ready
1. **A Google account** you want proposal documents created in (your Google Drive is where drafts land).
   - It can be a Google Workspace account or a personal Gmail — either works.
2. **Your BD profile details** (you'll enter these once, in-product):
   - Company name, **UEI** and **CAGE** code
   - **NAICS codes** you target (primary + secondary)
   - Agencies / contract types you focus on
   - Core **capabilities / competencies** (a few lines)
   - 2–3 **past-performance** references (client, scope, value)
3. **Your Product Key** — we'll send this to you separately.

> Round 1 runs on **BlueMeta's cloud** using BlueMeta's SAM.gov and AI infrastructure, so you
> do **not** need any API keys or cloud accounts of your own. (See *Privacy & data* below.)

---

## Step 1 — Get in
1. Go to **`<your Kaimi URL — we'll send it with your key>`**.
2. Enter your **Product Key** (`KAIMI-XXXX-XXXX-XXXX`). This unlocks access for your testing window.

## Step 2 — One-time setup (in-product onboarding)
The onboarding checklist walks you through:
1. **Company profile** — enter the BD details above. This is what tunes the hunt and scoring; you can edit it anytime.
2. **Connect Google Drive** — authorize the Google account where you want proposals created. Kaimi auto-creates a **"Kaimi Proposals"** folder there; you can change the destination.
3. **Run your first hunt** — kick off the first SAM.gov pass to populate your opportunities.

## Step 3 — Use it
1. **Opportunities** — review the scored list; open one to see the fit reasoning, eligibility, deadline, and estimated value.
2. **Pursue one** — select an opportunity to start a proposal. Watch the draft build section by section.
3. **Review at the gate** — Kaimi pauses for your review. The editor flags any **unresolved gaps** (missing facts the Writer couldn't ground) and shows a criteria checklist against the solicitation's must-haves.
4. **Approve or request changes** — request changes with a note and Kaimi revises addressing it; approve and it runs a final compliance check. The finished draft is in your **Kaimi Proposals** Drive folder.

---

## What we'd love feedback on
- Is the **opportunity feed relevant** to your business? (Too broad? Missing things?)
- Are the **fit scores + reasoning** trustworthy?
- Is the **drafted proposal** a useful first pass? Where does it fall short?
- Is the **gate / gap flagging** clear — did it catch what you'd expect?
- Anything confusing, slow, or broken.

How to send feedback: **`<feedback channel — email / shared doc>`**. Screenshots welcome.

---

## Not in this round (so you know)
- Self-serve deploy on *your own* cloud (round 1 is on BlueMeta's infrastructure).
- Desktop app (web only for now).
- Auto-submission to SAM.gov (by design — Kaimi never submits; you do).

---

## Privacy & data
- Your **company profile** and **generated proposals** are yours. Proposals are created in **your own Google Drive** (Kaimi requests the minimal `drive.file` scope — it can only see files it creates, not the rest of your Drive).
- Round 1 runs on **BlueMeta's managed cloud** (BlueMeta's SAM.gov + AI keys) so you can start with zero setup. Your environment is isolated; the long-term product is a per-customer isolated deployment.
- Access is gated by your time-limited Product Key and revocable at any time.

---

## Support
Questions or stuck? Contact **`<your name / email / phone>`** — happy to hop on a quick call to get you live.

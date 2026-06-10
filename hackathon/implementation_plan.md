# Kaimi - Hackathon Submission Plan

We are aiming for **Track 1: Build (Net-New Agents)** of the Google for Startups AI Agents Challenge. We have already completed Phase 0-1 of Kaimi, which gives us a solid, working Zone 1 pipeline (Hunter → Scorer → Queue) using the Google ADK and Gemini 2.5 Pro.

With the deadline of **June 11th** approaching quickly, we need to finalize the specific deliverables required for the submission.

## User Review Required

> [!IMPORTANT]
> Please review this plan. We need to decide whether we should spend time building more features (like the Zone 2 Manager/Writer) OR if we should lock down the current codebase (Phase 0-1) and focus purely on polishing the presentation, video, and test build. Given the tight deadline, I strongly recommend locking down the code and focusing on the deliverables.

## Open Questions

> [!WARNING]
> 1. **Video Demo:** How do you want to record the video? I can write a detailed script for you to follow, highlighting Kaimi's multi-agent architecture and Gemini integration.
> 2. **Testing Access:** For the judges, should we provide a pre-compiled binary, a Docker container, or just clear instructions to run `make build` and use test fixtures (since live SAM.gov API requires keys)?
> 3. **Architecture Diagram:** We have an awesome interactive React diagram (`SystemDesign.jsx`). Should we host this somewhere for the judges, or should I generate a static Mermaid diagram to embed directly in the README?

## Proposed Changes

### 1. Code Preparation
- Audit the current `Makefile` and build process to ensure it works flawlessly for external reviewers.
- Ensure all sensitive keys (like GCP, SAM.gov) are removed and `.env.example` is fully documented.
- Make sure the cached mode (using test fixtures) is the default so judges don't need a SAM.gov API key to run the Hunter and Scorer agents.

### 2. Architecture Diagram
- Create a high-quality Markdown Mermaid diagram based on `SystemDesign.jsx` that we can embed directly in the GitHub README.
- Polish `ARCHITECTURE.md` to highlight the use of MCP, Google ADK, and Gemini.

### 3. Video Demo Script
- Create a `DEMO_SCRIPT.md` artifact detailing exactly what to show on screen and what to say.
- The script will focus on the Track 1 criteria: Technical Implementation, Business Case, and Innovation.

### 4. Testing Access / Deployment
- Update the `README.md` with a dedicated "Hackathon Judges - Start Here" section.
- Detail exactly how to run the pipeline locally using mock data.

## Verification Plan

- Run a complete `make all` from scratch.
- Execute the Hunter and Scorer agents locally to verify the "demo mode" works end-to-end without failing due to missing API keys.
- Review the generated Architecture diagram for accuracy.

# Kaimi - Hackathon Demo Script
**Track 1: Build (Net-New Agents)**

> [!TIP]
> Keep the video under 3 minutes. Speak clearly, and emphasize how the Google ADK and Gemini are integrated to make the system fully autonomous but controllable.

## 0:00 - 0:30 | Introduction & The Problem
**[Visual]**: Start with a slide or screen showing the Kaimi logo / GitHub repo.
**[Script]**: "Hi, we are building **Kaimi**, an autonomous business-development pipeline for federal government contracting. We are submitting to Track 1 of the Google AI Agents Challenge. Government contracting is incredibly lucrative but painful—companies spend thousands of hours reading dense SAM.gov solicitations just to figure out if they should bid. We built a multi-agent system using the Google Agent Development Kit (ADK) and Gemini 2.5 Pro to automate this entire pipeline."

## 0:30 - 1:15 | Zone 1: The Pipeline (Architecture)
**[Visual]**: Show the `ARCHITECTURE.md` Mermaid diagram or `SystemDesign.jsx`.
**[Script]**: "Kaimi is split into two zones. Zone 1 is our scheduled batch pipeline. Every day, the **Hunter Agent** queries the SAM.gov API, filters out irrelevant opportunities based on our actual Capability Profile (NAICS codes, set-asides). Then, the **Scorer Agent** takes over. Using Gemini 2.5 Pro, it reads the complex solicitation documents, scores the opportunity against our capabilities, and provides a 'Bid', 'No Bid', or 'Review' recommendation with explainable reasoning. Let's see it in action."

## 1:15 - 2:00 | Live Demo
**[Visual]**: Open the terminal. Ensure your `.env` is populated with your API keys.
Run `go run ./cmd/pipeline --mode=live`.
Show the JSON output of the Scorer Agent with the reasoning and score.
**[Script]**: "Here we're running the live Kaimi pipeline. You can see it pulls real opportunities directly from SAM.gov, parses the requirements, and Gemini 2.5 Pro outputs a structured JSON assessment. Notice how the Scorer provides an explainable reason for its recommendation. All this is orchestrated cleanly using the ADK."

## 2:00 - 2:30 | Phase 2 & The Future
**[Visual]**: Go back to the Architecture diagram, pointing to Zone 2.
**[Script]**: "Once an opportunity hits our Queue, Zone 2 takes over. A Manager agent spins up for each proposal, orchestrating a Technical Writer and an Outline agent to draft the proposal. Crucially, we use the ADK's native human-in-the-loop features to pause the Manager at a 'Human Gate'. This ensures that a human reviews all outputs before submission—because we never let AI promise the government things we can't deliver."

## 2:30 - 3:00 | Conclusion
**[Visual]**: Show the GitHub CI/CD Actions page.
**[Script]**: "Finally, we treat AI quality as engineering. We built an AI-powered CI/CD pipeline using Gemini 2.5 Pro that automatically reviews PRs for bugs and even auto-fixes simple issues. Kaimi isn't just a prototype; it's a production-ready system ready for the Google Cloud Marketplace. Thank you!"

# Hackathon Testing Instructions

Welcome to the **Kaimi** testing guide! This document provides step-by-step instructions for judges to test the live Kaimi codebase.

Kaimi pulls federal contracting opportunities from the SAM.gov API and evaluates them using Gemini 2.5 Pro via the Google Agent Development Kit (ADK). 

## Prerequisites
1. **Go 1.21+** installed on your system.
2. A valid SAM.gov API key.
3. A Google Cloud Project ID with Vertex AI enabled (for Gemini 2.5 Pro).

---

## 1. Configure the Environment

Copy `.env.example` to a new `.env` file and fill in your keys:
```env
SAM_API_KEY=your-sam-api-key
GCP_PROJECT_ID=your-gcp-project-id
```

---

## 2. Verify the Test Suite

We treat AI quality as an engineering discipline: every package ships with a unit/contract
test layer that runs on every commit in CI, against cached fixtures (no live APIs).

Run the following command from the root of the repository:
```bash
go test ./...
```
**Expected Outcome**: You should see `ok` for every tested package. To run the live E2E test (real SAM.gov + Gemini, requires keys), use `KAIMI_E2E=1 go test -v ./internal/e2e`.

---

## 3. Running the Live Pipeline

Run the pipeline locally to see the system pull real opportunities from SAM.gov:
```bash
go run ./cmd/pipeline --mode=live
```

**What is happening under the hood?**
- **Hunter Agent** fetches live SAM.gov opportunities matching our NAICS codes.
- It filters out ineligible opportunities because BlueMeta's capability profile (`config/bluemeta_profile.yaml`) acts as a hard eligibility gate.
- **Scorer Agent** passes the eligible opportunities to Gemini 2.5 Pro to reason over the requirements and produce a `BID` or `NO_BID` score.
- Scored opportunities are written to the local Queue store.

## 4. Architecture Deep Dive
For a full view of how Kaimi uses the **Google Agent Development Kit (ADK)** to coordinate multiple agents (Zone 1 batch processing and Zone 2 per-proposal orchestration), please view the `ARCHITECTURE.md` file in the root directory. It contains a Mermaid diagram of the system flow.

Thank you for reviewing Kaimi!

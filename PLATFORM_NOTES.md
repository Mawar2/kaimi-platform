# PLATFORM_NOTES.md — ADK Go + Vertex AI Spike

Ticket: `1_platform_foundations_spike`

## What This Documents

Exact steps to reproduce a working Gemini call from a local Go program using Google ADK Go on Kaimi's GCP project. No cloud deployment required.

---

## SDK / Package Versions

| Package | Version | Role |
|---|---|---|
| `google.golang.org/adk` | v1.4.0 | Agent Development Kit (ADK) Go SDK |
| `google.golang.org/genai` | v1.59.0 | Pulled in transitively; provides `ClientConfig` and `Backend` constants |

---

## Prerequisites

1. **gcloud CLI** installed and Python available (gcloud is Python-based on Windows).
2. **Go 1.21+** (tested with go1.26.1 windows/amd64).
3. Authenticated to GCP:

```powershell
gcloud auth login
gcloud config set project kaimi-seeker
gcloud auth application-default login
```

Verify auth and project:

```powershell
gcloud auth list          # should show thaithimmy2003@gmail.com as active
gcloud config get-value project  # should print: kaimi-seeker
```

---

## Steps That Got It Working

### 1. Add ADK Go to the module

```powershell
go get google.golang.org/adk@v1.4.0
```

Then immediately run tidy — ADK has 30+ transitive deps and `go get` alone leaves go.sum incomplete:

```powershell
go mod tidy
```

**Gotcha #1:** If you skip `go mod tidy` and go straight to `go build`, you'll get a wall of `missing go.sum entry` errors. Just run tidy and they all resolve.

### 2. Write the agent

See `cmd/spike/main.go`. Key points:

- **Backend**: use `genai.BackendEnterprise` — this is the Vertex AI / Gemini Enterprise Agent Platform backend. Not `BackendVertexAI` (which also exists) and not `BackendGeminiAPI` (which needs an API key).
- **Auth**: no API key in code. `BackendEnterprise` picks up ADC automatically.
- **Project / Location**: hardcoded to `kaimi-seeker` / `us-east4`.
- **Launcher**: `full.NewLauncher()` from `google.golang.org/adk/cmd/launcher/full`. Passes `os.Args[1:]` so you can use subcommands.

### 3. Run it

```powershell
# Interactive (type prompts, Ctrl+Z to exit on Windows):
go run ./cmd/spike

# Non-interactive single prompt (pipe stdin):
echo "Hello from Kaimi!" | go run ./cmd/spike
```

Output seen:

```
User ->
Agent -> Kaimi platform OK — ADK agent running on Vertex AI.
User ->
EOF detected, exiting...
```

Confirmation with open-ended question (not in instruction):

```
echo "What model are you and what is today's date?" | go run ./cmd/spike
```

Output:

```
Agent -> I am a large language model, trained by Google.
         I cannot give you today's date as I do not have access to real-time information.
```

This confirms a live model call — not a cached or static response.

---

## Model Name Decision

**DECIDED (2026-06-09): locked to `gemini-2.5-pro`.** It is the current GA / stable pro
model on Vertex AI us-east4 and is what the code runs everywhere (`GEMINI_MODEL` default,
the Scorer, and the CI AI review). All docs now state Gemini 2.5 Pro.

Original spike notes (2026-06-04), kept for history:

| Model ID | Status | Notes |
|---|---|---|
| `gemini-2.5-pro` | **GA / stable — LOCKED IN** | What we run |
| `gemini-3-pro-preview` | Preview | Listed in Google AI API; not confirmed GA on Vertex AI us-east4 |
| `gemini-3.1-pro-preview` | Preview | Same |

A future upgrade to a Gemini 3 Pro model, once it is GA on Vertex AI, is a deliberate
follow-up change (swap `GEMINI_MODEL` / the scorer model constant) — not a doc fix.
Until then, "Gemini 2.5 Pro" is the single source of truth across the repo.

---

## Gotchas

1. **`go mod tidy` is required after `go get google.golang.org/adk`** — see Step 1 above.

2. **`gcloud` requires Python on Windows.** Run gcloud commands in PowerShell (not Git Bash `bash` tool) to avoid "Python was not found" errors.

3. **`BackendEnterprise` is the correct constant for Vertex AI.** The genai package has three backends: `BackendGeminiAPI` (API key), `BackendEnterprise` (Vertex AI / ADC), and `BackendVertexAI`. Use `BackendEnterprise` for the enterprise platform path.

4. **The full launcher defaults to interactive mode** (shows `User -> ` prompt). For scripted / one-shot use, pipe stdin: `echo "prompt" | go run ./cmd/spike`.

---

## Files Added

```
cmd/spike/main.go     — hello-world ADK agent (do not build Kaimi agents on top of this)
PLATFORM_NOTES.md     — this file
go.mod                — updated: added google.golang.org/adk v1.4.0 + transitive deps
go.sum                — updated: all dependency hashes
```

---

## What Was NOT Done (per ticket scope)

- No Memory Bank or Sessions configured
- No deployment to Vertex AI Agent Engine (local only)
- No Kaimi-specific agents (Hunter, Scorer, etc.) built on ADK yet
- No exhaustive docs survey

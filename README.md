# Kaimi - The Seeker

<!-- Kaimi automates federal BD: it hunts SAM.gov opportunities, scores them for fit, and drafts proposals — with human review before any submission. -->

**Kaimi** (Hawaiian for "the seeker") is an autonomous business-development pipeline for federal government contracting. It hunts live federal opportunities on SAM.gov, scores them bid/no-bid against a company's capabilities, and drafts tailored proposals - with human review before submission.

This is production infrastructure for BlueMeta Technologies' BD operations, built to run for years, not as a demo.

## 🏆 Judges — Start Here

**Run the Kaimi pipeline in one command — no API keys required.** Cached mode reads
bundled SAM.gov fixtures, runs the Hunter → Scorer agents, and writes scored
opportunities to a local queue:

```bash
go run ./cmd/pipeline --mode=cached --store-path=./queue
```

You'll see a Zone-1 summary (Fetched / Eligible / Dropped / Scored) and scored
`Opportunity` JSON records under `./queue/queue/`. Each record carries the Scorer's
bid/no-bid recommendation **and its explainable reasoning**.

**Verify the test suite (no keys, runs against fixtures):**

```bash
go test ./...
```

**Run it live (optional — needs keys):** real SAM.gov + Gemini 2.5 Pro via Vertex AI:

```bash
SAM_API_KEY=... GCP_PROJECT_ID=... go run ./cmd/pipeline --mode=live
```

**See the design:** [ARCHITECTURE.md](./ARCHITECTURE.md) has the two-zone system diagram;
[hackathon/DEMO_SCRIPT.md](./hackathon/DEMO_SCRIPT.md) is the 3-minute video walkthrough.

> 🖥️ **Dashboards:** web + desktop UIs that visualize the scored queue are in active
> development. Until they land, the pipeline run above is the fastest way to watch
> Kaimi work end to end; the scored JSON it produces is exactly what the dashboards render.

## Architecture

Kaimi operates in two distinct zones:

### Zone 1 - Scheduled Pipeline (Daily Batch)
```
Hunter → Scorer → Opportunity Queue (Dashboard)
```
- **Hunter**: Pulls and filters opportunities from SAM.gov by NAICS code
- **Scorer**: Scores each opportunity for bid/no-bid fit with reasoning
- **Queue**: Shared store of scored opportunities awaiting selection

### Zone 2 - Per-Proposal Lifecycle (Orchestrated)
```
Manager → Outline → Technical Writer → [HUMAN GATE] → Final Review
```
Triggered when an opportunity is selected. A Manager agent coordinates specialist agents to draft a complete proposal, pausing for human review before finalization.

See [ARCHITECTURE.md](./ARCHITECTURE.md) for full system design and [WORKFLOW.md](./WORKFLOW.md) for development workflow.

## Current Status

**The full pipeline is built and deployed**, and we are completing the product (dashboards
+ Zone-2 polish) for the Google AI Agents Challenge submission (June 11, 2026).

**Zone 1 — built & deployed:**
- ✅ **Hunter Agent** — SAM.gov integration with NAICS/eligibility gating against the real BlueMeta CapabilityProfile (UEI XVUEA59LY579, CAGE 9RY40; 11 NAICS codes; Small Business / SDB / Minority-Owned; 16 competencies, 9 past-performance projects)
- ✅ **Scorer Agent** — bid/no-bid scoring with explainable reasoning via **Gemini 2.5 Pro**
- ✅ **`cmd/pipeline`** — single-command Zone-1 runner (cached + live modes)
- ✅ **Deployed**: Cloud Run Job on Cloud Scheduler; scored queue persisted to GCS

**Zone 2 — built:**
- ✅ **Manager**, **Outline**, **Writer**, **Final Review** agents (`internal/manager`, `internal/outline`, `internal/writer`, `internal/finalreview`) with the human review gate
- ✅ **AgentResult** contract — the common return type every agent conforms to
- ✅ **Google Docs/Drive** integration foundation (`internal/gdocs`)

**Foundation & platform:**
- ✅ Forward-compatible `Opportunity` schema and `Store` interface (JSON-backed)
- ✅ **CI/CD** with automated AI code review + auto-fix bot (Gemini 2.5 Pro)
- ✅ `internal/dashboard` data layer (stage derivation + store-backed views)

**In active development:** web dashboard and offline-first desktop dashboard over the
shared `internal/dashboard` layer.

## Tech Stack

- **Language**: Go (for concurrency, Google-native fit, single-binary deployment)
- **Agent Framework**: Google ADK (Agent Development Kit) v1.0+
- **LLM**: Gemini 2.5 Pro via Vertex AI
- **Cloud**: Google Cloud Platform / Vertex AI

## Build Instructions

### Prerequisites
- Go 1.21 or later
- golangci-lint (for linting)

### Build
```bash
# Build all packages
make build

# Or build specific binary
go build -o bin/hunter ./cmd/hunter
```

## Run Instructions

### Zone-1 pipeline (Hunter → Scorer → Queue)

```bash
# Offline, no credentials — fixtures in, scored opportunities out
go run ./cmd/pipeline --mode=cached --store-path=./queue

# Live — real SAM.gov + Gemini 2.5 Pro (requires SAM_API_KEY and GCP_PROJECT_ID)
SAM_API_KEY=... GCP_PROJECT_ID=... go run ./cmd/pipeline --mode=live
```

Scored `Opportunity` records are written under the `--store-path` directory. See
[Judges — Start Here](#-judges--start-here) above for the full walkthrough.

## Test Instructions

```bash
# Run all tests
make test

# Run tests with verbose output
go test -v ./...

# Run tests for a specific package
go test -v ./internal/opportunity

# Run tests with coverage
go test -cover ./...
```

## Development Workflow

All development follows the workflow defined in [WORKFLOW.md](./WORKFLOW.md):
1. Work is tracked via GitHub Issues with approved acceptance criteria
2. Test-Driven Development (TDD) required
3. All PRs must pass tests and linter
4. **Automated AI code review** runs in CI using Gemini 2.5 Pro
5. **Auto-fix bot** automatically fixes simple issues (unused vars, formatting, basic best practices)
6. AI sub-agent code review before human review (optional deep analysis)
7. Human approval required for all merges

### AI-Powered CI/CD

The project uses **Gemini 2.5 Pro** for automated code quality:
- **AI Code Review**: Every PR gets reviewed for bugs, security, performance, and Go best practices
- **Auto-Fix Bot**: Simple issues are automatically fixed and committed to the PR
- **Cost**: ~$0.01-$0.06 per PR (within Gemini free tier)
- **Safety**: Auto-fixes require human review before merge; never auto-merges

See [CLAUDE.md](./CLAUDE.md) for details on the AI review and auto-fix system.

### Common Make Targets
```bash
make all        # Build, test, and lint (run before PR)
make build      # Build all binaries
make test       # Run all tests
make lint       # Run linter
make clean      # Remove build artifacts
make help       # Show all available targets
```

## Project Structure

```
.
├── cmd/
│   ├── pipeline/            # Zone-1 pipeline runner (Hunter → Scorer → Queue)
│   ├── hunter/              # Hunter agent entry point
│   ├── scorer/              # Scorer agent entry point
│   └── outline-probe/       # Outline developer probe tool
├── internal/
│   ├── agent/               # AgentResult contract and interfaces
│   ├── capability/          # CapabilityProfile for company capabilities
│   ├── opportunity/         # Opportunity schema
│   ├── store/               # Store interface for persistence (JSON-backed)
│   ├── samgov/              # SAM.gov API client
│   ├── pipeline/            # Zone-1 orchestration (Hunter + Scorer)
│   ├── scorer/              # Scoring logic and Gemini integration
│   ├── outline/             # Outline generation and formatting rules
│   ├── writer/              # Technical Writer agent (draft generation)
│   ├── manager/             # Zone-2 per-proposal orchestrator
│   ├── finalreview/         # Final Review agent with validation
│   ├── gdocs/               # Google Docs/Drive integration
│   ├── dashboard/           # Dashboard data layer (stage derivation + views)
│   ├── e2e/                 # End-to-end pipeline tests
│   └── github/              # GitHub API caching layer
├── config/
│   └── bluemeta_profile.yaml  # BlueMeta capability profile
├── test/
│   └── fixtures/            # Test fixtures (cached SAM.gov responses)
├── docs/                    # Additional documentation
├── .github/workflows/       # CI/CD pipeline (AI review + auto-fix)
├── ARCHITECTURE.md          # System architecture and design
├── WORKFLOW.md              # Engineering workflow contract
├── .golangci.yml            # Linter configuration
├── Makefile                 # Build automation
└── README.md                # This file
```

## Contributing

See [WORKFLOW.md](./WORKFLOW.md) for the complete development workflow. Key points:
- All work requires an approved GitHub Issue
- Follow TDD principles
- Maintain clear, well-commented Go code (legibility is a hard requirement)
- Run `make all` before submitting PRs

## License

Proprietary - BlueMeta Technologies

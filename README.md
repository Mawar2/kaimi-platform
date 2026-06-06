# Kaimi - The Seeker

<!-- Kaimi automates federal BD: it hunts SAM.gov opportunities, scores them for fit, and drafts proposals — with human review before any submission. -->

**Kaimi** (Hawaiian for "the seeker") is an autonomous business-development pipeline for federal government contracting. It hunts live federal opportunities on SAM.gov, scores them bid/no-bid against a company's capabilities, and drafts tailored proposals - with human review before submission.

This is production infrastructure for BlueMeta Technologies' BD operations, built to run for years, not as a demo.

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

**Phase 0-1 Complete**: Foundation, Hunter, and Capability Profile
- ✅ Go module initialized with full project structure
- ✅ Core interfaces defined (Store, SAM.gov Client, AgentResult contract)
- ✅ Opportunity schema designed for all phases
- ✅ **Hunter Agent** implemented with SAM.gov integration and eligibility gating
- ✅ **CapabilityProfile** implemented with real BlueMeta data (Issue #9)
  - UEI: XVUEA59LY579, CAGE: 9RY40
  - 11 NAICS codes (3 primary, 3 secondary, 5 tertiary)
  - Self-certified: Small Business, SDB, Minority-Owned
  - 16 core competencies, 9 past performance projects
- ✅ **Scorer Agent** implemented with Gemini 2.5 Pro integration (Issue #11)
- ✅ **Outline Agent** skeleton and formatting rules extraction (Issues #2, #4)
- ✅ **Final Review Agent** skeleton with validation logic (Issue #6)
  - Input validation and deadline checking
  - Ready for LLM-powered content checks (Issue #7)
- ✅ **CI/CD Pipeline** with AI code review and auto-fix bot
- ✅ GitHub API caching layer for performance

**In Progress**: Zone 2 agent development (Writer, Final Review checks)

**Total Closed Issues**: 10 (including foundation, Hunter, Scorer, Outline, Final Review skeleton, CI/CD)

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

### Hunter Agent
```bash
# Run the Hunter agent (placeholder in Phase 0)
./bin/hunter
```

**Note**: Full Hunter implementation is in progress. Current binary is a placeholder.

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
│   ├── hunter/              # Hunter agent entry point
│   ├── scorer/              # Scorer agent entry point
│   └── outline/             # Outline agent entry point
├── internal/
│   ├── agent/               # AgentResult contract and interfaces
│   ├── capability/          # CapabilityProfile for company capabilities
│   ├── opportunity/         # Opportunity schema
│   ├── store/               # Store interface for persistence
│   ├── samgov/              # SAM.gov API client
│   ├── scorer/              # Scoring logic and Gemini integration
│   ├── outline/             # Outline generation and formatting rules
│   ├── finalreview/         # Final review agent with validation
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

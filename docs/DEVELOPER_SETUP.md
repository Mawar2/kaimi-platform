# Developer Setup Guide - Kaimi Project

**Last updated:** 2026-06-09

Welcome to the Kaimi project! This guide will help you get set up for development.

## Your GCP Access

You've been granted access to the **kaimi-seeker** GCP project with the following permissions:

### Roles Granted
- ✅ **Vertex AI Admin** (`roles/aiplatform.admin`) - Full access to build and manage models with Gemini 2.5 Pro
- ✅ **Viewer** (`roles/viewer`) - View all project resources
- ✅ **Secret Manager Secret Accessor** (`roles/secretmanager.secretAccessor`) - Read API keys and secrets

### Project Details
- **Project ID:** `kaimi-seeker`
- **Project Name:** Kaimi - The Seeker
- **Region:** `us-east4`
- **Billing:** Google AI Hackathon account

---

## Prerequisites

### 1. Install Required Tools

**Go (1.21+):**
- Download: https://go.dev/download/
- Verify: `go version`

**gcloud CLI:**
- Download: https://cloud.google.com/sdk/docs/install
- Verify: `gcloud --version`

**golangci-lint:**
```bash
# macOS
brew install golangci-lint

# Linux
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin

# Windows
# Download from https://github.com/golangci/golangci-lint/releases
```

**Git:**
- Already installed if you can clone repos
- Verify: `git --version`

---

## GCP Setup

### 1. Authenticate with Google Cloud

```bash
# Login with your Google account (thaithimmy2003@gmail.com)
gcloud auth login

# Set the project
gcloud config set project kaimi-seeker

# Verify access
gcloud projects describe kaimi-seeker
```

You should see:
```
projectId: kaimi-seeker
name: Kaimi - The Seeker
```

### 2. Set Up Application Default Credentials (ADC)

For local development, set up ADC so your code can authenticate:

```bash
gcloud auth application-default login
```

This allows your local Go code to access:
- Vertex AI (Gemini 2.5 Pro)
- Secret Manager (SAM.gov API key)
- Other GCP services

### 3. Verify Access

**Test Vertex AI access:**
```bash
gcloud ai models list --region=us-east4 --limit=5
```

**Test Secret Manager access:**
```bash
gcloud secrets versions access latest --secret=samgov-api-key
```

You should see your SAM.gov API key value (it starts with `SAM-`). Never paste the raw key into docs or commit it.

---

## Project Setup

### 1. Clone the Repository

```bash
git clone https://github.com/Mawar2/Kaimi.git
cd Kaimi
```

### 2. Install Go Dependencies

```bash
go mod download
```

### 3. Setup Environment Variables (Automated)

Run the setup script to automatically fetch API keys from Secret Manager:

**On Windows:**
```cmd
scripts\setup-env.bat
```

**On Mac/Linux:**
```bash
chmod +x scripts/setup-env.sh
./scripts/setup-env.sh
```

This will automatically:
- Verify your GCP authentication
- Fetch API keys from Secret Manager
- Create your `.env` file with all necessary secrets

### 4. Verify Setup

```bash
# Run tests
make test

# Run linter
make lint

# Build binaries
make build
```

All commands should complete successfully.

---

## Environment Configuration

### Automated Setup (Recommended)

Use the setup scripts in the `scripts/` directory:
- `scripts/setup-env.bat` (Windows)
- `scripts/setup-env.sh` (Mac/Linux)

See [scripts/README.md](../scripts/README.md) for details.

### Manual Setup

If you prefer to set up manually:

#### Option 1: Retrieve API Keys from Secret Manager

You can fetch API keys directly from GCP Secret Manager:

```bash
# SAM.gov API Key
gcloud secrets versions access latest --secret=samgov-api-key

# Google AI Studio API Key (for ADK Go)
gcloud secrets versions access latest --secret=google-ai-studio-api-key
```

Add them to your `.env` file:

```bash
# SAM.gov API Key
SAM_API_KEY=$(gcloud secrets versions access latest --secret=samgov-api-key)

# Google AI Studio API Key
GOOGLE_API_KEY=$(gcloud secrets versions access latest --secret=google-ai-studio-api-key)
```

Or manually copy the output and paste into your `.env` file.

#### Option 2: Get API Keys from Team Lead

If you don't have Secret Manager access, contact the team lead (Malik) for the API keys.

**Important:**
- The `.env` file is in `.gitignore` and will NOT be committed
- Never commit API keys to the repository

### GCP Authentication Options

### Option 1: Use Application Default Credentials (Recommended for you)

Since you ran `gcloud auth application-default login`, your code will automatically use your user credentials.

**No additional setup needed!** Your user account has all necessary permissions.

### Option 2: Use Service Account Key (CI/CD method)

If you want to match the CI/CD environment exactly:

1. Ask the project owner for `kaimi-sa-key.json`
2. Set environment variable:
   ```bash
   export GOOGLE_APPLICATION_CREDENTIALS=/path/to/kaimi-sa-key.json
   ```

**For local development, Option 1 is easier and recommended.**

---

## Development Workflow

### Required Reading

Before writing any code, read these documents:

1. **[ARCHITECTURE.md](../ARCHITECTURE.md)** - System design and phase roadmap
2. **[WORKFLOW.md](../WORKFLOW.md)** - Engineering workflow contract (CRITICAL!)

### Key Workflow Rules

From WORKFLOW.md, you **must** follow these rules:

#### 1. No Work Without a Ticket
- Every task requires a GitHub Issue with approved acceptance criteria
- Create the Issue first, get approval, then code
- Reference the Issue number in commits and PRs

#### 2. Test-Driven Development (TDD)
```bash
# 1. Write the failing test first
# 2. Run it and watch it fail
make test

# 3. Write code to make it pass
# 4. Run tests again
make test

# 5. All tests must pass before committing
```

#### 3. Commit Format
```
<issue_number>_<feature_description>

Example: 12_hunter_samgov_cached_mode
```

#### 4. Before Opening a PR
```bash
# Run all checks
make all

# This runs:
# - make build (compiles code)
# - make test (runs all tests)
# - make lint (runs linter)

# All must pass!
```

#### 5. Pull Request Requirements
- PR must reference a GitHub Issue (in title or description)
- All tests must pass
- Linter must pass
- CI/CD pipeline must pass
- Human approval required (Malik or team lead)

### Common Make Commands

```bash
make help       # Show all available commands
make build      # Build all binaries
make test       # Run all tests
make lint       # Run linter
make all        # Build + test + lint (run before PR!)
make clean      # Remove build artifacts
```

---

## Live OAuth End-to-End Test (WS-B7)

The Workspace OAuth sign-in path has two test layers. The default `go test`
runs the fast, fully-mocked unit tests (`internal/httpapi/auth_test.go`). A
second, `live`-tagged test (`internal/httpapi/auth_live_test.go`) exercises the
pieces that can only be checked against **real Google**. It is **excluded from the
default `go test` and never runs in CI** — it requires a real Google Workspace
test account.

**What it verifies automatically** (no browser, no human):
- The real `AuthHandler` builds a genuine `accounts.google.com` consent URL
  carrying `client_id`, the configured `redirect_uri`, a CSRF `state`, an S256
  PKCE `code_challenge`, and the `hd` Workspace hint.
- If a real Google-issued ID token is supplied, it runs the real
  `idtoken.Validate` against live Google, then drives the real callback and
  asserts enforcement: an **in-domain, verified** token mints a session; an
  **out-of-domain** token is rejected **403** with no session.

**What is manual** (documented, not automated): obtaining the ID token. Full
3-legged OAuth needs an interactive browser consent, which a Go test cannot
drive. Complete the consent once with a Workspace **test account** (e.g. via the
[OAuth 2.0 Playground](https://developers.google.com/oauthplayground) configured
with this client id and the `openid email profile` scopes), or mint an OIDC ID
token whose audience equals `OAUTH_CLIENT_ID`. ID tokens expire (~1 hour) — mint
them just before running.

**Environment:**

```bash
# Required (mirrors LoadOAuthConfig); missing any → the test skips:
export OAUTH_CLIENT_ID=...           # also the ID-token audience
export OAUTH_CLIENT_SECRET=...
export OAUTH_REDIRECT_URL=https://app.example.com/auth/callback
export OAUTH_ALLOWED_DOMAIN=yourdomain.com
export SESSION_SECRET=...            # never commit; from Secret Manager in prod

# Optional — enable the real-token sub-cases (each skips if unset):
export KAIMI_TEST_ID_TOKEN=...           # in-domain, email-verified account
export KAIMI_TEST_ID_TOKEN_FOREIGN=...   # account OUTSIDE the allowed domain
```

**Run:**

```bash
go test -tags live ./internal/httpapi/...
```

Never paste real tokens or secrets into docs, commits, or logs — the test logs
only non-identifying structure (status codes, cookie flags), never the token or
email.

---

## Project State

**Built and deployed:**
- ✅ Project foundation, Go module, core interfaces (`Store`, SAM.gov client)
- ✅ `Opportunity` schema and the `AgentResult` contract
- ✅ Zone-1 pipeline (Hunter → Scorer → Queue) deployed as the `kaimi-pipeline` Cloud Run Job, run on Cloud Scheduler (07:00 / 12:00 / 17:00 ET); scored JSON store persisted to `gs://kaimi-seeker-queue`
- ✅ Zone-2 agents (Manager / Outline / Writer / Final Review / gdocs)

**In active development:**
- 🚧 Web and desktop dashboards over the `internal/dashboard` data layer

**Optional / future:**
- Firestore swap behind the `Store` interface (no agent code changes required)

**Scope Discipline:**
Build the full product, but keep every ticket tightly scoped to its acceptance criteria. Don't gold-plate. See ARCHITECTURE.md and CLAUDE.md for details.

---

## Vertex AI / Gemini Access

You have full admin access to Vertex AI. You can:

### Use Gemini 2.5 Pro

```go
// Example: Using Vertex AI in Go
import (
    aiplatform "cloud.google.com/go/aiplatform/apiv1"
    "google.golang.org/api/option"
)

// Your ADC will authenticate automatically
client, err := aiplatform.NewPredictionClient(ctx)
```

### Test Gemini Access

```bash
# List available models (you have permission)
gcloud ai models list --region=us-east4

# Test Gemini endpoint access
gcloud ai endpoints list --region=us-east4
```

---

## Secret Manager Access

You can read secrets (but not create/modify them).

### Read SAM.gov API Key

```bash
# Via gcloud
gcloud secrets versions access latest --secret=samgov-api-key

# In Go code (using Google ADK)
# The Hunter agent uses this for SAM.gov API calls in the deployed pipeline
```

---

## Project Structure

```
.
├── cmd/
│   └── hunter/              # Hunter agent entry point
├── internal/
│   ├── opportunity/         # Opportunity schema
│   ├── store/              # Store interface for persistence
│   └── samgov/             # SAM.gov API client
├── test/
│   └── fixtures/           # Test fixtures (cached responses)
├── docs/                   # Documentation
│   ├── GCP_SETUP.md       # GCP setup guide
│   └── DEVELOPER_SETUP.md # This file
├── ARCHITECTURE.md         # System architecture (READ FIRST!)
├── WORKFLOW.md            # Development workflow (READ FIRST!)
├── Makefile               # Build automation
└── README.md              # Project overview
```

---

## Git Workflow

### 1. Create a Feature Branch

```bash
# Format: <issue_number>_<feature_name>
git checkout -b 5_hunter_implementation
```

### 2. Make Changes (TDD!)

```bash
# Write tests first
# Run tests (they should fail)
make test

# Write code
# Run tests again (they should pass)
make test
```

### 3. Commit Changes

```bash
git add .
git commit -m "5_hunter_samgov_client

Implemented SAM.gov API client with:
- Opportunity fetching by NAICS code
- Cached mode for testing
- Error handling and retries

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

### 4. Push and Open PR

```bash
git push origin 5_hunter_implementation
```

Then open a PR on GitHub with:
- Title referencing the Issue: "[#5] Hunter SAM.gov client implementation"
- Description with acceptance criteria checklist

---

## CI/CD Pipeline

On every push, GitHub Actions runs:

1. ✅ **Tests** - All Go tests must pass
2. ✅ **Lint** - golangci-lint must pass
3. ✅ **GCP Verification** - Verify Vertex AI and Secret Manager access
4. ✅ **Acceptance Criteria Check** - PR must reference an Issue

**All must pass before merge!**

View pipeline status: https://github.com/Mawar2/Kaimi/actions

---

## Getting Help

### Documentation
- **Architecture:** [ARCHITECTURE.md](../ARCHITECTURE.md)
- **Workflow:** [WORKFLOW.md](../WORKFLOW.md)
- **GCP Setup:** [docs/GCP_SETUP.md](./GCP_SETUP.md)
- **README:** [README.md](../README.md)

### Team
- **Project Owner:** malik@bluemetatech.com
- **Repository:** https://github.com/Mawar2/Kaimi

### Common Issues

**"Permission denied" errors:**
- Make sure you ran `gcloud auth login`
- Verify project is set: `gcloud config get-value project`

**Tests failing:**
- Run `go mod download` to ensure dependencies are installed
- Check that you're in the project root directory

**Linter errors:**
- Run `golangci-lint run` to see specific issues
- Follow Go conventions and code style

---

## Quick Start Checklist

- [ ] Install Go, gcloud CLI, golangci-lint
- [ ] Authenticate: `gcloud auth login`
- [ ] Set project: `gcloud config set project kaimi-seeker`
- [ ] Set up ADC: `gcloud auth application-default login`
- [ ] Clone repository
- [ ] Run `go mod download`
- [ ] Verify: `make all`
- [ ] Read ARCHITECTURE.md
- [ ] Read WORKFLOW.md
- [ ] Create a GitHub Issue for your first task
- [ ] Get acceptance criteria approved
- [ ] Start coding with TDD!

---

## Welcome to Kaimi! 🚀

You're all set to start building. Remember:
- **Read ARCHITECTURE.md and WORKFLOW.md first**
- **No work without an approved GitHub Issue**
- **TDD is required (write tests first!)**
- **All checks must pass before PR**

Happy coding! 🎯

# Kaimi Documentation

This directory contains all project documentation for the Kaimi autonomous BD pipeline system.

## Quick Links

### Getting Started
- **[GCP Quick Start](./GCP_QUICKSTART.md)** - Fast-track GCP environment setup (5 minutes)
- **[GCP Setup Guide](./GCP_SETUP.md)** - Comprehensive GCP infrastructure documentation

### Core Documentation
- **[ARCHITECTURE.md](../ARCHITECTURE.md)** - System architecture and design principles
- **[WORKFLOW.md](../WORKFLOW.md)** - Engineering workflow and development process
- **[README.md](../README.md)** - Project overview and build instructions

## Documentation Index

### Infrastructure & Setup

| Document | Purpose | When to Read |
|----------|---------|--------------|
| [GCP_QUICKSTART.md](./GCP_QUICKSTART.md) | One-command GCP setup | First time setting up GCP environment |
| [GCP_SETUP.md](./GCP_SETUP.md) | Detailed GCP configuration guide | Need to understand or troubleshoot GCP setup |

### Architecture & Design

| Document | Purpose | When to Read |
|----------|---------|--------------|
| [ARCHITECTURE.md](../ARCHITECTURE.md) | System design and phase roadmap | Before writing any code; essential context |
| [WORKFLOW.md](../WORKFLOW.md) | Development workflow contract | Before starting any feature work |

### Development

| Document | Purpose | When to Read |
|----------|---------|--------------|
| [README.md](../README.md) | Build, test, and run instructions | Daily development reference |

## GCP Setup Summary

### Prerequisites
- gcloud CLI installed and authenticated
- GCP project with billing enabled
- Owner or Editor permissions

### Quick Setup
```bash
# Bash (Linux/Mac/Git Bash)
bash scripts/setup-gcp.sh

# PowerShell (Windows)
.\scripts\setup-gcp.ps1
```

### What Gets Created
- ✅ Service account: `kaimi-dev@PROJECT_ID.iam.gserviceaccount.com`
- ✅ Service account key: `kaimi-sa-key.json` (local only, never commit!)
- ✅ Secret Manager secret: `samgov-api-key`
- ✅ Environment file: `.env.gcp`
- ✅ APIs enabled: Vertex AI, Secret Manager, IAM, Cloud Build

### Post-Setup Actions
1. Add SAM.gov API key to Secret Manager
2. Configure GitHub repository secrets for CI/CD
3. Verify access with test commands

See [GCP_QUICKSTART.md](./GCP_QUICKSTART.md) for detailed post-setup steps.

## Development Workflow

All development follows the workflow defined in [WORKFLOW.md](../WORKFLOW.md):

1. **Work requires approved GitHub Issue** - No code without a ticket
2. **Test-Driven Development (TDD)** - Write tests first
3. **CI gates must pass** - Tests + linter + GCP verification
4. **AI sub-agent code review** - Before human review
5. **Human approval required** - For all merges

### Before Starting a Feature
1. [ ] Verify GitHub Issue exists with approved acceptance criteria
2. [ ] Read ARCHITECTURE.md to understand system design
3. [ ] Understand which phase you're working in (currently Phase 0)
4. [ ] Create feature branch: `<issue_number>_<feature_name>`

### Before Opening a PR
1. [ ] All tests pass: `make test`
2. [ ] Linter passes: `make lint`
3. [ ] Build succeeds: `make build`
4. [ ] Code reviewed by AI sub-agent
5. [ ] PR title/branch references the ticket number

## Architecture Quick Reference

### Two-Zone System

**Zone 1 - Scheduled Pipeline (no orchestrator)**
```
Hunter → Scorer → Opportunity Queue
```

**Zone 2 - Per-Proposal Lifecycle (orchestrated)**
```
Manager → Outline → Technical Writer → [HUMAN GATE] → Final Review
```

### Current Phase: Phase 0

**Building Now:**
- ✅ Project foundation and structure
- ✅ Go module and packages
- ✅ Core interfaces (Store, SAM.gov Client)
- ✅ Opportunity schema
- 🚧 Hunter agent implementation (next)

**Not Building Yet:**
- ❌ Scorer, Manager, or other agents
- ❌ Databases (Firestore)
- ❌ Scheduling infrastructure
- ❌ AgentResult contract

Principle: **Lazy provisioning, eager design**

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.21+ |
| Agent Framework | Google ADK (Agent Development Kit) |
| LLM | Gemini 2.5 Pro via Vertex AI |
| Cloud Platform | Google Cloud Platform |
| Testing | Go standard library + fixtures |
| Linting | golangci-lint |
| CI/CD | GitHub Actions |

## Security & Best Practices

### Never Commit (Already in .gitignore)
- ❌ `kaimi-sa-key.json` - Service account credentials
- ❌ `.env.gcp` - Environment configuration
- ❌ Any `*-sa-key.json` files
- ❌ `queue/*.json` - Local queue files

### Key Rotation
Rotate service account keys every 90 days:
```bash
gcloud iam service-accounts keys list --iam-account=kaimi-dev@PROJECT.iam.gserviceaccount.com
gcloud iam service-accounts keys delete KEY_ID --iam-account=kaimi-dev@PROJECT.iam.gserviceaccount.com
gcloud iam service-accounts keys create kaimi-sa-key.json --iam-account=kaimi-dev@PROJECT.iam.gserviceaccount.com
```

### Secret Management
- SAM.gov API key stored in GCP Secret Manager
- Never log secret values in application code
- Use `gcloud secrets` commands to update secrets

## Troubleshooting

### Common Issues

**Problem:** gcloud commands fail with permission errors
**Solution:** Verify you have Owner/Editor role on the GCP project

**Problem:** Vertex AI access denied
**Solution:** Ensure service account has `roles/aiplatform.user` role

**Problem:** CI pipeline fails on GCP verification
**Solution:** Verify GitHub secrets are configured correctly

See [GCP_SETUP.md](./GCP_SETUP.md#troubleshooting) for comprehensive troubleshooting.

## Cost Monitoring

**Phase 0 Expected Costs:** < $1/month (excluding Gemini usage)

| Service | Cost |
|---------|------|
| Vertex AI | Pay-per-use (minimal until Hunter runs) |
| Secret Manager | ~$0.06/month per secret |
| Cloud Build | 120 build-minutes/day free tier |
| IAM | Free |

Monitor: [GCP Console Billing](https://console.cloud.google.com/billing)

## Contributing

See [WORKFLOW.md](../WORKFLOW.md) for the complete development workflow.

Key principles:
- **Legibility is a hard requirement** - Code must be clear and well-commented
- **No work without a ticket** - All work references a GitHub Issue
- **TDD is required** - Write tests first
- **No building ahead** - Only build Phase 0 components now

## Additional Resources

### Google Cloud Platform
- [Vertex AI Documentation](https://cloud.google.com/vertex-ai/docs)
- [Secret Manager Documentation](https://cloud.google.com/secret-manager/docs)
- [Google ADK Documentation](https://cloud.google.com/agent-builder/docs)

### Go Development
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [golangci-lint Documentation](https://golangci-lint.run/)

### SAM.gov API
- [SAM.gov API Documentation](https://open.gsa.gov/api/entity-api/)
- [Opportunity Management API](https://sam.gov/data-services/Opportunity%20Management)

## Questions?

- **Architecture questions:** See [ARCHITECTURE.md](../ARCHITECTURE.md)
- **Workflow questions:** See [WORKFLOW.md](../WORKFLOW.md)
- **GCP setup issues:** See [GCP_SETUP.md](./GCP_SETUP.md#troubleshooting)
- **Build/test issues:** See [README.md](../README.md)

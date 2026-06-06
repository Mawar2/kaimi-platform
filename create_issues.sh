#!/bin/bash
# Script to create 8 issues in multi-agent-system repo
# Run this after ensuring gh CLI is authenticated

echo "Creating 8 issues in multi-agent-system repo..."

gh issue create --repo Mawar2/multi-agent-system \
  --title "Documentation - write operator runbook" \
  --body "Create docs/RUNBOOK.md with: Getting Started, Operation, Config, Troubleshooting, Monitoring, Maintenance, Security. **Skills:** Technical writing | **Effort:** 2 days

_Moved from Kaimi #38_"

gh issue create --repo Mawar2/multi-agent-system \
  --title "CI/CD pipeline - GitHub Actions for test, build, deploy" \
  --body "3 workflows: PR (test/lint/build), Main (staging deploy), Release (production). **Skills:** GitHub Actions, Docker | **Effort:** 3 days

_Moved from Kaimi #37_"

gh issue create --repo Mawar2/multi-agent-system \
  --title "Containerization - create Dockerfile and docker-compose.yml" \
  --body "Multi-stage Dockerfile (<50MB), docker-compose.yml, health check, docs. **Skills:** Docker | **Effort:** 2 days

_Moved from Kaimi #36_"

gh issue create --repo Mawar2/multi-agent-system \
  --title "Load testing - test with 100+ issues and 50+ workers" \
  --body "Stress test: find leaks, race conditions. Tools: go test -race, pprof. 4 scenarios. **Skills:** Go performance testing | **Effort:** 3 days

_Moved from Kaimi #35_"

gh issue create --repo Mawar2/multi-agent-system \
  --title "Build integration test suite with mocked GitHub and Claude CLI" \
  --body "E2E tests: happy path, worker failure, routing, duplicate detection. Mock backends. **Skills:** Go testing, mocking | **Effort:** 1 week

_Moved from Kaimi #34_"

gh issue create --repo Mawar2/multi-agent-system \
  --title "Build GeminiWorker with plan-execute pattern (Phase 2)" \
  --body "Worker using Gemini API: GeminiWorker → Gemini (plan) → GitExecutor (execute) → GitHub (PR). See .claude/plans/tingly-mapping-graham.md | **Skills:** Go, Gemini API, git | **Effort:** 2 weeks

_Moved from Kaimi #29_"

gh issue create --repo Mawar2/multi-agent-system \
  --title "Smart issue filtering - skip issues that should not be auto-worked" \
  --body "Filter out issues with PRs, labeled needs-human-design, or missing AC. Configurable in orchestrator.yml. **Skills:** Go, GitHub API | **Effort:** 1 week

_Moved from Kaimi #28_"

gh issue create --repo Mawar2/multi-agent-system \
  --title "Build observability dashboard for supervisor and workers" \
  --body "Create web dashboard with REST API endpoint /api/status showing active workers, queue depth, success rates. **Stack:** Go REST API, React/Vue | **Effort:** 1 week

_Moved from Kaimi #25_"

echo "Done! Created 8 issues in multi-agent-system."

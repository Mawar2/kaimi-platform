# Run Multi-Agent Supervisor - Self-Improvement Mode

## Objective
Run the multi-agent supervisor to autonomously work on its own improvement tasks. The supervisor will monitor the multi-agent-system repo, claim issues, and create PRs to improve itself.

## What This Does
The supervisor is configured to monitor TWO repositories:
1. **Kaimi** - Federal BD pipeline agents (existing work)
2. **multi-agent-system** - The orchestrator itself (META: self-improvement!)

The supervisor will autonomously:
- Poll multi-agent-system repo for open issues
- Route issues to ClaudeCodeWorker based on complexity
- Workers claim tasks, implement features, create PRs
- System literally improves itself

## Current State
✅ **8 issues ready** in multi-agent-system repo (#4-#11):
- #4: Documentation - operator runbook
- #5: CI/CD pipeline - GitHub Actions
- #6: Containerization - Docker/docker-compose
- #7: Load testing - 100+ issues, 50+ workers
- #8: Integration tests - mocked backends
- #9: GeminiWorker - plan-execute pattern
- #10: Smart issue filtering
- #11: Observability dashboard

✅ **Configuration complete**:
- `config/orchestrator.yml` - monitors both repos
- ClaudeCodeWorker enabled
- Quality gates configured

## How to Run

### 1. Verify Prerequisites
```bash
cd /c/Users/Owner/OneDrive/Documents/Builder/Pulse

# Check config exists
cat config/orchestrator.yml

# Verify supervisor builds
go build -o bin/supervisor cmd/supervisor/main.go
```

### 2. Run Supervisor
```bash
# Option A: Run directly
go run cmd/supervisor/main.go

# Option B: Run compiled binary
./bin/supervisor
```

### 3. What to Expect
The supervisor will:
1. Start polling loop (every 60 seconds)
2. Fetch issues from multi-agent-system repo
3. Route issues to workers (Simple → ClaudeCodeWorker)
4. Workers claim tasks and start implementing
5. Log output shows: `[Supervisor] Polling projects...`, `[Worker 1] Claimed task X`

### 4. Monitor Progress
Watch the logs for:
- `[Supervisor] Found X tasks in queue`
- `[Worker N] Starting task: <issue title>`
- `[Worker N] Creating PR for issue #X`
- `[QualityGates] Running pre-PR validation...`

### 5. Review PRs
As workers complete tasks, they'll create PRs in multi-agent-system repo:
- Check https://github.com/Mawar2/multi-agent-system/pulls
- Review each PR (tests, lint, quality checks should pass)
- Merge when ready

## Expected Behavior

**First run (~5-10 minutes):**
- Supervisor polls multi-agent-system
- Classifies issue #4 (Documentation) as "simple"
- Routes to ClaudeCodeWorker
- Worker claims task, clones repo
- Worker creates docs/RUNBOOK.md
- Runs quality gates (test/lint/fmt)
- Creates PR with title "feat: add operator runbook"

**Subsequent runs:**
- Workers continue claiming tasks in priority order
- Multiple workers can run concurrently (max 3 for ClaudeCodeWorker)
- PRs created as each task completes

## Troubleshooting

**No tasks claimed:**
- Check that issues exist: `gh issue list --repo Mawar2/multi-agent-system`
- Verify orchestrator.yml has correct repo name
- Check supervisor logs for errors

**Quality gates failing:**
- Expected! Some PRs may not pass first time
- Review the issue, create follow-up task to fix
- Or manually fix and push to PR branch

**Worker stuck:**
- Supervisor detects stalled tasks after 5 minutes
- Will release task back to queue automatically
- Check logs for error details

## Success Criteria
✅ Supervisor runs without errors
✅ At least 1 issue claimed by a worker
✅ At least 1 PR created in multi-agent-system repo
✅ Quality gates run on PR
✅ System demonstrates self-improvement capability

## Notes
- This is **Phase 1 MVP** - using ClaudeCodeWorker (local CLI)
- **Phase 2** will add GeminiWorker (one of the issues!)
- Supervisor can run indefinitely - will keep polling for new issues
- Safe to Ctrl+C to stop - tasks are persisted in JSON queue

## Meta Moment
This is the supervisor **improving itself**. The orchestrator is autonomously:
- Building its own CI/CD pipeline (#5)
- Containerizing itself (#6)
- Adding its own dashboard (#11)
- Implementing the GeminiWorker that will work alongside ClaudeCodeWorker (#9)

It's a **self-evolving system**. 🤯

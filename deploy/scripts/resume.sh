#!/bin/bash
# resume.sh — resume a paused Kaimi instance's pipeline schedule (cost control).
#
# Flips the Cloud Scheduler job back to ENABLED via gcloud, directly and
# instantly, WITHOUT a terraform run. After resuming, the pipeline fires again on
# its cron schedule (default 07:00/12:00/17:00 ET). Counterpart to pause.sh. For
# a durable, declarative resume prefer `terraform apply -var active=true`; this
# script is the operator one-liner for an immediate resume.
#
# Usage:
#   PROJECT=acme-kaimi-prod REGION=us-east4 ./resume.sh
#   ./resume.sh --project acme-kaimi-prod --region us-east4 [--job kaimi-pipeline-schedule]
#
# Args override env. Defaults: REGION=us-east4, JOB=kaimi-pipeline-schedule
# (matches the module's scheduler_job_name). PROJECT is required.
#
# Idempotent: resuming an already-enabled job is a no-op that still exits 0.

set -euo pipefail

PROJECT="${PROJECT:-}"
REGION="${REGION:-us-east4}"
JOB="${JOB:-kaimi-pipeline-schedule}"

while [ $# -gt 0 ]; do
  case "$1" in
    --project) PROJECT="$2"; shift 2 ;;
    --region)  REGION="$2";  shift 2 ;;
    --job)     JOB="$2";     shift 2 ;;
    -h|--help)
      echo "Usage: $0 --project <id> [--region <region>] [--job <name>]"
      echo "   or: PROJECT=<id> REGION=<region> JOB=<name> $0"
      exit 0 ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [ -z "$PROJECT" ]; then
  echo "Error: project is required (set PROJECT=... or pass --project <id>)." >&2
  exit 2
fi

echo "Resuming Kaimi pipeline schedule:"
echo "  project = $PROJECT"
echo "  region  = $REGION"
echo "  job     = $JOB"

# `scheduler jobs resume` is idempotent on an already-enabled job.
gcloud scheduler jobs resume "$JOB" --location "$REGION" --project "$PROJECT"

echo "Resumed. The pipeline will run again on its schedule (default 07:00/12:00/17:00 ET)."

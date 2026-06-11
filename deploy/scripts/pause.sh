#!/bin/bash
# pause.sh — pause a deployed Kaimi instance's pipeline schedule (cost control).
#
# This flips the Cloud Scheduler job to PAUSED via gcloud, directly and instantly,
# WITHOUT a terraform run. A paused schedule fires no pipeline runs, so the
# recurring Gemini/Vertex + SAM.gov spend stops. The Cloud Run Service and Job
# stay scaled to zero, so a paused deployment costs only tiny GCS/registry
# storage. No data is lost. Resume with resume.sh (or `terraform apply -var
# active=true`). For a durable, declarative pause prefer `terraform apply -var
# active=false`; this script is the operator one-liner for an immediate pause.
#
# Usage:
#   PROJECT=acme-kaimi-prod REGION=us-east4 ./pause.sh
#   ./pause.sh --project acme-kaimi-prod --region us-east4 [--job kaimi-pipeline-schedule]
#
# Args override env. Defaults: REGION=us-east4, JOB=kaimi-pipeline-schedule
# (matches the module's scheduler_job_name). PROJECT is required.
#
# Idempotent: pausing an already-paused job is a no-op that still exits 0.

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

echo "Pausing Kaimi pipeline schedule:"
echo "  project = $PROJECT"
echo "  region  = $REGION"
echo "  job     = $JOB"

# `scheduler jobs pause` is idempotent on an already-paused job (gcloud returns
# success), so no pre-check is needed.
gcloud scheduler jobs pause "$JOB" --location "$REGION" --project "$PROJECT"

echo "Paused. The pipeline will not run until resumed (resume.sh or 'terraform apply -var active=true')."
echo "Recurring Gemini/SAM spend is now stopped; data and infrastructure are unchanged."

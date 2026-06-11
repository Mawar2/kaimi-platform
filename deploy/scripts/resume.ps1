# resume.ps1 — resume a paused Kaimi instance's pipeline schedule (cost control).
#
# Flips the Cloud Scheduler job back to ENABLED via gcloud, directly and
# instantly, WITHOUT a terraform run. After resuming, the pipeline fires again on
# its cron schedule (default 07:00/12:00/17:00 ET). Counterpart to pause.ps1. For
# a durable, declarative resume prefer `terraform apply -var active=true`; this
# script is the operator one-liner for an immediate resume.
#
# Usage:
#   .\resume.ps1 -Project acme-kaimi-prod [-Region us-east4] [-Job kaimi-pipeline-schedule]
#   $env:PROJECT='acme-kaimi-prod'; .\resume.ps1
#
# Defaults: Region=us-east4, Job=kaimi-pipeline-schedule (matches the module's
# scheduler_job_name). Project is required (param or $env:PROJECT).
#
# Idempotent: resuming an already-enabled job is a no-op that still succeeds.

[CmdletBinding()]
param(
    [string]$Project = $env:PROJECT,
    [string]$Region  = $(if ($env:REGION) { $env:REGION } else { "us-east4" }),
    [string]$Job     = $(if ($env:JOB)    { $env:JOB }    else { "kaimi-pipeline-schedule" })
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Project)) {
    Write-Error "Project is required (pass -Project <id> or set `$env:PROJECT)."
    exit 2
}

Write-Host "Resuming Kaimi pipeline schedule:"
Write-Host "  project = $Project"
Write-Host "  region  = $Region"
Write-Host "  job     = $Job"

# `scheduler jobs resume` is idempotent on an already-enabled job.
gcloud scheduler jobs resume $Job --location $Region --project $Project
if ($LASTEXITCODE -ne 0) {
    Write-Error "gcloud scheduler jobs resume failed (exit $LASTEXITCODE)."
    exit $LASTEXITCODE
}

Write-Host "Resumed. The pipeline will run again on its schedule (default 07:00/12:00/17:00 ET)."

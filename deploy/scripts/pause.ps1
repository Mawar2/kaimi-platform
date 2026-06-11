# pause.ps1 — pause a deployed Kaimi instance's pipeline schedule (cost control).
#
# Flips the Cloud Scheduler job to PAUSED via gcloud, directly and instantly,
# WITHOUT a terraform run. A paused schedule fires no pipeline runs, so the
# recurring Gemini/Vertex + SAM.gov spend stops. The Cloud Run Service and Job
# stay scaled to zero, so a paused deployment costs only tiny GCS/registry
# storage. No data is lost. Resume with resume.ps1 (or `terraform apply -var
# active=true`). For a durable, declarative pause prefer `terraform apply -var
# active=false`; this script is the operator one-liner for an immediate pause.
#
# Usage:
#   .\pause.ps1 -Project acme-kaimi-prod [-Region us-east4] [-Job kaimi-pipeline-schedule]
#   $env:PROJECT='acme-kaimi-prod'; .\pause.ps1
#
# Defaults: Region=us-east4, Job=kaimi-pipeline-schedule (matches the module's
# scheduler_job_name). Project is required (param or $env:PROJECT).
#
# Idempotent: pausing an already-paused job is a no-op that still succeeds.

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

Write-Host "Pausing Kaimi pipeline schedule:"
Write-Host "  project = $Project"
Write-Host "  region  = $Region"
Write-Host "  job     = $Job"

# `scheduler jobs pause` is idempotent on an already-paused job.
gcloud scheduler jobs pause $Job --location $Region --project $Project
if ($LASTEXITCODE -ne 0) {
    Write-Error "gcloud scheduler jobs pause failed (exit $LASTEXITCODE)."
    exit $LASTEXITCODE
}

Write-Host "Paused. The pipeline will not run until resumed (resume.ps1 or 'terraform apply -var active=true')."
Write-Host "Recurring Gemini/SAM spend is now stopped; data and infrastructure are unchanged."

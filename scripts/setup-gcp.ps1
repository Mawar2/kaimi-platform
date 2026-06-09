# GCP Environment Setup for Kaimi (PowerShell)
# This script sets up the GCP project for Phase 0 development
# Run this script with: .\scripts\setup-gcp.ps1

# Requires PowerShell 5.1+ and gcloud CLI installed

param(
    [Parameter(Mandatory=$false)]
    [string]$ProjectId = "",

    [Parameter(Mandatory=$false)]
    [string]$Region = "us-east4",

    [Parameter(Mandatory=$false)]
    [string]$ServiceAccountName = "kaimi-dev"
)

$ErrorActionPreference = "Stop"

# Colors for output
function Write-Success { Write-Host $args -ForegroundColor Green }
function Write-Info { Write-Host $args -ForegroundColor Cyan }
function Write-Warning { Write-Host $args -ForegroundColor Yellow }
function Write-Error { Write-Host $args -ForegroundColor Red }

Write-Success "=== Kaimi GCP Setup (PowerShell) ==="
Write-Host "This script will configure your GCP project for Phase 0`n"

# Step 1: Get project ID
Write-Info "Step 1: Project Configuration"

if (-not $ProjectId) {
    $ProjectId = Read-Host "Enter your GCP Project ID"
}

if ([string]::IsNullOrWhiteSpace($ProjectId)) {
    Write-Error "Error: Project ID cannot be empty"
    exit 1
}

$ServiceAccountEmail = "$ServiceAccountName@$ProjectId.iam.gserviceaccount.com"
$KeyFile = "kaimi-sa-key.json"

Write-Host "Using project: $ProjectId"
Write-Host "Region: $Region"
Write-Host ""

# Check gcloud is installed
try {
    $null = Get-Command gcloud -ErrorAction Stop
} catch {
    Write-Error "gcloud CLI is not installed or not in PATH"
    Write-Host "Install from: https://cloud.google.com/sdk/docs/install"
    exit 1
}

# Step 2: Set active project
Write-Info "Step 2: Setting active GCP project"
gcloud config set project $ProjectId
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to set project. Check your project ID and permissions."
    exit 1
}
Write-Success "✓ Project set`n"

# Step 3: Enable required APIs
Write-Info "Step 3: Enabling required GCP APIs"
Write-Host "This may take a few minutes...`n"

$apis = @(
    "cloudresourcemanager.googleapis.com",
    "iam.googleapis.com",
    "aiplatform.googleapis.com",
    "secretmanager.googleapis.com",
    "cloudbuild.googleapis.com",
    "cloudkms.googleapis.com"
)

foreach ($api in $apis) {
    Write-Host "Enabling $api..."
    gcloud services enable $api --project=$ProjectId 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
        Write-Warning "Failed to enable $api - may already be enabled or insufficient permissions"
    }
}

Write-Success "✓ All APIs enabled`n"

# Step 4: Create service account
Write-Info "Step 4: Creating service account"

# Check if service account exists
$saExists = gcloud iam service-accounts describe $ServiceAccountEmail --project=$ProjectId 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "Service account $ServiceAccountEmail already exists"
} else {
    gcloud iam service-accounts create $ServiceAccountName `
        --display-name="Kaimi Development Service Account" `
        --description="Service account for Kaimi agent development and CI/CD" `
        --project=$ProjectId

    if ($LASTEXITCODE -eq 0) {
        Write-Success "✓ Service account created"
    } else {
        Write-Error "Failed to create service account"
        exit 1
    }
}
Write-Host ""

# Step 5: Grant IAM roles
Write-Info "Step 5: Granting IAM permissions"

$roles = @(
    "roles/aiplatform.user",
    "roles/secretmanager.admin",
    "roles/logging.logWriter",
    "roles/monitoring.metricWriter"
)

foreach ($role in $roles) {
    Write-Host "Granting $role..."
    gcloud projects add-iam-policy-binding $ProjectId `
        --member="serviceAccount:$ServiceAccountEmail" `
        --role=$role `
        --condition=None `
        --quiet 2>&1 | Out-Null
}

Write-Success "✓ IAM roles granted`n"

# Step 6: Generate service account key
Write-Info "Step 6: Generating service account key"

if (Test-Path $KeyFile) {
    Write-Warning "Warning: $KeyFile already exists"
    $overwrite = Read-Host "Overwrite? (y/n)"
    if ($overwrite -eq 'y' -or $overwrite -eq 'Y') {
        Remove-Item $KeyFile
        gcloud iam service-accounts keys create $KeyFile `
            --iam-account=$ServiceAccountEmail `
            --project=$ProjectId

        if ($LASTEXITCODE -eq 0) {
            Write-Success "✓ Service account key created: $KeyFile"
        }
    } else {
        Write-Host "Skipping key generation"
    }
} else {
    gcloud iam service-accounts keys create $KeyFile `
        --iam-account=$ServiceAccountEmail `
        --project=$ProjectId

    if ($LASTEXITCODE -eq 0) {
        Write-Success "✓ Service account key created: $KeyFile"
    } else {
        Write-Error "Failed to create service account key"
        exit 1
    }
}

Write-Host ""
Write-Warning "IMPORTANT: Keep $KeyFile secure!"
Write-Host "- Add $KeyFile to .gitignore (already done if using project .gitignore)"
Write-Host "- Never commit this file to version control"
Write-Host "- Store it securely for CI/CD configuration`n"

# Step 7: Create Secret Manager secrets
Write-Info "Step 7: Setting up Secret Manager"
Write-Host "Creating placeholder secrets for SAM.gov API credentials...`n"

$secretName = "samgov-api-key"
$secretExists = gcloud secrets describe $secretName --project=$ProjectId 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "Secret $secretName already exists"
} else {
    "placeholder-key-replace-with-real-value" | gcloud secrets create $secretName `
        --data-file=- `
        --replication-policy="automatic" `
        --project=$ProjectId

    if ($LASTEXITCODE -eq 0) {
        Write-Success "✓ Created secret: $secretName"
    }
}

Write-Host ""
Write-Warning "Note: Update the SAM.gov API key secret with:"
Write-Host "echo 'YOUR_ACTUAL_API_KEY' | gcloud secrets versions add $secretName --data-file=-`n"

# Step 8: Create environment configuration file
Write-Info "Step 8: Creating environment configuration"

$envContent = @"
# GCP Configuration for Kaimi
# Generated by scripts/setup-gcp.ps1

# Project settings
GCP_PROJECT_ID=$ProjectId
GCP_REGION=$Region

# Service Account
GCP_SERVICE_ACCOUNT_EMAIL=$ServiceAccountEmail
GOOGLE_APPLICATION_CREDENTIALS=$(Get-Location)\$KeyFile

# Vertex AI settings
VERTEX_AI_LOCATION=$Region
GEMINI_MODEL=gemini-3.0-pro

# Secret Manager
SAMGOV_API_KEY_SECRET=$secretName

# Application settings
LOG_LEVEL=info
ENVIRONMENT=development
"@

$envContent | Out-File -FilePath ".env.gcp" -Encoding UTF8
Write-Success "✓ Created .env.gcp`n"

# Step 9: Test Vertex AI access
Write-Info "Step 9: Testing Vertex AI access"
Write-Host "Attempting to list models to verify access...`n"

$env:GOOGLE_APPLICATION_CREDENTIALS = "$(Get-Location)\$KeyFile"

gcloud ai models list --region=$Region --project=$ProjectId --limit=1 2>&1 | Out-Null
if ($LASTEXITCODE -eq 0) {
    Write-Success "✓ Vertex AI access verified"
} else {
    Write-Warning "⚠ Could not verify Vertex AI access. This may be normal if no models are deployed yet."
}

Write-Host ""

# Step 10: Zone-1 deployment infrastructure (issue #122)
# Artifact Registry repo, GCS queue bucket, Cloud Run Job, Cloud Scheduler.
# All commands are idempotent: existing resources are left untouched.
Write-Info "Step 10: Zone-1 deployment infrastructure"

$PipelineImage = "$Region-docker.pkg.dev/$ProjectId/kaimi/pipeline:latest"
$QueueBucket = "$ProjectId-queue"

gcloud artifacts repositories describe kaimi --location=$Region --project=$ProjectId 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) {
    gcloud artifacts repositories create kaimi --repository-format=docker --location=$Region --description="Kaimi container images" --project=$ProjectId
}
Write-Success "✓ Artifact Registry repo 'kaimi' ready"

gcloud storage buckets describe "gs://$QueueBucket" --project=$ProjectId 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) {
    gcloud storage buckets create "gs://$QueueBucket" --location=$Region --project=$ProjectId --uniform-bucket-level-access
}
gcloud storage buckets add-iam-policy-binding "gs://$QueueBucket" --member="serviceAccount:$ServiceAccountEmail" --role=roles/storage.objectAdmin | Out-Null
Write-Success "✓ Queue bucket gs://$QueueBucket ready"

# The Cloud Run Job runs cmd/pipeline in live mode with the JSON store on a
# GCS volume mount, so scored opportunities persist across runs.
gcloud run jobs describe kaimi-pipeline --region=$Region --project=$ProjectId 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) {
    gcloud run jobs create kaimi-pipeline --image $PipelineImage --region $Region --project $ProjectId --service-account $ServiceAccountEmail --set-env-vars "MODE=live,GCP_PROJECT_ID=$ProjectId,GCP_REGION=$Region,STORE_PATH=/mnt/store/queue" --set-secrets "SAM_API_KEY=${secretName}:latest" --add-volume "name=store,type=cloud-storage,bucket=$QueueBucket" --add-volume-mount "volume=store,mount-path=/mnt/store" --memory 512Mi --cpu 1 --max-retries 1 --task-timeout 600
}
Write-Success "✓ Cloud Run Job kaimi-pipeline ready"

gcloud run jobs add-iam-policy-binding kaimi-pipeline --region $Region --project $ProjectId --member "serviceAccount:$ServiceAccountEmail" --role roles/run.invoker | Out-Null

# Three quota-friendly runs per day (SAM.gov allows 1,000 requests/day; the
# client caches aggressively so repeat pulls are cheap).
gcloud scheduler jobs describe kaimi-pipeline-schedule --location=$Region --project=$ProjectId 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) {
    gcloud scheduler jobs create http kaimi-pipeline-schedule --project $ProjectId --location $Region --schedule "0 7,12,17 * * *" --time-zone "America/New_York" --uri "https://$Region-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/$ProjectId/jobs/kaimi-pipeline:run" --http-method POST --oauth-service-account-email $ServiceAccountEmail
}
Write-Success "✓ Cloud Scheduler trigger ready (07:00/12:00/17:00 ET)"
Write-Host ""

# Summary
Write-Success "=== Setup Complete ===`n"
Write-Host "Your GCP environment is ready for Kaimi Phase 0 development.`n"

Write-Host "Configuration:"
Write-Host "  Project ID: $ProjectId"
Write-Host "  Region: $Region"
Write-Host "  Service Account: $ServiceAccountEmail"
Write-Host "  Key File: $KeyFile"
Write-Host "  Environment File: .env.gcp`n"

Write-Host "Next steps:"
Write-Host "  1. Add your real SAM.gov API key to Secret Manager:"
Write-Host "     echo 'YOUR_KEY' | gcloud secrets versions add $secretName --data-file=-`n"

Write-Host "  2. Activate the service account:"
Write-Host "     gcloud auth activate-service-account --key-file=$KeyFile`n"

Write-Host "  3. For CI/CD (GitHub Actions), add these secrets to your repository:"
Write-Host "     - GCP_PROJECT_ID: $ProjectId"
Write-Host "     - GCP_SA_KEY: (contents of $KeyFile)"
Write-Host "     - GCP_REGION: $Region`n"

Write-Host "  4. Test your setup:"
Write-Host "     gcloud ai models list --region=$Region --limit=5`n"

Write-Warning "Remember: Never commit $KeyFile to version control!"

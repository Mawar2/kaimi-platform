# GCP Setup Guide for Kaimi

**Last updated:** 2026-06-09

This document explains the GCP infrastructure setup for the Kaimi project. The
`kaimi-seeker` environment is already provisioned and the Zone-1 pipeline is deployed;
these steps remain valid for standing up a fresh environment from scratch.

## Overview

Kaimi uses Google Cloud Platform for:
- **Vertex AI**: Gemini 2.5 Pro model access for agent reasoning
- **Secret Manager**: Secure storage of SAM.gov API credentials
- **IAM**: Service account for authentication and authorization
- **Cloud Build**: CI/CD pipeline execution
- **Cloud Run Jobs + Cloud Scheduler**: the deployed Zone-1 pipeline (`kaimi-pipeline`) and its triggers
- **Cloud Storage**: the persisted scored store (`gs://kaimi-seeker-queue`)

Following the architecture's "provision lazily, design eagerly" principle, this guide
covers the core APIs (Vertex AI, Secret Manager, IAM, Cloud Build). The deploy path adds
Cloud Run, Artifact Registry, and Cloud Scheduler — see [DEPLOYMENT.md](./DEPLOYMENT.md).
The `Store` is JSON-backed on GCS today; Firestore remains an optional future swap.

## Prerequisites

- [gcloud CLI](https://cloud.google.com/sdk/docs/install) installed and authenticated
- An existing GCP project (or billing account to create one)
- Project owner or editor permissions

## Quick Start

Run the automated setup script:

```bash
# Make script executable (Unix/Linux/Mac)
chmod +x scripts/setup-gcp.sh

# Run setup
bash scripts/setup-gcp.sh
```

The script will prompt you for your GCP project ID and configure everything automatically.

## What the Script Does

### 1. Enables Required APIs

```bash
# Vertex AI (includes Gemini models)
gcloud services enable aiplatform.googleapis.com

# Secret Manager (for API keys)
gcloud services enable secretmanager.googleapis.com

# IAM and Resource Manager
gcloud services enable iam.googleapis.com
gcloud services enable cloudresourcemanager.googleapis.com

# Cloud Build (for CI/CD)
gcloud services enable cloudbuild.googleapis.com

# Cloud KMS (for Secret Manager encryption)
gcloud services enable cloudkms.googleapis.com
```

### 2. Creates Service Account

Service account: `kaimi-dev@YOUR_PROJECT_ID.iam.gserviceaccount.com`

This account is used for:
- Local development authentication
- CI/CD pipeline authentication (GitHub Actions)
- Programmatic access to GCP services

### 3. Grants IAM Permissions

The service account receives these roles:

| Role | Purpose |
|------|---------|
| `roles/aiplatform.user` | Access Vertex AI and Gemini models |
| `roles/secretmanager.admin` | Read/write secrets (SAM.gov API keys) |
| `roles/logging.logWriter` | Write application logs |
| `roles/monitoring.metricWriter` | Write monitoring metrics |

**Security note**: These are the least-privilege permissions for running agents. Deploying the pipeline adds Cloud Run, Artifact Registry, and Cloud Scheduler roles (see [DEPLOYMENT.md](./DEPLOYMENT.md)); an optional future Firestore swap would add `roles/datastore.user`.

### 4. Generates Service Account Key

Creates `kaimi-sa-key.json` in the project root. This JSON key file is used to authenticate as the service account.

**⚠️ CRITICAL SECURITY WARNINGS:**
- **Never commit this file to version control** (already in `.gitignore`)
- **Never share this file publicly**
- Store securely for CI/CD configuration
- Rotate keys periodically (recommended: every 90 days)

### 5. Creates Secret Manager Secrets

Creates a secret named `samgov-api-key` with a placeholder value.

**You must update this with your real SAM.gov API key:**

```bash
# Add your actual SAM.gov API key
echo 'YOUR_ACTUAL_SAMGOV_API_KEY' | \
  gcloud secrets versions add samgov-api-key --data-file=-
```

### 6. Creates Environment Configuration

Generates `.env.gcp` with project configuration:

```bash
GCP_PROJECT_ID=your-project-id
GCP_REGION=us-east4
GCP_SERVICE_ACCOUNT_EMAIL=kaimi-dev@your-project-id.iam.gserviceaccount.com
GOOGLE_APPLICATION_CREDENTIALS=/path/to/kaimi-sa-key.json
VERTEX_AI_LOCATION=us-east4
GEMINI_MODEL=gemini-2.5-pro
SAMGOV_API_KEY_SECRET=samgov-api-key
```

## Manual Setup (if script fails)

If the automated script fails, follow these manual steps:

### 1. Set Active Project

```bash
export PROJECT_ID="your-project-id"
gcloud config set project $PROJECT_ID
```

### 2. Enable APIs

```bash
gcloud services enable aiplatform.googleapis.com
gcloud services enable secretmanager.googleapis.com
gcloud services enable iam.googleapis.com
gcloud services enable cloudresourcemanager.googleapis.com
gcloud services enable cloudbuild.googleapis.com
gcloud services enable cloudkms.googleapis.com
```

### 3. Create Service Account

```bash
gcloud iam service-accounts create kaimi-dev \
  --display-name="Kaimi Development Service Account" \
  --description="Service account for Kaimi agent development and CI/CD"
```

### 4. Grant Roles

```bash
export SA_EMAIL="kaimi-dev@${PROJECT_ID}.iam.gserviceaccount.com"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/aiplatform.user"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/secretmanager.admin"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/logging.logWriter"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:${SA_EMAIL}" \
  --role="roles/monitoring.metricWriter"
```

### 5. Create Service Account Key

```bash
gcloud iam service-accounts keys create kaimi-sa-key.json \
  --iam-account="${SA_EMAIL}"
```

### 6. Create Secrets

```bash
echo 'placeholder-key' | \
  gcloud secrets create samgov-api-key \
    --data-file=- \
    --replication-policy="automatic"
```

## Local Development Setup

After running the setup script:

### 1. Activate Service Account

```bash
# Source the environment configuration
source .env.gcp

# Activate service account
gcloud auth activate-service-account --key-file=kaimi-sa-key.json
```

### 2. Test Vertex AI Access

```bash
# List available models
gcloud ai models list --region=us-east4 --limit=5

# Test Gemini access (requires Google ADK setup)
# The deployed agents exercise this path against gemini-2.5-pro
```

### 3. Set Environment Variables for Go

The Go code will read credentials from the `GOOGLE_APPLICATION_CREDENTIALS` environment variable:

```bash
# In your shell profile (.bashrc, .zshrc, etc.)
export GOOGLE_APPLICATION_CREDENTIALS="$(pwd)/kaimi-sa-key.json"

# Or source the .env.gcp file
source .env.gcp
```

## CI/CD Setup (GitHub Actions)

For the CI/CD pipeline defined in WORKFLOW.md:

### 1. Add GitHub Secrets

In your GitHub repository, go to Settings → Secrets and variables → Actions, and add:

| Secret Name | Value |
|-------------|-------|
| `GCP_PROJECT_ID` | Your GCP project ID |
| `GCP_SA_KEY` | Contents of `kaimi-sa-key.json` (entire JSON file) |
| `GCP_REGION` | `us-east4` |

### 2. GitHub Actions Workflow

The workflow will use these secrets to authenticate with GCP:

```yaml
- name: Authenticate to Google Cloud
  uses: google-github-actions/auth@v1
  with:
    credentials_json: ${{ secrets.GCP_SA_KEY }}

- name: Set up Cloud SDK
  uses: google-github-actions/setup-gcloud@v1
```

See `.github/workflows/ci.yml` for the complete CI pipeline configuration.

## Verification Checklist

After setup, verify each component:

- [ ] gcloud CLI authenticated and project set
- [ ] All required APIs enabled (check in GCP Console → APIs & Services)
- [ ] Service account created with correct roles
- [ ] Service account key file exists (`kaimi-sa-key.json`)
- [ ] Service account key is in `.gitignore`
- [ ] `.env.gcp` file created with correct values
- [ ] Secret Manager secret created for SAM.gov API key
- [ ] SAM.gov API key added to Secret Manager (not placeholder)
- [ ] Vertex AI access verified (`gcloud ai models list` succeeds)
- [ ] GitHub secrets configured (if using CI/CD)

## Troubleshooting

### "Permission denied" errors

**Symptom**: `ERROR: (gcloud.services.enable) User does not have permission to access project`

**Solution**: Ensure you have Owner or Editor role on the GCP project:
```bash
gcloud projects get-iam-policy YOUR_PROJECT_ID
```

### "API not enabled" errors

**Symptom**: `API [service.googleapis.com] not enabled on project`

**Solution**: Re-run the API enablement commands:
```bash
gcloud services enable aiplatform.googleapis.com --project=YOUR_PROJECT_ID
```

### Vertex AI access fails

**Symptom**: `ERROR: (gcloud.ai.models.list) PERMISSION_DENIED`

**Solution**:
1. Verify service account has `roles/aiplatform.user`
2. Ensure you're using the service account credentials:
```bash
gcloud auth activate-service-account --key-file=kaimi-sa-key.json
```

### Secret Manager errors

**Symptom**: `ERROR: (gcloud.secrets.create) Resource in projects [PROJECT_ID] is the subject of a conflict`

**Solution**: Secret already exists. Update it instead:
```bash
echo 'new-value' | gcloud secrets versions add samgov-api-key --data-file=-
```

### Region not supported for Vertex AI

**Symptom**: `Location not supported: us-east4`

**Solution**: Check [supported regions](https://cloud.google.com/vertex-ai/docs/general/locations) and update the script if needed. Recommended alternatives:
- `us-central1` (Iowa)
- `us-east1` (South Carolina)
- `us-west1` (Oregon)

## Cost Management

Baseline infrastructure costs:

| Service | Expected Cost |
|---------|---------------|
| **Vertex AI (Gemini 2.5 Pro)** | Pay-per-use, driven by pipeline scoring volume |
| **Secret Manager** | ~$0.06/month per secret + access costs (negligible) |
| **Cloud Build** | 120 build-minutes/day free tier |
| **Cloud Run Jobs** | Billed only while a scheduled run executes |
| **Cloud Scheduler** | ~$0.30/month (three triggers/day) |
| **Cloud Storage (queue bucket)** | Negligible |
| **IAM** | Free |

**Estimated baseline monthly cost**: < $1 (excluding Gemini usage and pipeline runtime)

**Note**: The main cost driver is Gemini API usage as the scheduled pipeline scores opportunities. Monitor usage in [GCP Console Billing](https://console.cloud.google.com/billing).

## Security Best Practices

### Service Account Key Management

1. **Never commit keys to git**: Already in `.gitignore`, but verify:
   ```bash
   git check-ignore kaimi-sa-key.json  # Should output the filename
   ```

2. **Rotate keys periodically**:
   ```bash
   # List existing keys
   gcloud iam service-accounts keys list --iam-account=kaimi-dev@PROJECT_ID.iam.gserviceaccount.com

   # Delete old key
   gcloud iam service-accounts keys delete KEY_ID --iam-account=kaimi-dev@PROJECT_ID.iam.gserviceaccount.com

   # Create new key
   gcloud iam service-accounts keys create kaimi-sa-key.json --iam-account=kaimi-dev@PROJECT_ID.iam.gserviceaccount.com
   ```

3. **Principle of least privilege**: Grant only the roles each service needs. The agent runtime uses a minimal set; the deploy path adds Cloud Run / Artifact Registry / Cloud Scheduler roles (see [DEPLOYMENT.md](./DEPLOYMENT.md)).

### Secret Manager Best Practices

1. **Never log secret values**: Ensure application code doesn't log secrets
2. **Use automatic replication**: Already configured for high availability
3. **Audit access**: Enable audit logging for secret access:
   ```bash
   gcloud logging read "protoPayload.serviceName=\"secretmanager.googleapis.com\""
   ```

## Next Steps

After GCP setup is complete:

1. **Verify Vertex AI access**: Confirm the service account can reach `gemini-2.5-pro`
2. **Add SAM.gov API key**: Update the Secret Manager secret with your real API key
3. **Configure CI/CD**: Add GitHub secrets and test the pipeline
4. **Wire up deployment**: Follow [DEPLOYMENT.md](./DEPLOYMENT.md) to deploy the `kaimi-pipeline`
   Cloud Run Job and its Cloud Scheduler triggers. The scored store persists to
   `gs://kaimi-seeker-queue`; Firestore remains an optional future swap behind the
   `Store` interface.

## References

- [Vertex AI Documentation](https://cloud.google.com/vertex-ai/docs)
- [Secret Manager Documentation](https://cloud.google.com/secret-manager/docs)
- [Service Account Best Practices](https://cloud.google.com/iam/docs/best-practices-service-accounts)
- [Google ADK Documentation](https://cloud.google.com/agent-builder/docs)

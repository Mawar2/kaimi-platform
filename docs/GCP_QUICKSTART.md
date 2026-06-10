# GCP Quick Start Guide

This is a condensed setup guide for getting the Kaimi GCP environment running quickly. For detailed information, see [GCP_SETUP.md](./GCP_SETUP.md).

## Prerequisites Checklist

- [ ] gcloud CLI installed ([Download](https://cloud.google.com/sdk/docs/install))
- [ ] Authenticated with gcloud: `gcloud auth login`
- [ ] GCP project created (or project ID available)
- [ ] Billing enabled on the project
- [ ] Git Bash or WSL2 installed (Windows users)

## One-Command Setup

From the project root, run:

```bash
bash scripts/setup-gcp.sh
```

The script will prompt you for your GCP project ID and configure everything automatically.

## What Gets Created

| Resource | Name/ID | Purpose |
|----------|---------|---------|
| Service Account | `kaimi-dev@PROJECT_ID.iam.gserviceaccount.com` | Authentication for dev and CI/CD |
| Service Account Key | `kaimi-sa-key.json` | Credentials file (local only, never commit!) |
| Secret | `samgov-api-key` | Stores SAM.gov API key |
| Environment File | `.env.gcp` | Local environment configuration |

### APIs Enabled

- Vertex AI (aiplatform.googleapis.com) - For Gemini 2.5 Pro
- Secret Manager (secretmanager.googleapis.com) - For API keys
- IAM (iam.googleapis.com) - For service accounts
- Cloud Build (cloudbuild.googleapis.com) - For CI/CD
- Cloud KMS (cloudkms.googleapis.com) - For encryption

## Post-Setup Tasks

### 1. Add Your SAM.gov API Key

```bash
# Replace YOUR_ACTUAL_KEY with your real API key
echo 'YOUR_ACTUAL_KEY' | gcloud secrets versions add samgov-api-key --data-file=-
```

### 2. Activate Service Account (Local Development)

```bash
# Option A: Source the environment file (sets GOOGLE_APPLICATION_CREDENTIALS)
source .env.gcp

# Option B: Activate service account directly
gcloud auth activate-service-account --key-file=kaimi-sa-key.json
```

### 3. Verify Setup

Test that everything works:

```bash
# Test 1: Verify you can list Vertex AI models
gcloud ai models list --region=us-east4 --limit=5

# Test 2: Verify Secret Manager access
gcloud secrets describe samgov-api-key

# Test 3: Verify service account authentication
gcloud auth list
```

Expected output for Test 3 should show:
```
ACTIVE  ACCOUNT
*       kaimi-dev@PROJECT_ID.iam.gserviceaccount.com
```

### 4. Configure GitHub Secrets (for CI/CD)

Go to your GitHub repository → Settings → Secrets and variables → Actions

Add these repository secrets:

| Secret Name | Where to Get the Value |
|-------------|------------------------|
| `GCP_PROJECT_ID` | Your GCP project ID (from setup script output) |
| `GCP_SA_KEY` | **Entire contents** of `kaimi-sa-key.json` file |
| `GCP_REGION` | `us-east4` (or your chosen region) |

**How to get `GCP_SA_KEY` value:**

```bash
# Windows (Git Bash)
cat kaimi-sa-key.json | clip

# Mac
cat kaimi-sa-key.json | pbcopy

# Linux
cat kaimi-sa-key.json | xclip -selection clipboard

# Or just open the file and copy all contents manually
```

## Environment Variables for Development

Add to your shell profile (`.bashrc`, `.zshrc`, etc.):

```bash
# Kaimi GCP Configuration
export GOOGLE_APPLICATION_CREDENTIALS="$HOME/path/to/kaimi/kaimi-sa-key.json"
export GCP_PROJECT_ID="your-project-id"
export GCP_REGION="us-east4"
```

Or just source `.env.gcp` when working on Kaimi:

```bash
cd /path/to/kaimi
source .env.gcp
```

## Troubleshooting

### "Permission denied" when running setup script

**Windows users:** Use Git Bash, WSL2, or install Windows Subsystem for Linux.

```bash
# Make script executable
chmod +x scripts/setup-gcp.sh

# Run with bash explicitly
bash scripts/setup-gcp.sh
```

### "API not enabled" errors

Re-run the API enablement:

```bash
gcloud services enable aiplatform.googleapis.com secretmanager.googleapis.com
```

### "Insufficient permissions" errors

Verify your user account has Owner or Editor role:

```bash
gcloud projects get-iam-policy YOUR_PROJECT_ID \
  --flatten="bindings[].members" \
  --filter="bindings.members:YOUR_EMAIL"
```

### Service account key not working

1. Verify the file exists and is valid JSON:
   ```bash
   cat kaimi-sa-key.json | jq
   ```

2. Re-activate the service account:
   ```bash
   gcloud auth activate-service-account --key-file=kaimi-sa-key.json
   ```

3. Check which account is active:
   ```bash
   gcloud auth list
   ```

## Security Reminders

### Never Commit These Files

Already in `.gitignore`, but double-check:

```bash
# Verify these are ignored
git check-ignore kaimi-sa-key.json  # Should output: kaimi-sa-key.json
git check-ignore .env.gcp           # Should output: .env.gcp
```

If either command produces no output, the file is **not** ignored!

### Rotate Keys Regularly

Recommended: Every 90 days

```bash
# List existing keys
gcloud iam service-accounts keys list \
  --iam-account=kaimi-dev@PROJECT_ID.iam.gserviceaccount.com

# Delete old key (use KEY_ID from list command)
gcloud iam service-accounts keys delete KEY_ID \
  --iam-account=kaimi-dev@PROJECT_ID.iam.gserviceaccount.com

# Create new key
gcloud iam service-accounts keys create kaimi-sa-key.json \
  --iam-account=kaimi-dev@PROJECT_ID.iam.gserviceaccount.com

# Update GitHub secret GCP_SA_KEY with new file contents
```

## What's Next?

After GCP setup is complete:

1. [ ] Verify all tests pass: `make test`
2. [ ] Verify linter passes: `make lint`
3. [ ] Verify CI pipeline runs successfully (push to GitHub)
4. [ ] Begin Hunter agent implementation (next Phase 0 task)

## Cost Monitoring

Set up a budget alert (recommended):

```bash
# Set a budget alert at $50/month
gcloud billing budgets create \
  --billing-account=YOUR_BILLING_ACCOUNT_ID \
  --display-name="Kaimi Monthly Budget" \
  --budget-amount=50 \
  --threshold-rule=percent=50 \
  --threshold-rule=percent=90 \
  --threshold-rule=percent=100
```

**Expected Phase 0 costs:** < $1/month (excluding Gemini API usage when Hunter runs)

Monitor spending: [GCP Console Billing](https://console.cloud.google.com/billing)

## Need Help?

- **Detailed setup guide:** [docs/GCP_SETUP.md](./GCP_SETUP.md)
- **Architecture context:** [ARCHITECTURE.md](../ARCHITECTURE.md)
- **Development workflow:** [WORKFLOW.md](../WORKFLOW.md)
- **GCP Documentation:** [cloud.google.com/docs](https://cloud.google.com/docs)

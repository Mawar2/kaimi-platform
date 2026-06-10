# Deployment Guide - Kaimi

**Last updated:** 2026-06-09

This document explains the GCP deployment pipeline for Kaimi.

## Current Status: Deployed and Operational

**Deployment Status:** Live in production

The Zone-1 pipeline (Hunter → Scorer → Queue) is **built and deployed**. It runs as a
Cloud Run **Job** (`kaimi-pipeline`, region `us-east4`) triggered by Cloud Scheduler at
07:00 / 12:00 / 17:00 ET. Scored opportunities are persisted as JSON to the GCS bucket
`gs://kaimi-seeker-queue`. The CI/CD pipeline deploys automatically on merge to `main`.

---

## Deployment Architecture

### Zone-1 Pipeline (Deployed)
- ✅ CI/CD pipeline deploys on merge to `main`
- ✅ Cloud Run Job `kaimi-pipeline` runs Hunter → Scorer → Queue
- ✅ Cloud Scheduler triggers the job at 07:00 / 12:00 / 17:00 ET
- ✅ Scored JSON store persisted to `gs://kaimi-seeker-queue`

The deployment flow is:
1. Build the pipeline Docker image
2. Push to Artifact Registry
3. Deploy/update the `kaimi-pipeline` Cloud Run Job
4. Cloud Scheduler invokes the job on the configured schedule

---

## Setting Up Manual Deployment Approval

The deployment job requires manual approval via a GitHub environment. Here's how to set it up:

### 1. Create the "production" Environment

**GitHub Repository → Settings → Environments → New environment**

1. **Name:** `production`
2. Click **"Configure environment"**

### 2. Configure Environment Protection Rules

Add these protection rules:

#### Required Reviewers
- ✅ Check **"Required reviewers"**
- Add reviewers:
  - `malik@bluemetatech.com` (you)
  - `thaithimmy2003@gmail.com` (developer)
- At least 1 approval required before deployment

#### Deployment Branches
- ✅ Select **"Selected branches"**
- Add branch: `main`
- This ensures only main branch can deploy

#### Wait Timer (Optional)
- Set a wait timer if you want a delay before deployment
- Recommended: 0 minutes (manual approval is the gate)

### 3. Environment Variables (Optional)

You can add environment-specific variables here, but we're using repository secrets which work fine.

### 4. Save Environment

Click **"Save protection rules"**

---

## How Manual Deployment Works

### Trigger

Deployment job runs when:
- ✅ Code is pushed to `main` branch
- ✅ All checks pass (tests, lint, GCP verification)
- ✅ `all-checks-pass` job succeeds

### Approval Flow

1. **CI runs automatically** on push to main
2. **All checks must pass** first
3. **Deployment job waits** for manual approval
4. **Notification sent** to required reviewers
5. **Reviewer approves** deployment in GitHub Actions UI
6. **Deployment executes** after approval

### Reviewing a Deployment

When deployment is triggered:

1. Go to **Actions** tab
2. Click on the workflow run
3. You'll see: **"Deploy to GCP - Waiting for approval"**
4. Click **"Review deployments"**
5. Select **production** environment
6. Add a comment (optional): "Approved for deploy"
7. Click **"Approve and deploy"**

---

## What Gets Deployed

### Pipeline to Cloud Run (Job)

**Job Name:** `kaimi-pipeline`
**Region:** `us-east4`
**Platform:** Cloud Run Jobs (managed)
**Runs:** Hunter → Scorer → Queue (Zone-1 pipeline), end to end

**Configuration:**
- Memory: 512Mi
- CPU: 1
- Task timeout: 300s (5 minutes)
- Max retries: per job config

**Environment Variables:**
- `GCP_PROJECT_ID` - Project ID
- `GCP_REGION` - Deployment region
- `QUEUE_BUCKET` - GCS bucket for the scored store (`kaimi-seeker-queue`)

**Secrets:**
- `samgov-api-key` - Mounted from Secret Manager

### Persisted Store

Scored opportunities are written as JSON to the GCS bucket
`gs://kaimi-seeker-queue`. The `Store` interface is JSON-backed today; Firestore
remains an optional future swap that requires no pipeline code changes.

### Cloud Scheduler

**Schedule:** 07:00 / 12:00 / 17:00 ET (three triggers per day)
**Target:** Cloud Run Job `kaimi-pipeline` (executes the job)
**Auth:** Service account OIDC token

---

## How the Deployment Runs

On merge to `main`, the CI/CD pipeline (`.github/workflows/ci.yml`) builds and
deploys automatically:
  1. Build the pipeline Docker image
  2. Push to Artifact Registry
  3. Deploy/update the `kaimi-pipeline` Cloud Run Job
  4. Cloud Scheduler invokes the job at 07:00 / 12:00 / 17:00 ET

---

## Standing Up the Deploy Path in a Fresh Environment

If you are provisioning a new project from scratch, the deploy path requires the
following APIs, repository, and IAM grants.

### 1. Enable Required APIs

```bash
# Artifact Registry (for Docker images)
gcloud services enable artifactregistry.googleapis.com

# Cloud Run
gcloud services enable run.googleapis.com

# Cloud Scheduler
gcloud services enable cloudscheduler.googleapis.com
```

### 2. Create Artifact Registry Repository

```bash
gcloud artifacts repositories create kaimi \
  --repository-format=docker \
  --location=us-east4 \
  --description="Kaimi Docker images"
```

### 3. Grant Deployment IAM Roles

The service account needs permissions to deploy:

```bash
# Cloud Run Admin (deploy services)
gcloud projects add-iam-policy-binding kaimi-seeker \
  --member="serviceAccount:kaimi-dev@kaimi-seeker.iam.gserviceaccount.com" \
  --role="roles/run.admin"

# Service Account User (act as service account)
gcloud projects add-iam-policy-binding kaimi-seeker \
  --member="serviceAccount:kaimi-dev@kaimi-seeker.iam.gserviceaccount.com" \
  --role="roles/iam.serviceAccountUser"

# Artifact Registry Writer (push images)
gcloud projects add-iam-policy-binding kaimi-seeker \
  --member="serviceAccount:kaimi-dev@kaimi-seeker.iam.gserviceaccount.com" \
  --role="roles/artifactregistry.writer"

# Cloud Scheduler Admin (create jobs)
gcloud projects add-iam-policy-binding kaimi-seeker \
  --member="serviceAccount:kaimi-dev@kaimi-seeker.iam.gserviceaccount.com" \
  --role="roles/cloudscheduler.admin"
```

### 4. Verify the Deployment

1. Merge a small change to `main`
2. Watch the CI/CD pipeline run
3. Approve the deployment if a manual gate is configured
4. Verify the `kaimi-pipeline` Cloud Run Job is deployed
5. Confirm the Cloud Scheduler triggers exist (07:00 / 12:00 / 17:00 ET)

---

## Deployment URLs

- **Cloud Run Jobs:** https://console.cloud.google.com/run/jobs?project=kaimi-seeker
- **GCP Console:** https://console.cloud.google.com/run?project=kaimi-seeker
- **Cloud Scheduler:** https://console.cloud.google.com/cloudscheduler?project=kaimi-seeker
- **Queue bucket:** https://console.cloud.google.com/storage/browser/kaimi-seeker-queue?project=kaimi-seeker

---

## Rollback Procedure

If deployment fails or has issues:

### Option 1: Redeploy a Previous Image

```bash
# List the job's executions
gcloud run jobs executions list --job=kaimi-pipeline --region=us-east4

# Update the job back to a known-good image
gcloud run jobs update kaimi-pipeline \
  --image=us-east4-docker.pkg.dev/kaimi-seeker/kaimi/pipeline:<previous-tag> \
  --region=us-east4
```

### Option 2: Revert Git Commit

```bash
# Revert the commit that caused issues
git revert <commit-hash>
git push origin main

# CI will redeploy the reverted version
```

---

## Monitoring Deployments

### View Logs

```bash
# Cloud Run Job execution logs
gcloud run jobs executions list --job=kaimi-pipeline --region=us-east4
gcloud logging read 'resource.type="cloud_run_job" resource.labels.job_name="kaimi-pipeline"' --limit=50

# Cloud Scheduler logs
gcloud scheduler jobs list --location=us-east4
```

### Cloud Console

- **Cloud Run Job:** https://console.cloud.google.com/run/jobs/details/us-east4/kaimi-pipeline?project=kaimi-seeker
- **Cloud Scheduler Logs:** https://console.cloud.google.com/cloudscheduler

---

## Cost Monitoring

### Expected Costs

| Service | Cost Model | Estimated |
|---------|------------|-----------|
| Cloud Run Jobs | Pay-per-use (CPU/memory while a task runs) | ~$5-10/month |
| Cloud Scheduler | $0.10/job/month | ~$0.30/month (three triggers) |
| Artifact Registry | $0.10/GB/month storage | ~$0.50/month |
| Cloud Storage (queue bucket) | $0.02/GB/month + ops | negligible |
| **Total** | | **~$5-11/month** |

Cloud Run Jobs only bill while a task is executing, so costs are minimal between
the three scheduled runs per day.

---

## Security Best Practices

### Service Account Permissions
- ✅ Use dedicated service account for Cloud Run
- ✅ Grant minimal permissions (least privilege)
- ✅ Use Secret Manager for sensitive data
- ✅ Enable VPC connector if accessing internal resources

### Cloud Run Configuration
- ✅ Disable unauthenticated access after testing
- ✅ Use OIDC tokens for Cloud Scheduler
- ✅ Set resource limits (memory, CPU)
- ✅ Enable request logging

### Docker Image Security
- ✅ Use minimal base images (alpine)
- ✅ Don't include secrets in image
- ✅ Scan images for vulnerabilities
- ✅ Use specific version tags, not `latest` in production

---

## Troubleshooting

### Deployment Fails: "Permission denied"

**Issue:** Service account missing Cloud Run permissions

**Fix:**
```bash
gcloud projects add-iam-policy-binding kaimi-seeker \
  --member="serviceAccount:kaimi-dev@kaimi-seeker.iam.gserviceaccount.com" \
  --role="roles/run.admin"
```

### Deployment Fails: "Artifact Registry not found"

**Issue:** Artifact Registry repository not created

**Fix:**
```bash
gcloud artifacts repositories create kaimi \
  --repository-format=docker \
  --location=us-east4
```

### Cloud Scheduler Trigger Fails

**Issue:** The scheduler's service account cannot execute the Cloud Run Job

**Fix:**
```bash
# Allow Cloud Scheduler to run the job
gcloud run jobs add-iam-policy-binding kaimi-pipeline \
  --member="serviceAccount:kaimi-dev@kaimi-seeker.iam.gserviceaccount.com" \
  --role="roles/run.invoker" \
  --region=us-east4
```

---

## Operations

### Live
- ✅ `kaimi-pipeline` Cloud Run Job deployed (Hunter → Scorer → Queue)
- ✅ Cloud Scheduler triggers at 07:00 / 12:00 / 17:00 ET
- ✅ Scored JSON store persisted to `gs://kaimi-seeker-queue`
- ✅ CI/CD deploys on merge to `main`

### Optional / Future
- [ ] Firestore swap for the `Store` interface (no pipeline code changes required)
- [ ] Tighten manual approval gate in the GitHub `production` environment
- [ ] Expand monitoring/alerting on job failures and run latency

---

## Questions?

- **CI/CD Issues:** Check [.github/workflows/ci.yml](.github/workflows/ci.yml)
- **GCP Setup:** See [docs/GCP_SETUP.md](./GCP_SETUP.md)
- **Architecture:** See [ARCHITECTURE.md](../ARCHITECTURE.md)
- **Workflow:** See [WORKFLOW.md](../WORKFLOW.md)

**The Zone-1 pipeline is deployed and running on schedule. 🚀**

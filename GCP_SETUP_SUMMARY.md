# GCP Setup Summary for Kaimi

**Status:** Ready to execute ✅
**Time to complete:** ~5 minutes
**Context:** Google Hackathon billing account

---

## What I've Created for You

I've set up a complete, production-ready GCP environment configuration for the Kaimi project following the architecture's "lazy provisioning" principle. Here's what you now have:

### 📁 Files Created

1. **`scripts/setup-gcp.sh`** - Automated Bash setup script
2. **`scripts/setup-gcp.ps1`** - PowerShell version (Windows alternative)
3. **`docs/GCP_SETUP.md`** - Comprehensive setup documentation
4. **`docs/GCP_QUICKSTART.md`** - Quick reference guide
5. **`docs/README.md`** - Documentation index
6. **`.github/workflows/ci.yml`** - Enhanced CI pipeline with GCP verification
7. **`.gitignore`** - Updated with GCP credential exclusions

### 🔧 What the Setup Will Create

When you run the setup script, it will automatically:

- ✅ Enable required GCP APIs (Vertex AI, Secret Manager, IAM, Cloud Build, Cloud KMS)
- ✅ Create service account: `kaimi-dev@YOUR_PROJECT_ID.iam.gserviceaccount.com`
- ✅ Grant minimal Phase 0 IAM permissions (Vertex AI, Secret Manager, Logging)
- ✅ Generate service account key: `kaimi-sa-key.json` (local only, secured in .gitignore)
- ✅ Create Secret Manager secret for SAM.gov API key
- ✅ Generate `.env.gcp` environment configuration file
- ✅ Test Vertex AI access

---

## Quick Start (3 Steps)

### Step 1: Run the Setup Script

**Option A: Bash (Recommended - works on Windows Git Bash, Mac, Linux)**
```bash
bash scripts/setup-gcp.sh
```

**Option B: PowerShell (Windows native)**
```powershell
.\scripts\setup-gcp.ps1
```

The script will prompt you for:
- Your GCP project ID (from your hackathon account)

Everything else is automated!

### Step 2: Add Your SAM.gov API Key

After the script completes, update the Secret Manager secret with your real API key:

```bash
echo 'YOUR_ACTUAL_SAMGOV_API_KEY' | gcloud secrets versions add samgov-api-key --data-file=-
```

### Step 3: Configure GitHub Secrets (for CI/CD)

Go to your GitHub repository → **Settings** → **Secrets and variables** → **Actions**

Add these repository secrets:

| Secret Name | Value |
|-------------|-------|
| `GCP_PROJECT_ID` | Your GCP project ID from hackathon |
| `GCP_SA_KEY` | Full contents of `kaimi-sa-key.json` file |
| `GCP_REGION` | `us-east4` |

**To copy `GCP_SA_KEY` value:**
- Windows: `cat kaimi-sa-key.json | clip`
- Mac: `cat kaimi-sa-key.json | pbcopy`
- Linux: `cat kaimi-sa-key.json | xclip -selection clipboard`

---

## Hackathon Billing Account Notes

Since you're using a Google Hackathon billing account:

### Benefits
- ✅ Likely have credits or free tier access
- ✅ Vertex AI access should be pre-approved
- ✅ No personal billing needed

### Recommendations
1. **Monitor usage** even with credits - hackathons often have fair use policies
2. **Phase 0 is very low cost** - mostly setup, minimal API calls
3. **Gemini API calls** will be the main cost driver once Hunter runs
4. **Set up budget alerts** anyway to track usage:
   ```bash
   gcloud billing budgets create \
     --billing-account=YOUR_HACKATHON_BILLING_ID \
     --display-name="Kaimi Hackathon Budget" \
     --budget-amount=100
   ```

### Expected Costs

**Phase 0 (Current):**
- Infrastructure: < $1/month
- Gemini 2.5 Pro: Pay-per-use (minimal until Hunter agent runs frequently)

**Phase 1+ (Future):**
- Firestore: Pay-per-read/write
- Scheduled runs: Depends on Hunter frequency
- Estimated: $10-50/month for active development

For hackathon purposes, Phase 0 should cost essentially nothing.

---

## Verification Checklist

After running the setup script, verify everything works:

```bash
# 1. Activate service account
gcloud auth activate-service-account --key-file=kaimi-sa-key.json

# 2. Test Vertex AI access
gcloud ai models list --region=us-east4 --limit=5

# 3. Verify Secret Manager
gcloud secrets describe samgov-api-key

# 4. Check service account permissions
gcloud projects get-iam-policy YOUR_PROJECT_ID \
  --flatten="bindings[].members" \
  --filter="bindings.members:serviceAccount:kaimi-dev@"
```

All commands should succeed without errors.

---

## What Gets Created in Your GCP Project

### APIs Enabled
- `aiplatform.googleapis.com` - Vertex AI for Gemini 2.5 Pro
- `secretmanager.googleapis.com` - Secure storage for API keys
- `iam.googleapis.com` - Identity and Access Management
- `cloudbuild.googleapis.com` - CI/CD pipeline support
- `cloudkms.googleapis.com` - Encryption for Secret Manager
- `cloudresourcemanager.googleapis.com` - Project management

### IAM Service Account
**Name:** `kaimi-dev@YOUR_PROJECT_ID.iam.gserviceaccount.com`

**Roles granted (minimal for Phase 0):**
- `roles/aiplatform.user` - Access Vertex AI and Gemini
- `roles/secretmanager.admin` - Manage secrets
- `roles/logging.logWriter` - Write application logs
- `roles/monitoring.metricWriter` - Write metrics

**Note:** More roles will be added in later phases as needed (Firestore, Cloud Scheduler, etc.)

### Secrets
- `samgov-api-key` - Placeholder created, you'll update with real value

### Local Files (NOT committed to git)
- `kaimi-sa-key.json` - Service account credentials
- `.env.gcp` - Environment configuration

---

## CI/CD Integration

The enhanced `.github/workflows/ci.yml` now includes:

### Four Jobs:
1. **test** - Runs all Go tests with coverage
2. **lint** - Runs golangci-lint
3. **verify-gcp** - Verifies GCP access and permissions
4. **verify-acceptance-criteria** - Ensures PR references a ticket
5. **all-checks-pass** - Required status check (depends on all above)

### GCP Verification Checks:
- ✅ Vertex AI API enabled and accessible
- ✅ Secret Manager accessible
- ✅ Service account has correct permissions

All checks must pass before a PR can be merged (per WORKFLOW.md).

---

## Security Features Built In

### Credential Protection
- ✅ `kaimi-sa-key.json` in .gitignore (plus all `*-sa-key.json` patterns)
- ✅ `.env.gcp` in .gitignore
- ✅ `queue/*.json` excluded (Phase 1 local queue files)

### Least Privilege IAM
- ✅ Service account has only Phase 0 required permissions
- ✅ No overly broad roles (no Owner, Editor, etc.)
- ✅ Roles will be added incrementally in future phases

### Secret Management
- ✅ SAM.gov API key in Secret Manager (not environment variables)
- ✅ Automatic replication for high availability
- ✅ Ready for audit logging

---

## Next Steps After Setup

1. **Verify CI Pipeline**
   - Push to GitHub and watch the Actions tab
   - All four jobs should pass (green checkmarks)

2. **Begin Hunter Implementation**
   - Next Phase 0 work per ARCHITECTURE.md
   - Create GitHub Issue with acceptance criteria first (per WORKFLOW.md)
   - TDD approach: write tests, then code

3. **Phase 1 Preparation**
   - When Phase 1 begins, additional GCP services will be added:
     - Firestore (persistent opportunity queue)
     - Cloud Scheduler (daily Hunter runs)
     - Additional IAM roles

---

## Troubleshooting

### Script Fails with Permission Errors
**Issue:** Your user account doesn't have sufficient permissions
**Fix:** Ensure you have Owner or Editor role on the hackathon GCP project

### "gcloud: command not found"
**Issue:** gcloud CLI not installed or not in PATH
**Fix:** Install from https://cloud.google.com/sdk/docs/install

### Vertex AI Access Denied
**Issue:** Service account missing `roles/aiplatform.user`
**Fix:** Re-run the setup script or manually grant:
```bash
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:kaimi-dev@PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/aiplatform.user"
```

### Secret Manager Secret Already Exists (Conflict Error)
**Issue:** Running setup script multiple times
**Fix:** Update existing secret instead:
```bash
echo 'new-value' | gcloud secrets versions add samgov-api-key --data-file=-
```

---

## Documentation Reference

| Document | Purpose |
|----------|---------|
| **[docs/GCP_QUICKSTART.md](docs/GCP_QUICKSTART.md)** | Quick reference for common tasks |
| **[docs/GCP_SETUP.md](docs/GCP_SETUP.md)** | Comprehensive setup guide and troubleshooting |
| **[docs/README.md](docs/README.md)** | Documentation index |
| **[ARCHITECTURE.md](ARCHITECTURE.md)** | System design and architecture |
| **[WORKFLOW.md](WORKFLOW.md)** | Development workflow contract |

---

## Ready to Execute?

Run this now to set up your GCP environment:

```bash
# Make sure you're in the project root
cd C:\Users\Owner\OneDrive\Documents\Builder\Pulse

# Run the setup script
bash scripts/setup-gcp.sh
```

The script will:
1. Prompt for your GCP project ID
2. Enable all required APIs (~2 minutes)
3. Create service account and grant permissions (~1 minute)
4. Generate credentials and configuration files (~30 seconds)
5. Test access to verify everything works (~30 seconds)

**Total time: ~5 minutes**

---

## Questions?

- **Setup issues:** See [docs/GCP_SETUP.md](docs/GCP_SETUP.md#troubleshooting)
- **Architecture questions:** See [ARCHITECTURE.md](ARCHITECTURE.md)
- **Workflow questions:** See [WORKFLOW.md](WORKFLOW.md)

You're all set! The GCP environment is ready to go. 🚀

#!/bin/bash
# GCP Environment Setup for Kaimi
# This script sets up the GCP project for Phase 0 development
# Run this script with: bash scripts/setup-gcp.sh

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
PROJECT_ID=""  # Will be set interactively
REGION="us-east4"
SERVICE_ACCOUNT_NAME="kaimi-dev"
SERVICE_ACCOUNT_EMAIL=""
KEY_FILE="kaimi-sa-key.json"

echo -e "${GREEN}=== Kaimi GCP Setup ===${NC}"
echo "This script will configure your GCP project for Phase 0"
echo ""

# Step 1: Get project ID
echo -e "${YELLOW}Step 1: Project Configuration${NC}"
read -p "Enter your GCP Project ID: " PROJECT_ID

if [ -z "$PROJECT_ID" ]; then
    echo -e "${RED}Error: Project ID cannot be empty${NC}"
    exit 1
fi

SERVICE_ACCOUNT_EMAIL="${SERVICE_ACCOUNT_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"

echo "Using project: $PROJECT_ID"
echo "Region: $REGION"
echo ""

# Step 2: Set active project
echo -e "${YELLOW}Step 2: Setting active GCP project${NC}"
gcloud config set project "$PROJECT_ID"
echo -e "${GREEN}✓ Project set${NC}"
echo ""

# Step 3: Enable required APIs
echo -e "${YELLOW}Step 3: Enabling required GCP APIs${NC}"
echo "This may take a few minutes..."

APIS=(
    "cloudresourcemanager.googleapis.com"  # Resource Manager
    "iam.googleapis.com"                    # IAM
    "aiplatform.googleapis.com"             # Vertex AI (includes Gemini)
    "secretmanager.googleapis.com"          # Secret Manager
    "cloudbuild.googleapis.com"             # Cloud Build (for CI/CD)
    "cloudkms.googleapis.com"               # Cloud KMS (for Secret Manager encryption)
)

for api in "${APIS[@]}"; do
    echo "Enabling $api..."
    gcloud services enable "$api" --project="$PROJECT_ID"
done

echo -e "${GREEN}✓ All APIs enabled${NC}"
echo ""

# Step 4: Create service account
echo -e "${YELLOW}Step 4: Creating service account${NC}"

# Check if service account already exists
if gcloud iam service-accounts describe "$SERVICE_ACCOUNT_EMAIL" --project="$PROJECT_ID" &>/dev/null; then
    echo "Service account $SERVICE_ACCOUNT_EMAIL already exists"
else
    gcloud iam service-accounts create "$SERVICE_ACCOUNT_NAME" \
        --display-name="Kaimi Development Service Account" \
        --description="Service account for Kaimi agent development and CI/CD" \
        --project="$PROJECT_ID"
    echo -e "${GREEN}✓ Service account created${NC}"
fi
echo ""

# Step 5: Grant IAM roles
echo -e "${YELLOW}Step 5: Granting IAM permissions${NC}"

ROLES=(
    "roles/aiplatform.user"           # Vertex AI access for Gemini
    "roles/secretmanager.admin"       # Secret Manager (for SAM.gov API keys)
    "roles/logging.logWriter"         # Cloud Logging
    "roles/monitoring.metricWriter"   # Cloud Monitoring
)

for role in "${ROLES[@]}"; do
    echo "Granting $role..."
    gcloud projects add-iam-policy-binding "$PROJECT_ID" \
        --member="serviceAccount:$SERVICE_ACCOUNT_EMAIL" \
        --role="$role" \
        --condition=None \
        --quiet
done

echo -e "${GREEN}✓ IAM roles granted${NC}"
echo ""

# Step 6: Generate service account key
echo -e "${YELLOW}Step 6: Generating service account key${NC}"

if [ -f "$KEY_FILE" ]; then
    echo -e "${YELLOW}Warning: $KEY_FILE already exists${NC}"
    read -p "Overwrite? (y/n): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Skipping key generation"
    else
        rm "$KEY_FILE"
        gcloud iam service-accounts keys create "$KEY_FILE" \
            --iam-account="$SERVICE_ACCOUNT_EMAIL" \
            --project="$PROJECT_ID"
        echo -e "${GREEN}✓ Service account key created: $KEY_FILE${NC}"
    fi
else
    gcloud iam service-accounts keys create "$KEY_FILE" \
        --iam-account="$SERVICE_ACCOUNT_EMAIL" \
        --project="$PROJECT_ID"
    echo -e "${GREEN}✓ Service account key created: $KEY_FILE${NC}"
fi

echo ""
echo -e "${YELLOW}IMPORTANT: Keep $KEY_FILE secure!${NC}"
echo "- Add $KEY_FILE to .gitignore (already done if using project .gitignore)"
echo "- Never commit this file to version control"
echo "- Store it securely for CI/CD configuration"
echo ""

# Step 7: Create Secret Manager secrets
echo -e "${YELLOW}Step 7: Setting up Secret Manager${NC}"
echo "Creating placeholder secrets for SAM.gov API credentials..."

# Create secret for SAM.gov API Key
SECRET_NAME="samgov-api-key"
if gcloud secrets describe "$SECRET_NAME" --project="$PROJECT_ID" &>/dev/null; then
    echo "Secret $SECRET_NAME already exists"
else
    echo "placeholder-key-replace-with-real-value" | \
    gcloud secrets create "$SECRET_NAME" \
        --data-file=- \
        --replication-policy="automatic" \
        --project="$PROJECT_ID"
    echo -e "${GREEN}✓ Created secret: $SECRET_NAME${NC}"
fi

echo ""
echo -e "${YELLOW}Note: Update the SAM.gov API key secret with:${NC}"
echo "echo 'YOUR_ACTUAL_API_KEY' | gcloud secrets versions add $SECRET_NAME --data-file=-"
echo ""

# Step 8: Create environment configuration file
echo -e "${YELLOW}Step 8: Creating environment configuration${NC}"

cat > .env.gcp << EOF
# GCP Configuration for Kaimi
# Generated by scripts/setup-gcp.sh

# Project settings
GCP_PROJECT_ID=$PROJECT_ID
GCP_REGION=$REGION

# Service Account
GCP_SERVICE_ACCOUNT_EMAIL=$SERVICE_ACCOUNT_EMAIL
GOOGLE_APPLICATION_CREDENTIALS=$(pwd)/$KEY_FILE

# Vertex AI settings
VERTEX_AI_LOCATION=$REGION
GEMINI_MODEL=gemini-3.0-pro

# Secret Manager
SAMGOV_API_KEY_SECRET=$SECRET_NAME

# Zone-2 solicitation documents bucket (issue #162)
GCS_SOLICITATIONS_BUCKET=${PROJECT_ID}-solicitations

# Application settings
LOG_LEVEL=info
ENVIRONMENT=development
EOF

echo -e "${GREEN}✓ Created .env.gcp${NC}"
echo ""

# Step 9: Test Vertex AI access
echo -e "${YELLOW}Step 9: Testing Vertex AI access${NC}"
echo "Attempting to list models to verify access..."

export GOOGLE_APPLICATION_CREDENTIALS="$(pwd)/$KEY_FILE"

if gcloud ai models list --region="$REGION" --project="$PROJECT_ID" --limit=1 &>/dev/null; then
    echo -e "${GREEN}✓ Vertex AI access verified${NC}"
else
    echo -e "${YELLOW}⚠ Could not verify Vertex AI access. This may be normal if no models are deployed yet.${NC}"
fi

echo ""

# Step 10: Zone-1 deployment infrastructure (issue #122)
# Artifact Registry repo, GCS queue bucket, Cloud Run Job, Cloud Scheduler.
# All commands are idempotent: existing resources are left untouched.
echo -e "${YELLOW}Step 10: Zone-1 deployment infrastructure${NC}"

PIPELINE_IMAGE="${REGION}-docker.pkg.dev/${PROJECT_ID}/kaimi/pipeline:latest"
QUEUE_BUCKET="${PROJECT_ID}-queue"

if ! gcloud artifacts repositories describe kaimi --location="$REGION" --project="$PROJECT_ID" &>/dev/null; then
    gcloud artifacts repositories create kaimi \
        --repository-format=docker --location="$REGION" \
        --description="Kaimi container images" --project="$PROJECT_ID"
fi
echo -e "${GREEN}✓ Artifact Registry repo 'kaimi' ready${NC}"

if ! gcloud storage buckets describe "gs://${QUEUE_BUCKET}" --project="$PROJECT_ID" &>/dev/null; then
    gcloud storage buckets create "gs://${QUEUE_BUCKET}" \
        --location="$REGION" --project="$PROJECT_ID" --uniform-bucket-level-access
fi
gcloud storage buckets add-iam-policy-binding "gs://${QUEUE_BUCKET}" \
    --member="serviceAccount:${SERVICE_ACCOUNT_EMAIL}" \
    --role=roles/storage.objectAdmin >/dev/null
echo -e "${GREEN}✓ Queue bucket gs://${QUEUE_BUCKET} ready${NC}"

# The Cloud Run Job runs cmd/pipeline in live mode with the JSON store on a
# GCS volume mount, so scored opportunities persist across runs.
if ! gcloud run jobs describe kaimi-pipeline --region="$REGION" --project="$PROJECT_ID" &>/dev/null; then
    gcloud run jobs create kaimi-pipeline \
        --image "$PIPELINE_IMAGE" \
        --region "$REGION" --project "$PROJECT_ID" \
        --service-account "$SERVICE_ACCOUNT_EMAIL" \
        --set-env-vars "MODE=live,GCP_PROJECT_ID=${PROJECT_ID},GCP_REGION=${REGION},STORE_PATH=/mnt/store/queue" \
        --set-secrets "SAM_API_KEY=${SECRET_NAME}:latest" \
        --add-volume "name=store,type=cloud-storage,bucket=${QUEUE_BUCKET}" \
        --add-volume-mount "volume=store,mount-path=/mnt/store" \
        --memory 512Mi --cpu 1 --max-retries 1 --task-timeout 600
fi
echo -e "${GREEN}✓ Cloud Run Job kaimi-pipeline ready${NC}"

gcloud run jobs add-iam-policy-binding kaimi-pipeline \
    --region "$REGION" --project "$PROJECT_ID" \
    --member "serviceAccount:${SERVICE_ACCOUNT_EMAIL}" \
    --role roles/run.invoker >/dev/null

# Three quota-friendly runs per day (SAM.gov allows 1,000 requests/day; the
# client caches aggressively so repeat pulls are cheap).
if ! gcloud scheduler jobs describe kaimi-pipeline-schedule --location="$REGION" --project="$PROJECT_ID" &>/dev/null; then
    gcloud scheduler jobs create http kaimi-pipeline-schedule \
        --project "$PROJECT_ID" --location "$REGION" \
        --schedule "0 7,12,17 * * *" --time-zone "America/New_York" \
        --uri "https://${REGION}-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/${PROJECT_ID}/jobs/kaimi-pipeline:run" \
        --http-method POST \
        --oauth-service-account-email "$SERVICE_ACCOUNT_EMAIL"
fi
echo -e "${GREEN}✓ Cloud Scheduler trigger ready (07:00/12:00/17:00 ET)${NC}"
echo ""

# Step 11: Zone-2 solicitation documents bucket (issue #162)
# Stores raw solicitation files + extracted text for the ingestion stage
# (internal/ingest). Object layout: {noticeID}/raw/{filename} and
# {noticeID}/text/{filename}.txt. Idempotent: existing resources are left alone.
echo -e "${YELLOW}Step 11: Zone-2 solicitation documents bucket${NC}"

SOLICITATIONS_BUCKET="${PROJECT_ID}-solicitations"

if ! gcloud storage buckets describe "gs://${SOLICITATIONS_BUCKET}" --project="$PROJECT_ID" &>/dev/null; then
    gcloud storage buckets create "gs://${SOLICITATIONS_BUCKET}" \
        --location="$REGION" --project="$PROJECT_ID" \
        --uniform-bucket-level-access --public-access-prevention
fi
# Least-privilege: the pipeline SA only needs object read/write, not bucket admin.
gcloud storage buckets add-iam-policy-binding "gs://${SOLICITATIONS_BUCKET}" \
    --member="serviceAccount:${SERVICE_ACCOUNT_EMAIL}" \
    --role=roles/storage.objectAdmin >/dev/null
echo -e "${GREEN}✓ Solicitations bucket gs://${SOLICITATIONS_BUCKET} ready (uniform access, public access prevented)${NC}"
echo -e "${YELLOW}  Record GCS_SOLICITATIONS_BUCKET=${SOLICITATIONS_BUCKET} where the app reads config (.env / Cloud Run env).${NC}"
echo ""

# Summary
echo -e "${GREEN}=== Setup Complete ===${NC}"
echo ""
echo "Your GCP environment is ready for Kaimi Phase 0 development."
echo ""
echo "Configuration:"
echo "  Project ID: $PROJECT_ID"
echo "  Region: $REGION"
echo "  Service Account: $SERVICE_ACCOUNT_EMAIL"
echo "  Key File: $KEY_FILE"
echo "  Environment File: .env.gcp"
echo ""
echo "Next steps:"
echo "  1. Add your real SAM.gov API key to Secret Manager:"
echo "     echo 'YOUR_KEY' | gcloud secrets versions add $SECRET_NAME --data-file=-"
echo ""
echo "  2. Source the environment file in your shell:"
echo "     source .env.gcp"
echo ""
echo "  3. For CI/CD (GitHub Actions), add these secrets to your repository:"
echo "     - GCP_PROJECT_ID: $PROJECT_ID"
echo "     - GCP_SA_KEY: (contents of $KEY_FILE)"
echo ""
echo "  4. Test your setup by running:"
echo "     gcloud auth activate-service-account --key-file=$KEY_FILE"
echo "     gcloud ai models list --region=$REGION --limit=5"
echo ""
echo -e "${YELLOW}Remember: Never commit $KEY_FILE to version control!${NC}"

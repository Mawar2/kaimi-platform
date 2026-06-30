#!/usr/bin/env bash
# provision-customer.sh — stand up a new, fully-isolated Kaimi deployment for one
# BlueMeta customer with ~one command.
#
# Topology (locked decision): ONE GCP project per customer, BlueMeta-managed, each
# fully self-contained — its own image COPY, buckets, Firestore, and secrets, with
# NO cross-project runtime dependency. Access is product-key gated (magic links).
#
# What it does (each step is idempotent and logged; SECRETS ARE NEVER PRINTED):
#   1. Create the customer GCP project (under your org/folder) + link billing.
#   2. Enable Artifact Registry + create the per-customer AR repo.
#   3. Copy the CENTRAL release images (api + pipeline) into that repo — so the
#      customer project holds its OWN image copy (full isolation, no runtime dep
#      on kaimi-seeker).
#   4. Render terraform.tfvars from the template into envs/<customer-id>/.
#   5. terraform init + apply (product-key mode; Firestore + instant-hunt IAM).
#   6. Seed the session secret (generated locally; never printed).
#   7. terraform output the API URL, then mint the first product key.
#   8. Print a summary: project, API URL, magic link, send-the-welcome reminder.
#
# What it deliberately does NOT do: enter the SAM.gov key (the customer enters it
# during onboarding) or wire Drive OAuth (Phase 3 — see the TODO below).
#
# Prereqs (see docs/PROVISION_CUSTOMER.md):
#   - gcloud authenticated as a user/SA with project-create on your org/folder and
#     billing-link on the billing account, AND read on the central release repo.
#   - terraform >= 1.5, go (for cmd/kaimi-key), docker NOT required (images are
#     COPIED server-side, not built here).
#   - The central release images already published (CI publish-release-images job).
#
# Usage:
#   scripts/provision-customer.sh \
#     --customer-id kaimi-ey3 \
#     --display-name "Ey3 Technologies" \
#     --billing-account 012345-6789AB-CDEF01 \
#     --org-id 123456789012            # OR --folder-id 987654321098
#     [--region us-east4] \
#     [--tester-label "Ey3 Technologies"] \
#     [--key-days 60] \
#     [--budget-usd 50] \
#     [--firestore-location us-east4] \
#     [--release-project kaimi-seeker] \
#     [--release-repo kaimi-release] \
#     [--image-tag latest] \
#     [--yes]   # skip the confirmation prompt
#
# Most flags also read from env (CUSTOMER_ID, DISPLAY_NAME, BILLING_ACCOUNT,
# ORG_ID, FOLDER_ID, REGION, TESTER_LABEL, KEY_DAYS, ...). Flags win over env.

set -euo pipefail

# --- Defaults ----------------------------------------------------------------
CUSTOMER_ID="${CUSTOMER_ID:-}"
DISPLAY_NAME="${DISPLAY_NAME:-}"
BILLING_ACCOUNT="${BILLING_ACCOUNT:-}"
ORG_ID="${ORG_ID:-}"
FOLDER_ID="${FOLDER_ID:-}"
REGION="${REGION:-us-east4}"
TESTER_LABEL="${TESTER_LABEL:-}"
KEY_DAYS="${KEY_DAYS:-60}"
BUDGET_USD="${BUDGET_USD:-50}"
FIRESTORE_LOCATION="${FIRESTORE_LOCATION:-}"
RELEASE_PROJECT="${RELEASE_PROJECT:-kaimi-seeker}"
RELEASE_REPO="${RELEASE_REPO:-kaimi-release}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
ASSUME_YES="${ASSUME_YES:-false}"

# --- Logging helpers (to stderr so stdout stays clean for any capture) --------
log()  { printf '\033[1;34m[provision]\033[0m %s\n' "$*" >&2; }
ok()   { printf '\033[1;32m[ ok ]\033[0m %s\n' "$*" >&2; }
warn() { printf '\033[1;33m[warn]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[fail]\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  sed -n '2,46p' "$0" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

# --- Parse flags -------------------------------------------------------------
while [ $# -gt 0 ]; do
  case "$1" in
    --customer-id)        CUSTOMER_ID="$2"; shift 2 ;;
    --display-name)       DISPLAY_NAME="$2"; shift 2 ;;
    --billing-account)    BILLING_ACCOUNT="$2"; shift 2 ;;
    --org-id)             ORG_ID="$2"; shift 2 ;;
    --folder-id)          FOLDER_ID="$2"; shift 2 ;;
    --region)             REGION="$2"; shift 2 ;;
    --tester-label)       TESTER_LABEL="$2"; shift 2 ;;
    --key-days)           KEY_DAYS="$2"; shift 2 ;;
    --budget-usd)         BUDGET_USD="$2"; shift 2 ;;
    --firestore-location) FIRESTORE_LOCATION="$2"; shift 2 ;;
    --release-project)    RELEASE_PROJECT="$2"; shift 2 ;;
    --release-repo)       RELEASE_REPO="$2"; shift 2 ;;
    --image-tag)          IMAGE_TAG="$2"; shift 2 ;;
    --yes|-y)             ASSUME_YES="true"; shift ;;
    -h|--help)            usage 0 ;;
    *) die "Unknown argument: $1 (see --help)" ;;
  esac
done

# --- Validate inputs ---------------------------------------------------------
[ -n "$CUSTOMER_ID" ]     || die "--customer-id is required (becomes the GCP project id, e.g. kaimi-ey3)."
[ -n "$DISPLAY_NAME" ]    || die "--display-name is required (e.g. \"Ey3 Technologies\")."
[ -n "$BILLING_ACCOUNT" ] || die "--billing-account is required (e.g. 012345-6789AB-CDEF01)."
if [ -z "$ORG_ID" ] && [ -z "$FOLDER_ID" ]; then
  die "exactly one of --org-id / --folder-id is required (where the project is created)."
fi
if [ -n "$ORG_ID" ] && [ -n "$FOLDER_ID" ]; then
  die "pass only ONE of --org-id / --folder-id, not both."
fi
# GCP project ids: 6-30 chars, lowercase letter start, lowercase/digits/hyphens.
echo "$CUSTOMER_ID" | grep -Eq '^[a-z][a-z0-9-]{4,28}[a-z0-9]$' \
  || die "--customer-id \"$CUSTOMER_ID\" is not a valid GCP project id (6-30 chars, lowercase, start with a letter, no trailing hyphen)."
echo "$KEY_DAYS" | grep -Eq '^[0-9]+$' || die "--key-days must be a positive integer."
[ "$KEY_DAYS" -gt 0 ] || die "--key-days must be > 0."

command -v gcloud    >/dev/null 2>&1 || die "gcloud not found on PATH."
command -v terraform >/dev/null 2>&1 || die "terraform not found on PATH (>= 1.5 required)."
command -v go        >/dev/null 2>&1 || die "go not found on PATH (needed to mint the first key)."

PROJECT_ID="$CUSTOMER_ID"
TENANT_ID="$CUSTOMER_ID"
[ -n "$TESTER_LABEL" ] || TESTER_LABEL="$DISPLAY_NAME"
[ -n "$FIRESTORE_LOCATION" ] || FIRESTORE_LOCATION="$REGION"

# Resolve repo paths.
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TEMPLATE_DIR="$REPO_ROOT/deploy/terraform/envs/_customer-template"
ENV_DIR="$REPO_ROOT/deploy/terraform/envs/$CUSTOMER_ID"
[ -d "$TEMPLATE_DIR" ] || die "customer template not found at $TEMPLATE_DIR"

CENTRAL_BASE="${REGION}-docker.pkg.dev/${RELEASE_PROJECT}/${RELEASE_REPO}"
CUSTOMER_REPO="kaimi"  # matches modules/kaimi local.artifact_repo (name_prefix empty)
CUSTOMER_BASE="${REGION}-docker.pkg.dev/${PROJECT_ID}/${CUSTOMER_REPO}"
PIPELINE_IMAGE="${CUSTOMER_BASE}/pipeline:${IMAGE_TAG}"
API_IMAGE="${CUSTOMER_BASE}/api:${IMAGE_TAG}"

# --- Confirm -----------------------------------------------------------------
log "About to provision a NEW Kaimi customer (creates a GCP project + billable infra):"
log "  customer-id / project : $PROJECT_ID"
log "  display name          : $DISPLAY_NAME"
log "  parent                : ${ORG_ID:+org $ORG_ID}${FOLDER_ID:+folder $FOLDER_ID}"
log "  billing account       : $BILLING_ACCOUNT"
log "  region                : $REGION"
log "  central images        : ${CENTRAL_BASE}/{api,pipeline}:${IMAGE_TAG}"
log "  customer image copy   : ${CUSTOMER_BASE}/{api,pipeline}:${IMAGE_TAG}"
log "  product key window    : $KEY_DAYS days (tester: $TESTER_LABEL)"
log "  terraform env dir     : $ENV_DIR"
if [ "$ASSUME_YES" != "true" ]; then
  printf '\033[1;33mProceed? This creates billable cloud resources. [y/N] \033[0m' >&2
  read -r reply
  case "$reply" in y|Y|yes|YES) ;; *) die "aborted by operator." ;; esac
fi

# =============================================================================
# Step a — create the project + link billing (idempotent)
# =============================================================================
log "Step a: ensure project $PROJECT_ID exists"
if gcloud projects describe "$PROJECT_ID" >/dev/null 2>&1; then
  ok "project $PROJECT_ID already exists — skipping create"
else
  PARENT_FLAG=()
  [ -n "$ORG_ID" ]    && PARENT_FLAG=(--organization "$ORG_ID")
  [ -n "$FOLDER_ID" ] && PARENT_FLAG=(--folder "$FOLDER_ID")
  gcloud projects create "$PROJECT_ID" \
    --name "$DISPLAY_NAME" \
    "${PARENT_FLAG[@]}" \
    || die "failed to create project $PROJECT_ID"
  ok "created project $PROJECT_ID"
fi

log "Step a: ensure billing is linked"
CURRENT_BA="$(gcloud billing projects describe "$PROJECT_ID" --format='value(billingAccountName)' 2>/dev/null || true)"
if [ "$CURRENT_BA" = "billingAccounts/$BILLING_ACCOUNT" ]; then
  ok "billing already linked to $BILLING_ACCOUNT"
else
  gcloud billing projects link "$PROJECT_ID" --billing-account "$BILLING_ACCOUNT" \
    || die "failed to link billing account $BILLING_ACCOUNT to $PROJECT_ID"
  ok "linked billing account $BILLING_ACCOUNT"
fi

# =============================================================================
# Step b — enable Artifact Registry API + create the per-customer repo
# =============================================================================
log "Step b: enable artifactregistry.googleapis.com (so we can create the repo + copy images pre-apply)"
gcloud services enable artifactregistry.googleapis.com --project "$PROJECT_ID" \
  || die "failed to enable Artifact Registry API on $PROJECT_ID"
ok "Artifact Registry API enabled"

log "Step b: ensure customer Artifact Registry repo \"$CUSTOMER_REPO\" exists in $PROJECT_ID"
if gcloud artifacts repositories describe "$CUSTOMER_REPO" \
     --project "$PROJECT_ID" --location "$REGION" >/dev/null 2>&1; then
  ok "repo $CUSTOMER_REPO already exists"
else
  gcloud artifacts repositories create "$CUSTOMER_REPO" \
    --project "$PROJECT_ID" --location "$REGION" \
    --repository-format=docker \
    --description="Kaimi container images (per-customer copy of the central release)" \
    || die "failed to create Artifact Registry repo $CUSTOMER_REPO"
  ok "created repo $CUSTOMER_REPO"
fi

# =============================================================================
# Step c — copy the CENTRAL release images into the customer's repo
#          (server-side copy; the customer project holds its OWN copy → isolation)
# =============================================================================
copy_image() {
  local name="$1"  # api | pipeline
  local src="${CENTRAL_BASE}/${name}:${IMAGE_TAG}"
  local dst="${CUSTOMER_BASE}/${name}:${IMAGE_TAG}"
  log "Step c: copy ${name} image into customer project"
  # `artifacts docker images copy` is a server-side copy; --quiet skips the
  # interactive prompt. Re-copying an identical tag is harmless (overwrites).
  gcloud artifacts docker images copy "$src" "$dst" --quiet \
    || die "failed to copy $src -> $dst (is the central release published? see CI publish-release-images)"
  ok "copied ${name}: ${dst}"
}
copy_image pipeline
copy_image api

# =============================================================================
# Step d — render terraform.tfvars from the template into envs/<customer-id>/
# =============================================================================
log "Step d: render terraform config into $ENV_DIR"
mkdir -p "$ENV_DIR"
# Copy the template main.tf (overwrite so it tracks the latest template).
cp "$TEMPLATE_DIR/main.tf" "$ENV_DIR/main.tf"

# Render tfvars by substituting the __TOKENS__. Use a sed-safe delimiter (|) and
# escape any | in values (none expected, but be defensive on display name).
esc() { printf '%s' "$1" | sed -e 's/[&|\\]/\\&/g'; }
sed \
  -e "s|__PROJECT_ID__|$(esc "$PROJECT_ID")|g" \
  -e "s|__REGION__|$(esc "$REGION")|g" \
  -e "s|__TENANT_ID__|$(esc "$TENANT_ID")|g" \
  -e "s|__TENANT_DISPLAY_NAME__|$(esc "$DISPLAY_NAME")|g" \
  -e "s|__PIPELINE_IMAGE__|$(esc "$PIPELINE_IMAGE")|g" \
  -e "s|__API_IMAGE__|$(esc "$API_IMAGE")|g" \
  -e "s|__FIRESTORE_LOCATION__|$(esc "$FIRESTORE_LOCATION")|g" \
  -e "s|__BILLING_ACCOUNT__|$(esc "$BILLING_ACCOUNT")|g" \
  -e "s|__BUDGET_AMOUNT_USD__|$(esc "$BUDGET_USD")|g" \
  "$TEMPLATE_DIR/terraform.tfvars.template" > "$ENV_DIR/terraform.tfvars"
ok "wrote $ENV_DIR/terraform.tfvars (tfvars is gitignored; holds no secret values)"

# =============================================================================
# Step e — terraform init + apply
# =============================================================================
# Terraform authenticates to GCP with an OAuth access token from gcloud, matching
# deploy/terraform/README.md's pattern (ADC also works; the token avoids needing
# `gcloud auth application-default login` separately when you're already gcloud-auth'd).
log "Step e: terraform init"
GOOGLE_OAUTH_ACCESS_TOKEN="$(gcloud auth print-access-token)"; export GOOGLE_OAUTH_ACCESS_TOKEN
terraform -chdir="$ENV_DIR" init -input=false \
  || die "terraform init failed in $ENV_DIR"

log "Step e: terraform apply (product-key mode: Firestore + instant-hunt IAM + Cloud Run)"
terraform -chdir="$ENV_DIR" apply -input=false -auto-approve \
  || die "terraform apply failed in $ENV_DIR"
ok "terraform apply complete"

# =============================================================================
# Step f — seed the session secret (generated locally; NEVER printed)
# =============================================================================
# Only the session-signing secret is generated here. The SAM.gov key is entered
# by the customer during onboarding (the runtime SA has secretVersionAdder on
# samgov-api-key for exactly that). Drive OAuth secrets are Phase 3.
# TODO(phase-3): seed drive-oauth-client-secret + set drive_oauth_client_id once
#                the customer-Drive connect flow ships.
log "Step f: seed the session signing secret (value generated locally, not printed)"
if command -v openssl >/dev/null 2>&1; then
  openssl rand -base64 32 | gcloud secrets versions add session-secret \
    --data-file=- --project "$PROJECT_ID" >/dev/null \
    || die "failed to add session-secret version"
  ok "session-secret seeded (a real version now overrides the placeholder)"
else
  warn "openssl not found — SKIPPED seeding session-secret. Seed it manually:"
  warn "  openssl rand -base64 32 | gcloud secrets versions add session-secret --data-file=- --project $PROJECT_ID"
fi

# =============================================================================
# Step g — read the API URL, then mint the first product key
# =============================================================================
log "Step g: read API URL + mint the first product key"
API_URL="$(terraform -chdir="$ENV_DIR" output -raw api_service_url)" \
  || die "could not read api_service_url from terraform output"
[ -n "$API_URL" ] || die "api_service_url output was empty"
ok "API URL: $API_URL"

# Mint the first key via the kaimi-key CLI against THIS project's Firestore.
# Capture output so we can surface the magic link in the summary.
MINT_OUT="$(cd "$REPO_ROOT" && GCP_PROJECT_ID="$PROJECT_ID" \
  go run ./cmd/kaimi-key -project "$PROJECT_ID" mint \
    --tester "$TESTER_LABEL" --days "$KEY_DAYS" --url "$API_URL")" \
  || die "kaimi-key mint failed (is Firestore provisioned + are you authed with datastore access?)"
MAGIC_LINK="$(printf '%s\n' "$MINT_OUT" | sed -n 's/^Magic link:[[:space:]]*//p')"

# =============================================================================
# Step h — summary
# =============================================================================
{
  echo
  echo "============================================================"
  echo " Kaimi customer provisioned: $DISPLAY_NAME"
  echo "============================================================"
  echo "  Project        : $PROJECT_ID"
  echo "  Region         : $REGION"
  echo "  API / dashboard: $API_URL"
  echo "  Image copy     : ${CUSTOMER_BASE}/{api,pipeline}:${IMAGE_TAG}"
  echo "  Gate mode      : product-key (magic-link sign-ups)"
  echo "  Terraform dir  : $ENV_DIR"
  echo
  echo "  First product key (window: $KEY_DAYS days, tester: $TESTER_LABEL):"
  printf '%s\n' "$MINT_OUT" | sed 's/^/    /'
  echo
  if [ -n "$MAGIC_LINK" ]; then
    echo "  >> SEND THE WELCOME EMAIL with this magic link:"
    echo "       $MAGIC_LINK"
  else
    echo "  >> Mint printed no magic link; re-run with --url to get one, or build it as"
    echo "       ${API_URL%/}/access?key=<KEY-ABOVE>"
  fi
  echo
  echo "  Verify:  curl -sI \"$API_URL\"   # expect a 302 (to /home or onboarding)"
  echo "  The customer enters their SAM.gov key during onboarding; that fires their first hunt."
  echo "  TODO(phase-3): Google Drive connect (drive-oauth secrets) is not wired yet."
  echo "  Teardown: scripts/deprovision-customer.sh --customer-id $PROJECT_ID"
  echo "============================================================"
} >&2

ok "done."

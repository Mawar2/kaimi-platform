#!/usr/bin/env bash
# deprovision-customer.sh — tear down a Kaimi customer provisioned with
# scripts/provision-customer.sh.
#
# By default this runs `terraform destroy` in the customer's env dir. Because the
# module sets force_destroy=false on the GCS data buckets, a destroy will FAIL
# SAFELY (HTTP 409 "Bucket not empty") on any bucket that still holds historical
# opportunity data — so casual teardown can't silently delete a customer's data.
# Pass --force-destroy-data to accept deleting the bucket contents along with the
# infra. Pass --delete-project to ALSO delete the whole GCP project afterward
# (the cleanest, most complete teardown — schedules project deletion in ~30 days,
# recoverable until then).
#
# Usage:
#   scripts/deprovision-customer.sh --customer-id kaimi-ey3
#   scripts/deprovision-customer.sh --customer-id kaimi-ey3 --force-destroy-data
#   scripts/deprovision-customer.sh --customer-id kaimi-ey3 --delete-project
#   scripts/deprovision-customer.sh --customer-id kaimi-ey3 --delete-project --skip-destroy
#
# Flags:
#   --customer-id <id>      REQUIRED. The project id / env dir used at provision.
#   --force-destroy-data    Pass force_destroy=true to terraform destroy (DELETES
#                           the queue + solicitations bucket contents).
#   --delete-project        After destroy, `gcloud projects delete <id>`.
#   --skip-destroy          Skip terraform destroy (use with --delete-project to
#                           just delete the whole project — fastest full teardown).
#   --yes / -y              Skip the confirmation prompt.

set -euo pipefail

CUSTOMER_ID="${CUSTOMER_ID:-}"
FORCE_DESTROY_DATA="false"
DELETE_PROJECT="false"
SKIP_DESTROY="false"
ASSUME_YES="${ASSUME_YES:-false}"

log()  { printf '\033[1;34m[deprovision]\033[0m %s\n' "$*" >&2; }
ok()   { printf '\033[1;32m[ ok ]\033[0m %s\n' "$*" >&2; }
warn() { printf '\033[1;33m[warn]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[fail]\033[0m %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --customer-id)       CUSTOMER_ID="$2"; shift 2 ;;
    --force-destroy-data) FORCE_DESTROY_DATA="true"; shift ;;
    --delete-project)    DELETE_PROJECT="true"; shift ;;
    --skip-destroy)      SKIP_DESTROY="true"; shift ;;
    --yes|-y)            ASSUME_YES="true"; shift ;;
    -h|--help)           sed -n '2,38p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "Unknown argument: $1 (see --help)" ;;
  esac
done

[ -n "$CUSTOMER_ID" ] || die "--customer-id is required."
command -v terraform >/dev/null 2>&1 || die "terraform not found on PATH."
command -v gcloud    >/dev/null 2>&1 || die "gcloud not found on PATH."

PROJECT_ID="$CUSTOMER_ID"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_DIR="$REPO_ROOT/deploy/terraform/envs/$CUSTOMER_ID"

log "Teardown plan for customer $PROJECT_ID:"
log "  terraform destroy : $([ "$SKIP_DESTROY" = true ] && echo SKIPPED || echo yes) (env dir: $ENV_DIR)"
log "  force-destroy data: $FORCE_DESTROY_DATA  (true = delete bucket contents)"
log "  delete project    : $DELETE_PROJECT"
if [ "$ASSUME_YES" != "true" ]; then
  printf '\033[1;31mThis is destructive. Proceed? [y/N] \033[0m' >&2
  read -r reply
  case "$reply" in y|Y|yes|YES) ;; *) die "aborted by operator." ;; esac
fi

if [ "$SKIP_DESTROY" != "true" ]; then
  [ -d "$ENV_DIR" ] || die "env dir not found: $ENV_DIR (was this customer provisioned here?)"
  GOOGLE_OAUTH_ACCESS_TOKEN="$(gcloud auth print-access-token)"; export GOOGLE_OAUTH_ACCESS_TOKEN
  log "terraform init (ensure providers present)"
  terraform -chdir="$ENV_DIR" init -input=false >/dev/null || die "terraform init failed"
  DESTROY_ARGS=(-input=false -auto-approve)
  [ "$FORCE_DESTROY_DATA" = "true" ] && DESTROY_ARGS+=(-var "force_destroy=true")
  log "terraform destroy ${FORCE_DESTROY_DATA:+(force_destroy=$FORCE_DESTROY_DATA)}"
  terraform -chdir="$ENV_DIR" destroy "${DESTROY_ARGS[@]}" \
    || die "terraform destroy failed (a non-empty data bucket fails SAFELY by default; pass --force-destroy-data to delete contents)."
  ok "terraform destroy complete"
fi

if [ "$DELETE_PROJECT" = "true" ]; then
  log "deleting GCP project $PROJECT_ID (recoverable for ~30 days)"
  gcloud projects delete "$PROJECT_ID" --quiet \
    || die "failed to delete project $PROJECT_ID"
  ok "project $PROJECT_ID scheduled for deletion"
fi

ok "deprovision done for $PROJECT_ID."

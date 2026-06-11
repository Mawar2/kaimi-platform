# Kaimi Terraform Module — Self-Serve Deploy

**Last updated:** 2026-06-11

This Terraform module deploys Kaimi into a **fresh GCP project you own**. It is
the declarative replacement for `scripts/setup-gcp.sh`: it provisions the same
APIs, runtime identity, container registry, GCS buckets, Cloud Run Job, Cloud
Scheduler trigger, and Secret Manager containers — **and adds the Cloud Run
service for the JSON API** (`cmd/api`).

Everything customer-specific is a variable. There are no hardcoded project IDs,
organization names, or model choices.

```
deploy/terraform/
├── modules/kaimi/            # the reusable module
│   ├── versions.tf           # Terraform + pinned provider versions
│   ├── variables.tf          # per-customer inputs
│   ├── main.tf               # all resources (grouped + commented)
│   └── outputs.tf            # URLs, names, SA email, bucket names
├── envs/example/             # a root config you copy
│   ├── main.tf               # provider config + module "kaimi" { ... }
│   └── terraform.tfvars.example
├── .gitignore                # keeps state + tfvars out of git
└── README.md                 # this file
```

## What gets created

| Resource | Terraform | Mirrors `setup-gcp.sh` |
|---|---|---|
| Enabled APIs (resourcemanager, iam, aiplatform, secretmanager, cloudbuild, cloudkms, run, scheduler, artifactregistry, storage, logging, monitoring) | `google_project_service.required` | Step 3 (+ implicit infra APIs) |
| Runtime service account (`kaimi-runtime`, **no JSON key**) | `google_service_account.runtime` | Step 4 (Step 6 key generation **dropped** — security improvement) |
| Project IAM: `aiplatform.user`, `secretmanager.secretAccessor` (**not admin**), `logging.logWriter`, `monitoring.metricWriter` | `google_project_iam_member.runtime_roles` | Step 5 (admin → accessor = least privilege) |
| Artifact Registry repo `kaimi` | `google_artifact_registry_repository.kaimi` | Step 10 |
| GCS buckets `${project}-queue`, `${project}-solicitations` (uniform access, public-access prevention) | `google_storage_bucket.{queue,solicitations}` | Steps 10–11 |
| Bucket IAM: `storage.objectAdmin` on each bucket | `google_storage_bucket_iam_member.*` | Steps 10–11 |
| Secret containers `samgov-api-key`, `oauth-client-secret`, `drive-oauth-client-secret`, `session-secret` (each seeded with a **placeholder** version) | `google_secret_manager_secret.secrets` + `google_secret_manager_secret_version.placeholder` | Step 7 (extended; **real** values added out-of-band) |
| Cloud Run **Job** `kaimi-pipeline` (GCS volume mount, secret env) | `google_cloud_run_v2_job.pipeline` | Step 10 |
| Cloud Run **Service** `kaimi-api` | `google_cloud_run_v2_service.api` | **NEW** (not in the script) |
| Cloud Scheduler `kaimi-pipeline-schedule` (cron `0 7,12,17 * * *`) | `google_cloud_scheduler_job.pipeline_schedule` | Step 10 |
| Run-invoker IAM (scheduler → job; optional public → API) | `google_cloud_run_v2_job_iam_member` / `..._service_iam_member` | Step 10 |

### Security improvements over the script
- **No service-account JSON key** is created. Cloud Run uses the attached SA's
  Application Default Credentials. (The script wrote `kaimi-sa-key.json`.)
- The SA gets **`secretmanager.secretAccessor`**, not `secretmanager.admin`.
- **Real secret values never enter Terraform state or tfvars.** The module creates
  each secret container and seeds it with a single, obvious **placeholder** version
  (`REPLACE_ME_VIA_GCLOUD`) — never a real secret. You add the real value with
  `gcloud` (below); a `lifecycle { ignore_changes = [secret_data] }` on the
  placeholder keeps Terraform from ever clobbering that real version on later
  applies. (The placeholder exists only because Cloud Run validates that a
  referenced secret version exists at create time — without it, the very first
  `apply` fails with `Secret version latest does not exist`.)

### Drive/Docs note
The customer-Drive and Google Docs integrations use **user OAuth scopes**, not
project-level service APIs, so they are **not** enabled as `google_project_service`
here. Configure them via the OAuth variables + secrets below.

## Prerequisites

- A fresh GCP **project** with **billing enabled**.
- `gcloud` and `terraform` (>= 1.5) installed; `docker` to build images.
- Deploy-time credentials: `gcloud auth application-default login` (or a
  deploy-time SA key in `GOOGLE_APPLICATION_CREDENTIALS`). This is separate from
  the runtime SA, which has no key.

## Zero-to-running

### 1. Create the project (if you haven't)
```sh
gcloud projects create acme-kaimi-prod
gcloud billing projects link acme-kaimi-prod --billing-account=XXXXXX-XXXXXX-XXXXXX
```

### 2. Copy and fill the config
```sh
cp -r deploy/terraform/envs/example deploy/terraform/envs/acme   # or work in-place
cd deploy/terraform/envs/acme
cp terraform.tfvars.example terraform.tfvars
# edit terraform.tfvars: project_id, region, tenant_id, tenant_display_name,
# pipeline_image, api_image (+ optional OAuth client id / domain).
```

### 3. First apply — create the registry + infra
For the very first apply, the image references don't need to resolve yet (Cloud
Run validates the image on deploy of the revision). Two equivalent paths:

- **Recommended:** bootstrap with sign-in disabled so you can reach the API to
  read its URL, then harden:
  ```sh
  terraform init
  terraform apply -var 'api_insecure_no_auth=true'
  ```
- Or apply normally if you already know your OAuth redirect URL.

### 4. Build and push the images
```sh
gcloud auth configure-docker us-east4-docker.pkg.dev
# repo path comes from: terraform output artifact_registry_repository
docker build -f cmd/pipeline/Dockerfile -t us-east4-docker.pkg.dev/acme-kaimi-prod/kaimi/pipeline:latest .
docker build -f cmd/api/Dockerfile      -t us-east4-docker.pkg.dev/acme-kaimi-prod/kaimi/api:latest .
docker push us-east4-docker.pkg.dev/acme-kaimi-prod/kaimi/pipeline:latest
docker push us-east4-docker.pkg.dev/acme-kaimi-prod/kaimi/api:latest
```
Then re-apply so the Job/Service pick up the pushed images:
```sh
terraform apply
```

### 5. Seed the secrets (out-of-band — values never touch Terraform)
> **Required before the deployment is functional.** Each secret was created with a
> harmless **placeholder** version (`REPLACE_ME_VIA_GCLOUD`) so the first `apply`
> succeeds. The running Job/Service will read that placeholder until you add the
> real value here. The commands below add a *new* version that becomes `:latest`;
> Terraform's `ignore_changes` on the placeholder means it will never overwrite
> your real value, and the real value never enters Terraform state or tfvars.
```sh
# SAM.gov API key (required for live hunting)
printf '%s' 'YOUR_SAM_GOV_KEY' | gcloud secrets versions add samgov-api-key --data-file=- --project=acme-kaimi-prod

# Sign-in OAuth client secret + session signing key (required for production auth)
printf '%s' 'YOUR_OAUTH_CLIENT_SECRET' | gcloud secrets versions add oauth-client-secret --data-file=- --project=acme-kaimi-prod
openssl rand -base64 48 | gcloud secrets versions add session-secret --data-file=- --project=acme-kaimi-prod

# Optional: customer-Drive connect client secret
printf '%s' 'YOUR_DRIVE_OAUTH_CLIENT_SECRET' | gcloud secrets versions add drive-oauth-client-secret --data-file=- --project=acme-kaimi-prod
```
> Cloud Run reads `:latest`, so adding a new version and redeploying the revision
> picks it up. Adding a version does NOT require `terraform apply`, but the running
> revision must be restarted to read a newly-added secret value.

### 6. Configure sign-in (production)
1. `terraform output api_service_url` → note the host.
2. In Google Cloud Console → APIs & Services → Credentials, create/edit an OAuth
   2.0 **Web application** client. Add `https://<api-host>/auth/callback` as an
   authorized redirect URI.
3. Put the client id + domain in `terraform.tfvars`
   (`oauth_client_id`, `allowed_workspace_domain`, `oauth_redirect_url`), set
   `api_insecure_no_auth = false`, then `terraform apply`.
   (The client *secret* was seeded in step 5; it is not in tfvars.)

### 7. Open the dashboard / API and onboard
```sh
terraform output api_service_url
```
Open the URL, sign in with a Workspace account in the allowed domain, and complete
onboarding (the company profile is written through the API and persists in the
queue bucket). The pipeline runs automatically on the schedule; to run it now:
```sh
gcloud run jobs execute kaimi-pipeline --region=us-east4 --project=acme-kaimi-prod
```

## Configuration reference

Required: `project_id`, `tenant_id`, `tenant_display_name`, `pipeline_image`,
`api_image`. Everything else has a sensible default (see
`modules/kaimi/variables.tf`). Defaults for models, region, and schedule match
`internal/config` and `setup-gcp.sh`.

Environment variables the images read (set by this module): `MODE`, `STORE_PATH`,
`GCP_PROJECT_ID`, `GCP_REGION`, `GEMINI_MODEL`, `OUTLINE_MODEL`,
`FINALREVIEW_MODEL`, `TENANT_ID`, `TENANT_DISPLAY_NAME`,
`ELIGIBILITY_PROFILE_PATH`, `NAICS_CODES`, `GCS_SOLICITATIONS_BUCKET`,
`API_HOST`, `CORS_ALLOWED_ORIGINS`, `KAIMI_INSECURE_NO_AUTH`, `OAUTH_CLIENT_ID`,
`OAUTH_REDIRECT_URL`, `OAUTH_ALLOWED_DOMAIN`, `DRIVE_OAUTH_CLIENT_ID`,
`DRIVE_OAUTH_REDIRECT_URL`, `DOCUMENTAI_PROCESSOR_ID`, `DOCUMENTAI_LOCATION`, and
the secret-sourced `SAM_API_KEY`, `OAUTH_CLIENT_SECRET`, `SESSION_SECRET`,
`DRIVE_OAUTH_CLIENT_SECRET`. `$PORT` is injected by Cloud Run and honored by the
API binary automatically.

## Teardown
```sh
terraform destroy
```
Enabled APIs are left on by default (`disable_on_destroy = false`) so a shared
project isn't broken; secret *containers* are destroyed but you may want to copy
out any versions first. Buckets must be empty to delete.

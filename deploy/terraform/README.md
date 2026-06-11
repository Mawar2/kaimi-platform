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

## Deploying into a shared project (`name_prefix`)

By default this module deploys with **fixed resource names** (`kaimi-pipeline`,
`kaimi-api`, `kaimi-pipeline-schedule`, the `kaimi` Artifact Registry repo,
`${project}-queue` / `${project}-solicitations` buckets, the `kaimi-runtime`
service account, and the `samgov-api-key` / `oauth-client-secret` /
`drive-oauth-client-secret` / `session-secret` secrets). That is ideal for a
**dedicated project** — leave `name_prefix` unset and nothing changes.

Set **`name_prefix`** only when you must deploy **a second, fully-separate Kaimi
stack into a project that already runs one** — for example `kaimi-seeker`, which
already runs the fixed-name hackathon pipeline. The prefix is prepended to every
collision-prone resource name so the two stacks coexist without clobbering each
other and **each remains cleanly destroyable on its own**:

| Resource | Default name | With `name_prefix = "bm"` |
|---|---|---|
| Cloud Run Job | `kaimi-pipeline` | `bm-kaimi-pipeline` |
| Cloud Run Service | `kaimi-api` | `bm-kaimi-api` |
| Cloud Scheduler job | `kaimi-pipeline-schedule` | `bm-kaimi-pipeline-schedule` |
| Artifact Registry repo | `kaimi` | `bm-kaimi` |
| Queue bucket | `${project}-queue` | `${project}-bm-queue` |
| Solicitations bucket | `${project}-solicitations` | `${project}-bm-solicitations` |
| Runtime service account | `kaimi-runtime` | `bm-kaimi-runtime` |
| Secret Manager ids | `samgov-api-key`, … | `bm-samgov-api-key`, … |

```hcl
# terraform.tfvars
name_prefix = "bm"
```

**Constraints (enforced by `variable "name_prefix"` validation):** the prefix must
be **lowercase / DNS-safe** — start with a letter, contain only lowercase letters,
digits, and hyphens, end with a letter or digit (no trailing hyphen), and be
**≤16 characters**. Two reasons: it flows into **GCS bucket names** (globally
unique, DNS-safe, ≤63 chars) and into the **service-account `account_id`** (max 30
chars; `<prefix>-kaimi-runtime` must fit). An empty prefix (the default) yields
byte-identical names to a greenfield deploy, so existing deployments are
unaffected.

> The buckets keep the project id as their first segment and insert the prefix as
> an infix (`${project}-bm-queue`), because bucket names are globally unique and
> already namespaced by the project. Every other resource takes the prefix as a
> leading segment.

When you set `name_prefix`, the **scheduler job name changes too**, so the
operator pause/resume scripts need the prefixed name (they default to the
un-prefixed `kaimi-pipeline-schedule`):

```sh
deploy/scripts/pause.sh  --project shared-proj --region us-east4 --job bm-kaimi-pipeline-schedule
deploy\scripts\pause.ps1 -Project shared-proj -Region us-east4 -Job bm-kaimi-pipeline-schedule
```

## Cost control / spin up & down

A deployed Kaimi is cheap to idle. The Cloud Run **Service** and **Job** both
scale to zero (`min_instance_count = 0`), so there is no always-on instance
billing — between requests/runs they cost nothing. The only recurring spend is
the **Cloud Scheduler** firing the pipeline three times a day, where each run
makes Gemini/Vertex + SAM.gov calls. There are three operating states:

### 1. ACTIVE — normal operation (default)
The scheduler fires the pipeline on `schedule_cron` (default 07:00/12:00/17:00
ET). This is the only state with recurring Gemini/SAM cost.
```sh
terraform apply -var active=true     # the default; nothing to do for a fresh deploy
```

### 2. PAUSED — near-$0 idle, resume in seconds (no data loss)
Pause the scheduler so **no pipeline runs fire** — the recurring Gemini/Vertex +
SAM.gov spend stops. The Service/Job remain scaled to zero. A paused deployment
costs only tiny GCS + Artifact Registry **storage** (cents/month). All data and
infrastructure stay in place; resume is instant.

- Declarative (recommended — Terraform stays the source of truth):
  ```sh
  terraform apply -var active=false   # pause
  terraform apply -var active=true    # resume
  ```
  This sets `paused = !var.active` on the Cloud Scheduler job.

- Operator one-liner (no Terraform run — flips the scheduler directly via gcloud):
  ```sh
  # bash / Linux / macOS
  deploy/scripts/pause.sh  --project acme-kaimi-prod --region us-east4
  deploy/scripts/resume.sh --project acme-kaimi-prod --region us-east4

  # PowerShell / Windows
  deploy\scripts\pause.ps1  -Project acme-kaimi-prod -Region us-east4
  deploy\scripts\resume.ps1 -Project acme-kaimi-prod -Region us-east4
  ```
  Project/region/job come from args or `PROJECT`/`REGION`/`JOB` env; the job
  defaults to `kaimi-pipeline-schedule`. The scripts are idempotent. If you set
  `name_prefix`, pass the prefixed job name (e.g. `--job bm-kaimi-pipeline-schedule`
  / `-Job bm-kaimi-pipeline-schedule`) — see "Deploying into a shared project".

  > If you pause with a script, the next `terraform apply` (with the default
  > `active=true`) will resume the schedule, because Terraform reconciles the
  > `paused` field. Use `-var active=false` to make the pause stick across applies.

### 3. DESTROYED — true $0, but DATA IS DELETED
`terraform destroy` removes all infrastructure and bills nothing — **but it
tries to delete the GCS buckets, which hold the historical opportunity queue and
the downloaded solicitations.** To prevent a surprise data loss, the buckets set
`force_destroy = var.force_destroy`, which **defaults to false**. Because GCP
**refuses to delete a non-empty bucket**, a `terraform destroy` **fails safely**
on any bucket that still holds data — the provider returns
`Error 409: ... Bucket you tried to delete is not empty` and the historical data
is left intact. No `prevent_destroy` / `count` gymnastics, no resource pairs.

To genuinely tear down, either empty the bucket(s) first or pass
`force_destroy=true` to explicitly accept deleting the queue/solicitations data:
```sh
# Option A — copy out anything you want, empty the buckets, then destroy as-is:
gsutil -m rm -r gs://acme-kaimi-prod-queue/** gs://acme-kaimi-prod-solicitations/**
terraform destroy

# Option B — deliberately delete the bucket contents along with the infra:
terraform destroy -var force_destroy=true   # deletes the queue/solicitations data
```
Enabled APIs are left on by default (`disable_on_destroy = false`) so a shared
project isn't broken; secret *containers* are destroyed but you may want to copy
out any versions first.

> **Why `force_destroy` instead of `prevent_destroy`?** Terraform's
> `prevent_destroy` only accepts a literal, so a variable-driven on/off toggle is
> impossible without a brittle dual-resource (`count`) pair — which itself breaks
> when flipped (it would destroy+recreate the bucket = data loss). Relying on
> GCP's native "can't delete a non-empty bucket" rule via `force_destroy=false`
> is simpler and safer: a casual `terraform destroy` can't silently nuke
> historical opportunities, but a deliberate teardown is one explicit flag (or an
> empty bucket) away.

## Teardown

See **DESTROYED** above — `terraform destroy` fails safely on a non-empty data
bucket by default (`force_destroy = false`). To truly tear down, empty the
queue/solicitations bucket(s) first, or run `terraform destroy -var force_destroy=true`
to delete the bucket contents along with the infrastructure (deliberate data loss).

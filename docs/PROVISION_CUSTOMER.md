# Provisioning a New Kaimi Customer — Operator Runbook

**Last updated:** 2026-06-29

This is the operator runbook for standing up a **new, fully-isolated Kaimi
deployment** for a BlueMeta customer with one command:
`scripts/provision-customer.sh`.

## Topology (locked decision)

- **One GCP project per customer**, BlueMeta-managed.
- Each project is **fully self-contained**: its own **image copy**, GCS buckets,
  Firestore database, and Secret Manager secrets. There is **no cross-project
  runtime dependency** — a customer project does not read from `kaimi-seeker` or
  any other customer at runtime.
- Access is **product-key gated** (`gate_mode = "product-key"`): the customer
  signs in via a time-limited magic link (`KAIMI-XXXX-XXXX-XXXX`).

The only shared resource is the **central release registry** — a BlueMeta-owned
Artifact Registry repo holding the source-of-truth images. The provision script
**copies** those images into each customer's own repo, so the running customer
project depends on nothing outside itself.

## Central release images (source of truth)

CI (`.github/workflows/ci.yml`, job `publish-release-images`) builds the api +
pipeline images **once** on every push to `main` (and on `release-*` / `v*`
tags) and pushes them to:

```
us-east4-docker.pkg.dev/kaimi-seeker/kaimi-release/api:<git-sha>      (+ :latest, + :<release-tag>)
us-east4-docker.pkg.dev/kaimi-seeker/kaimi-release/pipeline:<git-sha> (+ :latest, + :<release-tag>)
```

- Pin a customer to an **immutable `<git-sha>`** for reproducibility, or use
  `latest` to track newest `main`. Pass `--image-tag <sha-or-tag>` to the script.
- The repo lives in BlueMeta's `kaimi-seeker` project. Override with the
  `GCP_RELEASE_PROJECT` CI secret if it ever moves (defaults to `GCP_PROJECT_ID`).

## Prerequisites

1. **Tools on PATH:** `gcloud`, `terraform` (>= 1.5), `go` (mints the first key),
   and `openssl` (generates the session secret; the script warns and continues if
   absent). Docker is **not** required — images are copied server-side.
2. **gcloud auth** as a principal that can:
   - create projects under your **org or folder** (`resourcemanager.projects.create`),
   - **link billing** (`billing.resourceAssociations.create` on the billing account),
   - **read** the central release repo (`artifactregistry.reader` on `kaimi-seeker`),
   - and, after apply, **read Firestore** in the new project to mint a key
     (the script runs as you; you are Owner of the project you just created).

   Run `gcloud auth login` (and the script exports a `GOOGLE_OAUTH_ACCESS_TOKEN`
   from `gcloud auth print-access-token` for Terraform, mirroring
   `deploy/terraform/README.md`).
3. **Identifiers you need up front:** the customer id (becomes the project id,
   e.g. `kaimi-ey3`), the display name, the **billing account id**
   (`012345-6789AB-CDEF01`), and your **org id** *or* **folder id**.
4. The **central release images** must already be published (merge to `main`
   first, or push a `release-*` tag).

## The one command

```sh
scripts/provision-customer.sh \
  --customer-id   kaimi-ey3 \
  --display-name  "Ey3 Technologies" \
  --billing-account 012345-6789AB-CDEF01 \
  --org-id        123456789012 \
  --region        us-east4 \
  --tester-label  "Ey3 Technologies" \
  --key-days      60
```

Use `--folder-id` instead of `--org-id` to create the project under a folder.
Add `--yes` to skip the confirmation prompt (for automation). Other flags:
`--budget-usd` (default 50), `--firestore-location` (default = region),
`--image-tag` (default `latest`), `--release-project` / `--release-repo`.

Runs in **Git Bash on Windows** and on Linux/macOS.

### What it does (idempotent, fail-fast, never prints secrets)

| Step | Action |
|---|---|
| a | `gcloud projects create` under your org/folder (skipped if it exists) + `gcloud billing projects link`. |
| b | Enable `artifactregistry.googleapis.com` + create the per-customer `kaimi` AR repo. |
| c | `gcloud artifacts docker images copy` the central `api` + `pipeline` images into the customer repo (server-side; the project gets its **own** copy). |
| d | Render `deploy/terraform/envs/<customer-id>/` from `envs/_customer-template/` (main.tf + a `terraform.tfvars` with `gate_mode=product-key`, `create_artifact_registry=false`, the customer image refs, budget). |
| e | `terraform init` + `apply` (provisions Cloud Run Service+Job, buckets, secrets, the Firestore key registry, and the on-SAM-key-save instant-hunt IAM). |
| f | Generate a session secret with `openssl rand -base64 32` and add it as a `session-secret` version (the value is **never printed**). |
| g | Read `api_service_url` from `terraform output`, then `go run ./cmd/kaimi-key -project <proj> mint --tester "<label>" --days <n> --url <api_url>`. |
| h | Print a summary: project, API URL, the **magic link**, and a reminder to send the welcome email. |

Re-running the script for the same customer is safe: project create, billing
link, API enable, repo create, and image copy are all idempotent, and
`terraform apply` reconciles. (Step f/g add new secret/key versions on each run —
re-run only when you intend a fresh key.)

## Verify

1. **API reachable:**
   ```sh
   curl -sI "$(terraform -chdir=deploy/terraform/envs/kaimi-ey3 output -raw api_service_url)"
   ```
   Expect a **302** redirect (an unauthenticated request is sent to `/home` /
   onboarding, not 200 — the gate is doing its job).
2. **Open the magic link** the summary printed (`<api>/access?key=KAIMI-...`).
   You should land in onboarding and be prompted to enter the SAM.gov key.
3. **Firestore key registry exists:** `gcloud firestore databases list --project kaimi-ey3`
   should show the `(default)` database; `go run ./cmd/kaimi-key -project kaimi-ey3 list`
   should show the minted key as `active`.

## Send the customer their magic link

The summary prints:

```
>> SEND THE WELCOME EMAIL with this magic link:
     https://<api-host>/access?key=KAIMI-XXXX-XXXX-XXXX
```

Email that link to the customer contact. On first visit they sign in, then
complete onboarding by entering their **own SAM.gov API key** — which the runtime
SA saves as a new `samgov-api-key` version (it holds `secretVersionAdder` on just
that secret) and which fires their **first hunt immediately** (the product-key
mode wired `HUNT_PIPELINE_JOB` to this deployment's pipeline Job).

To mint additional or replacement keys later:
```sh
go run ./cmd/kaimi-key -project kaimi-ey3 mint --tester "Ey3 Technologies" --days 60 \
  --url "$(terraform -chdir=deploy/terraform/envs/kaimi-ey3 output -raw api_service_url)"
go run ./cmd/kaimi-key -project kaimi-ey3 list
go run ./cmd/kaimi-key -project kaimi-ey3 revoke KAIMI-XXXX-XXXX-XXXX
```

## Teardown

```sh
# Safe by default: terraform destroy fails on a non-empty data bucket (no silent
# data loss). Empty the buckets first, or pass --force-destroy-data.
scripts/deprovision-customer.sh --customer-id kaimi-ey3

# Delete the bucket contents along with the infra:
scripts/deprovision-customer.sh --customer-id kaimi-ey3 --force-destroy-data

# Fully remove the customer, including the GCP project (recoverable ~30 days):
scripts/deprovision-customer.sh --customer-id kaimi-ey3 --delete-project
# Fastest full teardown (skip per-resource destroy, just delete the project):
scripts/deprovision-customer.sh --customer-id kaimi-ey3 --delete-project --skip-destroy
```

## Still needs a human / not yet automated

- **Billing & org/folder permissions.** The operator running the script must
  already hold project-create + billing-link rights; the script cannot grant
  these to itself.
- **Google Drive connect (Phase 3).** The script seeds only the session secret.
  `drive-oauth-client-secret` / `drive_oauth_client_id` are **not** wired yet —
  see the `TODO(phase-3)` markers in `scripts/provision-customer.sh` and
  `deploy/terraform/modules/kaimi/main.tf`. The customer-Drive "Save to Drive"
  flow stays disabled (the connect endpoints answer 503) until that lands.
- **SAM.gov key.** Intentionally not entered by the operator — the customer
  enters their own during onboarding.
- **Sign-in OAuth (workspace mode).** Not used in the product-key model; if a
  customer ever needs Workspace sign-in instead, use `envs/example` with
  `gate_mode = "workspace-oauth"` and follow `deploy/terraform/README.md`.
```

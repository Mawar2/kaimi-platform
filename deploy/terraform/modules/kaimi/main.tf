# main.tf — the Kaimi deployment resources for one customer GCP project.
#
# This module is the declarative replacement for scripts/setup-gcp.sh: it
# provisions the same APIs, runtime identity, registry, buckets, Cloud Run Job,
# scheduler, and secret containers — and ADDS the Cloud Run SERVICE for the new
# JSON API (cmd/api). Resources are grouped with comments by concern. Everything
# is var-driven; nothing references any specific organization.
#
# Two deliberate security improvements over the procedural script:
#   1. The runtime service account has NO exported JSON key. Cloud Run uses the
#      attached SA's Application Default Credentials. (setup-gcp.sh Step 6
#      generated kaimi-sa-key.json; that step is intentionally dropped.)
#   2. The SA gets roles/secretmanager.secretAccessor (read one secret version),
#      not roles/secretmanager.admin. (setup-gcp.sh Step 5 granted admin.)

locals {
  # Derived names mirror setup-gcp.sh exactly so an existing manual deployment and
  # a Terraform one converge on the same resource names.
  queue_bucket         = "${var.project_id}-queue"          # setup-gcp.sh:209
  solicitations_bucket = "${var.project_id}-solicitations"  # setup-gcp.sh:266
  artifact_repo        = "kaimi"                            # setup-gcp.sh:212
  pipeline_job_name    = "kaimi-pipeline"                   # setup-gcp.sh:230
  api_service_name     = "kaimi-api"                        # NEW (cmd/api)
  scheduler_job_name   = "kaimi-pipeline-schedule"          # setup-gcp.sh:250
  sa_account_id        = "kaimi-runtime"                    # was kaimi-dev; runtime, ADC-only

  # GCS store path the pipeline writes to, on the mounted volume (setup-gcp.sh:234).
  store_mount_path = "/mnt/store"
  store_path       = "/mnt/store/queue"
}

# -----------------------------------------------------------------------------
# 1. Enable required Google Cloud APIs
#    Mirrors setup-gcp.sh Step 3 (lines 50-57) PLUS the service APIs the manual
#    Steps 10-11 implicitly required (run, scheduler, artifactregistry, storage).
#    drive/docs are USER-OAuth scopes, not project APIs — see README; not enabled here.
# -----------------------------------------------------------------------------
resource "google_project_service" "required" {
  for_each = toset([
    "cloudresourcemanager.googleapis.com", # setup-gcp.sh:51
    "iam.googleapis.com",                  # setup-gcp.sh:52
    "aiplatform.googleapis.com",           # setup-gcp.sh:53 (Vertex AI / Gemini)
    "secretmanager.googleapis.com",        # setup-gcp.sh:54
    "cloudbuild.googleapis.com",           # setup-gcp.sh:55
    "cloudkms.googleapis.com",             # setup-gcp.sh:56 (Secret Manager CMEK)
    "run.googleapis.com",                  # Cloud Run Job + Service (setup-gcp.sh:230,250)
    "cloudscheduler.googleapis.com",       # Cloud Scheduler (setup-gcp.sh:250)
    "artifactregistry.googleapis.com",     # Artifact Registry (setup-gcp.sh:212)
    "storage.googleapis.com",              # GCS buckets (setup-gcp.sh:219,269)
    "logging.googleapis.com",              # logWriter target (setup-gcp.sh:88)
    "monitoring.googleapis.com",           # metricWriter target (setup-gcp.sh:89)
  ])

  project = var.project_id
  service = each.value

  # Keep APIs enabled if the config is destroyed; disabling them can break other
  # workloads in a shared project. New customers can flip this if they want a
  # fully clean teardown.
  disable_on_destroy = false
}

# -----------------------------------------------------------------------------
# 2. Runtime service account
#    Mirrors setup-gcp.sh Step 4 (lines 67-79). NO JSON key is created (the
#    script's Step 6 is dropped): Cloud Run uses ADC from the attached SA.
# -----------------------------------------------------------------------------
resource "google_service_account" "runtime" {
  project      = var.project_id
  account_id   = local.sa_account_id
  display_name = "Kaimi Runtime Service Account"
  description  = "Runtime identity for Kaimi Cloud Run (pipeline Job + API service) and Scheduler. ADC only — no exported key."

  depends_on = [google_project_service.required]
}

# -----------------------------------------------------------------------------
# 3. Project-level IAM for the runtime SA
#    Mirrors setup-gcp.sh Step 5 (lines 85-90), with one least-privilege change:
#    secretmanager.secretAccessor instead of secretmanager.admin. Bucket-scoped
#    storage roles are bound on the buckets below (section 5), not project-wide.
# -----------------------------------------------------------------------------
resource "google_project_iam_member" "runtime_roles" {
  for_each = toset([
    "roles/aiplatform.user",               # setup-gcp.sh:86 (Vertex AI / Gemini)
    "roles/secretmanager.secretAccessor",  # least-privilege; was admin (setup-gcp.sh:87)
    "roles/logging.logWriter",             # setup-gcp.sh:88
    "roles/monitoring.metricWriter",       # setup-gcp.sh:89
  ])

  project = var.project_id
  role    = each.value
  member  = "serviceAccount:${google_service_account.runtime.email}"
}

# -----------------------------------------------------------------------------
# 4. Artifact Registry repository for container images
#    Mirrors setup-gcp.sh Step 10 (lines 211-215).
# -----------------------------------------------------------------------------
resource "google_artifact_registry_repository" "kaimi" {
  project       = var.project_id
  location      = var.region
  repository_id = local.artifact_repo
  format        = "DOCKER"
  description   = "Kaimi container images"
  labels        = var.labels

  depends_on = [google_project_service.required]
}

# -----------------------------------------------------------------------------
# 5. GCS buckets
#    Mirrors setup-gcp.sh Step 10 queue bucket (lines 218-225) and Step 11
#    solicitations bucket (lines 266-277). Both use uniform bucket-level access
#    and public-access prevention (the script set public-access-prevention only
#    on the solicitations bucket; we harden both — strictly safer). The runtime
#    SA gets objectAdmin scoped to each bucket (setup-gcp.sh:222,274), not the
#    project-wide grant.
# -----------------------------------------------------------------------------
resource "google_storage_bucket" "queue" {
  project                     = var.project_id
  name                        = local.queue_bucket
  location                    = var.region
  uniform_bucket_level_access = true # setup-gcp.sh:220 --uniform-bucket-level-access
  public_access_prevention    = "enforced"
  labels                      = var.labels

  depends_on = [google_project_service.required]
}

resource "google_storage_bucket" "solicitations" {
  project                     = var.project_id
  name                        = local.solicitations_bucket
  location                    = var.region
  uniform_bucket_level_access = true # setup-gcp.sh:271
  public_access_prevention    = "enforced" # setup-gcp.sh:271 --public-access-prevention
  labels                      = var.labels

  depends_on = [google_project_service.required]
}

resource "google_storage_bucket_iam_member" "queue_object_admin" {
  bucket = google_storage_bucket.queue.name
  role   = "roles/storage.objectAdmin" # setup-gcp.sh:224
  member = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_storage_bucket_iam_member" "solicitations_object_admin" {
  bucket = google_storage_bucket.solicitations.name
  role   = "roles/storage.objectAdmin" # setup-gcp.sh:276
  member = "serviceAccount:${google_service_account.runtime.email}"
}

# -----------------------------------------------------------------------------
# 6. Secret Manager secret CONTAINERS (created empty)
#    setup-gcp.sh Step 7 created only samgov-api-key (lines 138-149) and seeded a
#    placeholder version. Here we create the containers for every secret the
#    pipeline and API consume but add NO versions: the actual secret values never
#    enter Terraform state or tfvars. The customer runs `gcloud secrets versions
#    add ...` out-of-band (README.md). Secret accessor IAM is granted in
#    section 3 (project-wide secretAccessor) — sufficient and least-privilege.
# -----------------------------------------------------------------------------
resource "google_secret_manager_secret" "secrets" {
  for_each = toset([
    "samgov-api-key",            # SAM_API_KEY (pipeline + API) — setup-gcp.sh:139
    "oauth-client-secret",       # OAUTH_CLIENT_SECRET (API sign-in)
    "drive-oauth-client-secret", # DRIVE_OAUTH_CLIENT_SECRET (API customer-Drive)
    "session-secret",            # SESSION_SECRET (API session signing)
  ])

  project   = var.project_id
  secret_id = each.value
  labels    = var.labels

  replication {
    auto {} # setup-gcp.sh:146 --replication-policy=automatic
  }

  # NOTE: no google_secret_manager_secret_version here. Versions are added
  # out-of-band so secret values never live in state. See README.md.

  depends_on = [google_project_service.required]
}

# -----------------------------------------------------------------------------
# 7. Cloud Run Job — Zone-1 pipeline (cmd/pipeline)
#    Mirrors setup-gcp.sh Step 10 (lines 229-245): live mode, GCS volume mount,
#    SAM_API_KEY from Secret Manager, the runtime SA. Uses the beta provider for
#    the cloud-storage volume (mirrors --add-volume type=cloud-storage).
#    Env adds TENANT_ID/PROFILE_PATH/GEMINI_MODEL beyond the script's set, which
#    cmd/pipeline + internal/config already read.
# -----------------------------------------------------------------------------
resource "google_cloud_run_v2_job" "pipeline" {
  provider = google-beta

  project  = var.project_id
  name     = local.pipeline_job_name
  location = var.region
  labels   = var.labels

  template {
    template {
      service_account = google_service_account.runtime.email
      max_retries     = 1     # setup-gcp.sh:238 --max-retries 1
      timeout         = var.pipeline_task_timeout

      containers {
        image = var.pipeline_image

        resources {
          limits = {
            memory = var.pipeline_memory # setup-gcp.sh:238 --memory 512Mi
            cpu    = var.pipeline_cpu    # setup-gcp.sh:238 --cpu 1
          }
        }

        # Plain env mirrors setup-gcp.sh:234 (MODE/GCP_PROJECT_ID/GCP_REGION/
        # STORE_PATH) plus the model + tenant + profile vars the binary reads.
        env {
          name  = "MODE"
          value = "live"
        }
        env {
          name  = "GCP_PROJECT_ID"
          value = var.project_id
        }
        env {
          name  = "GCP_REGION"
          value = var.region
        }
        env {
          name  = "STORE_PATH"
          value = local.store_path
        }
        env {
          name  = "GEMINI_MODEL"
          value = var.gemini_model
        }
        env {
          name  = "TENANT_ID"
          value = var.tenant_id
        }
        env {
          name  = "TENANT_DISPLAY_NAME"
          value = var.tenant_display_name
        }
        env {
          name  = "ELIGIBILITY_PROFILE_PATH"
          value = var.profile_path
        }
        env {
          name  = "NAICS_CODES"
          value = var.naics_codes
        }
        env {
          name  = "GCS_SOLICITATIONS_BUCKET"
          value = google_storage_bucket.solicitations.name
        }

        # SAM_API_KEY sourced from Secret Manager, not an inline value
        # (setup-gcp.sh:235 --set-secrets SAM_API_KEY=samgov-api-key:latest).
        env {
          name = "SAM_API_KEY"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.secrets["samgov-api-key"].secret_id
              version = "latest"
            }
          }
        }

        # GCS volume mounted at /mnt/store so the JSON store persists across runs
        # (setup-gcp.sh:236-237 --add-volume/--add-volume-mount).
        volume_mounts {
          name       = "store"
          mount_path = local.store_mount_path
        }
      }

      volumes {
        name = "store"
        gcs {
          bucket    = google_storage_bucket.queue.name
          read_only = false
        }
      }
    }
  }

  depends_on = [
    google_project_iam_member.runtime_roles,
    google_storage_bucket_iam_member.queue_object_admin,
  ]
}

# -----------------------------------------------------------------------------
# 8. Cloud Run Service — JSON API (cmd/api)  [NEW vs. setup-gcp.sh]
#    A long-lived HTTP server (not in the procedural script). Cloud Run injects
#    $PORT, which the binary honors. Same runtime SA. Sign-in OAuth and CORS env
#    come from variables (non-secret); OAUTH_CLIENT_SECRET, DRIVE_OAUTH_CLIENT_
#    SECRET, SESSION_SECRET, and SAM_API_KEY come from Secret Manager. The
#    container args pass the store path + bind host (the API also reads $PORT).
# -----------------------------------------------------------------------------
resource "google_cloud_run_v2_service" "api" {
  project  = var.project_id
  name     = local.api_service_name
  location = var.region
  labels   = var.labels

  # Allow public ingress; the API enforces its own OAuth sign-in. Restrict via
  # api_allow_unauthenticated=false + a load balancer/IAP if a private surface is
  # required.
  ingress = "INGRESS_TRAFFIC_ALL"

  template {
    service_account = google_service_account.runtime.email

    # Mount the same queue bucket so the API and the pipeline Job share one JSON
    # store (the API reads the opportunity queue the pipeline writes).
    volumes {
      name = "store"
      gcs {
        bucket    = google_storage_bucket.queue.name
        read_only = false
      }
    }

    containers {
      image = var.api_image

      # The store path + bind host are flags (cmd/api/main.go); $PORT is honored
      # automatically by the binary, so it is NOT passed here.
      args = [
        "-store", local.store_path,
        "-host", "0.0.0.0",
      ]

      resources {
        limits = {
          memory = var.api_memory
          cpu    = var.api_cpu
        }
      }

      volume_mounts {
        name       = "store"
        mount_path = local.store_mount_path
      }

      # --- Tenant / GCP / models (plain env) ---
      env {
        name  = "GCP_PROJECT_ID"
        value = var.project_id
      }
      env {
        name  = "GCP_REGION"
        value = var.region
      }
      env {
        name  = "TENANT_ID"
        value = var.tenant_id
      }
      env {
        name  = "TENANT_DISPLAY_NAME"
        value = var.tenant_display_name
      }
      env {
        name  = "GEMINI_MODEL"
        value = var.gemini_model
      }
      env {
        name  = "OUTLINE_MODEL"
        value = var.outline_model
      }
      env {
        name  = "FINALREVIEW_MODEL"
        value = var.finalreview_model
      }
      env {
        name  = "ELIGIBILITY_PROFILE_PATH"
        value = var.profile_path
      }

      # --- API server / CORS ---
      env {
        name  = "API_HOST"
        value = "0.0.0.0"
      }
      env {
        name  = "CORS_ALLOWED_ORIGINS"
        value = var.cors_allowed_origins
      }
      env {
        name  = "KAIMI_INSECURE_NO_AUTH"
        value = var.api_insecure_no_auth ? "true" : "false"
      }

      # --- Sign-in OAuth (non-secret parts as plain env; secret from SM) ---
      env {
        name  = "OAUTH_CLIENT_ID"
        value = var.oauth_client_id
      }
      env {
        name  = "OAUTH_REDIRECT_URL"
        value = var.oauth_redirect_url
      }
      env {
        name  = "OAUTH_ALLOWED_DOMAIN"
        value = var.allowed_workspace_domain
      }

      # --- Customer-Drive connect (non-secret parts) ---
      env {
        name  = "DRIVE_OAUTH_CLIENT_ID"
        value = var.drive_oauth_client_id
      }
      env {
        name  = "DRIVE_OAUTH_REDIRECT_URL"
        value = var.drive_oauth_redirect_url
      }

      # --- Document AI (optional ingestion) ---
      env {
        name  = "DOCUMENTAI_PROCESSOR_ID"
        value = var.documentai_processor_id
      }
      env {
        name  = "DOCUMENTAI_LOCATION"
        value = var.documentai_location
      }

      # --- Secrets from Secret Manager (values added out-of-band) ---
      env {
        name = "SAM_API_KEY"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.secrets["samgov-api-key"].secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "OAUTH_CLIENT_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.secrets["oauth-client-secret"].secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "SESSION_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.secrets["session-secret"].secret_id
            version = "latest"
          }
        }
      }
      env {
        name = "DRIVE_OAUTH_CLIENT_SECRET"
        value_source {
          secret_key_ref {
            secret  = google_secret_manager_secret.secrets["drive-oauth-client-secret"].secret_id
            version = "latest"
          }
        }
      }
    }
  }

  depends_on = [
    google_project_iam_member.runtime_roles,
    google_storage_bucket_iam_member.queue_object_admin,
  ]
}

# Optionally expose the API publicly. The API still enforces OAuth sign-in; this
# only controls whether Cloud Run's front door accepts unauthenticated requests
# (the platform layer), which is required for browser users to reach the sign-in
# flow. Set api_allow_unauthenticated=false to keep the service private.
resource "google_cloud_run_v2_service_iam_member" "api_public" {
  count = var.api_allow_unauthenticated ? 1 : 0

  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_service.api.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}

# -----------------------------------------------------------------------------
# 9. Cloud Scheduler — trigger the pipeline Job
#    Mirrors setup-gcp.sh Step 10 (lines 242-256): the runtime SA may invoke the
#    Job (run.invoker), and an HTTP scheduler job calls the Cloud Run jobs:run
#    endpoint via OIDC as that SA.
# -----------------------------------------------------------------------------
resource "google_cloud_run_v2_job_iam_member" "scheduler_invoker" {
  project  = var.project_id
  location = var.region
  name     = google_cloud_run_v2_job.pipeline.name
  role     = "roles/run.invoker" # setup-gcp.sh:244
  member   = "serviceAccount:${google_service_account.runtime.email}"
}

resource "google_cloud_scheduler_job" "pipeline_schedule" {
  project   = var.project_id
  region    = var.region
  name      = local.scheduler_job_name
  schedule  = var.schedule_cron      # setup-gcp.sh:252 "0 7,12,17 * * *"
  time_zone = var.schedule_time_zone # setup-gcp.sh:252 America/New_York

  http_target {
    http_method = "POST" # setup-gcp.sh:254
    # The Cloud Run Admin API endpoint to execute the Job (setup-gcp.sh:253).
    uri = "https://${var.region}-run.googleapis.com/apis/run.googleapis.com/v1/namespaces/${var.project_id}/jobs/${google_cloud_run_v2_job.pipeline.name}:run"

    # OAuth token minted as the runtime SA (setup-gcp.sh:255
    # --oauth-service-account-email). The target is a first-party Google API
    # (run.googleapis.com), so an OAuth access token — not an OIDC id token — is
    # the correct credential, matching the script. The SA has run.invoker on the Job.
    oauth_token {
      service_account_email = google_service_account.runtime.email
      scope                 = "https://www.googleapis.com/auth/cloud-platform"
    }
  }

  depends_on = [
    google_project_service.required,
    google_cloud_run_v2_job_iam_member.scheduler_invoker,
  ]
}

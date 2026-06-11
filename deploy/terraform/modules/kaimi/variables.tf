# variables.tf — per-customer inputs for a Kaimi deployment.
#
# Everything that differs between customers is a variable here: there are NO
# hardcoded project IDs, organization names, or model choices in main.tf. A new
# customer fills terraform.tfvars (see envs/example) and runs apply.
#
# SECRETS ARE NOT VARIABLES. The actual values of the SAM.gov key, OAuth client
# secret, Drive OAuth client secret, and session-signing secret never appear in
# tfvars or Terraform state. This module creates the Secret Manager *containers*
# empty; the customer adds versions out-of-band with `gcloud secrets versions
# add` (documented in README.md). Only the OAuth *client id* (not secret) and the
# allowed Workspace domain — both non-sensitive — are variables.

# --- Identity & location -----------------------------------------------------

variable "project_id" {
  description = "The fresh GCP project ID to deploy Kaimi into. Must already exist with billing enabled."
  type        = string
}

variable "region" {
  description = "GCP region for all regional resources (Artifact Registry, buckets, Cloud Run, Scheduler). Matches setup-gcp.sh's default."
  type        = string
  default     = "us-east4"
}

# --- Shared-project name prefix ----------------------------------------------
#
# By default (empty) every GCP resource uses its original fixed name, so a
# greenfield deploy into a dedicated project is unchanged. Set name_prefix to
# deploy this whole stack into a SHARED project (e.g. kaimi-seeker, which already
# runs the fixed-name hackathon pipeline) WITHOUT colliding: it is prepended to
# every collision-prone resource NAME (Cloud Run Job/Service, Scheduler job,
# Artifact Registry repo, GCS buckets, runtime SA, Secret Manager ids) so the two
# stacks are fully separate and each cleanly destroyable.
#
# Charset/length: the prefix must be a DNS-safe label fragment because it flows
# into GCS bucket names (lowercase letters, digits, hyphens; no leading/trailing
# hyphen) AND into the 30-char service-account account_id ("kaimi-runtime" is 13
# chars, so the prefix + a hyphen must leave room — capped at 16 chars below).

variable "name_prefix" {
  description = "Optional name prefix so the stack can coexist in a SHARED GCP project without name collisions (e.g. \"bm\"). Empty (default) = greenfield: every resource keeps its original fixed name. When set it is prepended to every collision-prone resource name (Cloud Run Job/Service, Scheduler, Artifact Registry repo, GCS buckets, runtime SA, Secret Manager ids). Must be lowercase/DNS-safe: start with a letter, then lowercase letters/digits/hyphens, no trailing hyphen, ≤16 chars (keeps the runtime SA account_id within GCP's 30-char limit and bucket names DNS-safe)."
  type        = string
  default     = ""

  validation {
    # Empty is allowed (greenfield). Otherwise enforce a DNS-safe label fragment:
    # start with a lowercase letter, then lowercase letters/digits/hyphens, end
    # with a letter or digit (no trailing hyphen). This keeps GCS bucket names and
    # the SA account_id valid once the prefix is composed with the fixed suffixes.
    condition     = var.name_prefix == "" || can(regex("^[a-z][a-z0-9-]*[a-z0-9]$", var.name_prefix))
    error_message = "name_prefix must be empty or a DNS-safe fragment: lowercase, start with a letter, contain only lowercase letters/digits/hyphens, and not end with a hyphen."
  }

  validation {
    # account_id (runtime SA) max is 30 chars. The longest composed name is
    # "${name_prefix}-kaimi-runtime" = len(name_prefix) + 14. Cap name_prefix at 16
    # so the SA id stays ≤ 30 chars.
    condition     = length(var.name_prefix) <= 16
    error_message = "name_prefix must be ≤16 characters so the runtime service account account_id (\"${var.name_prefix}-kaimi-runtime\") stays within GCP's 30-char limit."
  }
}

# --- Tenant identity ---------------------------------------------------------

variable "tenant_id" {
  description = "Stable tenant identifier for this deployment (e.g. an org slug). Surfaced to the pipeline as TENANT_ID; namespaces this customer's data."
  type        = string
}

variable "tenant_display_name" {
  description = "Human-readable organization name for this tenant (shown in the UI). Surfaced as TENANT_DISPLAY_NAME."
  type        = string
}

# --- Container images --------------------------------------------------------
#
# The customer builds and pushes the pipeline and API images to the Artifact
# Registry repo this module creates, then passes the fully-qualified image
# references here. README.md documents the build/push flow.

variable "pipeline_image" {
  description = "Fully-qualified container image for the Zone-1 pipeline Job, e.g. us-east4-docker.pkg.dev/PROJECT/kaimi/pipeline:latest."
  type        = string
}

variable "api_image" {
  description = "Fully-qualified container image for the JSON API service (cmd/api), e.g. us-east4-docker.pkg.dev/PROJECT/kaimi/api:latest."
  type        = string
}

# --- Model selection (mirrors internal/config defaults) ----------------------

variable "gemini_model" {
  description = "Gemini model for the pipeline Scorer and the dashboard/API Writer. Maps to GEMINI_MODEL. Default matches internal/config defaultScorerModel."
  type        = string
  default     = "gemini-2.5-pro"
}

variable "outline_model" {
  description = "Gemini model for the Outline planner. Maps to OUTLINE_MODEL. Default matches internal/config defaultOutlineModel."
  type        = string
  default     = "gemini-3.5-flash"
}

variable "finalreview_model" {
  description = "Gemini model for the Final Review compliance pass. Maps to FINALREVIEW_MODEL. Default matches internal/config defaultFinalReviewModel."
  type        = string
  default     = "gemini-2.5-pro"
}

# --- Cost control: active / paused -------------------------------------------
#
# Kaimi's recurring spend is the Cloud Scheduler firing the pipeline (Gemini /
# Vertex + SAM.gov calls) on schedule_cron; the Cloud Run Job and Service both
# scale to zero between runs. Setting active=false PAUSES the scheduler so no
# pipeline runs fire — stopping the recurring Gemini/SAM cost — while leaving all
# data and infrastructure in place. Flip it back to true to resume in seconds.
# This is the cheap-to-idle, trivially-pausable knob (no data loss, no destroy).

variable "active" {
  description = "When true (default), the pipeline runs on schedule_cron. Set false to PAUSE the Cloud Scheduler job (no pipeline runs → no recurring Gemini/SAM spend); the Cloud Run Service/Job stay scaled to zero. Resume instantly with active=true. No data is lost; this is not a destroy."
  type        = bool
  default     = true
}

# --- Cost control: bucket data protection ------------------------------------
#
# `terraform destroy` tries to delete the GCS buckets, which hold the historical
# opportunity queue and downloaded solicitations. We protect that data with the
# native GCS rule rather than count/prevent_destroy gymnastics: GCP refuses to
# delete a NON-EMPTY bucket, so with force_destroy=false (default) a destroy
# fails SAFELY (409 "Bucket you tried to delete is not empty") on any bucket
# holding data. A deliberate teardown either empties the buckets first or sets
# force_destroy=true to accept the data deletion. See README "Cost control".

variable "force_destroy" {
  description = "When false (default), the queue and solicitations GCS buckets are NOT force-deleted: GCP refuses to delete a non-empty bucket, so `terraform destroy` fails safely (409 Bucket not empty) and historical opportunity data is never deleted by accident. Set true ONLY when you intend a full teardown and accept deleting the bucket contents."
  type        = bool
  default     = false
}

# --- Scheduling --------------------------------------------------------------

variable "schedule_cron" {
  description = "Cron schedule for the Zone-1 pipeline run. Default is three quota-friendly runs/day (07:00, 12:00, 17:00) matching setup-gcp.sh."
  type        = string
  default     = "0 7,12,17 * * *"
}

variable "schedule_time_zone" {
  description = "IANA time zone for the schedule. Matches setup-gcp.sh's America/New_York."
  type        = string
  default     = "America/New_York"
}

# --- Eligibility / hunting ---------------------------------------------------

variable "naics_codes" {
  description = "Optional comma-separated NAICS code overrides for the Hunter gate. Empty means use the company profile's codes. Maps to NAICS_CODES."
  type        = string
  default     = ""
}

variable "profile_path" {
  description = "Path inside the container to the company profile JSON/YAML. Maps to ELIGIBILITY_PROFILE_PATH / PROFILE_PATH. The image bakes config/profile.json at /app/config; a tenant-written profile in the store takes precedence at runtime."
  type        = string
  default     = "config/profile.json"
}

# --- API: OAuth sign-in (non-secret portion) + CORS --------------------------
#
# The OAuth *client secret* and the *session secret* are Secret Manager secrets
# (created empty here, populated out-of-band). The client id and allowed domain
# are non-sensitive and are variables so the API can validate ID tokens.

variable "oauth_client_id" {
  description = "Google OAuth client id for Workspace sign-in. Maps to OAUTH_CLIENT_ID. Non-secret. Leave empty to deploy the API with sign-in disabled (see api_insecure_no_auth)."
  type        = string
  default     = ""
}

variable "oauth_redirect_url" {
  description = "Absolute /auth/callback URL registered with Google for this API service. Maps to OAUTH_REDIRECT_URL. Set after the first apply once the API URL is known, then re-apply."
  type        = string
  default     = ""
}

variable "allowed_workspace_domain" {
  description = "Google Workspace domain ('hd' claim) permitted to sign in. Maps to OAUTH_ALLOWED_DOMAIN."
  type        = string
  default     = ""
}

variable "cors_allowed_origins" {
  description = "Comma-separated CORS allow-list for the API (full scheme+host origins; never '*'). Maps to CORS_ALLOWED_ORIGINS. Empty means same-origin only."
  type        = string
  default     = ""
}

variable "api_insecure_no_auth" {
  description = "DEV-ONLY. When true, sets KAIMI_INSECURE_NO_AUTH=true so the API starts WITHOUT sign-in auth. NEVER set true in production; leave false and configure OAuth secrets out-of-band."
  type        = bool
  default     = false
}

# --- Customer Drive connect (non-secret portion) -----------------------------

variable "drive_oauth_client_id" {
  description = "Optional Google OAuth client id for connecting the customer's own Drive. Maps to DRIVE_OAUTH_CLIENT_ID. Non-secret. Empty disables the connect endpoints (they answer 503)."
  type        = string
  default     = ""
}

variable "drive_oauth_redirect_url" {
  description = "Absolute Drive-connect callback URL. Maps to DRIVE_OAUTH_REDIRECT_URL. Required only when drive_oauth_client_id is set."
  type        = string
  default     = ""
}

# --- Document AI (optional ingestion) ----------------------------------------

variable "documentai_processor_id" {
  description = "Optional Document AI processor id for solicitation text extraction. Maps to DOCUMENTAI_PROCESSOR_ID. Empty disables live ingestion."
  type        = string
  default     = ""
}

variable "documentai_location" {
  description = "Document AI processor location. Maps to DOCUMENTAI_LOCATION. Default matches internal/config defaultDocAILocation."
  type        = string
  default     = "us"
}

# --- Cloud Run sizing --------------------------------------------------------

variable "pipeline_memory" {
  description = "Memory for the pipeline Job task. Matches setup-gcp.sh's 512Mi."
  type        = string
  default     = "512Mi"
}

variable "pipeline_cpu" {
  description = "CPU for the pipeline Job task. Matches setup-gcp.sh's 1."
  type        = string
  default     = "1"
}

variable "pipeline_task_timeout" {
  description = "Per-task timeout for the pipeline Job (seconds, as a duration string). Matches setup-gcp.sh's --task-timeout 600."
  type        = string
  default     = "600s"
}

variable "api_memory" {
  description = "Memory for the API service container."
  type        = string
  default     = "512Mi"
}

variable "api_cpu" {
  description = "CPU for the API service container."
  type        = string
  default     = "1"
}

variable "api_allow_unauthenticated" {
  description = "When true, grants roles/run.invoker to allUsers on the API service so the public internet can reach it (the API enforces its own OAuth sign-in). Set false to keep the service private and front it with IAP/a load balancer."
  type        = bool
  default     = true
}

# --- Labels ------------------------------------------------------------------

variable "labels" {
  description = "Optional resource labels applied to labelable resources (buckets, Cloud Run, etc.) for cost attribution and inventory."
  type        = map(string)
  default     = {}
}

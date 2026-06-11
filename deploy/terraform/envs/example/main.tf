# envs/example/main.tf — a self-contained root configuration a customer copies.
#
# This is the entry point a new customer runs `terraform init/plan/apply` from.
# It configures the providers and instantiates the kaimi module. Copy this whole
# envs/example directory (or point your own root at ../../modules/kaimi), fill in
# terraform.tfvars from terraform.tfvars.example, then apply.

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
    google-beta = {
      source  = "hashicorp/google-beta"
      version = "~> 6.0"
    }
  }

  # Recommended for real deployments: store state in a GCS bucket so it is shared
  # and locked, never committed. Create the bucket first, then uncomment and run
  # `terraform init -migrate-state`. Left commented so a first-time `init` works
  # with local state out of the box.
  #
  # backend "gcs" {
  #   bucket = "YOUR_PROJECT-tfstate"
  #   prefix = "kaimi"
  # }
}

# Both providers point at the same customer project/region. Authentication uses
# Application Default Credentials (run `gcloud auth application-default login` or
# set GOOGLE_APPLICATION_CREDENTIALS to a deploy-time SA key — distinct from the
# runtime SA, which has no key).
provider "google" {
  project = var.project_id
  region  = var.region
}

provider "google-beta" {
  project = var.project_id
  region  = var.region
}

module "kaimi" {
  source = "../../modules/kaimi"

  # --- Required ---
  project_id          = var.project_id
  region              = var.region
  tenant_id           = var.tenant_id
  tenant_display_name = var.tenant_display_name
  pipeline_image      = var.pipeline_image
  api_image           = var.api_image

  # --- Models (defaults match internal/config; override if desired) ---
  gemini_model      = var.gemini_model
  outline_model     = var.outline_model
  finalreview_model = var.finalreview_model

  # --- Sign-in OAuth (non-secret parts; secrets seeded out-of-band) ---
  oauth_client_id          = var.oauth_client_id
  oauth_redirect_url       = var.oauth_redirect_url
  allowed_workspace_domain = var.allowed_workspace_domain
  cors_allowed_origins     = var.cors_allowed_origins
  api_insecure_no_auth     = var.api_insecure_no_auth

  # --- Optional: customer Drive connect + Document AI ingestion ---
  drive_oauth_client_id    = var.drive_oauth_client_id
  drive_oauth_redirect_url = var.drive_oauth_redirect_url
  documentai_processor_id  = var.documentai_processor_id

  # --- Optional knobs ---
  naics_codes   = var.naics_codes
  schedule_cron = var.schedule_cron
  labels        = var.labels

  # --- Cost control (see README "Cost control / spin up & down") ---
  active          = var.active          # false PAUSES the scheduler (near-$0 idle)
  protect_buckets = var.protect_buckets # true guards data from terraform destroy
}

# Variable declarations for this root module. They simply forward to the kaimi
# module; defaults live in the module's variables.tf, so only the genuinely
# required ones (and any the customer wants to override) need values in tfvars.
variable "project_id" { type = string }
variable "region" {
  type    = string
  default = "us-east4"
}
variable "tenant_id" { type = string }
variable "tenant_display_name" { type = string }
variable "pipeline_image" { type = string }
variable "api_image" { type = string }

variable "gemini_model" {
  type    = string
  default = "gemini-2.5-pro"
}
variable "outline_model" {
  type    = string
  default = "gemini-3.5-flash"
}
variable "finalreview_model" {
  type    = string
  default = "gemini-2.5-pro"
}

variable "oauth_client_id" {
  type    = string
  default = ""
}
variable "oauth_redirect_url" {
  type    = string
  default = ""
}
variable "allowed_workspace_domain" {
  type    = string
  default = ""
}
variable "cors_allowed_origins" {
  type    = string
  default = ""
}
variable "api_insecure_no_auth" {
  type    = bool
  default = false
}

variable "drive_oauth_client_id" {
  type    = string
  default = ""
}
variable "drive_oauth_redirect_url" {
  type    = string
  default = ""
}
variable "documentai_processor_id" {
  type    = string
  default = ""
}

variable "naics_codes" {
  type    = string
  default = ""
}
variable "schedule_cron" {
  type    = string
  default = "0 7,12,17 * * *"
}
variable "labels" {
  type    = map(string)
  default = {}
}

variable "active" {
  type    = bool
  default = true
}
variable "protect_buckets" {
  type    = bool
  default = true
}

# Re-export the module outputs at the root so `terraform output` surfaces them.
output "api_service_url" { value = module.kaimi.api_service_url }
output "dashboard_url" { value = module.kaimi.dashboard_url }
output "pipeline_job_name" { value = module.kaimi.pipeline_job_name }
output "scheduler_job_name" { value = module.kaimi.scheduler_job_name }
output "service_account_email" { value = module.kaimi.service_account_email }
output "artifact_registry_repository" { value = module.kaimi.artifact_registry_repository }
output "queue_bucket" { value = module.kaimi.queue_bucket }
output "solicitations_bucket" { value = module.kaimi.solicitations_bucket }
output "secret_ids" { value = module.kaimi.secret_ids }

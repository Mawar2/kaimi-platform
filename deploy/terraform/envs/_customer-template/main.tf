# envs/_customer-template/main.tf — root config for a product-key customer.
#
# This is the template scripts/provision-customer.sh copies into
# deploy/terraform/envs/<customer-id>/ and applies. It is identical in spirit to
# envs/example/main.tf but pre-wired for the "one project per customer,
# product-key gated, images pre-copied" provisioning model:
#
#   - gate_mode defaults to "product-key" (magic-link sign-ups), so a single
#     apply also stands up the Firestore key registry + the on-SAM-key-save
#     instant-hunt IAM (see modules/kaimi/main.tf section 10).
#   - create_artifact_registry defaults to false: the provision script pre-creates
#     the customer's Artifact Registry repo and COPIES the central release images
#     in BEFORE apply, so the Cloud Run Job/Service reference images that already
#     exist on the first apply (full per-project image isolation).
#
# Everything customer-specific is still a variable; the script writes a
# terraform.tfvars next to this file (from terraform.tfvars.template) and runs
# `terraform init && terraform apply`.

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
  # with local state out of the box (the provision script uses local state).
  #
  # backend "gcs" {
  #   bucket = "YOUR_PROJECT-tfstate"
  #   prefix = "kaimi"
  # }
}

# Both providers point at the customer's project/region. Authentication uses
# Application Default Credentials (the provision script exports
# GOOGLE_OAUTH_ACCESS_TOKEN from `gcloud auth print-access-token`, mirroring
# deploy/terraform/README.md).
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

  # --- Required identity & images (images live in THIS project's AR repo) ---
  project_id          = var.project_id
  region              = var.region
  tenant_id           = var.tenant_id
  tenant_display_name = var.tenant_display_name
  pipeline_image      = var.pipeline_image
  api_image           = var.api_image

  # --- Provisioning model: product-key gate + pre-created AR repo -----------
  # The script copies the central release images into the customer's repo before
  # apply, so the module must NOT create the repo (it would race / collide).
  gate_mode                = var.gate_mode
  firestore_location       = var.firestore_location
  create_artifact_registry = var.create_artifact_registry

  # --- Optional monthly budget alert (skipped when billing_account is "") ---
  billing_account   = var.billing_account
  budget_amount_usd = var.budget_amount_usd

  # --- Optional knobs (sensible module defaults otherwise) -----------------
  gemini_model      = var.gemini_model
  outline_model     = var.outline_model
  finalreview_model = var.finalreview_model
  naics_codes       = var.naics_codes
  schedule_cron     = var.schedule_cron
  labels            = var.labels

  # --- Cost control --------------------------------------------------------
  active        = var.active
  force_destroy = var.force_destroy
}

# Variable declarations forward to the module; defaults live in the module's
# variables.tf, so only the genuinely required ones need values in tfvars.
variable "project_id" { type = string }
variable "region" {
  type    = string
  default = "us-east4"
}
variable "tenant_id" { type = string }
variable "tenant_display_name" { type = string }
variable "pipeline_image" { type = string }
variable "api_image" { type = string }

variable "gate_mode" {
  type    = string
  default = "product-key"
}
variable "firestore_location" {
  type    = string
  default = ""
}
variable "create_artifact_registry" {
  type    = bool
  default = false
}

variable "billing_account" {
  type    = string
  default = ""
}
variable "budget_amount_usd" {
  type    = number
  default = 50
}

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
variable "force_destroy" {
  type    = bool
  default = false
}

# Re-export the module outputs so `terraform output` surfaces them to the script.
output "api_service_url" { value = module.kaimi.api_service_url }
output "dashboard_url" { value = module.kaimi.dashboard_url }
output "pipeline_job_name" { value = module.kaimi.pipeline_job_name }
output "scheduler_job_name" { value = module.kaimi.scheduler_job_name }
output "service_account_email" { value = module.kaimi.service_account_email }
output "artifact_registry_repository" { value = module.kaimi.artifact_registry_repository }
output "queue_bucket" { value = module.kaimi.queue_bucket }
output "solicitations_bucket" { value = module.kaimi.solicitations_bucket }
output "secret_ids" { value = module.kaimi.secret_ids }
output "gate_mode" { value = module.kaimi.gate_mode }
output "product_key_next_step" { value = module.kaimi.product_key_next_step }

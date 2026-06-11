# versions.tf — Terraform and provider version constraints for the Kaimi module.
#
# Pinning the provider major version keeps `terraform apply` reproducible across
# customer environments: a new customer running `terraform init` next month gets
# the same google provider behavior we authored against. Bump these deliberately
# (and re-test) rather than letting `init` float to a new major.

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
    # google-beta is required for the Cloud Run GCS volume mount on the pipeline
    # Job. The mount (type=cloud-storage) mirrors setup-gcp.sh's --add-volume and
    # is still surfaced through the beta provider for google_cloud_run_v2_job.
    google-beta = {
      source  = "hashicorp/google-beta"
      version = "~> 6.0"
    }
  }
}

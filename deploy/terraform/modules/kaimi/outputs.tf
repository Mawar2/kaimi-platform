# outputs.tf — values a customer needs after `terraform apply`.
#
# These surface the deployed URLs and resource names for the post-apply steps in
# README.md: register the OAuth redirect URL, seed secrets, push images, and open
# the dashboard/API.

output "api_service_url" {
  description = "Public URL of the JSON API Cloud Run service (cmd/api). Use this to derive OAUTH_REDIRECT_URL and to point a front end at the API."
  value       = google_cloud_run_v2_service.api.uri
}

output "dashboard_url" {
  description = "Dashboard URL. The current module deploys the API service (which fronts the same store/proposal surfaces); this aliases api_service_url until a separate dashboard service is added."
  value       = google_cloud_run_v2_service.api.uri
}

output "pipeline_job_name" {
  description = "Name of the Zone-1 pipeline Cloud Run Job, for manual runs (gcloud run jobs execute) and log lookups."
  value       = google_cloud_run_v2_job.pipeline.name
}

output "scheduler_job_name" {
  description = "Name of the Cloud Scheduler job that triggers the pipeline on schedule_cron."
  value       = google_cloud_scheduler_job.pipeline_schedule.name
}

output "service_account_email" {
  description = "Email of the runtime service account attached to the pipeline Job and API service (ADC; no exported key)."
  value       = google_service_account.runtime.email
}

output "artifact_registry_repository" {
  description = "Fully-qualified Artifact Registry repo path to push pipeline/api images to (REGION-docker.pkg.dev/PROJECT/kaimi)."
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.kaimi.repository_id}"
}

output "queue_bucket" {
  description = "Name of the GCS bucket holding the JSON opportunity/proposal store (mounted by the pipeline Job and API service)."
  value       = google_storage_bucket.queue.name
}

output "solicitations_bucket" {
  description = "Name of the GCS bucket holding raw + extracted solicitation documents (Zone-2 ingestion)."
  value       = google_storage_bucket.solicitations.name
}

output "secret_ids" {
  description = "Secret Manager secret IDs created empty by this module. Add a version to each out-of-band before the corresponding feature works (see README.md)."
  value       = [for s in google_secret_manager_secret.secrets : s.secret_id]
}

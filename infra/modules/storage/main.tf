variable "project_id" { type = string }
variable "region" { type = string }
variable "env" { type = string }

resource "google_storage_bucket" "artifacts" {
  name     = "${var.project_id}-vox-artifacts-${var.env}"
  project  = var.project_id
  location = var.region

  uniform_bucket_level_access = true
  force_destroy               = true # Dev only — remove for prod

  versioning {
    enabled = false
  }

  lifecycle_rule {
    condition {
      age = 90
    }
    action {
      type = "Delete"
    }
  }
}

output "artifacts_bucket" {
  value = google_storage_bucket.artifacts.name
}

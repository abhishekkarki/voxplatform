output "cluster_name" {
  description = "GKE cluster name"
  value       = module.gke.cluster_name
}

output "cluster_endpoint" {
  description = "GKE cluster endpoint"
  value       = module.gke.cluster_endpoint
  sensitive   = true
}

output "registry_url" {
  description = "Artifact Registry URL for docker push"
  value       = module.registry.registry_url
}

output "artifacts_bucket" {
  description = "GCS bucket for eval datasets and event logs"
  value       = module.storage.artifacts_bucket
}

output "kubeconfig_command" {
  description = "Run this to configure kubectl"
  value       = "gcloud container clusters get-credentials ${module.gke.cluster_name} --zone ${var.zone} --project ${var.project_id}"
}

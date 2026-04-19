variable "project_id" { type = string }
variable "region" { type = string }
variable "zone" { type = string }
variable "env" { type = string }
variable "network_id" { type = string }
variable "subnet_id" { type = string }
variable "pods_range_name" { type = string }
variable "services_range_name" { type = string }
variable "cpu_machine_type" { type = string }
variable "cpu_min_nodes" { type = number }
variable "cpu_max_nodes" { type = number }
variable "cpu_disk_size_gb" { type = number }

# -------------------------------------------------------
# GKE Standard cluster (zonal = free management fee)
# -------------------------------------------------------
resource "google_container_cluster" "primary" {
  name     = "vox-cluster-${var.env}"
  project  = var.project_id
  location = var.zone # Zonal cluster — no management fee

  # We manage node pools separately
  remove_default_node_pool = true
  initial_node_count       = 1

  network    = var.network_id
  subnetwork = var.subnet_id

  ip_allocation_policy {
    cluster_secondary_range_name  = var.pods_range_name
    services_secondary_range_name = var.services_range_name
  }

  # Workload Identity — best practice for pod-to-GCP auth
  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }

  # Release channel — regular gives stable + recent features
  release_channel {
    channel = "REGULAR"
  }

  # Logging and monitoring via Google Cloud
  logging_config {
    enable_components = ["SYSTEM_COMPONENTS", "WORKLOADS"]
  }

  monitoring_config {
    enable_components = ["SYSTEM_COMPONENTS"]
    managed_prometheus {
      enabled = true
    }
  }

  # Network policy for future multi-tenancy
  network_policy {
    enabled = true
  }

  # Deletion protection — disable for dev, enable for prod
  deletion_protection = false
}

# -------------------------------------------------------
# CPU node pool — the workhorse for iteration 0-4
# -------------------------------------------------------
resource "google_container_node_pool" "cpu_pool" {
  name     = "cpu-pool"
  project  = var.project_id
  location = var.zone
  cluster  = google_container_cluster.primary.name

  autoscaling {
    min_node_count = var.cpu_min_nodes
    max_node_count = var.cpu_max_nodes
  }

  node_config {
    machine_type = var.cpu_machine_type
    disk_size_gb = var.cpu_disk_size_gb
    disk_type    = "pd-standard"

    # Use spot VMs for dev — 60-90% cheaper
    spot = true

    oauth_scopes = [
      "https://www.googleapis.com/auth/cloud-platform",
    ]

    # Workload Identity
    workload_metadata_config {
      mode = "GKE_METADATA"
    }

    labels = {
      env          = var.env
      node-purpose = "cpu-inference"
    }

    # Taint so only inference workloads land here (optional)
    # Uncomment when you have system workloads on a separate pool
    # taint {
    #   key    = "workload"
    #   value  = "inference"
    #   effect = "NO_SCHEDULE"
    # }
  }

  management {
    auto_repair  = true
    auto_upgrade = true
  }
}

# -------------------------------------------------------
# GPU node pool — added in iteration 5
# Uncomment when ready for GPU workloads
# -------------------------------------------------------
# resource "google_container_node_pool" "gpu_pool" {
#   name     = "gpu-pool"
#   project  = var.project_id
#   location = var.zone
#   cluster  = google_container_cluster.primary.name
#
#   autoscaling {
#     min_node_count = 0  # Scale to zero when idle
#     max_node_count = 1
#   }
#
#   node_config {
#     machine_type = "n1-standard-4"
#     disk_size_gb = 100
#     disk_type    = "pd-ssd"
#     spot         = true
#
#     guest_accelerator {
#       type  = "nvidia-tesla-t4"
#       count = 1
#       gpu_driver_installation_config {
#         gpu_driver_version = "LATEST"
#       }
#     }
#
#     oauth_scopes = [
#       "https://www.googleapis.com/auth/cloud-platform",
#     ]
#
#     workload_metadata_config {
#       mode = "GKE_METADATA"
#     }
#
#     labels = {
#       env          = var.env
#       node-purpose = "gpu-inference"
#     }
#
#     taint {
#       key    = "nvidia.com/gpu"
#       value  = "present"
#       effect = "NO_SCHEDULE"
#     }
#   }
#
#   management {
#     auto_repair  = true
#     auto_upgrade = true
#   }
# }

output "cluster_name" {
  value = google_container_cluster.primary.name
}

output "cluster_endpoint" {
  value     = google_container_cluster.primary.endpoint
  sensitive = true
}

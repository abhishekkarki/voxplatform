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

# GPU node pool — iteration 5, off by default (see gpu_enabled)
variable "gpu_enabled" { type = bool }
variable "gpu_machine_type" { type = string }
variable "gpu_accelerator_type" { type = string }
variable "gpu_min_nodes" { type = number }
variable "gpu_max_nodes" { type = number }
variable "gpu_disk_size_gb" { type = number }

# Zones within the cluster's region that actually have the requested
# accelerator. europe-west3 (the cluster's region) only has T4s in -b —
# not -a (where the cluster's control plane/CPU pool live) or -c. See
# ADR-010. Kept as its own var (not derived from var.zone) so other
# environments/regions can override it independently of the cluster zone.
variable "gpu_node_locations" { type = list(string) }

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
# GPU node pool — iteration 5. Off by default: fresh GCP projects have 0
# T4 quota until requested, and idle GPU nodes cost real money even at
# spot pricing. Set gpu_enabled = true (and request quota) to turn this on.
# See ADR-010 for the zone/driver/scheduling reasoning.
# -------------------------------------------------------
resource "google_container_node_pool" "gpu_pool" {
  count    = var.gpu_enabled ? 1 : 0
  name     = "gpu-pool"
  project  = var.project_id
  location = var.zone # cluster's control-plane zone — node_locations below is what actually places the VMs
  cluster  = google_container_cluster.primary.name

  # Places this pool's nodes in var.gpu_node_locations instead of var.zone.
  # A zonal cluster's default location doesn't have to be where every node
  # pool's VMs land, as long as the zones are in the same region.
  node_locations = var.gpu_node_locations

  autoscaling {
    min_node_count = var.gpu_min_nodes # 0 = scale to zero when idle
    max_node_count = var.gpu_max_nodes
  }

  node_config {
    machine_type = var.gpu_machine_type # T4 only attaches to the N1 family
    disk_size_gb = var.gpu_disk_size_gb
    disk_type    = "pd-ssd"
    spot         = true

    guest_accelerator {
      type  = var.gpu_accelerator_type
      count = 1
      gpu_driver_installation_config {
        gpu_driver_version = "LATEST" # GKE-managed driver install — no NVIDIA GPU Operator needed, see ADR-010
      }
    }

    oauth_scopes = [
      "https://www.googleapis.com/auth/cloud-platform",
    ]

    workload_metadata_config {
      mode = "GKE_METADATA"
    }

    labels = {
      env          = var.env
      node-purpose = "gpu-inference"
    }

    # GKE's ExtendedResourceToleration admission controller auto-tolerates
    # this taint for any pod requesting the matching nvidia.com/gpu
    # resource (which VoiceModel's device: gpu already sets) — no
    # nodeSelector/toleration needed on those pods. DaemonSets that don't
    # request nvidia.com/gpu (e.g. the DCGM exporter) need an explicit
    # toleration. See ADR-010.
    taint {
      key    = "nvidia.com/gpu"
      value  = "present"
      effect = "NO_SCHEDULE"
    }
  }

  management {
    auto_repair  = true
    auto_upgrade = true
  }
}

output "cluster_name" {
  value = google_container_cluster.primary.name
}

output "cluster_endpoint" {
  value     = google_container_cluster.primary.endpoint
  sensitive = true
}

output "gpu_pool_name" {
  value = var.gpu_enabled ? google_container_node_pool.gpu_pool[0].name : ""
}

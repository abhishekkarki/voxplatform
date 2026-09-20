variable "project_id" {
  description = "GCP project ID"
  type        = string
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "europe-west3"
}

variable "zone" {
  description = "GCP zone for zonal GKE cluster (free tier)"
  type        = string
  default     = "europe-west3-a"
}

variable "env" {
  description = "Environment name"
  type        = string
  default     = "dev"
}

variable "cpu_machine_type" {
  description = "Machine type for CPU node pool"
  type        = string
  default     = "e2-standard-4"
}

variable "cpu_min_nodes" {
  description = "Minimum nodes in CPU pool"
  type        = number
  default     = 1
}

variable "cpu_max_nodes" {
  description = "Maximum nodes in CPU pool"
  type        = number
  default     = 3
}

variable "cpu_disk_size_gb" {
  description = "Boot disk size for CPU nodes"
  type        = number
  default     = 50
}

variable "gpu_enabled" {
  description = "Create the GPU node pool. Off by default — fresh GCP projects have 0 T4 quota until requested, and idle GPU nodes cost money even at spot pricing. See docs/how-to/scale-cluster.md."
  type        = bool
  default     = false
}

variable "gpu_machine_type" {
  description = "Machine type for the GPU node pool. T4 only attaches to the N1 family."
  type        = string
  default     = "n1-standard-4"
}

variable "gpu_accelerator_type" {
  description = "GPU accelerator type"
  type        = string
  default     = "nvidia-tesla-t4"
}

variable "gpu_min_nodes" {
  description = "Minimum nodes in GPU pool (0 = scale to zero when idle)"
  type        = number
  default     = 0
}

variable "gpu_max_nodes" {
  description = "Maximum nodes in GPU pool"
  type        = number
  default     = 1
}

variable "gpu_disk_size_gb" {
  description = "Boot disk size for GPU nodes"
  type        = number
  default     = 100
}

variable "gpu_node_locations" {
  description = "Zones (within region) that actually have the requested accelerator. europe-west3 only has T4s in -b, not -a (the cluster's zone) or -c — see ADR-010."
  type        = list(string)
  default     = ["europe-west3-b"]
}

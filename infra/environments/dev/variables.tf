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

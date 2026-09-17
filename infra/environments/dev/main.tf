provider "google" {
  project = var.project_id
  region  = var.region
}

# --- Networking ---
module "network" {
  source = "../../modules/network"

  project_id = var.project_id
  region     = var.region
  env        = var.env
}

# --- GKE Cluster ---
module "gke" {
  source = "../../modules/gke"

  project_id          = var.project_id
  region              = var.region
  zone                = var.zone
  env                 = var.env
  network_id          = module.network.network_id
  subnet_id           = module.network.subnet_id
  pods_range_name     = module.network.pods_range_name
  services_range_name = module.network.services_range_name

  # CPU node pool config
  cpu_machine_type = var.cpu_machine_type
  cpu_min_nodes    = var.cpu_min_nodes
  cpu_max_nodes    = var.cpu_max_nodes
  cpu_disk_size_gb = var.cpu_disk_size_gb

  # GPU node pool config — iteration 5, off by default
  gpu_enabled          = var.gpu_enabled
  gpu_machine_type     = var.gpu_machine_type
  gpu_accelerator_type = var.gpu_accelerator_type
  gpu_min_nodes        = var.gpu_min_nodes
  gpu_max_nodes        = var.gpu_max_nodes
  gpu_disk_size_gb     = var.gpu_disk_size_gb
  gpu_node_locations   = var.gpu_node_locations
}

# --- Artifact Registry ---
module "registry" {
  source = "../../modules/registry"

  project_id = var.project_id
  region     = var.region
  env        = var.env
}

# --- Cloud Storage ---
module "storage" {
  source = "../../modules/storage"

  project_id = var.project_id
  region     = var.region
  env        = var.env
}

terraform {
  backend "gcs" {
    # Dedicated state bucket — created manually once, never managed by Terraform.
    # Lives permanently regardless of terraform destroy.
    bucket = "voxplatform-tfstate"
    prefix = "dev"
  }
}

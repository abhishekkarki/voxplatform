# Start with local backend. Move to GCS when you have a second environment.
#
# To migrate later:
# 1. Create a GCS bucket: gsutil mb gs://voxplatform-tfstate
# 2. Uncomment the block below
# 3. Run: terraform init -migrate-state
#
# terraform {
#   backend "gcs" {
#     bucket = "voxplatform-tfstate"
#     prefix = "dev"
#   }
# }

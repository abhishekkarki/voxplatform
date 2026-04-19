# ADR-003: Why GCP and GKE

## Status
Accepted

## Context
We need a Kubernetes cluster to run voxplatform. Options considered:
- Self-hosted on Hetzner (existing homelab infrastructure)
- GCP GKE
- AWS EKS
- Local minikube/kind for development only

## Decision
Use GCP GKE Standard (zonal) as the primary cluster.

## Rationale
- **Free tier:** One zonal GKE cluster has zero management fee. Combined
  with $300 new-account credits, this covers ~3 months of development.
- **Career alignment:** Isomorphic Labs (primary target) is Alphabet.
  Baseten runs on GCP. GKE experience is directly transferable.
- **GPU availability:** T4 spot instances in europe-west3 are ~$0.11/hr.
  When we add GPU in iteration 5, the path is a Terraform module change.
- **Managed Prometheus:** GKE has native managed Prometheus, reducing
  operational overhead for observability.
- **Artifact Registry:** Integrated container registry eliminates the need
  to self-host Harbor.
- **Workload Identity:** Clean pod-to-GCP auth without managing service
  account keys.

## Alternatives rejected
- **Hetzner:** Cheaper for long-term, but no managed K8s. RKE2 setup and
  maintenance would consume 1-2 weeks of the project timeline. GPU
  availability is limited to dedicated servers with long provisioning.
- **AWS EKS:** Higher management fee ($0.10/hr per cluster = $73/month).
  Less relevant to target employer stack.
- **Local only:** No realistic networking, no cloud-native patterns, not
  demoable to others.

## Consequences
- Monthly cost ~$100 (covered by credits for 3 months)
- After credits expire, decide: continue on GCP, migrate to Hetzner,
  or pause the cluster
- Terraform modules are GCP-specific but the Helm charts and operator
  are cloud-agnostic

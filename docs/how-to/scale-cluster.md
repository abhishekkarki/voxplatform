# Scale the cluster up and down

Vox is built around a scale-to-zero workflow to keep development costs low.

## Down (end of day)

```bash
make down       # scales node pool to 0, keeps control plane
```

This leaves the GKE control plane (free on autopilot, near-free on standard) and your Terraform state intact. Pods are gone, images stay in Artifact Registry, GCS data persists.

## Up (start of day)

```bash
make up         # scales node pool back to 2
kubectl wait --for=condition=ready node --all --timeout=180s
```

ArgoCD reconciles the platform back to the desired state automatically.

## GPU pool

The GPU node pool (T4 spot, iteration 5 — see [ADR-010](../adr/010-gpu-support.md))
is **off by default** (`gpu_enabled = false`). It's a separate pool from the
CPU one — turning it on/off doesn't touch the CPU pool or the whisper/gateway
workloads already running.

**Before turning it on:**

1. Request a T4 quota increase if this is a fresh project — GCP projects
   start with 0 GPU quota. IAM & Admin → Quotas → filter `NVIDIA T4 GPUs`,
   region `europe-west3`.
2. Check current spot pricing at
   [cloud.google.com/compute/gpus-pricing](https://cloud.google.com/compute/gpus-pricing)
   — it changes up to once a month.

**Turn on:**

```bash
cd infra/environments/dev
terraform apply -var gpu_enabled=true
```

This creates a 0–1 node autoscaling pool in `europe-west3-b` (not `-a`, where
the rest of the cluster lives — T4 isn't available there, see ADR-010).
Deploy the GPU workloads once the pool exists:

```bash
kubectl apply -f operator/config/samples/voicemodels/whisper-large-v3-gpu.yaml
helm upgrade --install summarizer deploy/helm/summarizer -n vox \
  -f deploy/helm/summarizer/values-gpu.yaml \
  --set image.repository=$REGISTRY/summarizer

# GPU metrics — Prometheus picks this up automatically (see ADR-010)
helm install gpu-metrics deploy/helm/gpu-metrics -n vox
# Then import NVIDIA's DCGM dashboard (ID 12239) into Grafana.
```

**Resize without a full `terraform apply`** (e.g. scale to 0 overnight
without tearing down the pool definition):

```bash
gcloud container clusters resize vox-cluster-dev \
  --node-pool gpu-pool --num-nodes 0 \
  --zone europe-west3-a --project $GCP_PROJECT
```

**Turn off entirely:**

```bash
terraform apply -var gpu_enabled=false   # destroys the GPU pool, CPU pool untouched
```

## Full destroy

```bash
cd infra && terraform destroy -var="project_id=$GCP_PROJECT"
```

Only do this if you want to release the static IP and start clean.

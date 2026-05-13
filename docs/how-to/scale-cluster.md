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

## Full destroy

```bash
cd infra && terraform destroy -var="project_id=$GCP_PROJECT"
```

Only do this if you want to release the static IP and start clean.

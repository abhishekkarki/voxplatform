# VoiceModel CRD Reference

**API Group:** `vox.vox.io/v1alpha1`
**Kind:** `VoiceModel`
**Scope:** Namespaced

## Overview

`VoiceModel` declares a single model server. The operator reconciles a
Deployment and a ClusterIP Service for it, tracks readiness as `status.phase`,
and cleans both up on deletion via a finalizer. It's the building block
`InferencePipeline` stages and `EvalRun`'s `gatewayURL` target point at.

## Spec

```yaml
apiVersion: vox.vox.io/v1alpha1
kind: VoiceModel
metadata:
  name: whisper-small
  namespace: vox
spec:
  model: Systran/faster-whisper-small.en
  device: cpu           # optional, default: "cpu"
  quantization: int8     # optional, default: "int8"
  replicas: 1             # optional, default: 1
  port: 8000               # optional, default: 8000
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model` | string | yes | Model identifier, e.g. `Systran/faster-whisper-small.en`. Passed to the image as `WHISPER__MODEL` unless `image` overrides the default faster-whisper-server image. |
| `replicas` | int32 | no | Number of inference pods. Default: `1`. |
| `device` | string | no | `cpu` or `gpu`. Default: `"cpu"`. See [GPU models](#gpu-models-device-gpu) below. |
| `quantization` | string | no | `int8`, `float16`, or `float32`. Default: `"int8"`. GPU models typically use `float16`. |
| `image` | string | no | Overrides the default inference server image. Required for non-faster-whisper models (diarizer, summarizer) — see `operator/config/samples/voicemodels/qwen-summarizer.yaml`. |
| `resources` | `corev1.ResourceRequirements` | no | CPU/memory (and, for GPU, `nvidia.com/gpu`) requests/limits. Defaults come from `defaultResources()` in `voicemodel_controller.go` when omitted — see below. |
| `port` | int32 | no | Container/Service port. Default: `8000`. |
| `health.path` | string | no | Readiness/liveness probe path. Default: `/health`. |
| `health.initialDelaySeconds` | int32 | no | Default: `60`. |
| `health.periodSeconds` | int32 | no | Default: `10`. |
| `metrics` | bool | no | Whether to annotate the pod for Prometheus scraping. Default: `true`. |

## Status

```yaml
status:
  phase: Ready
  readyReplicas: 1
  endpoint: "vox-whisper-small.vox.svc.cluster.local:8000"
  message: "1/1 replicas ready"
  lastTransitionTime: "2026-05-14T10:00:00Z"
```

| Field | Description |
|-------|-------------|
| `phase` | `Pending` → `Deploying` → `Ready` (or `Failed`). `Terminating` during deletion. |
| `readyReplicas` | Copied from the Deployment's `status.readyReplicas`. |
| `endpoint` | Internal Service DNS address — what `InferencePipeline` stage status and gateway env vars point at. |
| `message` | Human-readable status explanation. |
| `conditions` | Standard Kubernetes conditions list. |

## Default resources and images

When `spec.resources` / `spec.image` are omitted, the operator picks defaults
based on `spec.device` (`defaultResources()` / the `defaultCPUImage`/
`defaultGPUImage` constants in `voicemodel_controller.go`):

| `device` | Default image | Default resources |
|----------|---------------|--------------------|
| `cpu` (default) | `fedirz/faster-whisper-server:0.3.0-cpu` | 500m–1500m CPU, 1–3Gi memory |
| `gpu` | `fedirz/faster-whisper-server:0.3.0-cuda` | 1–4 CPU, 4–8Gi memory, **`nvidia.com/gpu: "1"`** (both request and limit) |

## GPU models (`device: gpu`)

```yaml
apiVersion: vox.vox.io/v1alpha1
kind: VoiceModel
metadata:
  name: whisper-large-v3-gpu
  namespace: vox
spec:
  model: Systran/faster-whisper-large-v3
  device: gpu
  quantization: float16
```

Requires the GPU node pool (`gpu_enabled = true` in Terraform — see
[Scale the cluster](../how-to/scale-cluster.md) and
[ADR-010](../adr/010-gpu-support.md)).

**No `nodeSelector` or `toleration` fields exist on `VoiceModelSpec`, and none
are needed.** `defaultResources("gpu")`'s `nvidia.com/gpu: "1"` request is
sufficient on its own:

- The Kubernetes scheduler only places pods requesting `nvidia.com/gpu` on
  nodes that advertise that allocatable resource — i.e. only the GPU pool.
- The GPU pool's `nvidia.com/gpu=present:NoSchedule` taint (set in
  `infra/modules/gke/main.tf`) is handled by GKE's **ExtendedResourceToleration**
  admission controller, enabled by default on GKE Standard clusters: any pod
  requesting the matching extended resource gets an automatic toleration for
  it. A CPU `VoiceModel` never requests `nvidia.com/gpu`, so it's never
  eligible for (or tolerant of) the GPU pool either way.

If that admission controller is ever disabled, GPU `VoiceModel`s would need
an explicit toleration added via a future `spec.tolerations` field — not
needed today.

`Qwen/Qwen2.5-7B-Instruct` (served by vLLM, not faster-whisper) is the other
GPU workload this iteration adds — see
`operator/config/samples/voicemodels/qwen-summarizer-gpu.yaml`, which sets
`spec.image`/`spec.resources` explicitly rather than relying on the
faster-whisper-shaped defaults above (the same pattern the CPU
`qwen-summarizer.yaml` sample already uses).

## kubectl quick reference

```bash
kubectl get voicemodels -n vox
kubectl describe voicemodel whisper-small -n vox
kubectl get voicemodels -n vox -w
```

## See also

- [ADR-010: GPU support](../adr/010-gpu-support.md) — why GPU scheduling needs no CRD changes, zone/driver decisions
- [Deploy a VoiceModel](../how-to/deploy-voicemodel.md) — practical walkthrough

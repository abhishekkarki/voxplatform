# InferencePipeline CRD Reference

**API Group:** `vox.vox.io/v1alpha1`  
**Kind:** `InferencePipeline`  
**Scope:** Namespaced

## Overview

`InferencePipeline` declares that a set of VoiceModel-backed inference stages should exist together and tracks their collective readiness. It does not create any Kubernetes resources — it is a health-check aggregate over existing VoiceModels.

When all referenced VoiceModels are Ready, the pipeline phase becomes `Ready`, meaning the gateway can successfully run all stages. If any model is not ready, the phase becomes `Degraded` or `Validating`.

## Spec

```yaml
apiVersion: vox.vox.io/v1alpha1
kind: InferencePipeline
metadata:
  name: default
  namespace: vox
spec:
  stages:
    - name: stt
      model: whisper-small
      enabled: true          # optional, default: true
    - name: diarize
      model: pyannote-diarizer
    - name: summarize
      model: qwen-summarizer
      enabled: false         # disable a stage without removing it
```

### `spec.stages[]`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Stage type. One of: `stt`, `diarize`, `summarize`. |
| `model` | string | yes | Name of a `VoiceModel` resource in the same namespace. |
| `enabled` | bool | no | Whether this stage runs at request time. Default: `true`. A disabled stage is still validated. |

## Status

```yaml
status:
  phase: Ready
  message: "3/3 stages ready"
  lastTransitionTime: "2026-05-14T10:00:00Z"
  stages:
    - name: stt
      modelRef: whisper-small
      ready: true
      endpoint: "vox-whisper-small.vox.svc.cluster.local:8000"
      message: "ready"
    - name: diarize
      modelRef: pyannote-diarizer
      ready: false
      message: "VoiceModel phase: Deploying"
    - name: summarize
      modelRef: qwen-summarizer
      ready: true
      endpoint: "vox-qwen-summarizer.vox.svc.cluster.local:8003"
      message: "ready"
```

### `status.phase`

| Phase | Meaning |
|-------|---------|
| `Pending` | Just created; controller has not yet evaluated the stages. |
| `Validating` | Controller is checking VoiceModel readiness; no stages are Ready yet. |
| `Ready` | All enabled stages have a Ready VoiceModel. |
| `Degraded` | Some stages are Ready, others are not. Traffic can flow but some stages may fail. |
| `Failed` | A referenced VoiceModel does not exist. Operator intervention required. |

### `status.stages[]`

| Field | Description |
|-------|-------------|
| `name` | Stage name (matches `spec.stages[].name`). |
| `modelRef` | VoiceModel name this stage references. |
| `ready` | `true` when the VoiceModel is in `Ready` phase. |
| `endpoint` | Internal K8s service address, copied from the VoiceModel's `status.endpoint`. |
| `message` | Human-readable status explanation. |

## kubectl quick reference

```bash
# List all pipelines
kubectl get inferencepipelines -n vox

# Inspect a pipeline
kubectl describe inferencepipeline default -n vox

# Watch pipeline status in real-time
kubectl get inferencepipelines -n vox -w
```

## Relationship to VoiceModel

`InferencePipeline` references `VoiceModel` resources but does not own them. Deleting an `InferencePipeline` does **not** delete the VoiceModels or their Deployments.

The operator watches VoiceModel changes. When a VoiceModel transitions to `Ready`, it immediately triggers a reconcile of all InferencePipelines in the same namespace that reference it. This means the pipeline status updates within seconds of the model becoming ready.

## Example — full pipeline definition

```yaml
apiVersion: vox.vox.io/v1alpha1
kind: VoiceModel
metadata:
  name: whisper-small
  namespace: vox
spec:
  model: Systran/faster-whisper-small.en
  device: cpu
  replicas: 1
---
apiVersion: vox.vox.io/v1alpha1
kind: VoiceModel
metadata:
  name: pyannote-diarizer
  namespace: vox
spec:
  model: pyannote/speaker-diarization-3.1
  image: europe-west3-docker.pkg.dev/voxplatform/vox-images-dev/diarizer:0.1.0
  port: 8002
  device: cpu
  health:
    path: /health
    initialDelaySeconds: 30
---
apiVersion: vox.vox.io/v1alpha1
kind: VoiceModel
metadata:
  name: qwen-summarizer
  namespace: vox
spec:
  model: Qwen/Qwen2.5-3B-Instruct-GGUF
  image: europe-west3-docker.pkg.dev/voxplatform/vox-images-dev/summarizer:0.1.0
  port: 8003
  device: cpu
  health:
    path: /health
    initialDelaySeconds: 120
---
apiVersion: vox.vox.io/v1alpha1
kind: InferencePipeline
metadata:
  name: default
  namespace: vox
spec:
  stages:
    - name: stt
      model: whisper-small
    - name: diarize
      model: pyannote-diarizer
    - name: summarize
      model: qwen-summarizer
```

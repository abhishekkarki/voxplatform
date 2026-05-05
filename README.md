# voxplatform

<img src ="images/voxplatform-logo.png" width="95%" height="25%">

> A self-hosted, Kubernetes-native inference platform - operated via CRDs and deployed via GitOps - that serves open-source voice AI models with built-in eval, observability, and multi-model pipeline composition.

## What is this?

Voxplatform is an open-source reference architecture for running voice AI inference on Kubernetes. It bridges the gap between voice agent frameworks (LiveKit, Pipecat) and generic inference platforms (Baseten, Modal, BentoML) - providing the infrastructure layer that manages model deployments, pipeline composition, quality regression testing, and observability specifically for voice workloads.

**What it does:**
- Deploys voice models (Whisper, pyannote, small LLMs) via Kubernetes CRDs
- Composes multi-model pipelines declaratively (audio -> STT -> diarize -> summarize)
- Runs automated WER regression tests before new model versions ship
- Provides per-model observability: latency, throughput, cost-per-request

**What it doesn't do:**
- Train models (use your existing training pipeline)
- Replace Gradium/ElevenLabs/Deepgram (use their open-source models *on* this platform)
- Build voice agents (use LiveKit/Pipecat *on top of* this platform)

## Architecture

```
voxctl CLI (Go)
    │
    v
Control Plane (Go operator)
    │  CRDs: VoiceModel │ InferencePipeline │ EvalRun
    │
    v
Data Plane (per request)
    │  Gateway (Go) -> VAD -> STT -> Diarize -> LLM
    │       │
    │       └─-> Event log (append-only)
    │
    v
Observability
    OTel traces │ Prometheus metrics │ Grafana dashboards
```

## Quickstart

### Prerequisites
- GCP account with billing enabled ($300 free credits for new accounts)
- `gcloud` CLI installed and authenticated
- `terraform` >= 1.5
- `kubectl`
- `helm`

### 1. Bootstrap GCP infrastructure
```bash
git clone https://github.com/abhishek/voxplatform.git
cd voxplatform
./hack/bootstrap-gcp.sh
```

This creates: VPC, GKE cluster (1 CPU node), Artifact Registry, GCS bucket.

### 2. Deploy monitoring
```bash
make deploy-monitoring
```

### 3. Deploy Whisper
```bash
make deploy-whisper
```

### 4. Test
```bash
# Generate test audio
chmod +x hack/generate-test-audio.sh
./hack/generate-test-audio.sh

# Port-forward and transcribe
kubectl port-forward -n vox svc/whisper 8000:8000 &
python clients/python/transcribe.py hack/sample-audio/test.wav
```

## Project status

This project follows a 14-week iteration plan. See [PROJECT_PLAN.md](PROJECT_PLAN.md) for details.

| Iteration | Status | Goal |
|-----------|--------|------|
| 0 - Skeleton | In progress | End-to-end transcription with metrics |
| 1 - Streaming | Planned | Real-time streaming with VAD |
| 2 - Operator | Planned | VoiceModel CRD and reconciliation |
| 3 - Pipelines | Planned | Multi-model composition + event log |
| 4 - Eval | Planned | WER regression testing in CI |
| 5 - GPU | Planned | GPU node pool and vLLM |
| 6 - Hardening | Planned | Canary deploys, docs, demo |

## Design decisions

All architectural decisions are documented as ADRs in [`docs/adr/`](docs/adr/).

## License

Apache 2.0

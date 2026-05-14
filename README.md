# voxplatform

<img src ="images/voxplatform-logo.png" width="95%" height="25%">

> A self-hosted, Kubernetes-native inference platform - operated via CRDs and deployed via GitOps - that serves open-source voice AI models with built-in eval, observability, and multi-model pipeline composition.

A Kubernetes-native voice AI inference platform built on GKE. Deploy, manage, and evaluate speech-to-text models through custom resources.


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


```yaml
apiVersion: vox.io/v1alpha1
kind: VoiceModel
metadata:
  name: whisper-small
spec:
  model: Systran/faster-whisper-small.en
  replicas: 1
  device: cpu
```

Apply this YAML and the operator creates the Deployment, Service, and monitoring config automatically.

## Architecture

```
Client (Python SDK / CLI)
    → Gateway (Go, :8080)
        → Whisper Model Server (:8000)
            → Transcription Response
```

The **gateway** handles request validation, structured logging, Prometheus metrics, and error handling. The **operator** manages model lifecycles via CRDs. The **eval harness** computes WER against ground truth datasets and gates deployments.

## Quickstart

**Prerequisites:** GCP account, Terraform, Helm, kubectl, Docker

```bash
# 1. Provision infrastructure
cd infra/environments/dev
terraform apply -target=module.network
terraform apply -target=module.gke
gcloud container clusters get-credentials vox-cluster-dev --zone=europe-west3-a

# 2. Deploy whisper
helm install whisper deploy/helm/faster-whisper -n vox --create-namespace

# 3. Deploy monitoring
helm install monitoring prometheus-community/kube-prometheus-stack \
  -n monitoring --create-namespace -f deploy/helm/monitoring/values.yaml

# 4. Build and deploy gateway
docker build --platform linux/amd64 -f Dockerfile.gateway \
  -t europe-west3-docker.pkg.dev/voxplatform/vox-images-dev/gateway:0.1.0 .
docker push europe-west3-docker.pkg.dev/voxplatform/vox-images-dev/gateway:0.1.0
helm install gateway deploy/helm/gateway -n vox

# 5. Test
kubectl port-forward -n vox svc/gateway 8080:8080 &
pip install -e './clients/python'
vox transcribe test.wav
```

## Project Structure

```
voxplatform/
├── cmd/gateway/              # Go gateway entry point
├── internal/gateway/         # Gateway handlers, middleware, proxy
├── operator/                 # Kubernetes operator (Kubebuilder)
│   ├── api/v1alpha1/         # VoiceModel CRD types
│   └── internal/controller/  # Reconciliation loop
├── clients/python/           # Python SDK + CLI
├── eval/                     # WER evaluation harness
├── deploy/helm/              # Helm charts (whisper, gateway, monitoring)
├── infra/                    # Terraform modules (network, gke, registry, storage)
└── docs/                     # ADRs, architecture, blog posts
```

## Components

### Go Gateway

HTTP gateway in front of model servers. Zero external routing dependencies (Go 1.22 ServeMux).

- Request ID propagation (X-Request-ID)
- Structured JSON logging (slog)
- Prometheus metrics (request count, latency histogram, in-flight gauge)
- Multipart file validation (32MB limit)
- Graceful shutdown (SIGTERM)

```bash
go test ./internal/gateway/ -v -race
```

### VoiceModel Operator

Kubernetes operator that reconciles VoiceModel custom resources into Deployments and Services.

- Level-triggered reconciliation
- Owner references for garbage collection
- Finalizers for cleanup
- Phase tracking (Pending → Deploying → Ready → Failed)
- Default resources based on device type (CPU/GPU)

```bash
cd operator && make build && make test
```

### Python SDK

Client library and CLI for the gateway API.

```python
from voxplatform import VoxClient

client = VoxClient("http://localhost:8080")
result = client.transcribe("meeting.wav")
print(result.text)
```

```bash
vox health
vox models
vox transcribe audio.wav
vox transcribe audio.wav --json
```

### Eval Harness

Automated speech recognition accuracy testing using Word Error Rate (WER).

```bash
vox-eval run eval/datasets/test --threshold 0.25
# Exit code 0 = passed, 1 = WER exceeded threshold
```

## Infrastructure

| Module | Creates | Cost |
|--------|---------|------|
| network | VPC, subnet, Cloud Router, Cloud NAT | NAT: ~$0.045/hr |
| gke | GKE Standard cluster, CPU node pool | ~$0.17/hr |
| registry | Artifact Registry | ~$0.10/GB/month |
| storage | GCS bucket | ~$0.02/GB/month |

Daily workflow: `terraform apply` in the morning, `terraform destroy` in the evening. A 2-hour session costs ~$0.40.

## Tech Stack

| Category | Technology |
|----------|-----------|
| Cloud | GCP (GKE, Artifact Registry, VPC, Cloud NAT) |
| IaC | Terraform |
| Orchestration | GKE Standard, REGULAR release channel |
| Gateway | Go 1.25, stdlib ServeMux, slog, prometheus/client_golang |
| Operator | Go, Kubebuilder, controller-runtime |
| Model Server | faster-whisper-server (CPU, int8) |
| Client SDK | Python 3.13, httpx, Pydantic |
| Eval | Python, jiwer (WER) |
| Packaging | Helm 3 |
| Observability | Prometheus, Grafana (kube-prometheus-stack) |
| CI | GitHub Actions |

## License

Apache 2.0
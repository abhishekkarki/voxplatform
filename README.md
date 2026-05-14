<div align="center">
  <img src="images/voxplatform-logo.png" width="480" alt="VoxPlatform" />

  <p><strong>A self-hosted, Kubernetes-native inference platform for voice AI models.</strong></p>

  <p>
    <a href="https://github.com/abhishekkarki/voxplatform/actions/workflows/ci.yml">
      <img src="https://github.com/abhishekkarki/voxplatform/actions/workflows/ci.yml/badge.svg" alt="CI" />
    </a>
    <a href="https://abhishekkarki.github.io/voxplatform/">
      <img src="https://img.shields.io/badge/docs-live-blue" alt="Docs" />
    </a>
    <img src="https://img.shields.io/badge/go-1.26-00ADD8?logo=go" alt="Go" />
    <img src="https://img.shields.io/badge/python-3.13-3776AB?logo=python" alt="Python" />
    <img src="https://img.shields.io/badge/license-Apache%202.0-green" alt="License" />
  </p>

  <p>
    <a href="https://abhishekkarki.github.io/voxplatform/tutorial/first-transcription/">Quickstart</a> ·
    <a href="https://abhishekkarki.github.io/voxplatform/">Documentation</a> ·
    <a href="https://abhishekkarki.github.io/voxplatform/explanation/architecture/">Architecture</a> ·
    <a href="https://abhishekkarki.github.io/voxplatform/explanation/adrs/">ADRs</a>
  </p>
</div>

---

VoxPlatform is an open-source reference architecture for running voice AI inference on Kubernetes. It sits between voice agent frameworks (LiveKit, Pipecat) and generic inference platforms (Baseten, Modal) — providing the infrastructure layer that manages model deployments, multi-model pipelines, quality regression testing, and observability specifically for voice workloads.

```yaml
apiVersion: vox.vox.io/v1alpha1
kind: VoiceModel
metadata:
  name: whisper-small
  namespace: vox
spec:
  model: Systran/faster-whisper-small.en
  device: cpu
```

Apply this YAML and the operator reconciles a Deployment, Service, and monitoring config automatically. No Helm values to tune, no Deployment YAML to write.

## What it does

| | |
|---|---|
| **Declarative model serving** | `VoiceModel` CRD → operator reconciles Deployment + Service + health probes |
| **Multi-stage pipelines** | `InferencePipeline` CRD chains STT → diarization → summarization |
| **Streaming transcription** | WebSocket endpoint with Silero VAD — only speech reaches the model |
| **Quality regression testing** | WER eval harness blocks deploys when accuracy drops |
| **Observability** | Prometheus metrics + Grafana dashboards per model — latency, throughput, cost |
| **GitOps-ready** | Helm charts + ArgoCD manifests for fully declarative deploys |

## What it is not

- A voice agent framework — use LiveKit or Pipecat *on top of* VoxPlatform
- A model training platform — bring your own models
- A SaaS product — this is a self-hosted reference architecture

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                Client  (Python SDK / CLI / curl)                │
└───────────────────────────────┬─────────────────────────────────┘
                                │ HTTP  ·  WebSocket
┌───────────────────────────────▼─────────────────────────────────┐
│                   Gateway  (Go · port 8080)                     │
│  POST /v1/audio/transcriptions   batch STT                      │
│  WS   /v1/audio/stream           streaming STT + VAD            │
│  POST /v1/pipeline/run           STT → diarize → summarize      │
│                  │                  │               │            │
│           VAD sidecar          Whisper         Diarizer          │
│           Silero :8001         :8000           :8002             │
│                                            Summarizer :8003      │
│  Event log → /tmp (dev)  ·  GCS bucket (production)             │
└───────────────────────────────┬─────────────────────────────────┘
                                │
┌───────────────────────────────▼─────────────────────────────────┐
│          Kubernetes Operator  (Go · Kubebuilder)                │
│  VoiceModel CRD         →  Deployment + Service                 │
│  InferencePipeline CRD  →  validates stage readiness            │
└─────────────────────────────────────────────────────────────────┘
```

## Quickstart

**Prerequisites:** GCP account · Terraform ≥ 1.5 · kubectl · Helm ≥ 3.12 · Docker

```bash
git clone https://github.com/abhishekkarki/voxplatform.git
cd voxplatform

# 1. Configure infrastructure
cat > infra/environments/dev/terraform.tfvars <<EOF
project_id       = "YOUR_GCP_PROJECT_ID"
region           = "europe-west3"
zone             = "europe-west3-a"
env              = "dev"
cpu_machine_type = "e2-standard-4"
cpu_min_nodes    = 1
cpu_max_nodes    = 2
cpu_disk_size_gb = 50
EOF

# 2. Provision GKE cluster (~10 min)
cd infra/environments/dev && terraform init && terraform apply -auto-approve
gcloud container clusters get-credentials vox-cluster-dev \
  --zone europe-west3-a --project YOUR_GCP_PROJECT_ID
cd ../../..

# 3. Build and push images
export REGISTRY=europe-west3-docker.pkg.dev/YOUR_GCP_PROJECT_ID/vox-images-dev
docker build --platform linux/amd64 -f Dockerfile.gateway -t $REGISTRY/gateway:0.3.0 .
docker push $REGISTRY/gateway:0.3.0

# 4. Install CRDs and deploy
kubectl apply -f operator/config/crd/bases/
helm upgrade --install whisper deploy/helm/faster-whisper -n vox --create-namespace --wait
helm upgrade --install gateway deploy/helm/gateway -n vox \
  --set image.repository=$REGISTRY/gateway --wait

# 5. Transcribe
kubectl port-forward -n vox svc/gateway 8080:8080 &
pip install -e ./clients/python
vox health
vox transcribe audio.wav
```

See the **[full deployment guide](https://abhishekkarki.github.io/voxplatform/how-to/gke-e2e-testing/)** for the complete end-to-end setup including the pipeline, operator, and monitoring.

## Pipeline

Run STT → speaker diarization → summarization as a single API call:

```bash
curl -X POST http://localhost:8080/v1/pipeline/run \
  -F file=@meeting.wav | jq '{transcript, segments, summary}'
```

```json
{
  "transcript": "The quick brown fox jumps over the lazy dog.",
  "segments": [
    { "start": 0.0, "end": 2.3, "speaker": "SPEAKER_00" },
    { "start": 2.5, "end": 3.5, "speaker": "SPEAKER_01" }
  ],
  "summary": "A brief exchange about a fox and a dog."
}
```

Run only specific stages:

```bash
vox transcribe --pipeline meeting.wav          # all stages
curl ... -F stages=stt                         # transcription only
curl ... -F stages=stt,diarize                 # no summarization
```

## Project Structure

```
voxplatform/
├── cmd/gateway/                  # Gateway binary entry point
├── internal/gateway/             # Handlers, middleware, proxy, streaming, pipeline, event log
├── operator/
│   ├── api/v1alpha1/             # VoiceModel + InferencePipeline CRD types
│   ├── internal/controller/      # Reconciliation loops
│   └── config/                   # CRD manifests, RBAC, sample CRs
├── services/
│   ├── vad/                      # Silero VAD sidecar (Python / FastAPI)
│   ├── diarizer/                 # pyannote-audio diarization service
│   └── summarizer/               # llama-cpp-python + Qwen 3B summarization
├── clients/python/               # VoxClient SDK + vox CLI
├── eval/                         # WER evaluation harness + vox-eval CLI
├── deploy/
│   ├── helm/                     # Helm charts: gateway, whisper, diarizer, summarizer, monitoring
│   └── argocd/                   # ArgoCD Application manifests
├── infra/
│   ├── modules/                  # Terraform modules: gke, network, registry, storage
│   └── environments/dev/         # Dev environment tfvars + backend config
└── docs/                         # MkDocs site: tutorials, how-tos, ADRs, reference
```

## Components

### Gateway — `internal/gateway/`

Go HTTP/WebSocket server with zero external routing dependencies.

- Batch transcription: `POST /v1/audio/transcriptions`
- Real-time streaming: `WebSocket /v1/audio/stream` with Silero VAD silence filtering
- Multi-stage pipeline: `POST /v1/pipeline/run` (STT → diarize → summarize)
- Per-request event log: append-only JSONL to GCS for replay and debugging
- Prometheus metrics: request count, latency histogram, in-flight gauge

```bash
go test ./internal/gateway/ -v -race
```

### Operator — `operator/`

Kubebuilder-based controller managing two CRDs:

**`VoiceModel`** — declares a model server. The controller creates a Deployment and Service, tracks phase (`Pending → Deploying → Ready → Failed`), and cleans up on deletion via finalizers.

**`InferencePipeline`** — declares a chain of VoiceModels as pipeline stages. The controller validates each referenced VoiceModel is Ready and reports aggregate health. Watches VoiceModel changes for immediate re-evaluation.

```bash
cd operator && make build && make test
kubectl apply -f operator/config/crd/bases/
kubectl apply -f operator/config/samples/voicemodels/whisper-small.yaml
kubectl get voicemodels -n vox -w
```

### Python SDK — `clients/python/`

Sync and async client with `vox` CLI.

```python
from voxplatform import VoxClient

client = VoxClient("http://localhost:8080")
result = client.transcribe("meeting.wav")
print(result.text)                    # transcript
print(f"{result.processing_time:.1f}s")
```

```bash
vox health                            # liveness check
vox transcribe meeting.wav            # batch transcription
vox transcribe meeting.wav --json     # raw JSON output
vox record --duration 10              # stream from microphone
```

### Eval Harness — `eval/`

Word Error Rate testing against ground-truth datasets. Blocks deployment when accuracy regresses.

```bash
pip install -e ./eval
vox-eval run eval/datasets/test --threshold 0.25
# Exit 0 = WER ≤ 25% ✓   Exit 1 = regression detected
```

## Infrastructure

Terraform modules provision everything on GCP. A 2-hour dev session costs ~$0.40.

| Module | Resources | Est. cost |
|--------|-----------|-----------|
| `gke` | GKE Standard cluster, CPU node pool (e2-standard-4) | ~$0.17/hr |
| `network` | VPC, subnet, Cloud Router, Cloud NAT | ~$0.05/hr |
| `registry` | Artifact Registry | ~$0.10/GB/month |
| `storage` | GCS bucket (eval datasets, event logs) | ~$0.02/GB/month |

Terraform state lives in a dedicated permanent bucket (`voxplatform-tfstate`) separate from the app bucket so `terraform destroy` never breaks state.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Cloud | GCP — GKE Standard, Artifact Registry, Cloud NAT, GCS |
| IaC | Terraform 1.5+ |
| Gateway | Go 1.26, stdlib `net/http`, `slog`, `nhooyr.io/websocket` |
| Operator | Go, Kubebuilder, `controller-runtime` |
| STT | `fedirz/faster-whisper-server` (CTranslate2, int8, CPU) |
| VAD | Python, Silero VAD via `torch.hub` |
| Diarization | Python, `pyannote-audio` 3.x |
| Summarization | Python, `llama-cpp-python`, Qwen 2.5 3B GGUF |
| Client SDK | Python 3.13, `httpx`, `pydantic` |
| Packaging | Helm 3 |
| Observability | Prometheus, Grafana (`kube-prometheus-stack`) |
| CI | GitHub Actions |

## Documentation

Full documentation is available at **[abhishekkarki.github.io/voxplatform](https://abhishekkarki.github.io/voxplatform/)**.

| Section | Contents |
|---------|----------|
| [Tutorial](https://abhishekkarki.github.io/voxplatform/tutorial/first-transcription/) | First transcription in 5 minutes |
| [How-to guides](https://abhishekkarki.github.io/voxplatform/how-to/) | Deploy models, run pipelines, eval harness, GKE E2E testing |
| [Reference](https://abhishekkarki.github.io/voxplatform/reference/) | HTTP API, WebSocket protocol, CRD schemas, Python SDK |
| [Explanation](https://abhishekkarki.github.io/voxplatform/explanation/) | Architecture, pipeline design, operator internals, ADRs |

## Iteration Roadmap

| # | Goal | Status |
|---|------|--------|
| 0 | GKE + Terraform + gateway + Grafana | ✅ Complete |
| 1 | Streaming + VAD sidecar | ✅ Complete |
| 2 | `VoiceModel` operator (CRD → Deployment) | ✅ Complete |
| 3 | `InferencePipeline` CRD · diarizer · summarizer · event log | ✅ Complete |
| 4 | `EvalRun` CRD + Argo Workflows + WER regression in CI | 🔜 Next |
| 5 | GPU node pool (T4 spot) · vLLM · whisper-large-v3 | Planned |
| 6 | Argo Rollouts canary · cost tracking · ArgoCD · demo | Planned |

## License

Apache 2.0 — see [LICENSE](LICENSE).

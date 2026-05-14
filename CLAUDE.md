# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

---

## Mission

> **VoxPlatform is a self-hosted, Kubernetes-native inference platform — operated via CRDs and deployed via GitOps — that serves open-source voice AI models with built-in eval, observability, and multi-model pipeline composition.**

Every feature, PR, and design decision must pass this test: *"Does this serve the sentence?"* If not → `MAYBE_LATER.md`.

---

## What this project IS and IS NOT

**IS:**
- A Kubernetes operator (Go) that manages voice model deployments via CRDs
- A streaming gateway (Go) that routes audio to inference backends
- An eval harness (Python) that catches model regressions in CI
- Observability (Prometheus + Grafana) for inference workloads
- Declarative multi-model pipeline composition (audio → STT → diarize → summarize)
- GitOps-deployed via ArgoCD on GKE, Terraform-provisioned on GCP

**IS NOT:**
- A voice agent / chatbot framework (that's LiveKit/Pipecat)
- A model training platform
- A SaaS product
- A frontend application (Grafana dashboards are the UI)
- A replacement for Deepgram/ElevenLabs (you USE their open-source models ON this platform)

**Stop immediately if you catch yourself:**
- Building a web UI that isn't Grafana
- Adding TTS before STT works end-to-end with evals
- Writing a custom VAD model instead of using Silero
- Adding authentication beyond basic mTLS/JWT
- Supporting non-voice models "just because"
- Optimizing for scale beyond 5 concurrent requests

---

## Current Iteration Status

The project follows a 14-week / 7-iteration plan. Based on the codebase:

| Iter | Goal | Status |
|------|------|--------|
| 0 | GKE + Terraform + gateway + Grafana | ✅ Complete |
| 1 | Streaming (WebSocket) + VAD sidecar | ✅ Complete |
| 2 | Kubernetes Operator (`VoiceModel` CRD) | ✅ Complete |
| **3** | **`InferencePipeline` CRD, pyannote diarization, llama.cpp/Qwen, event log on GCS** | **✅ Complete** |
| **4** | **`EvalRun` CRD + Argo Workflows, WER regression in CI** | **⬅ Next** |
| 5 | GPU node pool (T4 spot), vLLM, whisper-large-v3 | Planned |
| 6 | Argo Rollouts canary, cost tracking, ArgoCD, demo | Planned |

---

## Language Split (Locked)

| Component | Language | Rule |
|-----------|----------|------|
| Operator, Gateway | Go | Go services talk to Python services over HTTP only — never shared code |
| Eval harness, SDK, VAD sidecar, pipeline workers | Python | Python services are containerized; Go calls them over HTTP |

**Boundary rule:** Go and Python share contracts (JSON), never code.

---

## Build & Test Commands

### Go Gateway (repo root)
```bash
go build ./cmd/gateway/
go test ./internal/gateway/ -v -race
go test ./internal/gateway/ -run TestHandleTranscribe -v   # single test
go vet ./...
```

### Go Operator (`cd operator`)
```bash
make build          # generates manifests + deepcopy, then builds bin/manager
make test           # runs envtest (downloads K8s binaries on first run)
make manifests      # regenerate CRD YAML from Go types (run after editing api/v1alpha1/)
make generate       # regenerate DeepCopy methods
make fmt && make vet
make lint           # golangci-lint (downloads on first run)
```

Operator tests require `setup-envtest`. First run downloads K8s binaries (~200MB) to `operator/bin/`.

### Python SDK (`cd clients/python`)
```bash
pip install -e ".[dev]"
pytest -v --tb=short
pytest tests/test_client.py::test_transcribe -v   # single test
```

### Python Eval Harness (`cd eval`)
```bash
pip install -e "../clients/python"   # SDK is a dependency
pip install -e ".[dev]"
pytest -v --tb=short
vox-eval run eval/datasets/sample --threshold 0.25
```

### VAD Sidecar (`cd services/vad`)
```bash
pip install fastapi uvicorn torch numpy
uvicorn vad_server:app --port 8001
```

### Helm
```bash
helm lint deploy/helm/gateway
helm lint deploy/helm/faster-whisper
helm template gateway deploy/helm/gateway -n vox
```

---

## Architecture

### Request flows

**Batch transcription:**
```
Client POST /v1/audio/transcriptions
  → Gateway middleware stack (RequestID → Log → Metrics → Recovery)
  → proxy.go: reconstructs multipart form, forwards to Whisper :8000
  → returns enriched JSON (adds request_id, processing_time, created_at)
```

**Streaming transcription:**
```
Client WebSocket /v1/audio/stream
  → stream.go: receives 20ms PCM frames (640 bytes, 16kHz 16-bit mono)
  → each frame: POST to VAD sidecar :8001 → {speech: bool, confidence: float}
  → audioBuffer state machine: IDLE → BUFFERING (speech) → FLUSHING (silence >2s)
  → flush: constructs WAV, POST to Whisper :8000, sends JSON back to client
```

### Component map

```
cmd/gateway/main.go              → entry point, signal handling, graceful shutdown
internal/gateway/
  server.go                      → route registration, ServeMux wiring
  config.go                      → env var parsing (all gateway env vars)
  handlers.go                    → /v1/audio/transcriptions, /v1/models, /healthz, /readyz
  proxy.go                       → reverse proxy to faster-whisper (rebuilds multipart body)
  stream.go                      → WebSocket handler, audioBuffer, VAD integration
  pipeline.go                    → POST /v1/pipeline/run orchestrator (STT→diarize→summarize)
  eventlog.go                    → EventLogger interface, LocalFileLogger, GCSLogger
  middleware.go                  → RequestID, structured logging (slog), Prometheus, recovery
  request_id.go                  → X-Request-ID generation and propagation
  errors.go                      → typed error responses

operator/
  api/v1alpha1/
    voicemodel_types.go          → VoiceModel CRD schema
    inferencepipeline_types.go   → InferencePipeline CRD schema (iteration 3)
    zz_generated.deepcopy.go     → generated DeepCopy methods (DO NOT edit manually — run make generate)
  internal/controller/
    voicemodel_controller.go     → VoiceModel → Deployment + Service
    inferencepipeline_controller.go → validates VoiceModel readiness, aggregates pipeline phase
  config/crd/bases/              → generated CRD YAML (run make manifests after editing types)

services/
  vad/vad_server.py              → Silero VAD FastAPI, POST /vad
  diarizer/diarizer_server.py    → pyannote-audio FastAPI, POST /diarize
  summarizer/summarizer_server.py → llama-cpp-python FastAPI, POST /summarize

clients/python/voxplatform/
  client.py      → VoxClient (sync + async via httpx)
  models.py      → Pydantic response types
  cli.py         → `vox` CLI (transcribe, health, ready, models, record)

eval/vox_eval/
  runner.py      → EvalRunner: loads dataset, transcribes concurrently, computes WER
  cli.py         → `vox-eval` CLI

deploy/helm/
  gateway/          → gateway + VAD sidecar as one pod
  faster-whisper/   → faster-whisper-server deployment
  diarizer/         → pyannote diarizer deployment + PVC
  summarizer/       → Qwen summarizer deployment + PVC
  monitoring/       → kube-prometheus-stack values

infra/
  modules/{gke,network,registry,storage}/   → Terraform modules
  environments/dev/                          → dev tfvars (europe-west3, e2-standard-4)
```

### Key environment variables

| Service | Variable | Default | Notes |
|---------|----------|---------|-------|
| Gateway | `PORT` | `8080` | |
| Gateway | `WHISPER_URL` | `http://whisper.vox.svc.cluster.local:8000` | |
| Gateway | `VAD_ENDPOINT` | `http://localhost:8001` | |
| Gateway | `DIARIZER_URL` | _(empty — skipped)_ | Set to enable diarize stage |
| Gateway | `SUMMARIZER_URL` | _(empty — skipped)_ | Set to enable summarize stage |
| Gateway | `EVENT_LOG_BACKEND` | `local` | `local` or `gcs` |
| Gateway | `EVENT_LOG_DIR` | `/tmp/vox-events` | Used when backend=local |
| Gateway | `EVENT_LOG_BUCKET` | _(empty)_ | Required when backend=gcs |
| SDK/CLI | `VOX_GATEWAY_URL` | _(required)_ | |
| Diarizer | `HF_TOKEN` | _(empty — fallback mode)_ | HuggingFace token for pyannote |
| Summarizer | `MODEL_REPO` | `Qwen/Qwen2.5-3B-Instruct-GGUF` | |
| Summarizer | `MODEL_FILE` | `qwen2.5-3b-instruct-q4_k_m.gguf` | |

### VoiceModel CRD lifecycle

```
kubectl apply VoiceModel
  → controller reconciles: creates Deployment + Service
  → status.phase: Pending → Deploying → Ready
  → deletion: finalizer ensures Deployment/Service are cleaned up
```

After editing `operator/api/v1alpha1/voicemodel_types.go`, always run `make manifests && make generate` to regenerate CRDs and DeepCopy methods.

### Two separate Go modules

The repo has **two independent Go modules**:
- Root (`go.mod`): gateway only — `github.com/abhishekkarki/voxplatform`
- `operator/go.mod`: operator only — `github.com/abhishekkarki/voxplatform/operator`

Run Go commands from the correct directory. `go test ./...` from root will NOT pick up operator tests.

---

## Local Development Setup (without GKE)

GKE is used only to validate a completed iteration. Day-to-day development runs locally.

### Data plane (whisper + vad + gateway)

```bash
docker compose up --build         # first time: builds gateway + vad images, pulls whisper
docker compose up                 # subsequent runs (uses cached images)
docker compose up whisper vad     # start only the backends, run gateway with go run instead
```

First run is slow — Whisper downloads the model (~250MB) into the `whisper-model-cache` Docker volume. All later starts are fast.

To run the gateway from source (faster iteration on gateway code):
```bash
docker compose up whisper vad     # backends only
WHISPER_URL=http://localhost:8000 VAD_ENDPOINT=http://localhost:8001 go run ./cmd/gateway/
```

Test after the stack is up:
```bash
pip install -e ./clients/python
vox health
vox transcribe test.wav
```

### Operator development

Use `make test` (envtest — no cluster needed) for unit tests. Use `kind` only when you need real CRD apply/reconcile integration testing:

```bash
cd operator && make test                    # envtest
kind create cluster --name vox-local        # only for integration tests
cd operator && make test-e2e
kind delete cluster --name vox-local
```

---

## Infrastructure

- **Cloud:** GCP, region `europe-west3` (Frankfurt)
- **GKE workflow:** `terraform apply` to bring up cluster, test iteration end-to-end, `terraform destroy` when done. A 2-hour session costs ~$0.40.
- **Images:** `europe-west3-docker.pkg.dev/voxplatform/vox-images-dev/`
- **Namespace:** `vox` for all workloads, `monitoring` for Prometheus/Grafana

---

## Locked Technology Choices

Do not suggest alternatives to these unless the iteration plan explicitly unlocks them:

| What | Choice | Unlocks |
|------|--------|---------|
| STT model | faster-whisper (small.en, CPU, int8) | Iteration 5 (GPU + whisper-large-v3) |
| LLM | llama.cpp + Qwen 3B Q4 (CPU) | Iteration 5 |
| Diarization | pyannote-audio | Locked |
| VAD | Silero VAD | Locked |
| GitOps | ArgoCD | Iteration 6 |
| Event log | JSONL on GCS | Iteration 3 |

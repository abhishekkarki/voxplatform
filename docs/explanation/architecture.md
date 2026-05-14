# Architecture

VoxPlatform is structured around four layers: control plane, data plane, inference services, and observability. Each layer has a single responsibility and communicates with adjacent layers over well-defined HTTP interfaces.

---

## System overview

```
┌───────────────────────────────────────────────────────────────┐
│                Developer (Python SDK / CLI / curl)            │
└───────────────────────────────┬───────────────────────────────┘
                                │  HTTP
┌───────────────────────────────▼───────────────────────────────┐
│                  Gateway (Go, port 8080)                      │
│                                                               │
│  POST /v1/audio/transcriptions   (batch STT)                  │
│  WS   /v1/audio/stream           (streaming STT + VAD)        │
│  POST /v1/pipeline/run           (STT → diarize → summarize)  │
│  GET  /v1/models                                              │
│  GET  /healthz  /readyz  /metrics                             │
│                  │         │           │                      │
│              VAD sidecar  Whisper    Diarizer  Summarizer     │
│              :8001        :8000      :8002     :8003          │
│                  │                                            │
│              Event log → /tmp/vox-events (local)             │
│                        → GCS bucket     (production)         │
└───────────────────────────────────────────────────────────────┘
                                │
┌───────────────────────────────▼───────────────────────────────┐
│             Kubernetes Operator (Go, Kubebuilder)             │
│                                                               │
│  VoiceModel CRD         → Deployment + Service               │
│  InferencePipeline CRD  → validates VoiceModel readiness      │
│  (EvalRun CRD)          → Argo Workflows (iteration 4)        │
└───────────────────────────────────────────────────────────────┘
                                │
┌───────────────────────────────▼───────────────────────────────┐
│                   Observability                               │
│  Prometheus metrics  │  Grafana dashboards  │  Structured logs│
│  Per-model: latency, throughput, CPU-seconds, error rate      │
└───────────────────────────────────────────────────────────────┘
```

---

## Layer 1 — Gateway (data plane)

The gateway is the **single entry point for all inference traffic**. It handles:

- Request ID generation and propagation
- Structured JSON logging (slog)
- Prometheus metrics (request count, latency histogram, in-flight gauge)
- Multipart file validation (32MB limit)
- Routing to the right backend(s)
- Pipeline orchestration for multi-stage requests
- Event log writes

The gateway is stateless. Any number of replicas can run behind a load balancer. State lives in the event log (GCS) and the inference services, not in the gateway.

**Key design choice:** The gateway rebuilds multipart forms before forwarding to backends. It cannot stream the raw request body to Whisper because the body is consumed during validation. This adds ~1ms overhead but enables logging, metrics, and per-request validation.

---

## Layer 2 — Inference services

Each inference step runs as an independent HTTP service. The gateway calls them in sequence for pipeline requests.

| Service | Port | Technology | Startup |
|---------|------|-----------|---------|
| Whisper (STT) | 8000 | `fedirz/faster-whisper-server`, CTranslate2, int8 | ~60s (model load) |
| VAD sidecar | 8001 | Python/FastAPI, Silero VAD | ~10s (model baked into image) |
| Diarizer | 8002 | Python/FastAPI, pyannote-audio 3.x | ~30s (model from HF cache) |
| Summarizer | 8003 | Python/FastAPI, llama-cpp-python, Qwen 3B | ~60s (model from volume) |

All services expose `GET /health` and run on CPU by default. Each has a graceful degradation mode: if a model fails to load, the service still starts and returns a fallback response (single speaker, extractive summary). This means the pipeline is never completely broken.

**VAD is special:** it runs as a sidecar container in the same pod as the gateway (in Kubernetes), so the gateway reaches it via `localhost:8001`. In Docker Compose it's a separate container at `http://vad:8001`. This is the only topology difference between local dev and production.

---

## Layer 3 — Kubernetes Operator (control plane)

The operator runs as a single-replica Deployment managed by controller-runtime. It reconciles two CRDs:

### VoiceModel

`VoiceModel` is the primary CRD. It declares: "I want a model server running this model at this replica count."

The controller creates a `Deployment` and a `Service` in the same namespace. The Deployment image and environment variables are computed from the spec. When the CRD is deleted, the finalizer ensures the Deployment and Service are cleaned up.

```
VoiceModel.spec.model = "Systran/faster-whisper-small.en"
   │
   └─ Deployment: vox-whisper-small
   └─ Service:    vox-whisper-small  → DNS: vox-whisper-small.<ns>.svc.cluster.local
```

### InferencePipeline

`InferencePipeline` is a health-check aggregate. It declares: "These VoiceModels should exist together as a pipeline."

The controller validates that each referenced VoiceModel exists and is in `Ready` phase. It does **not** create any new K8s resources. Its only output is `status.phase` and `status.stages[]`. When a VoiceModel transitions to Ready, a watch event immediately triggers a pipeline reconcile.

```
InferencePipeline.spec.stages = [{stt: whisper-small}, {diarize: pyannote-diarizer}, ...]
   │
   └─ validates VoiceModel "whisper-small" → phase: Ready
   └─ validates VoiceModel "pyannote-diarizer" → phase: Deploying
   └─ status.phase = Degraded (1/2 stages ready)
```

---

## Layer 4 — Observability

Prometheus scrapes each inference pod via annotations. Grafana dashboards (deployed via `kube-prometheus-stack`) visualise:

- Request throughput and latency (p50, p99) per endpoint
- Error rate per endpoint
- CPU usage per model pod
- In-flight request count

The event log (JSONL per request on GCS) provides per-request traceability beyond what metrics offer. Events record what happened at each stage with input/output sizes and durations.

---

## How a pipeline request flows

```
POST /v1/pipeline/run  file=meeting.wav  stages=stt,diarize,summarize

1. Gateway parses multipart form, reads audio into memory (~1ms)
2. Logs pipeline.start event to local file or GCS

3. STT stage (~2s on CPU):
   Gateway → POST http://whisper:8000/v1/audio/transcriptions
           ← {"text": "The quick brown fox..."}
   Logs stage.start, stage.complete

4. Diarize stage (~2s):
   Gateway → POST http://diarizer:8002/diarize (audio bytes in multipart)
           ← {"segments": [{start:0.0, end:2.3, speaker:"SPEAKER_00"}, ...]}
   Logs stage.start, stage.complete (or stage.error — pipeline continues)

5. Summarize stage (~5s on CPU with Qwen 3B):
   Gateway → POST http://summarizer:8003/summarize
             body: {"transcript": "...", "segments": [...]}
           ← {"summary": "A brief greeting and test."}
   Logs stage.start, stage.complete (or stage.error — pipeline continues)

6. Logs pipeline.complete
7. Returns unified JSON to client (transcript + segments + summary)
```

Total: ~10s for a 5-second audio clip on CPU. Bottleneck is the summarizer (LLM inference).

---

## Two separate Go modules

The gateway and operator are in different Go modules:

| Module | Root | Why separate |
|--------|------|-------------|
| `github.com/abhishekkarki/voxplatform` | `/` | Gateway only — minimal deps |
| `github.com/abhishekkarki/voxplatform/operator` | `/operator` | Operator — heavy k8s deps |

Running `go test ./...` from the root tests the gateway. Operator tests require `cd operator && make test`.

---

## Further reading

- [Pipeline Design](pipeline-design.md) — detailed walkthrough of the multi-stage pipeline
- [Streaming Design](streaming-design.md) — how WebSocket + VAD + audio buffering works
- [Operator Design](operator-design.md) — reconciliation loop and CRD lifecycle
- [Why VAD as Sidecar](why-vad-sidecar.md) — why Python VAD lives next to the Go gateway
- [ADR-007](../adr/007-event-sourcing.md) — why append-only JSONL on GCS
- [ADR-008](../adr/008-pipeline-composition.md) — why gateway orchestrates, not a workflow engine

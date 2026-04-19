# Voxplatform - project plan

## The sentence (never forget this)

> **Voxplatform is a self-hosted, Kubernetes-native inference platform - operated via CRDs and deployed via GitOps - that serves open-source voice AI models with built-in eval, observability, and multi-model pipeline composition.**

Every feature, PR, and design decision must pass this test:
"Does this serve the sentence?" If not -> `MAYBE_LATER.md`.

---

## Scope guard - what this project IS and IS NOT

### It IS:
- A Kubernetes operator (Go) that manages voice model deployments via CRDs
- A streaming gateway (Go) that routes audio to inference backends
- An eval harness (Python) that catches model regressions in CI
- Observability (OTel + Prometheus + Grafana) for inference workloads
- Declarative multi-model pipeline composition (audio -> STT -> diarize -> summarize)
- GitOps-deployed via ArgoCD on GKE
- Terraform-provisioned infrastructure on GCP

### It is NOT:
- A voice agent / chatbot framework (that's LiveKit/Pipecat territory)
- A model training platform
- A SaaS product (open-source reference architecture first)
- A frontend application (Grafana dashboards are the UI)
- A replacement for Gradium/ElevenLabs/Deepgram (you USE their open-source models)

### Scope creep triggers - stop immediately if you catch yourself:
- Building a web UI that isn't Grafana
- Adding TTS (text-to-speech) before STT works end-to-end with evals
- Writing a custom VAD model instead of using Silero
- Adding authentication beyond basic mTLS/JWT
- Supporting non-voice models "just because"
- Optimizing for scale beyond 5 concurrent requests
- Fine-tuning models instead of serving them

---

## Language split - locked decision

| Component | Language | Reason |
|-----------|----------|--------|
| Operator | Go | Kubebuilder is Go-only, non-negotiable |
| Gateway | Go | gRPC streaming, goroutine concurrency |
| CLI (voxctl) | Go | Single binary, complements operator |
| Eval harness | Python | jiwer, rouge-score, dataset loading |
| Pipeline workers | Python | faster-whisper, pyannote, llama.cpp |
| Client SDK | Python | sounddevice for mic, protobuf client |
| Argo Workflow steps | Python | Each eval step is a containerized script |

Boundary rule: Go services talk to Python services over gRPC or HTTP.
They share contracts (protobuf), never code.

---

## Infrastructure - GCP + GKE + Terraform

### Why GCP:
- GKE is the industry standard for ML inference
- $300 free credits for new accounts (~2-3 months of this project)
- Aligns with Isomorphic Labs (Alphabet) and Baseten (runs on GCP)
- GPU node pools available when needed (T4 spot instances are cheap)
- Artifact Registry for container images (no self-hosted Harbor needed)

### GCP resource layout:
```
project: voxplatform-dev
├── VPC: vox-vpc (europe-west3, Frankfurt - closest to Munich)
│   └── Subnet: vox-subnet (10.0.0.0/20)
├── GKE: vox-cluster (Standard mode, 1 node pool)
│   ├── Node pool: cpu-pool (e2-standard-4, 1-3 nodes, autoscaling)
│   └── Node pool: gpu-pool (n1-standard-4 + T4, 0-1 nodes, added in iteration 5)
├── Artifact Registry: vox-images
├── Cloud Storage: vox-artifacts (eval datasets, audio samples, event logs)
└── Cloud DNS: vox zone (optional, for ingress)
```

### Terraform layout:
```
infra/
├── modules/
│   ├── gke/           # GKE cluster + node pools
│   ├── network/       # VPC, subnets, firewall rules
│   ├── registry/      # Artifact Registry
│   └── storage/       # GCS buckets
├── environments/
│   └── dev/
│       ├── main.tf
│       ├── variables.tf
│       ├── outputs.tf
│       ├── terraform.tfvars
│       ├── backend.tf
│       └── versions.tf
└── README.md
```

### Cost estimate (free tier + credits):
- GKE cluster fee: $0 (free tier: one zonal cluster)
- e2-standard-4 (1 node): ~$100/month
- Artifact Registry: ~$1/month
- GCS: ~$1/month
- Total: ~$102/month -> covered by $300 credits for ~3 months

---

## Architecture - the four pillars

```
┌─────────────────────────────────────────────────────────┐
│                    voxctl CLI (Go)                      │
│              Developer's entry point                    │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────v──────────────────────────────────┐
│              Control plane (Go operator)                │
│                                                         │
│  CRDs:  VoiceModel │ InferencePipeline │ EvalRun        │
│                                                         │
│  Reconciles CRDs into K8s Deployments, Services,        │
│  ServiceMonitors, Argo Workflows                        │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────v──────────────────────────────────┐
│                Data plane (per request)                 │
│                                                         │
│  Gateway (Go, gRPC/WS) -> VAD -> STT -> Diarize -> LLM      │
│       │                                                 │
│       └──-> Event log (append-only, per request)         │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────v──────────────────────────────────┐
│                 Observability                           │
│                                                         │
│  OTel traces │ Prometheus metrics │ Grafana dashboards  │
│  Per-model: latency, throughput, CPU/GPU-sec, cost      │
└─────────────────────────────────────────────────────────┘
```

---

## Iterations - 14 weeks, 7 iterations of 2 weeks each

### Iteration 0 - Skeleton (weeks 1–2)

**Goal:** GKE cluster provisioned via Terraform, an audio file goes in,
a transcript comes out, with metrics visible in Grafana.

**Why this serves the sentence:** Proves "Kubernetes-native" and
"serves open-source voice AI models with observability" end-to-end.

**Week 1 tasks (infra + deploy):**
1. GCP project created, billing enabled, APIs enabled
2. Terraform modules written and applied:
   - VPC + subnet in europe-west3
   - GKE Standard cluster with cpu-pool (e2-standard-4, 1-3 nodes)
   - Artifact Registry for container images
   - GCS bucket for artifacts
3. kubectl access configured, cluster verified
4. kube-prometheus-stack deployed via Helm (Prometheus + Grafana)
5. faster-whisper-server deployed via Helm (CPU, small.en, int8)
6. Verify: curl POST a WAV file -> get transcript JSON back
7. Verify: Grafana shows faster-whisper metrics

**Week 2 tasks (gateway + wiring):**
1. Repo scaffolded with Go + Python structure
2. Protobuf definitions for vox.v1.Transcribe service
3. Go gateway: gRPC endpoint accepts audio bytes, calls faster-whisper,
   returns transcript
4. Gateway containerized, pushed to Artifact Registry, deployed to GKE
5. OTel tracing: gateway -> STT span
6. Grafana dashboard: request count, latency p50/p99, CPU usage
7. ADR-001: "Why CPU-first"
8. ADR-002: "Why faster-whisper over Triton for v0"
9. ADR-003: "Why GCP/GKE"

**Verification checkpoint:**
```bash
# This command must work by end of week 2:
grpcurl -d '{"audio_data": "<base64>"}' \
  gateway.vox.svc.cluster.local:50051 vox.v1.TranscribeService/Transcribe
# Returns: {"transcript": "hello world", "duration_ms": 1847}
# Grafana shows the request
```

**Scope traps:**
- Don't add streaming yet (iteration 1)
- Don't write the operator (iteration 2)
- Don't add VAD (iteration 1)
- Don't add TLS/auth
- Don't add ArgoCD yet (get things working with kubectl first)

---

### Iteration 1 - Streaming and VAD (weeks 3–4)

**Goal:** Real-time streaming transcription with voice activity detection.

**Deliverables:**
1. gRPC bidirectional streaming on gateway
2. Silero VAD integrated - audio only forwarded when speech detected
3. Python CLI client: `voxcli record` streams mic, prints transcripts
4. New metrics: VAD voiced-ratio, time-to-first-partial
5. OTel child spans for VAD and STT
6. ADR-004: "Streaming architecture"

**Verification:** Speak into mic -> partial transcripts in <500ms.

---

### Iteration 2 - The Operator (weeks 5–6)

**Goal:** K8s operator in Go that turns VoiceModel CRDs into running
inference deployments.

**Deliverables:**
1. Kubebuilder project with VoiceModel CRD
2. Reconciliation: CRD -> Deployment + Service + ServiceMonitor + HPA
3. Existing faster-whisper managed by VoiceModel CR
4. ADR-005: "CRD design - VoiceModel schema"
5. ADR-006: "Why Kubebuilder"

**Verification:** `kubectl apply -f voicemodel.yaml` -> running endpoint.

---

### Iteration 3 - Pipeline Composition + Events (weeks 7–8)

**Goal:** Multi-model pipeline via InferencePipeline CRD with event log.

**Deliverables:**
1. InferencePipeline CRD (stages: stt -> diarize -> summarize)
2. pyannote for diarization, llama.cpp + Qwen 3B for summarization
3. Append-only event log per request (JSONL on GCS)
4. Replay CLI: `voxctl replay <request-id>`
5. ADR-007: "Event sourcing for inference"
6. ADR-008: "Pipeline composition"

**Verification:** Submit audio -> get transcript + speakers + summary.

---

### Iteration 4 - Eval Harness (weeks 9–10)

**Goal:** Automated quality regression testing in CI.

**Deliverables:**
1. EvalRun CRD -> Argo Workflow
2. WER computation on LibriSpeech subset
3. Results to Postgres, Grafana dashboard
4. CI blocks merge on regression
5. ADR-009: "Eval design"
6. Blog: "CPU transcription cost analysis"

**Verification:** Bad model config -> EvalRun status: Fail.

---

### Iteration 5 - GPU Support (weeks 11–12)

**Goal:** GPU as a compute class without breaking the abstraction.

**Deliverables:**
1. Terraform: GPU node pool (T4 spot instance)
2. NVIDIA GPU Operator on GKE
3. VoiceModel CRD with GPU resources
4. vLLM serving whisper-large-v3 and Qwen 7B
5. GPU metrics (DCGM exporter)
6. Cost comparison: CPU vs GPU
7. ADR-010: "Adding GPU support"
8. Blog: "CPU vs GPU inference - the numbers"

**Verification:** Same CRD interface, GPU and CPU models side by side.

---

### Iteration 6 - Production Hardening (weeks 13–14)

**Goal:** Canary deploys, cost tracking, docs, demo video.

**Deliverables:**
1. Argo Rollouts canary deployment
2. Per-request cost tracking
3. ArgoCD for GitOps (all manifests in git)
4. README rewrite, ADR index
5. Blog: "Building a K8s operator for voice AI"
6. 3-minute demo video

**Verification:** Stranger can follow README quickstart on fresh GKE.

---

## Tech stack - locked decisions

| Component | Choice | Locked until |
|-----------|--------|-------------|
| Infra | GCP + GKE Standard + Terraform | Forever |
| Region | europe-west3 (Frankfurt) | Forever |
| Operator lang | Go + Kubebuilder | Forever |
| Gateway lang | Go | Forever |
| Eval/ML lang | Python | Forever |
| STT v0 | faster-whisper (small.en, CPU) | Iteration 5 |
| STT v1 | vLLM + whisper-large-v3 (GPU) | - |
| LLM v0 | llama.cpp + Qwen 3B Q4 (CPU) | Iteration 5 |
| LLM v1 | vLLM + Qwen 7B (GPU) | - |
| Diarization | pyannote-audio | Forever |
| VAD | Silero VAD | Forever |
| Metrics | Prometheus + Grafana | Forever |
| Traces | OpenTelemetry + Tempo | Forever |
| GitOps | ArgoCD (iteration 6) | Forever |
| IaC | Terraform | Forever |
| Container registry | GCP Artifact Registry | Forever |
| Object storage | GCS | Forever |
| Eval: WER | jiwer (Python) | Forever |
| Eval orchestration | Argo Workflows | Forever |
| Event log | JSONL on GCS | Forever |

---

## The verification ritual - every Sunday

1. Read THE sentence out loud
2. Ask: What did I ship? What am I tempted by? Can I demo it?
3. Write `docs/journal/YYYY-MM-DD.md` (5 min)
4. Check MAYBE_LATER.md (>10 items = too distracted)

---

## Success criteria - the project is done when:

1. `helm install voxplatform` on fresh GKE works in <10 minutes
2. VoiceModel CRD -> running inference endpoint with metrics
3. InferencePipeline CRD -> multi-model flow
4. EvalRun CRD -> WER regression tests
5. Grafana dashboard shows latency, throughput, cost
6. README quickstart works for a stranger
7. You can explain every component in a 45-min interview
8. 2+ blog posts published
9. Demo video exists

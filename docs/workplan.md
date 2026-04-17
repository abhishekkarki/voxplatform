# Workplan - Iterations

## Iteration 0 - Skelton (weeks 1-2)
**Goal:** An audio file goes in, a transcript comes out, on your K8s cluster,
with metrics visible in Grafana.
 
**Why this serves the sentence:** Proves the end-to-end path exists —
Kubernetes cluster serving an open-source voice model with observability.
 
**Deliverables:**
1. Repo scaffolded (monorepo, Go modules, Helm charts, Terraform)
2. faster-whisper-server deployed on RKE2 via Helm chart (CPU, small.en model)
3. Minimal Go gateway: gRPC endpoint accepts audio bytes, POSTs to
   faster-whisper, returns transcript
4. Prometheus scraping faster-whisper /metrics endpoint
5. One Grafana dashboard: request count, latency p50/p99, CPU usage
6. ADR-001: "Why CPU-first"
7. ADR-002: "Why faster-whisper over Triton for v0"
**Verification checkpoint:**
```bash
# This command must work by end of week 2:
grpcurl -d '{"audio_path": "sample.wav"}' \
  your-cluster:443 vox.v1.Transcribe/TranscribeFile
# Returns: {"transcript": "hello world", "duration_ms": 1847}
# Grafana shows the request in the dashboard
```
 
**Scope traps to avoid:**
- Don't add streaming yet (that's iteration 1)
- Don't write the operator yet (that's iteration 2)
- Don't add VAD (iteration 1)
- Don't build a CLI beyond curl/grpcurl
- Don't touch TLS/auth (iteration 4)
  
**Repo structure after this iteration:**
```
voxplatform/
├── cmd/
│   └── gateway/main.go
├── internal/
│   └── gateway/
│       ├── server.go          # gRPC server
│       └── stt_client.go      # HTTP client to faster-whisper
├── api/
│   └── proto/vox/v1/          # protobuf definitions
├── deploy/
│   ├── helm/
│   │   ├── gateway/
│   │   └── faster-whisper/
│   └── terraform/
│       └── hetzner/
├── docs/
│   ├── adr/
│   │   ├── 001-why-cpu-first.md
│   │   └── 002-why-faster-whisper.md
│   └── journal/
├── hack/                       # helper scripts
│   └── sample-audio/
├── MAYBE_LATER.md
├── go.mod
└── README.md
```

 
---
 
### Iteration 1 — Streaming and VAD (weeks 3–4)
 
**Goal:** Real-time streaming transcription with voice activity detection —
audio streams in live, partial transcripts come back as you speak.
 
**Why this serves the sentence:** "serves open-source voice AI models" —
this is what serving actually means for voice: streaming, not batch.
 
**Deliverables:**
1. gRPC bidirectional streaming: client sends 20ms PCM frames, server
   returns TranscriptEvent (partial or final)
2. Silero VAD integrated in gateway — audio only forwarded to STT when
   speech detected (CPU-efficient)
3. Python CLI client: `voxcli record` streams mic audio to gateway,
   prints transcripts live
4. Metrics added: VAD voiced-ratio, STT time-to-first-partial,
   audio-seconds-processed
5. OTel tracing: one span per request, child spans for VAD and STT
6. ADR-003: "Streaming architecture — chunked vs continuous"
**Verification checkpoint:**
```bash
# This must work:
python voxcli/record.py --server your-cluster:443
# Speak into mic → see partial transcripts appear in <500ms
# Grafana shows VAD voiced-ratio metric (should be ~40-60% for
# normal speech, proving VAD is filtering silence)
```
 
**Scope traps:**
- Don't add WebSocket transport yet (gRPC only for now)
- Don't optimize latency — correctness first
- Don't add multiple STT model options (one model, one config)
---
 
### Iteration 2 — The Operator (weeks 5–6)
 
**Goal:** A Kubernetes operator in Go that turns VoiceModel CRDs into
running inference deployments — the platform engineering core.
 
**Why this serves the sentence:** "operated via CRDs" — this IS the
sentence. This iteration is the project's reason for existing.
 
**Deliverables:**
1. Kubebuilder project: `VoiceModel` CRD with fields:
   - `modelName` (e.g., "faster-whisper-small-en")
   - `image` (container image)
   - `replicas`, `resources` (CPU/memory/GPU requests)
   - `quantization` (int8, fp16, etc.)
   - `metrics.port`, `metrics.path`
2. Reconciliation controller that creates/updates:
   - Deployment (with resource requests from CRD)
   - Service (ClusterIP)
   - ServiceMonitor (for Prometheus auto-discovery)
   - HPA (based on CPU or custom metrics)
3. Status subresource: `Ready`, `ModelLoaded`, `Endpoint`
4. Helm chart for the operator itself
5. The existing faster-whisper deployment is now managed by a VoiceModel CR
6. ADR-004: "CRD design — VoiceModel schema decisions"
7. ADR-005: "Why Kubebuilder over raw client-go"
**Verification checkpoint:**
```yaml
# Apply this, operator creates everything:
apiVersion: vox.platform/v1alpha1
kind: VoiceModel
metadata:
  name: whisper-small-en
spec:
  modelName: faster-whisper-small-en
  image: fedirz/faster-whisper-server:latest
  replicas: 1
  resources:
    requests:
      cpu: "4"
      memory: "4Gi"
  quantization: int8
  metrics:
    port: 8000
    path: /metrics
```
```bash
kubectl get voicemodel
# NAME               READY   ENDPOINT                        AGE
# whisper-small-en   True    whisper-small-en.vox.svc:8000   2m
```
 
**Scope traps:**
- Don't add InferencePipeline CRD yet (iteration 3)
- Don't add GPU support yet (iteration 5)
- Don't over-engineer status conditions — Ready/NotReady is enough
- Don't build admission webhooks yet
---
 
### Iteration 3 — Pipeline Composition and Event Sourcing (weeks 7–8)
 
**Goal:** Multi-model pipeline declared as a CRD — audio flows through
STT → diarize → summarize, with every step logged as an event.
 
**Why this serves the sentence:** "multi-model pipeline composition" and
the event log enables "built-in eval" later.
 
**Deliverables:**
1. `InferencePipeline` CRD:
   ```yaml
   apiVersion: vox.platform/v1alpha1
   kind: InferencePipeline
   metadata:
     name: call-intelligence
   spec:
     stages:
       - name: stt
         modelRef: whisper-small-en
         type: streaming
       - name: diarize
         modelRef: pyannote-speaker
         type: batch
         trigger: on-utterance-end
       - name: summarize
         modelRef: qwen-3b
         type: batch
         trigger: on-pipeline-end
   ```
2. Operator reconciles InferencePipeline into gateway routing config
3. Gateway routes requests through stages based on pipeline spec
4. Deploy pyannote-audio for speaker diarization (CPU, batch mode)
5. Deploy llama.cpp server with Qwen 2.5 3B Q4_K_M for summarization
6. Event log: append-only typed events per request:
   - `RequestStarted`, `VADSpeechDetected`, `STTPartialTranscript`,
     `STTFinalTranscript`, `SpeakerIdentified`, `SummaryGenerated`,
     `RequestCompleted`
7. Replay CLI: `voxctl replay <request-id>` replays events from log
8. ADR-006: "Event sourcing for inference — why append-only"
9. ADR-007: "Pipeline composition — declarative vs imperative"
**Verification checkpoint:**
```bash
# Submit a 2-minute audio file:
voxcli transcribe --pipeline call-intelligence sample-call.wav
 
# Output includes:
# - Full transcript with timestamps
# - Speaker labels (Speaker_0, Speaker_1)
# - Summary with action items
 
# Replay works:
voxctl replay req-abc123
# Prints all events in order with timestamps
```
 
**Scope traps:**
- Don't build a custom event store — JSONL files on MinIO/local disk
- Don't add more than 3 pipeline stages
- Don't optimize diarization quality (just prove the pipeline works)
- Don't add conditional branching in pipelines
---
 
### Iteration 4 — Eval Harness (weeks 9–10)
 
**Goal:** Automated quality regression testing — every model config change
is tested against a held-out dataset before it ships.
 
**Why this serves the sentence:** "built-in eval" — the thing that makes
this a platform, not a deployment script.
 
**Deliverables:**
1. `EvalRun` CRD:
   ```yaml
   apiVersion: vox.platform/v1alpha1
   kind: EvalRun
   metadata:
     name: whisper-small-regression
   spec:
     modelRef: whisper-small-en
     dataset: librispeech-test-clean
     metrics: [wer, cer]
     baseline:
       wer: 0.045
       regressionThreshold: 0.05  # fail if WER degrades >5%
   ```
2. Operator creates Argo Workflow that:
   - Pulls dataset from MinIO/local storage
   - Runs inference on every sample
   - Computes WER (word error rate) and CER (character error rate)
   - Writes results to Postgres (model_version, dataset, metric, value, timestamp)
   - Updates EvalRun status: Pass/Fail with metric values
3. Grafana dashboard: eval metrics over time, per model
4. GitHub Actions / Gitea Actions integration: eval runs on PR that
   changes model config, blocks merge on regression
5. LibriSpeech test-clean subset (100 samples) as first eval dataset
6. For LLM summarization: LLM-as-judge eval + ROUGE scores
7. ADR-008: "Eval design — what to measure and why"
8. Blog post: "How much does a minute of CPU transcription actually cost?"
**Verification checkpoint:**
```bash
kubectl get evalrun
# NAME                       STATUS   WER      BASELINE   RESULT
# whisper-small-regression   Pass     0.043    0.045      pass
 
# Change model to a worse config, re-run:
kubectl get evalrun
# NAME                       STATUS   WER      BASELINE   RESULT
# whisper-small-regression   Fail     0.089    0.045      regression
```
 
**Scope traps:**
- Don't build a custom eval framework — jiwer for WER, rouge-score for ROUGE
- Don't eval more than 100 samples (speed over completeness)
- Don't add real-time eval (batch only)
- Don't build a leaderboard UI
---
 
### Iteration 5 — GPU Support and Model Lifecycle (weeks 11–12)
 
**Goal:** Add GPU as a compute class — the platform absorbs a new
accelerator without breaking the abstraction.
 
**Why this serves the sentence:** "Kubernetes-native inference platform" that
handles both CPU and GPU workloads via the same CRD interface.
 
**Deliverables:**
1. Terraform module for Hetzner GPU node (GEX44) OR
   documentation for adding any GPU node to RKE2
2. NVIDIA GPU Operator deployed via Helm
3. VoiceModel CRD extended with GPU fields:
   ```yaml
   resources:
     requests:
       nvidia.com/gpu: "1"
     limits:
       nvidia.com/gpu: "1"
   ```
4. New VoiceModel: whisper-large-v3 on vLLM (GPU)
5. New VoiceModel: Qwen 2.5 7B on vLLM (GPU, replacing llama.cpp 3B)
6. Eval comparison: CPU-whisper-small vs GPU-whisper-large (WER, latency, cost)
7. GPU metrics in Grafana: DCGM exporter (utilization, memory, temperature)
8. Cost-per-request calculation: CPU-seconds AND GPU-seconds
9. ADR-009: "Adding GPU support — what changed and what didn't"
10. Blog post: "CPU vs GPU voice inference — the numbers"
**Verification checkpoint:**
```bash
kubectl get voicemodel
# NAME                READY   ACCELERATOR   LATENCY-P99   COST/REQ
# whisper-small-en    True    cpu           1200ms         $0.0003
# whisper-large-v3    True    gpu           180ms          $0.0008
# qwen-7b            True    gpu           340ms          $0.0012
```
 
**Scope traps:**
- Don't add multi-GPU / tensor parallelism (single GPU only)
- Don't add MIG or GPU time-slicing
- Don't optimize vLLM config beyond defaults
- If GPU budget isn't available yet, write the Terraform + operator
  code and test with a mock, then add real GPU when budget allows
---
 
### Iteration 6 — Production Hardening (weeks 13–14)
 
**Goal:** Make the platform reliable enough that you'd trust it with real
workloads — canary deploys, cost tracking, and proper docs.
 
**Why this serves the sentence:** "self-hosted inference platform" — the word
"platform" implies it's reliable, documented, and operable.
 
**Deliverables:**
1. Canary deployment via Argo Rollouts:
   - New model version gets 10% traffic
   - Auto-promotes if latency and error rate stay within thresholds
   - Auto-rolls-back on regression
2. Per-request cost tracking:
   - Gateway annotates each request with CPU-seconds and GPU-seconds
   - Cost computed from instance price / total capacity
   - Exposed as Prometheus metric and in event log
3. Health checks and readiness probes on all components
4. Proper error handling in gateway (timeout, retry, circuit breaker)
5. Grafana dashboard: "Platform Overview" — all models, all pipelines,
   total cost, request volume, error rate
6. README rewrite: architecture diagram, quickstart, CRD reference
7. ADR index page
8. Blog post: "Building a K8s operator for voice AI inference"
9. Demo video: 3-minute walkthrough
**Verification checkpoint:**
- Deploy a new whisper model version → watch canary promotion in Argo
- Intentionally deploy a bad model → watch auto-rollback
- README quickstart works from scratch on a fresh cluster
- Demo video recorded and published
**Scope traps:**
- Don't add multi-tenancy (that's a future project)
- Don't build a custom dashboard (Grafana only)
- Don't add authentication beyond what you have
- Don't start marketing / Product Hunt before the demo is solid
---
 
## The verification ritual — do this every Sunday
 
Every week, before you write any code on Monday, do this 10-minute check:
 
### 1. Read THE sentence out loud
> "Voxplatform is a self-hosted, Kubernetes-native inference platform —
> operated via CRDs and deployed via GitOps — that serves open-source
> voice AI models with built-in eval, observability, and multi-model
> pipeline composition."
 
### 2. Ask three questions:
- **What did I ship this week that serves the sentence?**
  If the answer is "nothing" two weeks in a row, you've drifted.
- **What am I tempted to build that DOESN'T serve the sentence?**
  Write it in `MAYBE_LATER.md` and move on.
- **Can I demo what I have right now?**
  If not, prioritize making it demoable over adding features.
### 3. Write one journal entry
`docs/journal/YYYY-MM-DD.md` — 5 minutes, unpolished:
- What worked
- What broke
- What I learned
- What's next
### 4. Check the MAYBE_LATER.md
If it has more than 10 items, you're being tempted too often.
Tighten your focus.
 
---
 
## MAYBE_LATER.md — scope graveyard (start this on day 1)
 
```markdown
# Ideas that don't serve THE sentence right now
 
- [ ] WebSocket transport (gRPC is enough for v1)
- [ ] TTS / text-to-speech pipeline
- [ ] Web UI / React dashboard
- [ ] Multi-tenant namespace isolation
- [ ] Fine-tuning pipeline
- [ ] Custom VAD model training
- [ ] SIP/telephony integration
- [ ] Voice cloning
- [ ] Multi-region deployment
- [ ] Billing / usage metering
- [ ] Terraform Cloud integration
```
 
---
 
## Writing schedule — the portfolio artifacts
 
| Week | ADR | Blog post |
|------|-----|-----------|
| 1 | 001: Why CPU-first | — |
| 2 | 002: Why faster-whisper | — |
| 4 | 003: Streaming architecture | — |
| 6 | 004-005: CRD design, Kubebuilder | — |
| 8 | 006-007: Event sourcing, pipelines | — |
| 10 | 008: Eval design | "CPU transcription cost analysis" |
| 12 | 009: Adding GPU support | "CPU vs GPU inference — the numbers" |
| 14 | — | "Building a K8s operator for voice AI" |
 
---
 
## Tech stack — locked decisions (do not revisit)
 
| Component | Choice | Why | Locked until |
|-----------|--------|-----|-------------|
| Language (operator) | Go | K8s ecosystem, your strength | Forever |
| Language (gateway) | Go | Performance, gRPC native | Forever |
| Language (CLI client) | Python | sounddevice, fast prototyping | Forever |
| Operator framework | Kubebuilder | Industry standard | Forever |
| STT v0 | faster-whisper (small.en) | CPU-capable, good quality | Iteration 5 |
| STT v1 | vLLM + whisper-large-v3 | GPU, production quality | — |
| LLM v0 | llama.cpp + Qwen 3B Q4 | CPU-capable | Iteration 5 |
| LLM v1 | vLLM + Qwen 7B | GPU | — |
| Diarization | pyannote-audio | Only real open option | Forever |
| VAD | Silero VAD | Fast, accurate, CPU | Forever |
| Metrics | Prometheus + Grafana | You know it already | Forever |
| Traces | OpenTelemetry + Tempo | Industry standard | Forever |
| GitOps | ArgoCD | You know it already | Forever |
| IaC | Terraform | You know it already | Forever |
| Eval: WER | jiwer library | Standard, simple | Forever |
| Eval: LLM | ROUGE + LLM-as-judge | Good enough | Forever |
| Eval orchestration | Argo Workflows | K8s native | Forever |
| Event log | JSONL on local/MinIO | Simple, replayable | Forever |
 
"Locked until" means: do NOT reconsider this decision before that iteration.
Decision fatigue kills side projects.
 
---
 
## Risk register — what actually kills this project
 
| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Lose motivation in weeks 3-5 | High | Dogfood it (transcribe your own meetings). Demo to a friend every 2 weeks. |
| Scope creep into voice agents | High | THE sentence. MAYBE_LATER.md. |
| GPU budget delays | Medium | CPU-first design. Iteration 5 is optional — project is complete at iteration 4. |
| Kubebuilder learning curve | Medium | Budget 3 full days for tutorials before writing operator code. |
| faster-whisper-server API changes | Low | Pin image version in Helm chart. |
| Burnout from evening/weekend work | Medium | Max 15 hrs/week. Skip a week if needed — momentum > speed. |
 
---
 
## Success criteria — how you know the project is done
 
The project is "done" when ALL of these are true:
 
1. `helm install voxplatform` on a fresh K8s cluster gives you a working
   voice inference platform in under 10 minutes
2. A VoiceModel CRD creates a running inference endpoint with metrics
3. An InferencePipeline CRD composes multiple models into a flow
4. An EvalRun CRD runs WER regression tests and blocks bad models
5. A Grafana dashboard shows per-model latency, throughput, and cost
6. The README has a quickstart that a stranger can follow
7. You can explain every component in a 45-minute interview
8. At least 2 blog posts are published
9. A 3-minute demo video exists
When these 9 things are true, you have a portfolio piece that gets you
interviews at Baseten, Modal, Isomorphic, Databricks, and Datadog.
Everything after that is bonus.

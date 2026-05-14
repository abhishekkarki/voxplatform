---
hide:
  - navigation
  - toc
---

# VoxPlatform

**A Kubernetes-native voice AI inference platform.** Streaming speech-to-text, speaker diarization, and summarization on GKE — operated via CRDs, deployed via GitOps, with built-in eval and observability.

[Get started in 5 minutes :material-arrow-right:](tutorial/first-transcription.md){ .md-button .md-button--primary }
[View the source on GitHub :material-github:](https://github.com/abhishekkarki/voxplatform){ .md-button }

---

## What it does

```mermaid
flowchart LR
    A[Audio in<br/>mic or file] --> B[Go gateway<br/>:8080]
    B --> C[VAD sidecar<br/>Silero :8001]
    B --> D[Whisper STT<br/>VoiceModel :8000]
    D --> E[Diarizer<br/>pyannote :8002]
    E --> F[Summarizer<br/>Qwen 3B :8003]
    F --> G[Transcript +<br/>speakers + summary]

    style A fill:#eef
    style G fill:#efe
```

- **Batch and streaming** — `POST /v1/audio/transcriptions` for files, `WS /v1/audio/stream` for live audio with VAD.
- **Multi-stage pipeline** — `POST /v1/pipeline/run` runs STT → speaker diarization → summarization as a single call.
- **Declarative model serving** — apply a `VoiceModel` CR and the operator reconciles a Deployment + Service. `InferencePipeline` aggregates readiness across all stages.
- **Append-only event log** — every pipeline request writes JSONL to GCS for replay and debugging.
- **Continuous evaluation** — `jiwer`-based WER computed on a held-out dataset on every PR.

## How these docs are organised

This site follows the [Diataxis](https://diataxis.fr/) framework. Four sections, four different jobs:

<div class="grid cards" markdown>

-   :material-school: **[Tutorial](tutorial/index.md)**

    Learning-oriented. Start here if you've never used Vox. We'll get you from `git clone` to a live transcription in five minutes.

-   :material-tools: **[How-to guides](how-to/index.md)**

    Task-oriented. Recipes for the things you'll actually do — deploy a model, run the eval harness, scale the cluster, use the SDK.

-   :material-book-open-variant: **[Reference](reference/index.md)**

    Information-oriented. The HTTP API, the WebSocket protocol, the `VoiceModel` CRD spec, the Python SDK, the `vox` CLI. Look things up here.

-   :material-lightbulb-on: **[Explanation](explanation/index.md)**

    Understanding-oriented. Why a VAD sidecar instead of in-process VAD? Why an operator? What did we get wrong the first time? Read for context, not for instructions.

</div>

## Repository at a glance

| Component | Path | Stack |
|-----------|------|-------|
| HTTP/WS gateway | `gateway/` | Go, gorilla/websocket, Prometheus |
| VAD sidecar | `vad/` | Python, Silero |
| Model serving | Helm chart | faster-whisper |
| Operator | `operator/` | Kubebuilder, controller-runtime |
| Python SDK + CLI | `sdk/` | httpx, click |
| Evaluation | `eval/` | jiwer v4 |
| Infrastructure | `infra/` | Terraform, GKE, ArgoCD |

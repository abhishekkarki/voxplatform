# Pipeline Design — How the Inference Chain Works

This document explains the multi-stage inference pipeline added in iteration 3: what each component does, how they connect, and why the system is designed the way it is.

---

## The problem

A raw audio recording contains three kinds of information that are useful separately:

1. **What was said** — the transcript (Whisper)
2. **Who said what** — speaker identity per sentence (pyannote)
3. **What it means** — a concise summary (Qwen)

These require three different models, and each model has different hardware requirements, startup time, and failure modes. The challenge is exposing them as a single coherent operation without coupling the models together.

---

## The two-layer design

VoxPlatform splits pipeline concerns across two layers:

```
Control plane (Kubernetes)          Data plane (per-request)
─────────────────────────           ──────────────────────────────────────────
InferencePipeline CRD               POST /v1/pipeline/run
  └─ stage: stt → whisper-small       → STT (Whisper :8000)
  └─ stage: diarize → pyannote          → Diarize (pyannote :8002)
  └─ stage: summarize → qwen-3b           → Summarize (Qwen :8003)
     status: Ready (3/3)             ← unified JSON response
```

**The CRD answers:** "Are all three models deployed and healthy right now?"  
**The gateway answers:** "Run this audio through all three and give me the result."

These are independent questions. The CRD can be Ready while a specific request is in-flight. The gateway can run requests even if the CRD controller is restarting.

---

## Stage 1 — STT (Speech-to-Text)

**Service:** `faster-whisper-server` (existing from iterations 0–1)  
**Model:** `Systran/faster-whisper-small.en` (CPU, int8 quantized)  
**Input:** raw audio file (WAV, MP3, etc.)  
**Output:** full transcript text

The gateway already has a direct proxy to Whisper from the `POST /v1/audio/transcriptions` endpoint. The pipeline handler reuses the same `WhisperProxy` — the audio bytes are read into memory once and passed as a `bytes.Reader` so the proxy can reconstruct the multipart form.

STT is the **only non-optional stage**. If Whisper fails, there's no transcript, and the pipeline aborts with a 502. All other stages are best-effort.

---

## Stage 2 — Diarization (Speaker Labelling)

**Service:** `diarizer` (new in iteration 3)  
**Model:** `pyannote/speaker-diarization-3.1` (requires HuggingFace token)  
**Input:** same audio file  
**Output:** list of `{start, end, speaker}` time segments

Diarization runs on the same audio bytes as STT. The two stages run sequentially (not in parallel) because the merge step that assigns transcript words to speakers needs both outputs.

**Graceful degradation:** if `HF_TOKEN` is not set, the diarizer runs in single-speaker fallback mode — it returns a single `SPEAKER_00` segment covering the whole audio. The pipeline continues without breaking. This is intentional: diarization is a "nice to have" that shouldn't make the whole pipeline unavailable.

**Fallback mode is also useful for testing:** you can run the full pipeline locally without a HuggingFace account.

---

## Stage 3 — Summarization

**Service:** `summarizer` (new in iteration 3)  
**Model:** `Qwen/Qwen2.5-3B-Instruct-GGUF` (Q4_K_M quantization, CPU)  
**Input:** transcript text + optional speaker segments  
**Output:** concise summary (≤3 sentences)

The summarizer receives the transcript from stage 1 and, if available, the speaker segments from stage 2. It builds a speaker-aware prompt when segments have `text` fields (e.g., "SPEAKER_00: Hello world"). When segments are only time-based (no text), it summarizes the plain transcript.

The Qwen 3B model (~2GB GGUF) downloads to a persistent volume on first startup. Subsequent starts load from the volume (~30s load time on CPU).

**Graceful degradation:** if llama-cpp-python isn't installed or the model isn't available, the summarizer returns an extractive summary — the first 3 sentences of the transcript. The pipeline still returns a valid `summary` field.

---

## Event log

Every pipeline request writes an append-only JSONL log. Each line is one event:

```
pipeline.start → stage.start(stt) → stage.complete(stt) 
              → stage.start(diarize) → stage.complete(diarize)
              → stage.start(summarize) → stage.complete(summarize)
              → pipeline.complete
```

Stage failures write `stage.error` events and the pipeline continues (unless the failed stage is STT).

**Local development:** events write to `/tmp/vox-events/<request_id>.jsonl`  
**Production (GKE):** events write to `gs://<bucket>/events/<date>/<request_id>.jsonl` via the GCS REST API using Workload Identity for authentication.

The event log is the foundation for `voxctl replay <request-id>` in a later iteration.

---

## InferencePipeline CRD — what the operator does

The operator watches `InferencePipeline` objects and, for each stage, looks up the referenced `VoiceModel` to check its phase. It does **not** create any Kubernetes resources — the VoiceModels already own their Deployments and Services.

The CRD's status shows aggregate health:

```
$ kubectl get inferencepipelines -n vox
NAME       PHASE     MESSAGE              AGE
default    Ready     3/3 stages ready     2d
```

If `whisper-small` (the STT VoiceModel) is restarting, the pipeline shows `Degraded` with a message explaining which stage is not ready. When the VoiceModel returns to Ready, a watch event triggers an immediate pipeline reconcile — you don't have to wait for the requeue timer.

---

## Request flow — end to end

```
Client
  POST /v1/pipeline/run
  file=meeting.wav
  stages=stt,diarize,summarize    ← optional; default is all three
       │
       ▼
Gateway (port 8080)
  1. Parse multipart form, read audio bytes once
  2. Log pipeline.start event
  3. STT: forward audio to Whisper → get transcript
     Log stage.start / stage.complete
  4. Diarize: POST audio to diarizer → get segments
     Log stage.start / stage.complete (or stage.error if diarizer down)
  5. Summarize: POST {transcript, segments} to summarizer → get summary
     Log stage.start / stage.complete (or stage.error)
  6. Log pipeline.complete
  7. Return unified JSON:
     {
       "transcript": "...",
       "segments":   [{start, end, speaker}, ...],
       "summary":    "...",
       "stages":     {stt: {success, duration}, ...},
       "processing_time_seconds": 12.5
     }
```

---

## Running a subset of stages

The `stages` form field selects which stages to run:

```bash
# Full pipeline
vox transcribe --pipeline meeting.wav

# STT only (no diarization, no summarization)
curl -X POST localhost:8080/v1/pipeline/run \
  -F file=@meeting.wav -F stages=stt

# STT + diarization (skip summarization)
curl -X POST localhost:8080/v1/pipeline/run \
  -F file=@meeting.wav -F stages=stt,diarize
```

If a requested stage has no backend URL configured (e.g., `DIARIZER_URL` is empty), that stage is silently skipped.

---

## Local development setup

The docker-compose stack includes all four services. Diarizer and summarizer start in fallback mode by default, so the pipeline works even without a HuggingFace token or a downloaded GGUF model:

```bash
docker compose up --build

# Test the full pipeline (returns transcript + single-speaker segments + extractive summary)
curl -X POST localhost:8080/v1/pipeline/run -F file=@test.wav | jq

# Enable full diarization (requires HF token and model acceptance)
HF_TOKEN=hf_... docker compose up
```

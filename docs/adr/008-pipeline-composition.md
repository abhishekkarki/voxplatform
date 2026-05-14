# ADR-008: Pipeline Composition — InferencePipeline CRD + Gateway Orchestration

**Status:** Accepted  
**Date:** 2026-05  
**Iteration:** 3

---

## Context

VoxPlatform needs to run multi-step inference: audio goes in, and the result is a transcript, speaker labels, and a summary. That means chaining three models: Whisper (STT), pyannote (diarization), and Qwen (summarization).

The design question is: **who chains them, and how is that expressed?**

## Options Considered

### Option A: Client-side chaining
The client calls each service separately and assembles the result.

**Problem:** Every client has to re-implement the chaining logic. If we change the pipeline (add a stage, change retry policy), every client breaks. The client also needs to know all three service URLs, which leaks infrastructure details.

### Option B: Argo Workflows
Declare the pipeline as a DAG of container steps in Argo Workflows. Each node is a containerized function.

**Problem:** Argo Workflows is powerful but heavyweight for this stage. It requires installing the Argo controller, defining Workflow templates, managing step artifacts, and handling retries at the Argo level. Each request would create a new Workflow object in etcd. Overkill for a 3-step sequential pipeline.

### Option C: Gateway-orchestrated pipeline + InferencePipeline CRD (chosen)

Split the problem in two:

1. **`InferencePipeline` CRD** (operator layer): declares that a set of named VoiceModels should exist and be Ready together. The operator reconciles this and reports aggregate health. This is the *control plane* view — it answers "is my pipeline deployed and healthy?"

2. **Gateway pipeline handler** (data plane): at request time, the gateway calls STT → diarize → summarize in sequence, collects results, and returns a single unified response. This is the *data plane* execution.

## Decision

We use Option C: InferencePipeline CRD + gateway orchestration.

**Why separate control plane and data plane:**  
The CRD and the runtime are different concerns. The CRD is about *what should exist* and *is it healthy*. The gateway is about *running a request right now*. Mixing them (e.g., having the CRD controller actually proxy requests) would make both harder to reason about.

**Why the gateway orchestrates, not a separate service:**  
The gateway already handles request ID propagation, structured logging, Prometheus metrics, and error formatting. Moving orchestration elsewhere would require duplicating all of that. The gateway is the right place to sequence HTTP calls between services.

**Why sequential, not parallel:**  
STT must complete before diarization can start (diarizer takes the same audio but the transcript is needed to assign text to speaker segments in the merge step). Summarization requires both. True DAG execution is deferred to Argo Workflows in iteration 4.

**Why the pipeline stages are non-fatal except STT:**  
If diarization fails (model not ready, pyannote not configured), the pipeline still returns a transcript and summary — just without speaker labels. If summarization fails, we still return transcript + segments. Only STT failure aborts the pipeline, because there's nothing to return without a transcript.

**Why InferencePipeline doesn't own Deployments:**  
The individual VoiceModels already own their Deployments. The InferencePipeline is a *reference* to existing VoiceModels, not a new owner. This avoids circular ownership and keeps the resource graph clean.

## Consequences

- `kubectl get inferencepipelines` gives operators a quick health view of the whole pipeline.
- The gateway's `POST /v1/pipeline/run` is a single endpoint for clients — they don't need to know about the three underlying services.
- Adding a 4th stage (e.g., translation) means: add a new VoiceModel, update the InferencePipeline spec, update the gateway pipeline handler. No client changes.
- The `stages` form field lets callers run a subset: `stages=stt` for transcription-only, `stages=stt,diarize` to skip summarization.
- In iteration 4, `EvalRun` CRD will trigger Argo Workflows that call `POST /v1/pipeline/run` as part of WER regression testing.

# ADR-007: Append-only Event Log for Inference Requests

**Status:** Accepted  
**Date:** 2026-05  
**Iteration:** 3

---

## Context

When a pipeline request runs through STT → diarize → summarize, several things can go wrong:

- The diarizer returns wrong speaker counts
- The summarizer produces a hallucinated result
- A stage times out and the gateway retries
- A model is swapped mid-day and quality drops

Without a record of what actually happened for each request, diagnosing these issues means staring at log lines and guessing. We need a way to **replay** exactly what happened for any given request, and to **correlate** inputs, outputs, and timings across all stages.

The question is: what form should that record take?

## Options Considered

### Option A: Structured logs only
Write each stage's input/output into the existing slog JSON stream.

**Problem:** Logs are write-once and hard to query per-request. A pipeline request spans 10–30 log lines across multiple goroutines. Reassembling them requires a log query tool and correlation by request ID. You can't "replay" a request from logs alone because the raw audio isn't there.

### Option B: Distributed tracing (OTel spans)
Emit an OTel trace per pipeline request with child spans per stage.

**Problem:** Traces are great for latency analysis but not for data inspection. Traces don't store input/output payloads — only timing and error status. You still can't answer "what transcript did the STT produce for this request?"

### Option C: Append-only JSONL event log per request (chosen)
For every pipeline request, write a JSONL file (one JSON object per line) to GCS. Each event captures: request ID, timestamp, event type, stage name, and stage-specific payload.

```jsonl
{"request_id":"a1b2","type":"pipeline.start","timestamp":"...","data":{"stages":["stt","diarize","summarize"]}}
{"request_id":"a1b2","type":"stage.start","stage":"stt","timestamp":"..."}
{"request_id":"a1b2","type":"stage.complete","stage":"stt","timestamp":"...","data":{"duration_seconds":2.1}}
{"request_id":"a1b2","type":"stage.start","stage":"diarize","timestamp":"..."}
{"request_id":"a1b2","type":"stage.complete","stage":"diarize","timestamp":"...","data":{"num_segments":3}}
{"request_id":"a1b2","type":"stage.complete","stage":"summarize","timestamp":"...","data":{"duration_seconds":5.2}}
{"request_id":"a1b2","type":"pipeline.complete","timestamp":"...","data":{"duration_seconds":9.8}}
```

## Decision

We use the append-only JSONL event log (Option C).

**Why JSONL:**  
One event per line means the file is readable with `cat`, processable with `jq`, and streamable — you don't need to parse a full JSON document to read the first event.

**Why GCS:**  
GCS is already in the infrastructure (it exists for eval datasets). Appending a 1KB file costs essentially nothing. In production the operator runs on GKE with Workload Identity, so no credentials file is needed — the gateway fetches a token from the metadata server.

**Why one file per request:**  
Partitioning by request ID means looking up a single request is a single object GET: `gsutil cat gs://vox-artifacts/events/2026-05-14/a1b2c3d4.jsonl`. No query needed.

**Why not a database:**  
Postgres or BigQuery would require a new infra dependency. GCS is append-only and already provisioned. Structured query over events (e.g., "average STT latency last week") can be done with BigQuery's external table feature pointing at the GCS bucket — no schema migration needed.

## Consequences

- Every pipeline request writes ~10 small events to GCS. At 1,000 requests/day that's 10,000 writes (~$0.05/day).
- Local development uses a local file backend (`/tmp/vox-events/`) — no GCS needed during iteration.
- The `EVENT_LOG_BACKEND` env var selects the backend: `"local"` (default) or `"gcs"`.
- Stage errors are logged as `stage.error` events, so a failed pipeline still has a partial log.
- This is the foundation for the `voxctl replay <request-id>` command in a later iteration.

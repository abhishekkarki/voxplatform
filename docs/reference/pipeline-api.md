# Pipeline API Reference

## POST /v1/pipeline/run

Runs an audio file through the multi-stage inference pipeline (STT → diarize → summarize). Returns a single unified response containing all stage outputs.

### Request

**Content-Type:** `multipart/form-data`

| Field    | Type   | Required | Description |
|----------|--------|----------|-------------|
| `file`   | file   | yes      | Audio file. Supported: `.wav`, `.mp3`, `.m4a`, `.ogg`, `.flac`, `.webm`. Max 32MB. |
| `model`  | string | no       | STT model ID (default: `Systran/faster-whisper-small.en`) |
| `language` | string | no    | BCP-47 language code for STT (default: auto-detect) |
| `stages` | string | no       | Comma-separated list of stages to run. Default: `stt,diarize,summarize`. |

**`stages` examples:**
- `stt` — transcription only
- `stt,diarize` — transcription + speaker labels, no summary
- `stt,summarize` — transcription + summary, no diarization

### Response `200 OK`

```json
{
  "request_id":              "a5a0c665250590f7",
  "created_at":              "2026-05-14T10:00:00Z",
  "processing_time_seconds": 9.4,
  "stages": {
    "stt": {
      "success":          true,
      "duration_seconds": 2.1
    },
    "diarize": {
      "success":          true,
      "duration_seconds": 1.8
    },
    "summarize": {
      "success":          false,
      "duration_seconds": 0.1,
      "error":            "summarizer returned 503"
    }
  },
  "transcript": "The quick brown fox jumps over the lazy dog.",
  "segments": [
    { "start": 0.0, "end": 2.3, "speaker": "SPEAKER_00" },
    { "start": 2.5, "end": 5.1, "speaker": "SPEAKER_01" }
  ],
  "summary": ""
}
```

**Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `request_id` | string | Unique request identifier, included in all event log entries. |
| `created_at` | string | RFC3339 timestamp when the pipeline started. |
| `processing_time_seconds` | float | Total wall-clock time from request receipt to response. |
| `stages` | object | Per-stage result. Keys match the requested stages. |
| `stages.<name>.success` | bool | Whether the stage completed without error. |
| `stages.<name>.duration_seconds` | float | Wall-clock time spent in this stage. |
| `stages.<name>.error` | string | Error message if `success` is false. |
| `transcript` | string | Full transcription from the STT stage. Empty if STT was not requested. |
| `segments` | array | Speaker segments from the diarize stage. Empty array if diarize was not run or failed. |
| `segments[].start` | float | Segment start time in seconds. |
| `segments[].end` | float | Segment end time in seconds. |
| `segments[].speaker` | string | Speaker label (e.g., `SPEAKER_00`). |
| `segments[].text` | string | Transcript text for this segment, if available. |
| `summary` | string | Summary from the summarize stage. Empty string if not run or failed. |

### Error responses

If STT fails, the pipeline aborts and returns an error:

```json
{
  "error": {
    "code":       "backend_error",
    "message":    "STT stage failed: whisper returned 503",
    "request_id": "a5a0c665250590f7"
  }
}
```

All other stage failures are **non-fatal** — the pipeline continues and the stage result has `"success": false`.

### Stage availability

Stages only run if their backend URL is configured:

| Stage | Gateway env var | Default | Notes |
|-------|----------------|---------|-------|
| `stt` | `WHISPER_URL` | `http://whisper.vox.svc.cluster.local:8000` | Always required |
| `diarize` | `DIARIZER_URL` | _(empty — skipped)_ | Skipped if URL not set |
| `summarize` | `SUMMARIZER_URL` | _(empty — skipped)_ | Skipped if URL not set |

In the docker-compose stack, all URLs are pre-configured to the service containers.

---

## Event log format

Every pipeline request emits JSONL events. The backend is configured via `EVENT_LOG_BACKEND`:

- `local` (default): writes to `$EVENT_LOG_DIR/<request_id>.jsonl`
- `gcs`: writes to `gs://$EVENT_LOG_BUCKET/events/<date>/<request_id>/<type>.jsonl`

**Event types:**

| Type | Description |
|------|-------------|
| `pipeline.start` | Pipeline started. `data.stages` lists requested stages. |
| `stage.start` | Stage began. `stage` names the stage. |
| `stage.complete` | Stage succeeded. `data.duration_seconds` is the stage time. |
| `stage.error` | Stage failed. `data.error` has the error message. |
| `pipeline.complete` | All stages finished. `data.duration_seconds` is total time. |
| `pipeline.error` | Pipeline aborted (only on STT failure). |

**Example event log for a full pipeline run:**

```jsonl
{"request_id":"a1b2","type":"pipeline.start","timestamp":"2026-05-14T10:00:00Z","data":{"stages":["stt","diarize","summarize"],"filename":"meeting.wav"}}
{"request_id":"a1b2","type":"stage.start","stage":"stt","timestamp":"2026-05-14T10:00:00.100Z"}
{"request_id":"a1b2","type":"stage.complete","stage":"stt","timestamp":"2026-05-14T10:00:02.200Z","data":{"duration_seconds":2.1,"text_length":44}}
{"request_id":"a1b2","type":"stage.start","stage":"diarize","timestamp":"2026-05-14T10:00:02.210Z"}
{"request_id":"a1b2","type":"stage.complete","stage":"diarize","timestamp":"2026-05-14T10:00:04.010Z","data":{"duration_seconds":1.8,"num_segments":2}}
{"request_id":"a1b2","type":"stage.start","stage":"summarize","timestamp":"2026-05-14T10:00:04.020Z"}
{"request_id":"a1b2","type":"stage.complete","stage":"summarize","timestamp":"2026-05-14T10:00:09.220Z","data":{"duration_seconds":5.2}}
{"request_id":"a1b2","type":"pipeline.complete","timestamp":"2026-05-14T10:00:09.230Z","data":{"duration_seconds":9.4}}
```

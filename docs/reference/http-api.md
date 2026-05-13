# HTTP API

Base URL: `http://<gateway-host>` (defaults to `:8080` in-cluster).

## `POST /transcribe`

Batch transcription of a complete audio file.

**Request**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `audio` | multipart file | yes | WAV, FLAC, or MP3, ≤25 MB |
| `model` | form field | yes | Name of a deployed `VoiceModel` |
| `language` | form field | no | ISO-639-1 hint, e.g. `en`, `de` |

**Response** `200 OK`

```json
{
  "text": "string",
  "segments": [
    { "start": 0.0, "end": 2.4, "text": "string" }
  ],
  "model": "whisper-tiny",
  "duration_sec": 12.3
}
```

**Errors**

| Status | Meaning |
|--------|---------|
| `400` | Invalid multipart payload or unsupported audio format |
| `404` | Requested model is not deployed |
| `413` | Audio file exceeds 25 MB |
| `503` | Model is `Pending` or `Failed` |

## `GET /health`

Liveness. Returns `200 OK` with an empty body when the gateway process is up.

## `GET /health/ready`

Readiness. Returns `200 OK` when at least one model reports `Ready` to the gateway's discovery loop.

## `GET /health/models`

Lists `VoiceModel` instances known to the gateway and their last-observed phase.

## `GET /metrics`

Prometheus exposition. See [Architecture overview](../explanation/architecture.md#observability) for the metric catalog.

# How to Run the Inference Pipeline

This guide covers running the full STT → diarize → summarize pipeline against a local Docker Compose stack or a GKE deployment.

---

## Prerequisites

- Docker Compose stack running: `docker compose up --build`
- Python SDK installed: `pip install -e ./clients/python`

---

## Run the full pipeline via CLI

```bash
vox transcribe --pipeline meeting.wav
```

This calls `POST /v1/pipeline/run` and prints:

```
Transcript:
  The quick brown fox jumps over the lazy dog.

Speakers:
  SPEAKER_00  0.0s – 2.3s
  SPEAKER_01  2.5s – 5.1s

Summary:
  A brief discussion about a fox and a dog.

---
Stages:  stt ✓ (2.1s)  diarize ✓ (1.8s)  summarize ✓ (5.2s)
Total:   9.4s  |  Request ID: a5a0c665250590f7
```

---

## Run the pipeline via curl

```bash
curl -X POST http://localhost:8080/v1/pipeline/run \
  -F file=@meeting.wav | jq
```

Response:

```json
{
  "request_id": "a5a0c665250590f7",
  "created_at": "2026-05-14T10:00:00Z",
  "processing_time_seconds": 9.4,
  "stages": {
    "stt":       {"success": true, "duration_seconds": 2.1},
    "diarize":   {"success": true, "duration_seconds": 1.8},
    "summarize": {"success": true, "duration_seconds": 5.2}
  },
  "transcript": "The quick brown fox jumps over the lazy dog.",
  "segments": [
    {"start": 0.0, "end": 2.3, "speaker": "SPEAKER_00"},
    {"start": 2.5, "end": 5.1, "speaker": "SPEAKER_01"}
  ],
  "summary": "A brief discussion about a fox and a dog."
}
```

---

## Run only specific stages

Pass `stages=stt,diarize` to skip summarization, or `stages=stt` for transcription only:

```bash
# Transcription + diarization (no summary)
curl -X POST http://localhost:8080/v1/pipeline/run \
  -F file=@meeting.wav -F stages=stt,diarize | jq .segments

# Transcription only
curl -X POST http://localhost:8080/v1/pipeline/run \
  -F file=@meeting.wav -F stages=stt | jq .transcript
```

---

## Enable full diarization (HuggingFace token required)

The diarizer runs in single-speaker fallback mode by default. To enable full speaker diarization:

1. Accept the model terms at https://huggingface.co/pyannote/speaker-diarization-3.1
2. Create a HuggingFace token at https://huggingface.co/settings/tokens
3. Pass the token when starting the stack:

```bash
export HF_TOKEN=hf_xxxxxxxxxxxxxxxx
docker compose up
```

---

## Deploy the pipeline on GKE

After the full iteration is validated locally, deploy each component:

```bash
# Build and push images
docker build --platform linux/amd64 -f services/diarizer/Dockerfile \
  -t europe-west3-docker.pkg.dev/voxplatform/vox-images-dev/diarizer:0.1.0 \
  services/diarizer
docker push europe-west3-docker.pkg.dev/voxplatform/vox-images-dev/diarizer:0.1.0

docker build --platform linux/amd64 -f services/summarizer/Dockerfile \
  -t europe-west3-docker.pkg.dev/voxplatform/vox-images-dev/summarizer:0.1.0 \
  services/summarizer
docker push europe-west3-docker.pkg.dev/voxplatform/vox-images-dev/summarizer:0.1.0

# Deploy
helm install diarizer deploy/helm/diarizer -n vox \
  --set env.HF_TOKEN=$HF_TOKEN

helm install summarizer deploy/helm/summarizer -n vox

# Upgrade gateway with new env vars
helm upgrade gateway deploy/helm/gateway -n vox \
  --set env.DIARIZER_URL=http://diarizer.vox.svc.cluster.local:8002 \
  --set env.SUMMARIZER_URL=http://summarizer.vox.svc.cluster.local:8003 \
  --set env.EVENT_LOG_BACKEND=gcs \
  --set env.EVENT_LOG_BUCKET=vox-artifacts
```

---

## Apply the InferencePipeline CRD

```yaml
# deploy/k8s/pipeline-default.yaml
apiVersion: vox.vox.io/v1alpha1
kind: InferencePipeline
metadata:
  name: default
  namespace: vox
spec:
  stages:
    - name: stt
      model: whisper-small
    - name: diarize
      model: pyannote-diarizer
    - name: summarize
      model: qwen-summarizer
```

```bash
kubectl apply -f deploy/k8s/pipeline-default.yaml

# Check pipeline health
kubectl get inferencepipelines -n vox
# NAME      PHASE   MESSAGE          AGE
# default   Ready   3/3 stages ready 2m
```

---

## Inspect event logs

**Local:**
```bash
# List recent pipeline event files
ls /tmp/vox-events/

# Inspect a specific request
cat /tmp/vox-events/a5a0c665250590f7.jsonl | jq .
```

**GCS (production):**
```bash
gsutil ls gs://vox-artifacts/events/2026-05-14/
gsutil cat gs://vox-artifacts/events/2026-05-14/a5a0c665250590f7.jsonl | jq .
```

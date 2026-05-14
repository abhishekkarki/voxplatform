# GKE End-to-End Testing Guide

Deploys and validates every component on a live GKE cluster — iterations 0 through 3.

**Expected time:** ~90 min on a fresh cluster (most of it waiting for model downloads).

**Working directory for all commands:** `~/dev/voxplatform`

**State:** Terraform state lives permanently in `gs://voxplatform-tfstate/dev/` — a manually-created bucket that survives `terraform destroy`.

---

## Prerequisites

```bash
gcloud --version      # Google Cloud SDK
terraform --version   # >= 1.5
kubectl version       # >= 1.28
helm version          # >= 3.12
docker --version

# Install vox CLI
pip install -e ./clients/python

# Authenticate
gcloud auth login
gcloud auth configure-docker europe-west3-docker.pkg.dev
```

---

## Phase 1 — Infrastructure (≈15 min)

### 1.1 Create terraform.tfvars

This file is gitignored — recreate it every time you clone or change machines:

```bash
cat > infra/environments/dev/terraform.tfvars <<EOF
project_id       = "voxplatform"
region           = "europe-west3"
zone             = "europe-west3-a"
env              = "dev"
cpu_machine_type = "e2-standard-4"
cpu_min_nodes    = 1
cpu_max_nodes    = 2
cpu_disk_size_gb = 50
EOF
```

### 1.2 Provision all infrastructure

```bash
cd infra/environments/dev
terraform init          # connects to gs://voxplatform-tfstate/dev/
terraform apply -auto-approve
cd ~/dev/voxplatform
```

Creates VPC, GKE cluster, Artifact Registry, and GCS artifacts bucket in one pass (~10 min).

### 1.3 Connect kubectl

```bash
gcloud container clusters get-credentials vox-cluster-dev \
  --zone europe-west3-a --project voxplatform

kubectl get nodes   # expect 1-2 nodes STATUS=Ready
```

**Checkpoint ✓** At least one node `Ready`.

---

## Phase 2 — Build and Push All Images (≈20 min)

The Artifact Registry is recreated with the cluster — images must be pushed on every fresh cluster.

```bash
export REGISTRY=europe-west3-docker.pkg.dev/voxplatform/vox-images-dev
```

```bash
# Gateway
docker build --platform linux/amd64 -f Dockerfile.gateway \
  -t $REGISTRY/gateway:0.3.0 . && docker push $REGISTRY/gateway:0.3.0

# VAD sidecar
docker build --platform linux/amd64 -f services/vad/Dockerfile services/vad \
  -t $REGISTRY/vad:0.1.0 && docker push $REGISTRY/vad:0.1.0

# Diarizer (~5 min — large PyTorch build)
docker build --platform linux/amd64 -f services/diarizer/Dockerfile services/diarizer \
  -t $REGISTRY/diarizer:0.1.0 && docker push $REGISTRY/diarizer:0.1.0

# Summarizer (~10 min — C++ llama.cpp compile)
docker build --platform linux/amd64 -f services/summarizer/Dockerfile services/summarizer \
  -t $REGISTRY/summarizer:0.1.0 && docker push $REGISTRY/summarizer:0.1.0

# Operator
docker build --platform linux/amd64 -f operator/Dockerfile operator \
  -t $REGISTRY/operator:0.1.0 && docker push $REGISTRY/operator:0.1.0
```

**Checkpoint ✓**

```bash
gcloud artifacts docker images list $REGISTRY
# expect 5 images: gateway, vad, diarizer, summarizer, operator
```

---

## Phase 3 — Monitoring (≈5 min)

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

helm upgrade --install monitoring prometheus-community/kube-prometheus-stack \
  -n monitoring --create-namespace \
  -f deploy/helm/monitoring/values.yaml \
  --wait --timeout 5m
```

**Checkpoint ✓**

```bash
kubectl get pods -n monitoring | grep -E "grafana|prometheus"
```

---

## Phase 4 — Kubernetes Operator (≈5 min)

### 4.1 Install CRDs

```bash
kubectl apply -f operator/config/crd/bases/
kubectl get crds | grep vox.vox.io
# voicemodels.vox.vox.io
# inferencepipelines.vox.vox.io
```

### 4.2 Deploy operator

```bash
kubectl create namespace vox --dry-run=client -o yaml | kubectl apply -f -

cd operator && make deploy IMG=$REGISTRY/operator:0.1.0 && cd ..

kubectl get pods -n operator-system -w
# controller-manager-xxx   1/1   Running
```

**Checkpoint ✓** Operator pod `Running`.

---

## Phase 5 — Whisper Model Server (≈10 min first run)

```bash
helm upgrade --install whisper deploy/helm/faster-whisper \
  -n vox --wait --timeout 10m
```

Downloads `Systran/faster-whisper-small.en` (~250 MB) on first start.

```bash
kubectl logs -n vox -l app=whisper -f   # watch download progress
```

**Checkpoint ✓**

```bash
kubectl exec -n vox deploy/whisper -- \
  python3 -c "import urllib.request; print(urllib.request.urlopen('http://localhost:8000/health').read())"
# b'OK'
```

---

## Phase 6 — VoiceModel CR for Whisper (≈2 min)

```bash
kubectl apply -f operator/config/samples/voicemodels/whisper-small.yaml

kubectl get voicemodels -n vox -w
# NAME            PHASE    READY
# whisper-small   Ready    1
```

**Checkpoint ✓** VoiceModel phase = `Ready`.

---

## Phase 7 — Gateway (≈3 min)

```bash
# Kill any existing port-forward on 8080 before deploying
lsof -ti :8080 | xargs kill -9 2>/dev/null; true

helm upgrade --install gateway deploy/helm/gateway \
  -n vox \
  --set image.repository=$REGISTRY/gateway \
  --set vad.image.repository=$REGISTRY/vad \
  --wait --timeout 3m

kubectl port-forward -n vox svc/gateway 8080:8080 &
```

**Iteration 0 — batch transcription:**

```bash
vox health                    # Gateway: ok
vox ready                     # Gateway: ready
vox transcribe eval/datasets/test/test.wav
# The quick brown fox jumps over the lazy dog.
```

**Iteration 1 — streaming:**

```bash
vox record --duration 5
# Speak — partial transcripts appear in real-time
```

**Checkpoint ✓** Transcription works, streaming returns partials.

---

## Phase 8 — Iteration 2 Operator Validation (≈5 min)

```bash
# Scale up — operator should add a replica
kubectl patch voicemodel whisper-small -n vox \
  --type=merge -p '{"spec":{"replicas":2}}'
kubectl get voicemodel whisper-small -n vox -w
# READY 1 → 2, PHASE stays Ready

# Scale back
kubectl patch voicemodel whisper-small -n vox \
  --type=merge -p '{"spec":{"replicas":1}}'

# Delete — operator removes Deployment + Service
kubectl delete voicemodel whisper-small -n vox
kubectl get deployments -n vox | grep whisper   # gone
kubectl get services -n vox | grep whisper       # gone

# Re-apply for iteration 3
kubectl apply -f operator/config/samples/voicemodels/whisper-small.yaml
```

**Checkpoint ✓** Operator creates, scales, and cleans up on CRD changes.

---

## Phase 9 — Diarizer and Summarizer (≈15 min first run)

```bash
# Diarizer — runs in single-speaker fallback mode without HF_TOKEN
helm upgrade --install diarizer deploy/helm/diarizer \
  -n vox \
  --set image.repository=$REGISTRY/diarizer \
  --set image.tag=0.1.0 \
  --wait --timeout 5m

# Summarizer — Qwen 3B model downloads to PVC on first start (~2GB)
# Run in background — model download takes time
helm upgrade --install summarizer deploy/helm/summarizer \
  -n vox \
  --set image.repository=$REGISTRY/summarizer \
  --set image.tag=0.1.0 \
  --timeout 15m &

kubectl logs -n vox -l app=summarizer -f   # watch model download
```

**Optional — enable full pyannote diarization:**

```bash
# Accept terms first: https://huggingface.co/pyannote/speaker-diarization-3.1
kubectl create secret generic diarizer-hf-token \
  -n vox --from-literal=HF_TOKEN=hf_xxxxxxxxxxxxxxxx

helm upgrade diarizer deploy/helm/diarizer -n vox \
  --set image.repository=$REGISTRY/diarizer \
  --set image.tag=0.1.0 \
  --set "env.HF_TOKEN=$(kubectl get secret -n vox diarizer-hf-token \
    -o jsonpath='{.data.HF_TOKEN}' | base64 -d)"
```

**Checkpoint ✓**

```bash
kubectl get pods -n vox
# diarizer-xxx    1/1   Running
# summarizer-xxx  1/1   Running  (may take 15 min on first start)
```

---

## Phase 10 — VoiceModel CRs for Diarizer and Summarizer (≈2 min)

Apply one file per model — do not use the combined `voicemodel_samples.yaml`:

```bash
kubectl apply -f operator/config/samples/voicemodels/pyannote-diarizer.yaml
kubectl apply -f operator/config/samples/voicemodels/qwen-summarizer.yaml

kubectl get voicemodels -n vox -w
# pyannote-diarizer   Ready   1
# qwen-summarizer     Ready   1   ← waits for model download to finish
# whisper-small       Ready   1
```

---

## Phase 11 — InferencePipeline (≈2 min)

```bash
kubectl apply -f operator/config/samples/voicemodels/inferencepipeline-default.yaml

kubectl get inferencepipelines -n vox -w
# NAME      PHASE       MESSAGE
# default   Validating  0/3 stages ready
# default   Degraded    1/3 stages ready
# default   Ready       3/3 stages ready
```

```bash
kubectl describe inferencepipeline default -n vox
# Stages:
#   stt        Ready: true   Endpoint: vox-whisper-small.vox.svc...
#   diarize    Ready: true   Endpoint: vox-pyannote-diarizer.vox.svc...
#   summarize  Ready: true   Endpoint: vox-qwen-summarizer.vox.svc...
```

**Checkpoint ✓** InferencePipeline phase = `Ready`.

---

## Phase 12 — Upgrade Gateway with Pipeline URLs (≈1 min)

The `deploy/helm/gateway/values.yaml` already has `DIARIZER_URL`, `SUMMARIZER_URL`, and `EVENT_LOG_*` set. Just upgrade to pick them up:

```bash
helm upgrade gateway deploy/helm/gateway \
  -n vox \
  --set image.repository=$REGISTRY/gateway \
  --set image.tag=0.3.0 \
  --set vad.image.repository=$REGISTRY/vad \
  --wait
```

---

## Phase 13 — Pipeline Tests (≈15 min)

### Full pipeline

```bash
curl -X POST http://localhost:8080/v1/pipeline/run \
  -F file=@eval/datasets/test/test.wav | jq
```

Expected:

```json
{
  "transcript": "The quick brown fox jumps over the lazy dog.",
  "segments": [{"start": 0.0, "end": 3.5, "speaker": "SPEAKER_00"}],
  "summary": "A sentence about a fox jumping over a dog.",
  "stages": {
    "stt":       {"success": true, "duration_seconds": 2.1},
    "diarize":   {"success": true, "duration_seconds": 1.8},
    "summarize": {"success": true, "duration_seconds": 8.2}
  }
}
```

### STT-only

```bash
curl -X POST http://localhost:8080/v1/pipeline/run \
  -F file=@eval/datasets/test/test.wav -F stages=stt | jq '{transcript, stages}'
```

### Graceful degradation

```bash
kubectl scale deployment summarizer -n vox --replicas=0

curl -X POST http://localhost:8080/v1/pipeline/run \
  -F file=@eval/datasets/test/test.wav | jq '.stages'
# summarize.success = false, stt and diarize still succeed

kubectl scale deployment summarizer -n vox --replicas=1
```

### CRD tracks degradation

```bash
kubectl scale deployment vox-qwen-summarizer -n vox --replicas=0
kubectl get inferencepipeline default -n vox
# PHASE: Degraded   MESSAGE: 2/3 stages ready

kubectl scale deployment vox-qwen-summarizer -n vox --replicas=1
kubectl get inferencepipeline default -n vox -w
# PHASE: Ready      MESSAGE: 3/3 stages ready  ← within ~10s
```

**Checkpoint ✓** Pipeline returns results, degradation non-fatal, CRD phase tracks reality.

---

## Phase 14 — Event Log Validation (≈5 min)

```bash
# Note request_id from a pipeline test above, then:
gsutil ls gs://voxplatform-vox-artifacts-dev/events/$(date +%Y-%m-%d)/

gsutil cat gs://voxplatform-vox-artifacts-dev/events/$(date +%Y-%m-%d)/<request_id>*.jsonl | jq .
```

Expected — one JSON object per line:

```json
{"type":"pipeline.start","request_id":"a5a0...","timestamp":"..."}
{"type":"stage.start","stage":"stt"}
{"type":"stage.complete","stage":"stt","data":{"duration_seconds":2.1}}
{"type":"stage.start","stage":"diarize"}
{"type":"stage.complete","stage":"diarize","data":{"num_segments":1}}
{"type":"stage.start","stage":"summarize"}
{"type":"stage.complete","stage":"summarize","data":{"duration_seconds":8.2}}
{"type":"pipeline.complete","data":{"duration_seconds":12.4}}
```

**Checkpoint ✓** Full JSONL log in GCS with timing for every stage.

---

## Phase 15 — Eval Harness (≈5 min)

```bash
pip install -e ./eval
vox-eval run eval/datasets/test --url http://localhost:8080 --threshold 0.25
# Exit 0 = WER ≤ 25% ✓
```

---

## Phase 16 — Grafana Dashboard (≈2 min)

```bash
kubectl port-forward -n monitoring svc/monitoring-grafana 3000:80 &
open http://localhost:3000   # admin / prom-operator
```

Check: request count increments, latency histogram has values, in-flight gauge spikes during transcription.

---

## Tear-down (saves ~$0.40/hr)

```bash
# Kill port-forwards
lsof -ti :8080 | xargs kill -9 2>/dev/null; true
lsof -ti :3000 | xargs kill -9 2>/dev/null; true

# Destroy all GCP resources
cd ~/dev/voxplatform/infra/environments/dev
terraform destroy -auto-approve
```

Terraform destroys all 7 resources. State is saved back to `gs://voxplatform-tfstate/dev/` which remains intact. Next `terraform apply` picks it up and starts fresh cleanly.

**Do not destroy `voxplatform-tfstate` bucket** — it is permanent and not managed by Terraform.

---

## Quick-start checklist (returning session)

```bash
cd ~/dev/voxplatform
# 1. Recreate tfvars (gitignored)
cat > infra/environments/dev/terraform.tfvars <<EOF
project_id = "voxplatform"
region = "europe-west3"
zone = "europe-west3-a"
env = "dev"
cpu_machine_type = "e2-standard-4"
cpu_min_nodes = 1
cpu_max_nodes = 2
cpu_disk_size_gb = 50
EOF
# 2. Bring up infra
cd infra/environments/dev && terraform init && terraform apply -auto-approve && cd ~/dev/voxplatform
# 3. Rebuild images (registry is recreated each time)
export REGISTRY=europe-west3-docker.pkg.dev/voxplatform/vox-images-dev
# ... (Phase 2 commands)
# 4. Connect kubectl and deploy (Phases 3–12)
```

---

## Test checklist

| Test | Command | Pass criteria |
|------|---------|---------------|
| Batch STT | `vox transcribe eval/datasets/test/test.wav` | Transcript returned |
| Streaming STT | `vox record --duration 5` | Partial transcripts in real-time |
| VoiceModel CRD | `kubectl get voicemodels -n vox` | All 3 = Ready |
| Operator scale | `kubectl patch voicemodel whisper-small -n vox --type=merge -p '{"spec":{"replicas":2}}'` | Deployment scales |
| Operator delete | `kubectl delete voicemodel whisper-small -n vox` | Deployment + Service removed |
| InferencePipeline | `kubectl get inferencepipelines -n vox` | Phase = Ready, 3/3 stages |
| Full pipeline | `POST /v1/pipeline/run` | transcript + segments + summary |
| Stages param | `POST /v1/pipeline/run -F stages=stt` | Only transcript returned |
| Graceful degradation | Scale summarizer to 0, run pipeline | `stages.summarize.success=false`, pipeline continues |
| CRD degradation | Check pipeline after scaling down | Phase = Degraded, 2/3 stages ready |
| Event log | `gsutil cat gs://voxplatform-vox-artifacts-dev/events/...` | Full JSONL per request |
| WER eval | `vox-eval run eval/datasets/test --threshold 0.25` | Exit code 0 |

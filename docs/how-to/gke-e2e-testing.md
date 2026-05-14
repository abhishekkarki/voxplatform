# GKE End-to-End Testing Guide

This guide walks through deploying and validating every component of VoxPlatform on a live GKE cluster — from infrastructure provisioning through iteration 3 (InferencePipeline + event log).

**Expected total time:** ~90 minutes on a fresh cluster (most of it waiting for model downloads).

---

## Prerequisites

```bash
# Tools required
gcloud --version      # Google Cloud SDK
terraform --version   # >= 1.5
kubectl version       # >= 1.28
helm version          # >= 3.12
docker --version      # for image builds
pip install -e ./clients/python   # vox CLI

# Authenticate
gcloud auth login
gcloud auth configure-docker europe-west3-docker.pkg.dev
```

---

## Phase 1 — Infrastructure (≈15 min)

### 1.1 Create terraform.tfvars

```bash
cat > infra/environments/dev/terraform.tfvars <<EOF
project_id       = "YOUR_GCP_PROJECT_ID"
region           = "europe-west3"
zone             = "europe-west3-a"
env              = "dev"
cpu_machine_type = "e2-standard-4"
cpu_min_nodes    = 1
cpu_max_nodes    = 2
cpu_disk_size_gb = 50
EOF
```

### 1.2 Provision GKE cluster

```bash
cd infra/environments/dev

terraform init
terraform apply -target=module.network -auto-approve
terraform apply -target=module.gke -auto-approve
terraform apply -target=module.registry -auto-approve
terraform apply -target=module.storage -auto-approve
```

### 1.3 Connect kubectl

```bash
gcloud container clusters get-credentials vox-cluster-dev \
  --zone europe-west3-a \
  --project YOUR_GCP_PROJECT_ID

kubectl get nodes   # expect 1-2 nodes, STATUS=Ready
```

**Checkpoint ✓** At least one node in `Ready` state.

---

## Phase 2 — Build and Push All Images (≈20 min)

Set your registry prefix once:

```bash
export REGISTRY=europe-west3-docker.pkg.dev/YOUR_GCP_PROJECT_ID/vox-images-dev
```

Build all images for linux/amd64 (GKE nodes are x86):

```bash
# Gateway (includes iteration 3 pipeline + event log)
docker build --platform linux/amd64 \
  -f Dockerfile.gateway \
  -t $REGISTRY/gateway:0.3.0 .
docker push $REGISTRY/gateway:0.3.0

# VAD sidecar
docker build --platform linux/amd64 \
  -f services/vad/Dockerfile services/vad \
  -t $REGISTRY/vad:0.1.0
docker push $REGISTRY/vad:0.1.0

# Diarizer (pyannote-audio, large build — ~5 min)
docker build --platform linux/amd64 \
  -f services/diarizer/Dockerfile services/diarizer \
  -t $REGISTRY/diarizer:0.1.0
docker push $REGISTRY/diarizer:0.1.0

# Summarizer (llama-cpp-python C++ compile — ~10 min)
docker build --platform linux/amd64 \
  -f services/summarizer/Dockerfile services/summarizer \
  -t $REGISTRY/summarizer:0.1.0
docker push $REGISTRY/summarizer:0.1.0

# Operator
cd operator
docker build --platform linux/amd64 \
  -t $REGISTRY/operator:0.1.0 .
docker push $REGISTRY/operator:0.1.0
cd ..
```

**Checkpoint ✓** All 5 images visible in Artifact Registry:

```bash
gcloud artifacts docker images list \
  europe-west3-docker.pkg.dev/YOUR_GCP_PROJECT_ID/vox-images-dev
```

---

## Phase 3 — Monitoring (≈5 min)

```bash
kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -

helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

helm install monitoring prometheus-community/kube-prometheus-stack \
  -n monitoring \
  -f deploy/helm/monitoring/values.yaml \
  --wait --timeout 5m
```

**Checkpoint ✓** Grafana and Prometheus pods running:

```bash
kubectl get pods -n monitoring | grep -E "grafana|prometheus"
```

---

## Phase 4 — Kubernetes Operator (≈5 min)

### 4.1 Install CRDs

```bash
kubectl apply -f operator/config/crd/bases/
```

Verify both CRDs are registered:

```bash
kubectl get crds | grep vox.vox.io
# voicemodels.vox.vox.io          ...
# inferencepipelines.vox.vox.io   ...
```

### 4.2 Deploy operator

```bash
kubectl create namespace vox --dry-run=client -o yaml | kubectl apply -f -

cd operator
make deploy IMG=$REGISTRY/operator:0.1.0
cd ..
```

Watch it start:

```bash
kubectl get pods -n operator-system -w
# controller-manager-xxx   Running
```

**Checkpoint ✓** Operator pod in `Running` state.

---

## Phase 5 — Whisper Model Server (≈10 min first run)

```bash
helm install whisper deploy/helm/faster-whisper \
  -n vox --create-namespace \
  --wait --timeout 10m
```

The pod downloads `Systran/faster-whisper-small.en` (~250 MB) on first start.

```bash
kubectl get pods -n vox -w          # wait for Running
kubectl logs -n vox -l app=whisper  # watch model download progress
```

**Checkpoint ✓** Whisper pod `Running`, `/health` returns 200:

```bash
kubectl exec -n vox deploy/whisper -- \
  python3 -c "import urllib.request; print(urllib.request.urlopen('http://localhost:8000/health').read())"
```

---

## Phase 6 — Apply VoiceModel CR for Whisper (≈2 min)

```bash
kubectl apply -f operator/config/samples/voicemodel_samples.yaml \
  --selector='metadata.name=whisper-small'
```

Watch the operator reconcile:

```bash
kubectl get voicemodels -n vox -w
# NAME            MODEL                              DEVICE  REPLICAS  READY  PHASE
# whisper-small   Systran/faster-whisper-small.en    cpu     1         1      Ready
```

**Checkpoint ✓** VoiceModel phase = `Ready`.

---

## Phase 7 — Gateway (≈3 min)

Update the values with your registry before deploying:

```bash
helm install gateway deploy/helm/gateway \
  -n vox \
  --set image.repository=$REGISTRY/gateway \
  --set vad.image.repository=$REGISTRY/vad \
  --wait --timeout 3m
```

**Iteration 0 test — batch transcription:**

```bash
kubectl port-forward -n vox svc/gateway 8080:8080 &

vox health          # Gateway: ok
vox ready           # Gateway: ready  (whisper reachable)
vox transcribe test.wav
# The quick brown fox jumps over the lazy dog.
```

**Iteration 1 test — streaming:**

```bash
vox record --duration 5
# Speak something, get partial transcripts back in real-time
```

**Checkpoint ✓** Transcription works, streaming returns partials.

---

## Phase 8 — Iteration 2 — Operator Validation (≈2 min)

Verify the VoiceModel controller round-trip properly:

```bash
# Edit replicas and watch operator respond
kubectl patch voicemodel whisper-small -n vox \
  --type=merge -p '{"spec":{"replicas":2}}'

kubectl get voicemodel whisper-small -n vox -w
# READY goes from 1 → 2, PHASE stays Ready

# Scale back down
kubectl patch voicemodel whisper-small -n vox \
  --type=merge -p '{"spec":{"replicas":1}}'
```

**Test deletion + finalizer cleanup:**

```bash
kubectl delete voicemodel whisper-small -n vox
kubectl get deployments -n vox | grep whisper    # deployment gone
kubectl get services -n vox | grep whisper       # service gone

# Re-apply (needed for iteration 3)
kubectl apply -f operator/config/samples/voicemodel_samples.yaml \
  --selector='metadata.name=whisper-small'
```

**Checkpoint ✓** Operator creates, updates, and cleans up Deployments on CRD changes.

---

## Phase 9 — Diarizer and Summarizer (≈15 min first run)

```bash
# Diarizer (runs in fallback mode without HF_TOKEN — still useful for testing)
helm install diarizer deploy/helm/diarizer \
  -n vox \
  --set image.repository=$REGISTRY/diarizer \
  --set image.tag=0.1.0 \
  --wait --timeout 5m

# Summarizer (downloads Qwen 3B ~2GB on first start — be patient)
helm install summarizer deploy/helm/summarizer \
  -n vox \
  --set image.repository=$REGISTRY/summarizer \
  --set image.tag=0.1.0 \
  --timeout 15m &    # run in background — model download takes time

# Watch summarizer download progress
kubectl logs -n vox -l app=summarizer -f
```

**Optional: Enable full diarization with HuggingFace token**

```bash
# Accept terms at https://huggingface.co/pyannote/speaker-diarization-3.1 first
kubectl create secret generic diarizer-hf-token \
  -n vox \
  --from-literal=HF_TOKEN=hf_xxxxxxxxxxxxxxxx

helm upgrade diarizer deploy/helm/diarizer -n vox \
  --set image.repository=$REGISTRY/diarizer \
  --set image.tag=0.1.0 \
  --set "env.HF_TOKEN=$(kubectl get secret -n vox diarizer-hf-token -o jsonpath='{.data.HF_TOKEN}' | base64 -d)"
```

**Checkpoint ✓** Both services healthy:

```bash
kubectl get pods -n vox
# NAME               READY   STATUS    RESTARTS
# diarizer-xxx       1/1     Running   0
# summarizer-xxx     1/1     Running   0
```

---

## Phase 10 — Apply VoiceModel CRs for Diarizer and Summarizer (≈2 min)

```bash
kubectl apply -f operator/config/samples/voicemodel_samples.yaml
```

Watch all three VoiceModels go Ready:

```bash
kubectl get voicemodels -n vox -w
# NAME               PHASE        READY
# pyannote-diarizer  Ready        1
# qwen-summarizer    Ready        1
# whisper-small      Ready        1
```

**Note:** `qwen-summarizer` will stay in `Deploying` until the model download finishes. This is expected.

---

## Phase 11 — InferencePipeline CRD (≈2 min)

```bash
kubectl apply -f operator/config/samples/inferencepipeline_sample.yaml

kubectl get inferencepipelines -n vox -w
# NAME      PHASE      MESSAGE            AGE
# default   Validating  0/3 stages ready  10s
# default   Degraded    1/3 stages ready  20s
# default   Ready       3/3 stages ready  45s
```

Inspect stage detail:

```bash
kubectl describe inferencepipeline default -n vox
# Status:
#   Phase:    Ready
#   Message:  3/3 stages ready
#   Stages:
#     Name:      stt        Ready: true   Endpoint: vox-whisper-small.vox.svc...
#     Name:      diarize    Ready: true   Endpoint: vox-pyannote-diarizer.vox.svc...
#     Name:      summarize  Ready: true   Endpoint: vox-qwen-summarizer.vox.svc...
```

**Checkpoint ✓** InferencePipeline phase = `Ready`.

---

## Phase 12 — Upgrade Gateway with Pipeline Env Vars (≈1 min)

```bash
helm upgrade gateway deploy/helm/gateway \
  -n vox \
  --set image.repository=$REGISTRY/gateway \
  --set image.tag=0.3.0 \
  --set vad.image.repository=$REGISTRY/vad \
  --wait
```

---

## Phase 13 — End-to-End Pipeline Test (≈15 min)

### 13.1 Full pipeline

```bash
curl -X POST http://localhost:8080/v1/pipeline/run \
  -F file=@test.wav | jq
```

Expected response:

```json
{
  "request_id": "a5a0c665250590f7",
  "processing_time_seconds": 12.4,
  "stages": {
    "stt":       {"success": true, "duration_seconds": 2.1},
    "diarize":   {"success": true, "duration_seconds": 1.8},
    "summarize": {"success": true, "duration_seconds": 8.2}
  },
  "transcript": "The quick brown fox jumps over the lazy dog.",
  "segments": [
    {"start": 0.0, "end": 3.5, "speaker": "SPEAKER_00"}
  ],
  "summary": "A sentence about a fox jumping over a dog."
}
```

### 13.2 STT-only (verify stages param)

```bash
curl -X POST http://localhost:8080/v1/pipeline/run \
  -F file=@test.wav -F stages=stt | jq '{transcript, stages}'
```

### 13.3 Graceful degradation — scale summarizer to 0

```bash
kubectl scale deployment summarizer -n vox --replicas=0

curl -X POST http://localhost:8080/v1/pipeline/run \
  -F file=@test.wav | jq '.stages'
# expect: stt.success=true, diarize.success=true, summarize.success=false
# pipeline still returns transcript + segments

kubectl scale deployment summarizer -n vox --replicas=1
```

### 13.4 InferencePipeline reflects degradation

```bash
kubectl scale deployment vox-qwen-summarizer -n vox --replicas=0
kubectl get inferencepipeline default -n vox
# PHASE: Degraded   MESSAGE: 2/3 stages ready

kubectl scale deployment vox-qwen-summarizer -n vox --replicas=1
kubectl get inferencepipeline default -n vox -w
# PHASE: Ready      MESSAGE: 3/3 stages ready  ← updates within ~10s
```

**Checkpoint ✓** Pipeline returns results, degradation is non-fatal, CRD phase tracks reality.

---

## Phase 14 — Event Log Validation (≈5 min)

Note the `request_id` from your pipeline test above, then:

```bash
# List event files written to GCS
gsutil ls gs://vox-artifacts/events/$(date +%Y-%m-%d)/

# Inspect one request's full event log
gsutil cat gs://vox-artifacts/events/$(date +%Y-%m-%d)/<request_id>*.jsonl | jq .
```

Expected output (one JSON object per line):

```json
{"type":"pipeline.start","request_id":"a5a0...","timestamp":"..."}
{"type":"stage.start","stage":"stt","request_id":"a5a0..."}
{"type":"stage.complete","stage":"stt","data":{"duration_seconds":2.1}}
{"type":"stage.start","stage":"diarize"}
{"type":"stage.complete","stage":"diarize","data":{"num_segments":1}}
{"type":"stage.start","stage":"summarize"}
{"type":"stage.complete","stage":"summarize","data":{"duration_seconds":8.2}}
{"type":"pipeline.complete","data":{"duration_seconds":12.4}}
```

**Checkpoint ✓** Full event log in GCS with timing for every stage.

---

## Phase 15 — Eval Harness Against Live Cluster (≈5 min)

```bash
pip install -e ./eval
vox-eval run eval/datasets/test \
  --url http://localhost:8080 \
  --threshold 0.25

# Exit 0 = WER ≤ 25%  ✓
# Exit 1 = WER > 25%  — check the JSON report
```

---

## Phase 16 — Grafana Dashboard (≈2 min)

```bash
kubectl port-forward -n monitoring svc/monitoring-grafana 3000:80 &
open http://localhost:3000   # admin / prom-operator
```

Navigate to Dashboards → VoxPlatform. Verify:
- Request count increments when you run transcriptions
- Latency histogram shows real values
- In-flight gauge goes to 1 during a slow CPU transcription

---

## Tear-down (saves ~$0.40/hr)

```bash
# Remove all K8s resources
helm uninstall gateway whisper diarizer summarizer monitoring -n vox 2>/dev/null
kubectl delete namespace vox monitoring operator-system 2>/dev/null

# Destroy GKE cluster (keep registry and storage — cheap)
cd infra/environments/dev
terraform destroy -target=module.gke -auto-approve
terraform destroy -target=module.network -auto-approve
```

---

## Iteration 3 test checklist

| Test | Command | Pass criteria |
|------|---------|---------------|
| Batch STT | `vox transcribe test.wav` | Transcript returned |
| Streaming STT | `vox record --duration 5` | Partial transcripts in real-time |
| VoiceModel CRD | `kubectl get voicemodels -n vox` | All 3 = Ready |
| Operator scale | `kubectl patch voicemodel whisper-small --replicas=2` | Deployment scales, phase stays Ready |
| Operator delete | `kubectl delete voicemodel whisper-small` | Deployment + Service removed |
| InferencePipeline | `kubectl get inferencepipelines -n vox` | Phase = Ready, 3/3 stages |
| Full pipeline | `POST /v1/pipeline/run` | transcript + segments + summary in response |
| Stages param | `POST /v1/pipeline/run -F stages=stt` | Only transcript, no segments/summary |
| Graceful degradation | Scale summarizer to 0, run pipeline | Pipeline returns, `stages.summarize.success=false` |
| CRD reflects degradation | Check pipeline CRD after scaling down | Phase = Degraded, Message = 2/3 stages ready |
| Event log | `gsutil cat gs://vox-artifacts/events/...` | Full JSONL log per request |
| WER eval | `vox-eval run eval/datasets/test --threshold 0.25` | Exit code 0 |

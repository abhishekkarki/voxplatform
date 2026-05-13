# First transcription in 5 minutes

By the end of this page you'll have transcribed live audio from your microphone against a running Vox cluster.

!!! tip "Cost"
    Following this tutorial start to finish on a fresh project costs roughly **€0.15–€0.30** in GKE credits, assuming you tear the cluster down at the end with `make down`.

## 1. Clone and bootstrap

```bash
git clone https://github.com/abhishekkarki/voxplatform.git
cd voxplatform
make bootstrap   # checks gcloud, kubectl, terraform, helm versions
```

## 2. Bring up the cluster

```bash
cd infra
terraform init
terraform apply -var="project_id=$GCP_PROJECT"
```

This provisions the VPC, the `vox-cluster-dev` GKE cluster in `europe-west3-a`, the Artifact Registry, and the GCS bucket. Takes about 4 minutes.

## 3. Install the platform

```bash
cd ..
make install   # installs operator, gateway, VAD sidecar, faster-whisper Helm release
kubectl apply -f examples/voicemodel-whisper-tiny.yaml
kubectl wait --for=condition=ready voicemodel/whisper-tiny --timeout=120s
```

## 4. Stream from your microphone

```bash
pip install -e sdk/
vox record --model whisper-tiny
```

Speak. Partial transcripts will start streaming back within ~300 ms of you talking. Press `Ctrl+C` to stop.

## 5. Tear down

```bash
cd infra && terraform destroy -var="project_id=$GCP_PROJECT"
```

## What just happened?

Your `vox record` opened a WebSocket to the gateway. The gateway forwarded raw PCM frames to the VAD sidecar, which only let speech segments through. Those segments went to the `whisper-tiny` Deployment that the operator had reconciled from the `VoiceModel` CR you applied in step 3. Partial and final transcripts came back over the same WebSocket.

For the **why** behind these choices, read [Architecture overview](../explanation/architecture.md) and [Why a VAD sidecar](../explanation/why-vad-sidecar.md). For **other things you can do**, see [How-to guides](../how-to/index.md).

# Deploy a VoiceModel

This guide shows how to deploy a new STT model variant by applying a `VoiceModel` custom resource.

## Prerequisites

- Cluster running with the operator installed (see [Tutorial step 3](../tutorial/first-transcription.md#3-install-the-platform))
- For a custom image (e.g. diarizer, summarizer): pushed to your Artifact Registry. faster-whisper-server-based models can skip this — `spec.image` defaults per `spec.device`, see the [CRD reference](../reference/voicemodel-crd.md#default-resources-and-images).

## Steps

1. **Write the CR.** Save the following as `voicemodel-whisper-small.yaml`:

    ```yaml
    apiVersion: vox.vox.io/v1alpha1
    kind: VoiceModel
    metadata:
      name: whisper-small
      namespace: vox
    spec:
      model: Systran/faster-whisper-small.en
      device: cpu
      quantization: int8
      replicas: 1
    ```

2. **Apply it.**

    ```bash
    kubectl apply -f voicemodel-whisper-small.yaml
    ```

3. **Watch the operator reconcile.**

    ```bash
    kubectl get voicemodel whisper-small -n vox -w
    ```

    The phase will move `Pending → Deploying → Ready`. The operator creates a `Deployment` named `vox-whisper-small` and a matching `Service`.

4. **Verify.**

    ```bash
    kubectl get deploy,svc -n vox -l vox.io/model=whisper-small
    kubectl get voicemodel whisper-small -n vox -o jsonpath='{.status.endpoint}'
    ```

## GPU variant

Same steps, `device: gpu` — the operator picks the CUDA image and requests
`nvidia.com/gpu` automatically, no other fields needed:

```yaml
apiVersion: vox.vox.io/v1alpha1
kind: VoiceModel
metadata:
  name: whisper-large-v3-gpu
  namespace: vox
spec:
  model: Systran/faster-whisper-large-v3
  device: gpu
  quantization: float16
```

Requires the GPU node pool first (`gpu_enabled = true` in Terraform) — see
[Scale the cluster](scale-cluster.md#gpu-pool) and
[ADR-010](../adr/010-gpu-support.md).

## See also

- [VoiceModel CRD reference](../reference/voicemodel-crd.md) — every field, every default
- [Operator design](../explanation/operator-design.md) — why the CRD looks like this

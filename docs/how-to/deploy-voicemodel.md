# Deploy a VoiceModel

This guide shows how to deploy a new STT model variant by applying a `VoiceModel` custom resource.

## Prerequisites

- Cluster running with the operator installed (see [Tutorial step 3](../tutorial/first-transcription.md#3-install-the-platform))
- Image for the model variant pushed to your Artifact Registry

## Steps

1. **Write the CR.** Save the following as `voicemodel-whisper-small.yaml`:

    ```yaml
    apiVersion: vox.io/v1alpha1
    kind: VoiceModel
    metadata:
      name: whisper-small
    spec:
      modelRef: whisper-small
      replicas: 1
      resources:
        requests:
          cpu: "1"
          memory: 2Gi
        limits:
          cpu: "2"
          memory: 4Gi
    ```

2. **Apply it.**

    ```bash
    kubectl apply -f voicemodel-whisper-small.yaml
    ```

3. **Watch the operator reconcile.**

    ```bash
    kubectl get voicemodel whisper-small -w
    ```

    The phase will move `Pending → Deploying → Ready`. The operator creates a `Deployment` named `vox-whisper-small` and a matching `Service`.

4. **Verify.**

    ```bash
    kubectl get deploy,svc -l vox.io/model=whisper-small
    curl http://gateway/health/models
    ```

## See also

- [VoiceModel CRD reference](../reference/voicemodel-crd.md) — every field, every default
- [Operator design](../explanation/operator-design.md) — why the CRD looks like this

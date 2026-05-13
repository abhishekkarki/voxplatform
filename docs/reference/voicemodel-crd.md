# VoiceModel CRD

!!! note "Auto-generated"
    This page is generated from the Go types in `operator/api/v1alpha1/voicemodel_types.go` by [`crd-ref-docs`](https://github.com/elastic/crd-ref-docs) and committed by CI. Edits to this page directly will be overwritten — change the Go markers and docstrings instead.

<!-- BEGIN crd-ref-docs -->

## `VoiceModel`

`apiVersion: vox.io/v1alpha1`, `kind: VoiceModel`

| Field | Type | Description |
|-------|------|-------------|
| `metadata` | `ObjectMeta` | Standard Kubernetes object metadata |
| `spec` | `VoiceModelSpec` | Desired state |
| `status` | `VoiceModelStatus` | Observed state — set by the operator |

### `VoiceModelSpec`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `modelRef` | `string` | — | Image tag of the model variant (e.g. `whisper-tiny`) |
| `replicas` | `int32` | `1` | Number of pods to run |
| `resources` | `corev1.ResourceRequirements` | see operator defaults | CPU/memory requests and limits |

### `VoiceModelStatus`

| Field | Type | Description |
|-------|------|-------------|
| `phase` | `string` | One of `Pending`, `Deploying`, `Ready`, `Failed` |
| `readyReplicas` | `int32` | Number of pods reporting Ready |
| `lastTransitionTime` | `metav1.Time` | When `phase` last changed |
| `conditions` | `[]metav1.Condition` | Standard condition list |

<!-- END crd-ref-docs -->

## See also

- [Operator design](../explanation/operator-design.md) — why the spec looks the way it does
- [Deploy a VoiceModel](../how-to/deploy-voicemodel.md) — practical walkthrough

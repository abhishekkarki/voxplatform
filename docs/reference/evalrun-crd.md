# EvalRun CRD Reference

**API Group:** `vox.vox.io/v1alpha1`
**Kind:** `EvalRun`
**Scope:** Namespaced

## Overview

`EvalRun` declares a one-shot WER regression check against a gateway. It is
the in-cluster counterpart to running `vox-eval` by hand: instead of creating
any long-lived resource itself, the operator submits a single
[Argo Workflow](https://argoproj.github.io/argo-workflows/) that runs the
`vox-eval` harness in a container, then mirrors the Workflow's outcome back
into `EvalRun` status. Deleting the `EvalRun` garbage-collects its Workflow.

**Prerequisite:** Argo Workflows must be installed in the cluster — it is not
bundled by this repo's Helm charts. Install it with the upstream quick-start
manifest before applying an `EvalRun`.

## Spec

```yaml
apiVersion: vox.vox.io/v1alpha1
kind: EvalRun
metadata:
  name: sample-wer-check
  namespace: vox
spec:
  dataset: datasets/sample          # path resolved inside spec.image
  gatewayURL: http://gateway.vox.svc.cluster.local:8080
  image: europe-west3-docker.pkg.dev/voxplatform/vox-images-dev/vox-eval:latest
  model: whisper-small               # optional — omit for the gateway default
  werThreshold: "0.25"               # optional, default: "0.3"
  timeoutSeconds: 600                # optional, default: 600
  suspend: false                     # optional, default: false
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `dataset` | string | yes | Path to the dataset directory (`manifest.csv` + audio), resolved inside `spec.image`. |
| `gatewayURL` | string | yes | Gateway endpoint to evaluate against. |
| `image` | string | yes | Eval harness container image — bundles `vox-eval`, the SDK, and the dataset. Built from `Dockerfile.eval` at the repo root. |
| `model` | string | no | Pins which model the gateway uses for transcription. Default: gateway's own default. |
| `werThreshold` | string | no | Maximum acceptable corpus WER. Default: `"0.3"`. |
| `timeoutSeconds` | int32 | no | Bounds how long the Workflow may run (`activeDeadlineSeconds`). Default: `600`. |
| `suspend` | bool | no | When `true`, the controller never submits a Workflow. Default: `false`. |

## Status

```yaml
status:
  phase: Failed
  workflowRef: sample-wer-check
  corpusWER: "0.4200"
  meanWER: "0.5000"
  passed: false
  totalSamples: 3
  message: "corpus WER 0.4200 exceeds threshold 0.25"
  startTime: "2026-05-14T10:00:00Z"
  completionTime: "2026-05-14T10:02:30Z"
```

### `status.phase`

| Phase | Meaning |
|-------|---------|
| `Pending` | No Workflow submitted yet — freshly created, or `spec.suspend: true`. |
| `Running` | Workflow submitted; has not yet reached a terminal phase. |
| `Succeeded` | The eval ran **and** its corpus WER was within `spec.werThreshold`. |
| `Failed` | Either a WER regression, or the Workflow ended without producing a report (crashed before writing it, timed out, etc). |

`Phase` is derived from the eval harness's own JSON report (its `passed`
field), not from the raw Argo Workflow phase — `vox-eval run` exits non-zero
on a WER regression, which fails the Argo step/Workflow too, but Argo's wait
sidecar still captures the declared output parameter after the main container
exits either way. This means `Failed` uniformly covers "bad model config" —
regardless of *why* the run failed, `kubectl get evalrun` shows the same
signal.

### `status` fields

| Field | Description |
|-------|-------------|
| `workflowRef` | Name of the Argo Workflow submitted for this run (same as the EvalRun's own name). |
| `corpusWER` / `meanWER` | Corpus-level / mean per-sample WER, copied from the eval report. Empty until the Workflow reaches a terminal phase. |
| `passed` | Whether `corpusWER` was within `spec.werThreshold`. `nil` until a report is available. |
| `totalSamples` | Number of dataset samples evaluated. |
| `message` | Human-readable status explanation. |
| `startTime` / `completionTime` | When the Workflow was submitted / reached a terminal phase. |

## kubectl quick reference

```bash
# List all eval runs
kubectl get evalruns -n vox

# Inspect one
kubectl describe evalrun sample-wer-check -n vox

# Watch status in real-time
kubectl get evalruns -n vox -w

# Inspect the underlying Argo Workflow (created with the same name)
kubectl get workflow sample-wer-check -n vox
argo logs sample-wer-check -n vox
```

## The Go/Python contract

The controller does not parse Python or import any eval code — the entire
contract is JSON, per the project's Go/Python boundary rule. The eval
container's `vox-eval run ... --report-path /tmp/outputs/report.json` writes
the harness's existing JSON report to a fixed path; the Workflow template
declares that path as an output parameter named `eval-report`; the controller
reads the parameter's value off the Workflow's `status.outputs.parameters` and
`json.Unmarshal`s the fields it needs (`passed`, `corpus_wer`, `mean_wer`,
`total_samples`).

## Re-running an eval

`EvalRun` is one-shot, like a Job — it submits exactly one Workflow and never
resubmits. To run again, delete and recreate the `EvalRun` (which also deletes
its owned Workflow):

```bash
kubectl delete evalrun sample-wer-check -n vox
kubectl apply -f operator/config/samples/evalrun_sample.yaml
```

There is no scheduling/cron field — that's out of this iteration's scope. For
recurring checks, wrap the delete+apply above in a `CronJob` or trigger it
from CI.

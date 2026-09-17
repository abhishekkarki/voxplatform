# ADR-009: Eval Orchestration — EvalRun CRD + Argo Workflows

**Status:** Accepted
**Date:** 2026-05
**Iteration:** 4

---

## Context

The `vox-eval` harness (WER via `jiwer`) already runs standalone against any
gateway URL. This iteration wires it into the cluster: a CRD that triggers a
WER regression check and reports pass/fail as status, plus a CI gate using
the same harness. Three design questions came up.

## Decision 1 — Submit Argo Workflows as `unstructured.Unstructured`, not via the Argo Go SDK

**Options considered:**
- Vendor `github.com/argoproj/argo-workflows/v3` for typed `Workflow` objects.
- Build/read `Workflow` objects as `unstructured.Unstructured` with a manual
  GVK, using the controller-runtime client already in use for everything else.

**Chosen: unstructured.** The Argo SDK pulls in a large transitive dependency
tree for a single object kind the operator only ever creates and reads two
fields from (`status.phase`, `status.outputs.parameters`). controller-runtime
already supports unstructured objects as first-class `client.Object`s —
`Create`, `Get`, `Status().Update`, and `Owns()` in `SetupWithManager` all work
identically to typed objects. This keeps `operator/go.mod` free of a dependency
whose only value here would be compile-time field names.

## Decision 2 — `EvalRun` is one-shot, like a Job, not scheduled

**Options considered:**
- A `schedule` field (cron-like), matching `CronWorkflow`.
- One-shot: exactly one Workflow per `EvalRun`, deterministically named after it.

**Chosen: one-shot.** The locked iteration scope is "CRD + Argo Workflows +
WER regression in CI" — a recurring eval isn't part of that, and this project
stops deliberately at "5 concurrent requests" scale, not a scheduling system.
Recurring checks are better served by wrapping `kubectl delete/apply` in a
`CronJob` or a CI schedule trigger than by growing the CRD's state machine.
If a scheduling need appears later, it's a `spec.schedule` field and a
`CronWorkflow` swap — additive, not a rewrite.

## Decision 3 — the Go/Python contract is a single JSON output parameter, not split fields or a shared struct

**Options considered:**
- Have the eval container write several small files (one per metric) and map
  each to its own Argo output parameter.
- Vendor a shared schema/proto between the Go controller and the Python harness.
- Write the harness's existing `EvalReport.to_dict()` JSON to one file, expose
  it as one Argo output parameter, and have the Go controller
  `json.Unmarshal` the handful of fields it needs.

**Chosen: the third.** CLAUDE.md's boundary rule is explicit: Go and Python
share contracts (JSON), never code. `EvalReport.to_dict()` already exists and
is already the harness's stable output shape (used by `--json` and `--output`
today) — reusing it as-is via one `--report-path <file>` flag is the smallest
change that satisfies the rule, versus inventing a second, narrower format
just for this Workflow step.

**A consequence worth calling out:** `vox-eval run` exits `1` on a WER
regression, which fails the Argo step/Workflow — but Argo's wait sidecar
captures declared output parameters after the main container exits regardless
of its exit code, so the report is still readable. The `EvalRun` controller
therefore derives `status.phase` from the report's own `passed` field when a
report exists, falling back to the raw Workflow phase only when no report was
produced at all (e.g. a crash before writing it). This makes `Failed` mean
"bad model config" uniformly, whether the underlying cause was a real
regression or an infrastructure problem — matching this iteration's
verification line.

## Consequences

- `kubectl get evalruns` gives a quick pass/fail view without needing `argo
  logs` or cluster-side Python — the CRD status is the whole surface.
- Adding Argo Workflows as a cluster dependency is new for this iteration; it
  is not installed by this repo's Helm charts and must be installed
  separately (see [`EvalRun` CRD reference](../reference/evalrun-crd.md)).
- The CI `wer-regression` gate does not depend on any of the above — it runs
  `vox-eval` directly against a docker-compose stack. CI has no cluster (GKE
  is only used to validate a completed iteration), so it uses the same
  harness and exit-code contract without Kubernetes or Argo in the loop.

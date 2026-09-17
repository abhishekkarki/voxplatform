# Run the eval harness

The eval harness (`vox-eval`) computes Word Error Rate (WER) using `jiwer`
against a dataset of `(audio, ground_truth)` pairs — a directory containing
`manifest.csv` plus the referenced audio files. See `eval/datasets/sample` for
the smallest example.

## Local run

```bash
pip install -e clients/python -e "eval[dev]"

vox-eval run eval/datasets/sample \
  --url http://localhost:8080 \
  --threshold 0.25
```

`vox-eval run <dataset>` exits `0` when the corpus WER is within
`--threshold`, and `1` otherwise — that exit code is the whole contract CI and
the `EvalRun` CRD gate on. `--json` prints the full report to stdout instead
of a human-readable summary; `--output <dir>` (default `eval/results`) saves a
timestamped JSON report.

## Against the live cluster

```bash
GATEWAY=$(kubectl get svc gateway -n vox -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
vox-eval run eval/datasets/sample --url "http://${GATEWAY}:8080" --threshold 0.25
```

## Via the `EvalRun` CRD

In-cluster, `EvalRun` delegates the same `vox-eval` invocation to an Argo
Workflow and reports pass/fail as CRD status — see the
[`EvalRun` CRD reference](../reference/evalrun-crd.md) for the full field
list. This requires [Argo Workflows](https://argoproj.github.io/argo-workflows/)
installed in the cluster (not bundled by this repo — install it with the
upstream quick-start manifest).

```bash
kubectl apply -f operator/config/crd/bases/
kubectl apply -f operator/config/samples/evalrun_sample.yaml
kubectl get evalruns -n vox -w
kubectl describe evalrun sample-wer-check -n vox
```

The eval container image (`vox-eval`) bundles the SDK, harness, and dataset —
build it with `docker build -f Dockerfile.eval -t vox-eval .` from the repo
root and push it to the path referenced by the `EvalRun`'s `spec.image`.

## In CI

The `wer-regression` job in `.github/workflows/ci.yml` runs on every PR
against `main`: it brings up `whisper` + `vad` + `gateway` via
`docker compose`, then runs `vox-eval run eval/datasets/sample --threshold
0.25` against it. A regression above that threshold fails the job and blocks
the merge — the same exit-code contract as the local and in-cluster paths
above, just without Kubernetes.

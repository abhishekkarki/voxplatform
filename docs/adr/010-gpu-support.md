# ADR-010: GPU Support — Node Pool, vLLM, whisper-large-v3

**Status:** Accepted
**Date:** 2026-09
**Iteration:** 5

---

## Context

CLAUDE.md's locked scope for iteration 5 is "GPU node pool (T4 spot), vLLM,
whisper-large-v3." Much of the scaffolding for this already existed before
this iteration started: `VoiceModelSpec.device` already had a `gpu` enum
value, `voicemodel_controller.go`'s `defaultResources("gpu")` already
requested `nvidia.com/gpu`, `defaultGPUImage` already pointed at a CUDA
faster-whisper build, and `infra/modules/gke/main.tf` had a **commented-out**
GPU node pool block. This iteration activates that scaffolding and makes five
decisions along the way.

## Decision 1 — GPU node pool zone: `europe-west3-b`, not the cluster's `europe-west3-a`

**Verified via GCP's GPU regions/zones documentation:** NVIDIA T4 is
available in `europe-west3-b` but **not** in `europe-west3-a` (the cluster's
zone — only G2/L4 there) or `europe-west3-c` (no GPUs). The pre-existing
commented Terraform block used `location = var.zone` for the node pool,
which would have failed to provision — T4 simply isn't offered in that zone.

**Fix:** GKE lets a node pool set its own `node_locations`, independent of
the cluster's (control-plane) zone, as long as it's within the same region.
The GPU pool now sets `node_locations = ["europe-west3-b"]`; the cluster and
CPU pool stay in `-a`, untouched. This is a one-field fix, not a cluster
migration.

## Decision 2 — GKE-managed driver install, not the NVIDIA GPU Operator

PROJECT_PLAN.md's original phrasing was "NVIDIA GPU Operator on GKE." The
pre-existing commented block already had
`gpu_driver_installation_config { gpu_driver_version = "LATEST" }` on the
node pool — GKE's own managed driver lifecycle (a GKE-owned DaemonSet, no
extra CRDs). The standalone NVIDIA GPU Operator is the pattern for
self-managed Kubernetes (bare-metal, or managed services without an
equivalent) and would duplicate what GKE already does natively here, plus add
its own CRDs/controllers for a single node pool serving at most one node.
Kept as `gpu_driver_installation_config`, not the Operator — "stop
immediately if... adding tech not needed."

## Decision 3 — vLLM serves the Qwen summarizer; Whisper large-v3 stays on faster-whisper

The one place this iteration deviates from a literal "vLLM ... whisper-large-v3"
reading (as "vLLM serves Whisper"):

- vLLM's maturity and OpenAI-compatible serving story is for **text LLMs**.
  Whisper/audio serving in vLLM is newer and far less battle-tested than
  `faster-whisper` (CTranslate2) — which is already what `defaultGPUImage`
  points at (`fedirz/faster-whisper-server:0.3.0-cuda`), and the operator's
  Deployment builder already reads `WHISPER__MODEL`/`WHISPER__INFERENCE_DEVICE`/
  `WHISPER__COMPUTE_TYPE`, i.e. it's already shaped for this exact image
  family. Moving STT to vLLM means building and trusting an unproven serving
  path with no way to validate it in this environment (no GPU, no `vllm`
  package here).
- vLLM **is** the standard, well-supported choice for serving Qwen —
  replacing `llama.cpp` (CPU-only today) is the natural GPU upgrade for the
  summarizer, and Qwen2.5-7B-Instruct is a stock HF model vLLM serves
  out of the box, no GGUF conversion needed.

Net effect: `whisper-large-v3` ships on the already-half-built faster-whisper
GPU path (a model id + a sample manifest, see
`operator/config/samples/voicemodels/whisper-large-v3-gpu.yaml`); `vLLM`
ships as a new summarizer engine (see Decision 5). Both named deliverables
land; the implementation risk sits on the well-supported side of each choice.

## Decision 4 — No `VoiceModelSpec` changes for GPU scheduling

`defaultResources("gpu")` already sets `nvidia.com/gpu: "1"` in both requests
and limits. That alone is sufficient:

- The scheduler only places pods requesting `nvidia.com/gpu` on nodes that
  advertise that allocatable resource — i.e. only the GPU pool.
- The GPU pool's `nvidia.com/gpu=present:NoSchedule` taint is handled by
  GKE's **ExtendedResourceToleration** admission controller (enabled by
  default on GKE Standard clusters): pods requesting the matching extended
  resource get an automatic toleration for it.

No `nodeSelector`/`tolerations` fields were added to `VoiceModelSpec`. If
that admission controller were ever disabled, GPU VoiceModels would need an
explicit toleration — not needed today, and not worth the CRD surface area
ahead of that.

**This does not generalize to every GPU-adjacent workload**, which is why the
DCGM exporter (Decision 6) needed an *explicit* toleration: it deliberately
does **not** request `nvidia.com/gpu` (see that section for why), so it's
never eligible for the automatic one.

## Decision 5 — Summarizer gets an `ENGINE` switch, not a second service

`services/summarizer/summarizer_server.py` already isolated model-loading
behind a `_mode` variable and `_summarize_llm()`. Adding `ENGINE=vllm`
(`_load_model_vllm()` / `_summarize_vllm()`, sharing the same ChatML prompt
builder) keeps the exact same `/summarize` request/response JSON — the
gateway's pipeline handler (`internal/gateway/pipeline.go`) needs zero
changes, matching the project's Go/Python boundary rule (share JSON, never
code — and here, not even the JSON shape changes). `MODEL_REPO` is
reinterpreted as a plain HF model id under `ENGINE=vllm` (vLLM downloads and
loads it directly) rather than a GGUF repo. A second Dockerfile
(`Dockerfile.gpu`, CUDA base + `vllm` instead of `llama-cpp-python`) sits next
to the existing CPU one, which is unchanged and stays the default.

**Not verified against a real GPU or the `vllm` package** — no GPU hardware
or CUDA runtime in this environment. Verified: Python syntax only
(`py_compile`). Review and smoke-test against a real T4 node before relying
on `ENGINE=vllm` in production.

## Decision 6 — DCGM exporter DaemonSet + plain ServiceMonitor, no new dashboard

`deploy/helm/monitoring/values.yaml` already sets
`serviceMonitorSelectorNilUsesHelmValues: false`, so kube-prometheus-stack
scrapes any `ServiceMonitor` in the cluster with no extra chart wiring. The
new `deploy/helm/gpu-metrics` chart is a DaemonSet running
`nvcr.io/nvidia/k8s/dcgm-exporter` + a `Service` + that `ServiceMonitor`.

The exporter's DaemonSet deliberately does **not** request `nvidia.com/gpu`
itself: on a single-GPU T4 node, doing so would consume the node's only
allocatable GPU unit and starve the actual inference workload it's supposed
to be monitoring. It needs an explicit `nodeSelector` (matching
`cloud.google.com/gke-accelerator: nvidia-tesla-t4`) and an explicit
`toleration` for the GPU taint instead — see Decision 4's caveat.

There are no existing Grafana dashboard JSON files anywhere in this repo
(dashboards are the stock kube-prometheus-stack ones); hand-authoring a
bespoke DCGM dashboard is a separate, bigger effort than "GPU metrics" calls
for, and not something renderable/verifiable without a real Grafana +
DCGM feed in front of me. NVIDIA publishes a
[DCGM Grafana dashboard](https://grafana.com/grafana/dashboards/12239-nvidia-dcgm-exporter-dashboard/)
(ID `12239`) that can be imported once the exporter is live — pointed to in
the how-to guide instead of duplicated here.

## Decision 7 — GPU pool off by default

`gpu_enabled = false` in `infra/environments/dev/terraform.tfvars`. Fresh GCP
projects have 0 T4 quota until requested, and idle GPU nodes cost real money
even at spot pricing. `terraform apply` for anyone still working through
iterations 0–4 is unaffected; turning the pool on is `-var gpu_enabled=true`
plus a quota request, documented in
[Scale the cluster](../how-to/scale-cluster.md#gpu-pool).

## Decision 8 — Cost comparison: a model, not a benchmark

PROJECT_PLAN.md's "cost comparison" / "blog: CPU vs GPU — the numbers"
deliverables need real throughput and latency numbers from an actual run,
which this session can't produce (no GPU hardware, no live cluster). What
follows is the cost **model** — nodes × spot $/hr — not a throughput
comparison:

```
GPU pool cost ≈ node_count × T4_spot_$/hr × hours_running
CPU pool cost ≈ node_count × e2-standard-4_spot_$/hr × hours_running
```

Public secondary sources (GetDeploying, Thunder Compute, DevZero — see links
below) put NVIDIA T4 spot/preemptible pricing on GCP in roughly the
**$0.10–$0.30/hr** range depending on region and how discounts are
calculated at the time; none of them confirm a `europe-west3-b`-specific spot
rate, and spot prices change up to once a month. **Treat that range as
order-of-magnitude only** — get the current number from
[Google Cloud's GPU pricing page](https://cloud.google.com/compute/gpus-pricing)
or `gcloud` before making a real budgeting decision, and don't cite the range
above as a quote.

The throughput/latency half of the comparison (req/s, p50/p99 latency on T4
vs CPU, cost-per-1000-transcriptions) needs a real run against
`whisper-large-v3-gpu` and `qwen-summarizer-gpu` — that's the manual
follow-up this ADR flags rather than fabricates. No blog post ships with this
iteration for the same reason: it would need those numbers to be honest.

## Consequences

- `gpu_enabled = true` plus a T4 quota request is the only step between "iterations
  0–4 dev setup" and a working GPU pool — no code changes needed to turn it on.
- `whisper-large-v3-gpu` and `qwen-summarizer-gpu` VoiceModel samples exist
  and validate against the CRD schema, but are unverified against real
  hardware, same caveat as Decision 5.
- A future GPU-serving addition (e.g. a second GPU model) follows the same
  pattern: `device: gpu` + `defaultResources`/image defaults if it's another
  faster-whisper-shaped model, or an explicit `image`/`resources` override
  (like the Qwen GPU sample) if it isn't.
- The cost/throughput comparison and blog post remain open until someone runs
  this against real hardware — tracked as follow-up work, not silently
  dropped.

# ADR-001: CPU-first inference strategy

## Status
Accepted

## Context
We need to choose whether to start with GPU or CPU inference for the
initial iterations of voxplatform. GPU gives better latency and throughput
but costs significantly more and adds infrastructure complexity (drivers,
device plugins, scheduling).

## Decision
Start CPU-only for iterations 0-4. Add GPU as a node pool in iteration 5.

## Rationale
- **Cost:** GCP free credits ($300) last ~3 months on CPU. A T4 GPU spot
  instance adds ~$120/month, cutting runway to ~1 month.
- **Learning:** CPU inference forces understanding of quantization (INT8),
  memory bandwidth bottlenecks, and batching economics — fundamentals that
  GPU-only engineers often skip.
- **Platform design:** If the CRD abstraction works for CPU, adding GPU is
  a config change (`nvidia.com/gpu: 1`), not a redesign. This proves the
  platform's accelerator-agnostic design.
- **Model availability:** faster-whisper-small.en runs real-time on 4 CPU
  cores with INT8 quantization. llama.cpp serves Qwen 3B at 15-25 tok/s
  on CPU. Both are viable for development and eval.

## Consequences
- Iteration 0-4 latency will be higher than production targets
- Some GPU-specific patterns (KV cache, continuous batching, VRAM
  management) are deferred to iteration 5
- The eval baselines established on CPU will need re-baselining on GPU

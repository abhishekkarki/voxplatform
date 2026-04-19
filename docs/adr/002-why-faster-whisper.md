# ADR-002: Why faster-whisper-server over Triton for v0

## Status
Accepted

## Context
Multiple inference servers can serve Whisper models: NVIDIA Triton,
faster-whisper-server, whisper.cpp server, and vLLM. We need to pick
one for iteration 0 that runs on CPU with minimal setup.

## Decision
Use `fedirz/faster-whisper-server` for iterations 0-4.
Switch to vLLM with whisper-large-v3 in iteration 5 when GPU is added.

## Rationale
- **CPU performance:** CTranslate2 backend is the fastest CPU inference
  for encoder-decoder models. Real-time factor ~0.3x on 4 cores for
  small.en with INT8.
- **OpenAI-compatible API:** Exposes `/v1/audio/transcriptions` — same
  contract as OpenAI Whisper API. Gateway code won't need rewriting.
- **Single container:** No model repository setup, no config.pbtxt, no
  ensemble pipeline config. One image, one env var for model name.
- **Prometheus metrics:** Built-in `/metrics` endpoint. No sidecar needed.
- **Triton is overkill for v0:** Triton's value is multi-model serving,
  dynamic batching across model types, and GPU scheduling — none of which
  we need until iteration 5. Its setup cost (model repository, config
  files, ensemble pipelines) would consume 2-3 days for zero benefit.

## Consequences
- We're locked to CTranslate2-compatible models (faster-whisper variants)
- Streaming is HTTP chunked, not native gRPC streaming (gateway handles
  the translation)
- Migration to vLLM/Triton in iteration 5 requires new Helm chart but
  the VoiceModel CRD abstracts this from consumers

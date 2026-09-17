"""Summarization service — llama-cpp-python + Qwen GGUF (CPU) or vLLM + Qwen (GPU).

Accepts a transcript and optional speaker segments, returns a concise summary.
Runs as a standalone service the gateway calls as the final stage of the
InferencePipeline (STT → diarize → summarize).

Two interchangeable engines, selected by ENGINE, both producing the exact
same /summarize request/response shape — the gateway's pipeline handler
doesn't know or care which one is running:
    ENGINE=llama_cpp (default)  CPU, GGUF, via llama-cpp-python. See
                                 services/summarizer/Dockerfile.
    ENGINE=vllm                 GPU, via vLLM. See
                                 services/summarizer/Dockerfile.gpu. Needs the
                                 GPU node pool (iteration 5) — see ADR-010.
                                 NOT verified against real vLLM/GPU hardware
                                 in this repo's CI; review before relying on
                                 it in production.

Configuration (environment variables):
    ENGINE          "llama_cpp" (default) or "vllm"
    MODEL_REPO      For llama_cpp: HuggingFace repo containing the GGUF file
                    (default: "Qwen/Qwen2.5-3B-Instruct-GGUF").
                    For vllm: a plain HuggingFace model id, no GGUF needed
                    (e.g. "Qwen/Qwen2.5-7B-Instruct").
    MODEL_FILE      llama_cpp only: GGUF filename inside MODEL_REPO
                    (default: "qwen2.5-3b-instruct-q4_k_m.gguf")
    MODEL_DIR       llama_cpp only: local directory for the downloaded model
                    (default: /data/models)
    N_CTX           llama_cpp only: context window in tokens (default: 2048)
    GPU_MEMORY_UTILIZATION  vllm only: fraction of GPU memory vLLM may use
                    (default: 0.85)
    MAX_TOKENS      Maximum tokens in the summary response (default: 256)
    SUMMARIZER_PORT Port to listen on (default: 8003)

API:
    POST /summarize
        JSON body: {"transcript": "...", "segments": [...], "max_tokens": 150}
        Returns: {"summary": "...", "duration_seconds": F}

    GET /health
        Returns {"status": "ok", "mode": "llm"|"vllm"|"extractive"}
"""

from __future__ import annotations

import logging
import os
import time
from pathlib import Path
from typing import Any

from fastapi import FastAPI, HTTPException
from fastapi.responses import JSONResponse
from pydantic import BaseModel

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
logger = logging.getLogger(__name__)

app = FastAPI(title="VoxPlatform Summarizer", version="0.1.0")

# Global model handle
_llm = None
_mode = "extractive"  # "llm", "vllm", or "extractive"

ENGINE     = os.getenv("ENGINE", "llama_cpp")
MODEL_REPO = os.getenv("MODEL_REPO", "Qwen/Qwen2.5-3B-Instruct-GGUF")
MODEL_FILE = os.getenv("MODEL_FILE", "qwen2.5-3b-instruct-q4_k_m.gguf")
MODEL_DIR  = Path(os.getenv("MODEL_DIR", "/data/models"))
N_CTX      = int(os.getenv("N_CTX", "2048"))
MAX_TOKENS = int(os.getenv("MAX_TOKENS", "256"))
GPU_MEMORY_UTILIZATION = float(os.getenv("GPU_MEMORY_UTILIZATION", "0.85"))


class SummarizeRequest(BaseModel):
    transcript: str
    segments: list[dict[str, Any]] = []
    max_tokens: int = MAX_TOKENS


@app.on_event("startup")
async def load_model() -> None:
    global _llm, _mode

    loader = _load_model_vllm if ENGINE == "vllm" else _load_model_llama_cpp

    try:
        loader()
    except ImportError:
        logger.warning(
            "%s not installed — running in extractive fallback mode",
            "vllm" if ENGINE == "vllm" else "llama-cpp-python or huggingface_hub",
        )
        _mode = "extractive"
    except Exception as exc:
        logger.warning("failed to load %s model (%s) — running in extractive mode", ENGINE, exc)
        _mode = "extractive"


def _load_model_llama_cpp() -> None:
    """Load a GGUF model via llama-cpp-python (CPU)."""
    global _llm, _mode

    from llama_cpp import Llama
    from huggingface_hub import hf_hub_download

    MODEL_DIR.mkdir(parents=True, exist_ok=True)
    model_path = MODEL_DIR / MODEL_FILE

    if not model_path.exists():
        logger.info("downloading %s/%s ...", MODEL_REPO, MODEL_FILE)
        hf_hub_download(
            repo_id=MODEL_REPO,
            filename=MODEL_FILE,
            local_dir=str(MODEL_DIR),
        )
        logger.info("model downloaded to %s", model_path)

    logger.info("loading GGUF model from %s ...", model_path)
    _llm = Llama(
        model_path=str(model_path),
        n_ctx=N_CTX,
        n_threads=os.cpu_count() or 4,
        verbose=False,
    )
    _mode = "llm"
    logger.info("summarizer model ready (llm mode)")


def _load_model_vllm() -> None:
    """Load a HuggingFace model via vLLM (GPU).

    Unlike llama_cpp, MODEL_REPO here is a plain HF model id (e.g.
    "Qwen/Qwen2.5-7B-Instruct") — vLLM downloads and loads it directly, no
    GGUF conversion step. Requires services/summarizer/Dockerfile.gpu and a
    GPU node (device: gpu) — see ADR-010. Not exercised against real
    hardware in this repo's tests; review before relying on it in production.
    """
    global _llm, _mode

    from vllm import LLM

    logger.info("loading %s via vLLM ...", MODEL_REPO)
    _llm = LLM(
        model=MODEL_REPO,
        dtype="float16",
        gpu_memory_utilization=GPU_MEMORY_UTILIZATION,
        max_model_len=N_CTX,
    )
    _mode = "vllm"
    logger.info("summarizer model ready (vllm mode)")


@app.get("/health")
async def health() -> JSONResponse:
    return JSONResponse({"status": "ok", "mode": _mode})


@app.post("/summarize")
async def summarize(req: SummarizeRequest) -> JSONResponse:
    """Summarize a transcript, optionally incorporating speaker segments."""
    if not req.transcript.strip():
        raise HTTPException(status_code=400, detail="transcript is empty")

    start = time.monotonic()

    if _mode == "llm":
        summary = _summarize_llm(req.transcript, req.segments, req.max_tokens)
    elif _mode == "vllm":
        summary = _summarize_vllm(req.transcript, req.segments, req.max_tokens)
    else:
        summary = _summarize_extractive(req.transcript)

    duration = time.monotonic() - start
    logger.info(
        "summarization complete mode=%s input_chars=%d summary_chars=%d duration=%.2fs",
        _mode, len(req.transcript), len(summary), duration,
    )

    return JSONResponse({
        "summary": summary,
        "duration_seconds": round(duration, 3),
    })


def _build_prompt(transcript: str, segments: list[dict[str, Any]]) -> str:
    """Build the ChatML prompt shared by both the llama_cpp and vllm engines."""
    # Build speaker-aware context if we have diarization segments
    if segments:
        speaker_lines = []
        for seg in segments:
            speaker = seg.get("speaker", "Speaker")
            text    = seg.get("text", "")
            if text:
                speaker_lines.append(f"{speaker}: {text}")
        context = "\n".join(speaker_lines) if speaker_lines else transcript
    else:
        context = transcript

    return (
        "<|im_start|>system\n"
        "You are a helpful assistant that writes concise meeting summaries. "
        "Keep summaries under 3 sentences. Focus on key decisions and action items.\n"
        "<|im_end|>\n"
        "<|im_start|>user\n"
        f"Summarize this transcript:\n\n{context}\n"
        "<|im_end|>\n"
        "<|im_start|>assistant\n"
    )


def _summarize_llm(
    transcript: str,
    segments: list[dict[str, Any]],
    max_tokens: int,
) -> str:
    """Full summarization via Qwen GGUF model (llama-cpp-python, CPU)."""
    prompt = _build_prompt(transcript, segments)

    response = _llm(
        prompt,
        max_tokens=max_tokens,
        stop=["<|im_end|>"],
        echo=False,
    )
    return response["choices"][0]["text"].strip()


def _summarize_vllm(
    transcript: str,
    segments: list[dict[str, Any]],
    max_tokens: int,
) -> str:
    """Full summarization via Qwen model served by vLLM (GPU).

    Same prompt template and stop sequence as _summarize_llm, so the two
    engines produce comparable output for the same input.
    """
    from vllm import SamplingParams

    prompt = _build_prompt(transcript, segments)
    sampling_params = SamplingParams(max_tokens=max_tokens, stop=["<|im_end|>"])

    outputs = _llm.generate([prompt], sampling_params)
    return outputs[0].outputs[0].text.strip()


def _summarize_extractive(transcript: str) -> str:
    """Fallback: return the first 3 sentences as an extractive summary.

    Used when llama-cpp-python is not installed or the model failed to load.
    The gateway still gets a valid response so the pipeline can return a result.
    """
    # Simple sentence splitter (handles . ! ?)
    import re
    sentences = re.split(r"(?<=[.!?])\s+", transcript.strip())
    summary = " ".join(sentences[:3])
    return summary if summary else transcript[:500]

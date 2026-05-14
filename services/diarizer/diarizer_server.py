"""Speaker diarization service using pyannote-audio 3.x.

Accepts an audio file, returns time-stamped speaker segments.
Runs as a sidecar-style service that the gateway calls as part of the
InferencePipeline (STT → diarize → summarize).

Configuration (environment variables):
    HF_TOKEN         HuggingFace token for downloading the pyannote model.
                     Required for full diarization. Without it, the service
                     falls back to single-speaker mode (all audio is SPEAKER_00).
    DIARIZER_PORT    Port to listen on (default: 8002).
    MIN_SPEAKERS     Minimum number of speakers hint (default: 1).
    MAX_SPEAKERS     Maximum number of speakers hint (default: 10).

API:
    POST /diarize
        multipart/form-data with field "file" (WAV, 16kHz recommended)
        Returns: {"segments": [...], "num_speakers": N, "duration_seconds": F}

    GET /health
        Returns {"status": "ok", "mode": "pyannote"|"fallback"}
"""

from __future__ import annotations

import io
import logging
import os
import tempfile
import time
from typing import Any

import numpy as np
from fastapi import FastAPI, Request, UploadFile, File, HTTPException
from fastapi.responses import JSONResponse

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
logger = logging.getLogger(__name__)

app = FastAPI(title="VoxPlatform Diarizer", version="0.1.0")

# Global pipeline — loaded once at startup
_pipeline = None
_mode = "fallback"  # "pyannote" or "fallback"

HF_TOKEN = os.getenv("HF_TOKEN", "")
MIN_SPEAKERS = int(os.getenv("MIN_SPEAKERS", "1"))
MAX_SPEAKERS = int(os.getenv("MAX_SPEAKERS", "10"))


@app.on_event("startup")
async def load_model() -> None:
    global _pipeline, _mode

    if not HF_TOKEN:
        logger.warning(
            "HF_TOKEN not set — running in fallback (single-speaker) mode. "
            "Set HF_TOKEN and accept the pyannote model terms at "
            "https://huggingface.co/pyannote/speaker-diarization-3.1 "
            "to enable full diarization."
        )
        _mode = "fallback"
        return

    try:
        from pyannote.audio import Pipeline

        logger.info("loading pyannote/speaker-diarization-3.1 ...")
        _pipeline = Pipeline.from_pretrained(
            "pyannote/speaker-diarization-3.1",
            use_auth_token=HF_TOKEN,
        )
        _mode = "pyannote"
        logger.info("pyannote pipeline loaded")
    except Exception as exc:
        logger.warning(
            "failed to load pyannote (%s) — running in fallback mode", exc
        )
        _mode = "fallback"


@app.get("/health")
async def health() -> JSONResponse:
    return JSONResponse({"status": "ok", "mode": _mode})


@app.post("/diarize")
async def diarize(file: UploadFile = File(...)) -> JSONResponse:
    """Diarize an audio file and return speaker-labelled time segments."""
    start = time.monotonic()
    audio_bytes = await file.read()

    if _mode == "pyannote":
        segments, num_speakers = _diarize_pyannote(audio_bytes)
    else:
        segments, num_speakers = _diarize_fallback(audio_bytes)

    duration = time.monotonic() - start
    logger.info(
        "diarization complete mode=%s speakers=%d segments=%d duration=%.2fs",
        _mode, num_speakers, len(segments), duration,
    )

    return JSONResponse({
        "segments": segments,
        "num_speakers": num_speakers,
        "duration_seconds": round(duration, 3),
    })


def _diarize_pyannote(audio_bytes: bytes) -> tuple[list[dict[str, Any]], int]:
    """Full diarization via pyannote-audio 3.x."""
    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as f:
        f.write(audio_bytes)
        tmp_path = f.name

    try:
        diarization = _pipeline(
            tmp_path,
            min_speakers=MIN_SPEAKERS,
            max_speakers=MAX_SPEAKERS,
        )
    finally:
        os.unlink(tmp_path)

    segments = []
    speakers = set()
    for turn, _, speaker in diarization.itertracks(yield_label=True):
        segments.append({
            "start": round(turn.start, 3),
            "end":   round(turn.end,   3),
            "speaker": speaker,
        })
        speakers.add(speaker)

    return segments, len(speakers)


def _diarize_fallback(audio_bytes: bytes) -> tuple[list[dict[str, Any]], int]:
    """Fallback: return the whole audio as a single SPEAKER_00 segment.

    Used when HF_TOKEN is not configured or pyannote failed to load.
    The gateway still gets a valid response so the pipeline can continue.
    """
    duration = _estimate_duration(audio_bytes)
    segments = [{"start": 0.0, "end": round(duration, 3), "speaker": "SPEAKER_00"}]
    return segments, 1


def _estimate_duration(audio_bytes: bytes) -> float:
    """Estimate audio duration from WAV header or by sample count."""
    try:
        # WAV header: bytes 24-27 = sample rate, bytes 28-31 = byte rate
        if len(audio_bytes) >= 44 and audio_bytes[:4] == b"RIFF":
            import struct
            sample_rate = struct.unpack_from("<I", audio_bytes, 24)[0]
            byte_rate   = struct.unpack_from("<I", audio_bytes, 28)[0]
            data_size   = len(audio_bytes) - 44
            if byte_rate > 0:
                return data_size / byte_rate
    except Exception:
        pass
    # Rough fallback: assume 16kHz 16-bit mono
    return max(len(audio_bytes) / (16000 * 2), 0.1)

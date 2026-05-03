"""Voice Activity Detection (VAD) service using Silero VAD.

Runs as a lightweight sidecar container alongside the Go gateway.
The gateway sends 20ms audio frames (raw PCM), the VAD service
responds with {"speech": true/false}.

Silero VAD is a small, fast, accurate model specifically designed
for real-time voice activity detection. It runs on CPU without
any GPU needed.

Usage:
    uvicorn vad_server:app --host 0.0.0.0 --port 8001

Protocol:
    POST /vad
    Body: raw PCM audio bytes (16kHz, 16-bit, mono)
    Response: {"speech": true, "confidence": 0.87}

    GET /health
    Response: {"status": "ok"}
"""

from __future__ import annotations

import io
import struct
import logging

import numpy as np
import torch
from fastapi import FastAPI, Request, Response
from fastapi.responses import JSONResponse

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
logger = logging.getLogger(__name__)

app = FastAPI(title="VoxPlatform VAD Service", version="0.1.0")

# Global model — loaded once at startup
vad_model = None
SAMPLE_RATE = 16000
# Silero VAD expects specific window sizes: 512 for 16kHz
WINDOW_SIZE = 512
# Speech probability threshold — above this = speech detected
THRESHOLD = 0.5


@app.on_event("startup")
async def load_model():
    """Load Silero VAD model at startup.

    The model is downloaded from torch.hub on first run,
    then cached locally. It's ~2MB and loads in <1 second.
    """
    global vad_model
    logger.info("Loading Silero VAD model...")

    vad_model, _ = torch.hub.load(
        repo_or_dir="snakers4/silero-vad",
        model="silero_vad",
        trust_repo=True,
    )
    vad_model.eval()

    logger.info("Silero VAD model loaded successfully")


@app.get("/health")
async def health():
    """Health check — used by Kubernetes liveness probe."""
    if vad_model is None:
        return JSONResponse(
            status_code=503,
            content={"status": "not ready", "reason": "model not loaded"},
        )
    return {"status": "ok"}


@app.post("/vad")
async def detect_speech(request: Request):
    """Detect speech in an audio frame.

    Expects raw PCM audio bytes (16kHz, 16-bit signed, mono).
    A typical frame is 20ms = 640 bytes (320 samples).

    Silero VAD works on 512-sample windows at 16kHz.
    If the input frame is a different size, we pad or truncate.

    Returns:
        {"speech": true/false, "confidence": 0.0-1.0}
    """
    if vad_model is None:
        return JSONResponse(
            status_code=503,
            content={"error": "model not loaded"},
        )

    # Read raw PCM bytes
    raw_bytes = await request.body()

    if len(raw_bytes) < 2:
        return {"speech": False, "confidence": 0.0}

    # Convert raw PCM bytes to float32 numpy array
    # PCM 16-bit signed little-endian → float32 [-1.0, 1.0]
    num_samples = len(raw_bytes) // 2
    samples = struct.unpack(f"<{num_samples}h", raw_bytes[:num_samples * 2])
    audio = np.array(samples, dtype=np.float32) / 32768.0

    # Silero expects exactly WINDOW_SIZE samples
    if len(audio) < WINDOW_SIZE:
        # Pad with zeros
        audio = np.pad(audio, (0, WINDOW_SIZE - len(audio)))
    elif len(audio) > WINDOW_SIZE:
        # Process in windows and take max confidence
        max_confidence = 0.0
        for i in range(0, len(audio) - WINDOW_SIZE + 1, WINDOW_SIZE):
            chunk = audio[i:i + WINDOW_SIZE]
            tensor = torch.from_numpy(chunk)
            confidence = vad_model(tensor, SAMPLE_RATE).item()
            max_confidence = max(max_confidence, confidence)

        return {
            "speech": max_confidence >= THRESHOLD,
            "confidence": round(max_confidence, 4),
        }

    # Single window
    tensor = torch.from_numpy(audio)
    confidence = vad_model(tensor, SAMPLE_RATE).item()

    return {
        "speech": confidence >= THRESHOLD,
        "confidence": round(confidence, 4),
    }


@app.post("/vad/reset")
async def reset_state():
    """Reset VAD internal state.

    Silero VAD is stateful — it tracks context across frames
    for better accuracy. Call this between sessions to reset.
    """
    if vad_model is not None:
        vad_model.reset_states()
    return {"status": "reset"}
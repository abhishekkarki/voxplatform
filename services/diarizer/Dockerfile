# Speaker diarization sidecar — pyannote-audio 3.x on CPU
# The pyannote model (~1GB) is NOT baked in; it downloads at first startup
# and is cached in the HF_HOME volume. Subsequent starts are fast.
#
# Required at runtime:
#   HF_TOKEN  — HuggingFace token. Without it the service runs in
#               fallback (single-speaker) mode.

FROM python:3.12-slim

WORKDIR /app

# Install CPU-only PyTorch (avoids pulling the 3GB CUDA wheels)
RUN pip install --no-cache-dir \
    torch==2.3.1 torchaudio==2.3.1 \
    --index-url https://download.pytorch.org/whl/cpu

# Install pyannote and service dependencies
RUN pip install --no-cache-dir \
    pyannote.audio==3.3.2 \
    fastapi==0.115.0 \
    uvicorn==0.30.0 \
    python-multipart==0.0.9 \
    numpy>=1.26.0

COPY diarizer_server.py .

RUN useradd -m -u 1000 appuser
USER appuser

# HF_HOME is mounted as a Docker volume so the model survives restarts
ENV HF_HOME=/data/huggingface

EXPOSE 8002

CMD ["uvicorn", "diarizer_server:app", "--host", "0.0.0.0", "--port", "8002"]

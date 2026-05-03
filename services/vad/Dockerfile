# VAD sidecar — Silero VAD as a FastAPI service
# Runs alongside the Go gateway in the same pod
# CPU only, ~500MB image with PyTorch CPU

FROM python:3.12-slim

WORKDIR /app

# Install CPU-only PyTorch (much smaller than CUDA version)
RUN pip install --no-cache-dir \
    torch --index-url https://download.pytorch.org/whl/cpu

# Install remaining dependencies
RUN pip install --no-cache-dir \
    fastapi==0.115.0 \
    uvicorn==0.30.0 \
    numpy>=1.26.0 \
    torchaudio --index-url https://download.pytorch.org/whl/cpu \
    packaging \
    onnxruntime

COPY vad_server.py .

# Pre-download the Silero VAD model during build so startup is fast
RUN python -c "import torch; torch.hub.load('snakers4/silero-vad', 'silero_vad', trust_repo=True)"

# Don't run as root
RUN useradd -m -u 1000 appuser
USER appuser

EXPOSE 8001

CMD ["uvicorn", "vad_server:app", "--host", "0.0.0.0", "--port", "8001"]
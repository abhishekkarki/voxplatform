# Summarization service — llama-cpp-python + Qwen 3B GGUF on CPU
# The model (~2GB) is NOT baked in; it downloads to the MODEL_DIR volume
# on first startup. Subsequent starts load from the volume (~10s).
#
# Build args:
#   CMAKE_ARGS  passed to llama-cpp-python build (default: CPU-only BLAS)

FROM python:3.12-slim

WORKDIR /app

# System deps for llama-cpp-python C++ compilation
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    cmake \
    && rm -rf /var/lib/apt/lists/*

# Install llama-cpp-python (CPU build — no CUDA/Metal)
# FORCE_CMAKE=1 avoids the pre-built wheel which may not match our libc
RUN CMAKE_ARGS="-DGGML_BLAS=OFF" pip install --no-cache-dir \
    llama-cpp-python==0.3.4 \
    huggingface_hub>=0.23.0

RUN pip install --no-cache-dir \
    fastapi==0.115.0 \
    uvicorn==0.30.0 \
    pydantic>=2.0

COPY summarizer_server.py .

RUN useradd -m -u 1000 appuser
USER appuser

# MODEL_DIR is mounted as a Docker volume so the model survives restarts
ENV MODEL_DIR=/data/models

EXPOSE 8003

CMD ["uvicorn", "summarizer_server:app", "--host", "0.0.0.0", "--port", "8003"]

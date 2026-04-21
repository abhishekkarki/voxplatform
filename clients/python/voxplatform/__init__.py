""" VoxPlatform Python Client SDK.

Usage: 
    from voxplatform import VoxClient

    client = VoxClient("http://localhost:8080")
    result = client.transcribe("audio.wav")
    print(result.text)
"""

from voxplatform.client import VoxClient
from voxplatform.models import (
    TranscriptionResult,
    ModelInfo,
    HealthStatus,
    VoxError,
)

__version__ = "0.1.0"
__all__ = [
    "VoxClient",
    "TranscriptionResult",
    "ModelInfo",
    "HealthStatus",
    "VoxError",
]
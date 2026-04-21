"""Data models for VoxPlatform API responses.

Every response from the gateway is validated through these Pydantic models.
If the gateway returns unexpected data, you get a clear validation error instead of a KeyError buried deep in your code.
"""
from __future__ import annotations
 
from pydantic import BaseModel, Field
 
 
class TranscriptionResult(BaseModel):
    """Result of a transcription request.
 
    Example:
        result = client.transcribe("audio.wav")
        print(result.text)              # "Hello world"
        print(result.processing_time)   # 2.34 (seconds)
        print(result.request_id)        # "a1b2c3d4e5f6g7h8"
    """
 
    text: str = Field(description="The transcribed text")
    model: str = Field(description="Model used for transcription")
    processing_time: float = Field(
        alias="processing_time_seconds",
        description="Time taken to process the audio in seconds",
    )
    request_id: str = Field(description="Unique request identifier for debugging")
    created_at: str = Field(description="Timestamp of the transcription")
 
 
class ModelInfo(BaseModel):
    """Information about an available model."""
 
    id: str = Field(description="Model identifier")
    type: str = Field(description="Model type (e.g., 'stt')")
    state: str = Field(description="Model state (e.g., 'loaded')")
 
 
class ModelsResponse(BaseModel):
    """Response from the /v1/models endpoint."""
 
    models: list[ModelInfo]
 
 
class HealthStatus(BaseModel):
    """Health check response."""
 
    status: str = Field(description="'ok' or 'not ready'")
    reason: str | None = Field(default=None, description="Reason if not ready")
 
 
class APIErrorDetail(BaseModel):
    """Structured error from the gateway."""
 
    code: str = Field(description="Error code (e.g., 'file_required')")
    message: str = Field(description="Human-readable error message")
    request_id: str = Field(description="Request ID for debugging")
 
 
class APIErrorResponse(BaseModel):
    """Wrapper for error responses."""
 
    error: APIErrorDetail
 
 
class VoxError(Exception):
    """Exception raised when the gateway returns an error.
 
    Attributes:
        code: Error code from the gateway (e.g., 'backend_error')
        message: Human-readable error message
        request_id: Request ID for correlating with server logs
        status_code: HTTP status code
 
    Example:
        try:
            result = client.transcribe("audio.wav")
        except VoxError as e:
            print(f"Error: {e.message}")
            print(f"Request ID: {e.request_id}")  # Give this to the infra team
    """
 
    def __init__(
        self,
        code: str,
        message: str,
        request_id: str,
        status_code: int,
    ):
        self.code = code
        self.message = message
        self.request_id = request_id
        self.status_code = status_code
        super().__init__(f"[{code}] {message} (request_id={request_id})")
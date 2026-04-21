"""VoxPlatform client for sending audio and receiving transcriptions.

Supports both synchronous and asynchronous usage:

    # Sync
    client = VoxClient("http://localhost:8080")
    result = client.transcribe("audio.wav")

    # Async
    async with VoxClient("http://localhost:8080") as client:
        result = await client.atranscribe("audio.wav")
"""

from __future__ import annotations

from pathlib import Path
from typing import BinaryIO

import httpx

from voxplatform.models import (
    APIErrorResponse,
    HealthStatus,
    ModelInfo,
    ModelsResponse,
    TranscriptionResult,
    VoxError,
)

# Audio formats the gateway accepts
SUPPORTED_FORMATS = {".wav", ".mp3", ".m4a", ".ogg", ".flac", ".webm"}

# MIME type mapping for multipart uploads
MIME_TYPES = {
    ".wav": "audio/wav",
    ".mp3": "audio/mpeg",
    ".m4a": "audio/mp4",
    ".ogg": "audio/ogg",
    ".flac": "audio/flac",
    ".webm": "audio/webm",
}


class VoxClient:
    """Client for the VoxPlatform inference gateway.

    Args:
        base_url: Gateway URL (e.g., "http://localhost:8080")
        timeout: Request timeout in seconds. Transcription can be slow
                 on CPU, so the default is generous.
        request_id: Optional request ID to send with every request.
                    Useful for tracing requests across services.

    Example:
        client = VoxClient("http://localhost:8080")

        # Check if the gateway is ready
        health = client.health()
        print(health.status)  # "ok"

        # List available models
        models = client.models()
        for m in models:
            print(f"{m.id} ({m.state})")

        # Transcribe an audio file
        result = client.transcribe("meeting.wav")
        print(result.text)
        print(f"Took {result.processing_time:.1f}s")
    """

    def __init__(
        self,
        base_url: str = "http://localhost:8080",
        timeout: float = 120.0,
        request_id: str | None = None,
    ):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self._default_request_id = request_id

        # httpx client — connection pooling, keeps TCP connections alive
        # across multiple requests (unlike requests which opens a new
        # connection each time by default)
        self._client = httpx.Client(
            base_url=self.base_url,
            timeout=httpx.Timeout(timeout, connect=10.0),
        )

        # Async client — created lazily in async context manager
        self._async_client: httpx.AsyncClient | None = None

    def close(self) -> None:
        """Close the HTTP connection pool."""
        self._client.close()

    # -- Context manager support --

    def __enter__(self) -> VoxClient:
        return self

    def __exit__(self, *args) -> None:
        self.close()

    async def __aenter__(self) -> VoxClient:
        self._async_client = httpx.AsyncClient(
            base_url=self.base_url,
            timeout=httpx.Timeout(self.timeout, connect=10.0),
        )
        return self

    async def __aexit__(self, *args) -> None:
        if self._async_client:
            await self._async_client.aclose()

    # -- Headers --

    def _headers(self) -> dict[str, str]:
        headers = {}
        if self._default_request_id:
            headers["X-Request-ID"] = self._default_request_id
        return headers

    # -- Error handling --

    def _handle_error(self, response: httpx.Response) -> None:
        """Parse gateway error response and raise VoxError.

        The gateway always returns structured errors with code, message,
        and request_id. If parsing fails (shouldn't happen), fall back
        to the raw response body.
        """
        if response.status_code < 400:
            return

        try:
            error_data = APIErrorResponse.model_validate(response.json())
            raise VoxError(
                code=error_data.error.code,
                message=error_data.error.message,
                request_id=error_data.error.request_id,
                status_code=response.status_code,
            )
        except (ValueError, KeyError):
            # Gateway returned non-JSON or unexpected format
            raise VoxError(
                code="unknown_error",
                message=response.text[:200],
                request_id=response.headers.get("X-Request-ID", "unknown"),
                status_code=response.status_code,
            )

    # -- Sync API --

    def health(self) -> HealthStatus:
        """Check gateway liveness.

        Returns HealthStatus with status="ok" if the gateway is running.
        This does NOT check if the Whisper backend is ready — use ready() for that.
        """
        response = self._client.get("/healthz", headers=self._headers())
        self._handle_error(response)
        return HealthStatus.model_validate(response.json())

    def ready(self) -> HealthStatus:
        """Check if the gateway can serve traffic.

        Returns HealthStatus with status="ready" if both the gateway
        and the Whisper backend are healthy. If Whisper is down,
        returns status="not ready" with a reason.
        """
        response = self._client.get("/readyz", headers=self._headers())
        # Don't raise on 503 — it's a valid "not ready" response
        if response.status_code == 503:
            return HealthStatus.model_validate(response.json())
        self._handle_error(response)
        return HealthStatus.model_validate(response.json())

    def models(self) -> list[ModelInfo]:
        """List available models.

        Returns a list of ModelInfo with id, type, and state.
        """
        response = self._client.get("/v1/models", headers=self._headers())
        self._handle_error(response)
        data = ModelsResponse.model_validate(response.json())
        return data.models

    def transcribe(
        self,
        audio: str | Path | BinaryIO,
        *,
        model: str | None = None,
        language: str | None = None,
        response_format: str | None = None,
    ) -> TranscriptionResult:
        """Transcribe an audio file.

        Args:
            audio: Path to an audio file, or a file-like object.
            model: Model to use. Defaults to the gateway's default model.
            language: Language code (e.g., "en"). Auto-detected if omitted.
            response_format: Response format ("json" or "verbose_json").

        Returns:
            TranscriptionResult with text, model, processing_time, and request_id.

        Raises:
            FileNotFoundError: If the audio file doesn't exist.
            ValueError: If the audio format is not supported.
            VoxError: If the gateway returns an error.

        Example:
            # From a file path
            result = client.transcribe("meeting.wav")

            # From a file object
            with open("meeting.wav", "rb") as f:
                result = client.transcribe(f)

            # With options
            result = client.transcribe(
                "meeting.wav",
                model="Systran/faster-whisper-small.en",
                language="en",
            )
        """
        file_obj, filename = self._prepare_audio(audio)
        try:
            return self._send_transcription(file_obj, filename, model, language, response_format)
        finally:
            # Close the file if we opened it (not if the user passed a file object)
            if isinstance(audio, (str, Path)):
                file_obj.close()

    def _prepare_audio(self, audio: str | Path | BinaryIO) -> tuple[BinaryIO, str]:
        """Validate and open the audio source.

        Returns (file_object, filename) tuple.
        """
        if isinstance(audio, (str, Path)):
            path = Path(audio)

            if not path.exists():
                raise FileNotFoundError(f"Audio file not found: {path}")

            suffix = path.suffix.lower()
            if suffix not in SUPPORTED_FORMATS:
                raise ValueError(
                    f"Unsupported audio format '{suffix}'. "
                    f"Supported: {', '.join(sorted(SUPPORTED_FORMATS))}"
                )

            return open(path, "rb"), path.name

        # File-like object — use a generic filename
        name = getattr(audio, "name", "audio.wav")
        return audio, Path(name).name

    def _send_transcription(
        self,
        file_obj: BinaryIO,
        filename: str,
        model: str | None,
        language: str | None,
        response_format: str | None,
    ) -> TranscriptionResult:
        """Build multipart form and send to the gateway."""
        suffix = Path(filename).suffix.lower()
        mime_type = MIME_TYPES.get(suffix, "application/octet-stream")

        # Build the multipart form data
        files = {"file": (filename, file_obj, mime_type)}
        data = {}

        if model:
            data["model"] = model
        if language:
            data["language"] = language
        if response_format:
            data["response_format"] = response_format

        response = self._client.post(
            "/v1/audio/transcriptions",
            files=files,
            data=data,
            headers=self._headers(),
        )
        self._handle_error(response)
        return TranscriptionResult.model_validate(response.json())

    # -- Async API --
    # Mirrors the sync API but with async/await.
    # The eval harness will use this for concurrent test runs.

    async def ahealth(self) -> HealthStatus:
        """Async version of health()."""
        assert self._async_client, "Use 'async with VoxClient(...) as client:'"
        response = await self._async_client.get("/healthz", headers=self._headers())
        self._handle_error(response)
        return HealthStatus.model_validate(response.json())

    async def aready(self) -> HealthStatus:
        """Async version of ready()."""
        assert self._async_client, "Use 'async with VoxClient(...) as client:'"
        response = await self._async_client.get("/readyz", headers=self._headers())
        if response.status_code == 503:
            return HealthStatus.model_validate(response.json())
        self._handle_error(response)
        return HealthStatus.model_validate(response.json())

    async def amodels(self) -> list[ModelInfo]:
        """Async version of models()."""
        assert self._async_client, "Use 'async with VoxClient(...) as client:'"
        response = await self._async_client.get("/v1/models", headers=self._headers())
        self._handle_error(response)
        data = ModelsResponse.model_validate(response.json())
        return data.models

    async def atranscribe(
        self,
        audio: str | Path | BinaryIO,
        *,
        model: str | None = None,
        language: str | None = None,
        response_format: str | None = None,
    ) -> TranscriptionResult:
        """Async version of transcribe()."""
        assert self._async_client, "Use 'async with VoxClient(...) as client:'"

        file_obj, filename = self._prepare_audio(audio)
        try:
            suffix = Path(filename).suffix.lower()
            mime_type = MIME_TYPES.get(suffix, "application/octet-stream")

            files = {"file": (filename, file_obj, mime_type)}
            data = {}
            if model:
                data["model"] = model
            if language:
                data["language"] = language
            if response_format:
                data["response_format"] = response_format

            response = await self._async_client.post(
                "/v1/audio/transcriptions",
                files=files,
                data=data,
                headers=self._headers(),
            )
            self._handle_error(response)
            return TranscriptionResult.model_validate(response.json())
        finally:
            if isinstance(audio, (str, Path)):
                file_obj.close()
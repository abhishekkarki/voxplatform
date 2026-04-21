"""Tests for VoxClient.

Uses respx to mock HTTP responses — no real gateway needed.
All tests run locally with zero network calls.
"""

from __future__ import annotations

import io
from pathlib import Path

import httpx
import pytest
import respx

from voxplatform import VoxClient, VoxError


GATEWAY_URL = "http://test-gateway:8080"


# -- Fixtures --

@pytest.fixture
def client():
    """Create a VoxClient pointed at a fake URL."""
    c = VoxClient(base_url=GATEWAY_URL)
    yield c
    c.close()


@pytest.fixture
def mock_audio(tmp_path: Path) -> Path:
    """Create a fake audio file for testing."""
    audio_file = tmp_path / "test.wav"
    audio_file.write_bytes(b"RIFF" + b"\x00" * 100)  # Minimal WAV header
    return audio_file


# -- Health --

@respx.mock
def test_health_ok(client: VoxClient):
    respx.get(f"{GATEWAY_URL}/healthz").mock(
        return_value=httpx.Response(200, json={"status": "ok"})
    )

    result = client.health()
    assert result.status == "ok"


@respx.mock
def test_health_request_id_header(client: VoxClient):
    """Verify that custom request IDs are sent in headers."""
    client_with_id = VoxClient(base_url=GATEWAY_URL, request_id="trace-123")

    route = respx.get(f"{GATEWAY_URL}/healthz").mock(
        return_value=httpx.Response(200, json={"status": "ok"})
    )

    client_with_id.health()

    assert route.calls[0].request.headers["X-Request-ID"] == "trace-123"
    client_with_id.close()


# -- Readiness --

@respx.mock
def test_ready_ok(client: VoxClient):
    respx.get(f"{GATEWAY_URL}/readyz").mock(
        return_value=httpx.Response(200, json={"status": "ready"})
    )

    result = client.ready()
    assert result.status == "ready"


@respx.mock
def test_ready_not_ready(client: VoxClient):
    """When Whisper is down, ready() returns status without raising."""
    respx.get(f"{GATEWAY_URL}/readyz").mock(
        return_value=httpx.Response(503, json={
            "status": "not ready",
            "reason": "whisper backend unreachable",
        })
    )

    result = client.ready()
    assert result.status == "not ready"
    assert result.reason == "whisper backend unreachable"


# -- Models --

@respx.mock
def test_models(client: VoxClient):
    respx.get(f"{GATEWAY_URL}/v1/models").mock(
        return_value=httpx.Response(200, json={
            "models": [
                {"id": "Systran/faster-whisper-small.en", "type": "stt", "state": "loaded"}
            ]
        })
    )

    models = client.models()
    assert len(models) == 1
    assert models[0].id == "Systran/faster-whisper-small.en"
    assert models[0].type == "stt"
    assert models[0].state == "loaded"


# -- Transcription --

@respx.mock
def test_transcribe_success(client: VoxClient, mock_audio: Path):
    respx.post(f"{GATEWAY_URL}/v1/audio/transcriptions").mock(
        return_value=httpx.Response(200, json={
            "text": "hello world",
            "model": "Systran/faster-whisper-small.en",
            "processing_time_seconds": 1.23,
            "request_id": "abc123",
            "created_at": "2026-04-20T12:00:00Z",
        })
    )

    result = client.transcribe(mock_audio)

    assert result.text == "hello world"
    assert result.model == "Systran/faster-whisper-small.en"
    assert result.processing_time == 1.23
    assert result.request_id == "abc123"


@respx.mock
def test_transcribe_with_model(client: VoxClient, mock_audio: Path):
    """Verify the model parameter is sent in the form data."""
    route = respx.post(f"{GATEWAY_URL}/v1/audio/transcriptions").mock(
        return_value=httpx.Response(200, json={
            "text": "test",
            "model": "custom-model",
            "processing_time_seconds": 0.5,
            "request_id": "def456",
            "created_at": "2026-04-20T12:00:00Z",
        })
    )

    client.transcribe(mock_audio, model="custom-model")

    # Check the request was sent with the model field
    request = route.calls[0].request
    assert b"custom-model" in request.content


@respx.mock
def test_transcribe_file_object(client: VoxClient):
    """Transcribe from a file-like object instead of a path."""
    respx.post(f"{GATEWAY_URL}/v1/audio/transcriptions").mock(
        return_value=httpx.Response(200, json={
            "text": "from file object",
            "model": "test",
            "processing_time_seconds": 0.1,
            "request_id": "ghi789",
            "created_at": "2026-04-20T12:00:00Z",
        })
    )

    fake_file = io.BytesIO(b"fake audio data")
    fake_file.name = "test.wav"

    result = client.transcribe(fake_file)
    assert result.text == "from file object"


def test_transcribe_file_not_found(client: VoxClient):
    with pytest.raises(FileNotFoundError, match="Audio file not found"):
        client.transcribe("/nonexistent/audio.wav")


def test_transcribe_unsupported_format(client: VoxClient, tmp_path: Path):
    bad_file = tmp_path / "audio.xyz"
    bad_file.write_bytes(b"data")

    with pytest.raises(ValueError, match="Unsupported audio format"):
        client.transcribe(bad_file)


# -- Error handling --

@respx.mock
def test_structured_error(client: VoxClient, mock_audio: Path):
    """Gateway structured errors are parsed into VoxError."""
    respx.post(f"{GATEWAY_URL}/v1/audio/transcriptions").mock(
        return_value=httpx.Response(502, json={
            "error": {
                "code": "backend_error",
                "message": "whisper backend error: timeout",
                "request_id": "err123",
            }
        })
    )

    with pytest.raises(VoxError) as exc_info:
        client.transcribe(mock_audio)

    err = exc_info.value
    assert err.code == "backend_error"
    assert "timeout" in err.message
    assert err.request_id == "err123"
    assert err.status_code == 502


@respx.mock
def test_non_json_error(client: VoxClient, mock_audio: Path):
    """Non-JSON errors (e.g., nginx 502) are still caught."""
    respx.post(f"{GATEWAY_URL}/v1/audio/transcriptions").mock(
        return_value=httpx.Response(
            502,
            text="<html>Bad Gateway</html>",
            headers={"X-Request-ID": "nginx-err"},
        )
    )

    with pytest.raises(VoxError) as exc_info:
        client.transcribe(mock_audio)

    err = exc_info.value
    assert err.code == "unknown_error"
    assert err.request_id == "nginx-err"


# -- Async --

@respx.mock
@pytest.mark.asyncio
async def test_async_health():
    respx.get(f"{GATEWAY_URL}/healthz").mock(
        return_value=httpx.Response(200, json={"status": "ok"})
    )

    async with VoxClient(base_url=GATEWAY_URL) as client:
        result = await client.ahealth()
        assert result.status == "ok"


@respx.mock
@pytest.mark.asyncio
async def test_async_transcribe(tmp_path: Path):
    audio_file = tmp_path / "test.wav"
    audio_file.write_bytes(b"RIFF" + b"\x00" * 100)

    respx.post(f"{GATEWAY_URL}/v1/audio/transcriptions").mock(
        return_value=httpx.Response(200, json={
            "text": "async result",
            "model": "test",
            "processing_time_seconds": 0.5,
            "request_id": "async123",
            "created_at": "2026-04-20T12:00:00Z",
        })
    )

    async with VoxClient(base_url=GATEWAY_URL) as client:
        result = await client.atranscribe(audio_file)
        assert result.text == "async result"
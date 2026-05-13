# Use the Python SDK

The `vox` Python package wraps the gateway HTTP and WebSocket APIs.

## Install

```bash
uv pip install -e sdk/
```

## Batch transcription

```python
from vox import VoxClient

client = VoxClient(endpoint="http://gateway.example.com")
result = client.transcribe("meeting.wav", model="whisper-small")
print(result.text)
print(result.segments)
```

## Streaming from a file

```python
from vox import VoxClient

client = VoxClient(endpoint="http://gateway.example.com")
async for partial in client.stream_file("meeting.wav", model="whisper-tiny"):
    print(partial.text, "(final)" if partial.is_final else "(partial)")
```

## Streaming from a microphone

The `vox record` CLI uses this same path — see [vox CLI reference](../reference/cli.md#record).

## Full API reference

See the [Python SDK reference](../reference/python-sdk.md) for the complete `VoxClient` API.

<!-- Once sdk/vox/ exists, uncomment to auto-generate from docstrings here too:
::: vox.client.VoxClient
    options:
      show_root_heading: false
      heading_level: 3
-->

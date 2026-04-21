# VoxPlatform Python Client SDK

Python client for the VoxPlatform voice inference gateway.

## Install

```bash
cd clients/python
pip install -e ".[dev]"
```

## Usage

### As a library

```python
from voxplatform import VoxClient

client = VoxClient("http://localhost:8080")

# Check health
print(client.health())

# Transcribe audio
result = client.transcribe("meeting.wav")
print(result.text)
print(f"Took {result.processing_time:.1f}s")
```

### As a CLI

```bash
vox health
vox models
vox transcribe audio.wav
vox transcribe audio.wav --json | jq .text
```

### Async

```python
from voxplatform import VoxClient

async with VoxClient("http://localhost:8080") as client:
    result = await client.atranscribe("audio.wav")
    print(result.text)
```

## Run tests

```bash
pip install -e ".[dev]"
pytest -v
```
# `vox` CLI

The `vox` CLI is installed alongside the Python SDK (`uv pip install -e sdk/`).

## Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `--endpoint URL` | `$VOX_ENDPOINT` or `http://localhost:8080` | Gateway base URL |
| `--model NAME` | `$VOX_MODEL` or `whisper-tiny` | Default `VoiceModel` name |
| `-v, --verbose` | off | Log HTTP/WS frames |

## Commands

### `transcribe`

Batch transcription of a file.

```
vox transcribe FILE [--language LANG] [--output FILE]
```

### `record`

Stream from the system microphone over WebSocket.

```
vox record [--device DEVICE] [--sample-rate HZ] [--output FILE]
```

### `health`

Print the gateway's readiness and the list of known models.

```
vox health
```

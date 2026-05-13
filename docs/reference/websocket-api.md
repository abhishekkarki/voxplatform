# WebSocket API

Endpoint: `ws://<gateway-host>/stream`

## Handshake

Open the WebSocket with these query parameters:

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `model` | yes | — | Name of a deployed `VoiceModel` |
| `sample_rate` | yes | — | One of `8000`, `16000`, `24000`, `48000` |
| `language` | no | autodetect | ISO-639-1 hint |

## Client → server frames

Send **binary** WebSocket frames containing raw 16-bit PCM, little-endian, mono, at the negotiated sample rate. A 320-sample frame (20 ms at 16 kHz) is a good default.

To end a stream cleanly, send a text frame with the JSON body:

```json
{ "type": "end" }
```

## Server → client frames

The server sends **text** frames containing JSON:

```json
{
  "type": "partial" | "final",
  "text": "string",
  "start_sec": 0.0,
  "end_sec": 2.4,
  "is_final": false
}
```

Partials may be revised by subsequent partials within the same speech segment. A `final` message ends the segment and will not be revised.

## Close codes

| Code | Meaning |
|------|---------|
| `1000` | Normal closure |
| `1003` | Unsupported frame type |
| `1008` | Policy violation (e.g. unknown `model`) |
| `1011` | Internal error |

See [Streaming design](../explanation/streaming-design.md) for the rationale behind these choices.

# Streaming design

> Replace this stub with the relevant section from `voxplatform-docs-v2.docx`. Keep it as *explanation* — the protocol itself belongs on the [WebSocket API reference](../reference/websocket-api.md).

Things this page should cover:

- Why WebSocket and not gRPC streaming or HTTP/2 server-sent events
- Why 20 ms PCM frames at 16 kHz
- The partial-vs-final segment model and why it matches how Whisper actually works
- The gateway's buffer-and-release strategy when VAD says "still speech"
- Backpressure: what happens when the model can't keep up
- The Gorilla `Hijacker` interaction with Go's net/http (one of your lessons-learned)

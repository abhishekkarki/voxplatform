# Why a VAD sidecar

> Status: explanation. The decision itself is recorded in [ADR-0002](adrs.md).

VAD ("voice activity detection") gates audio so the expensive STT model only sees speech, not silence or noise. We had three places to put it:

1. **In the gateway process.** Same Go binary, vendored bindings to a VAD library.
2. **In the model container.** Push VAD into the Whisper deployment.
3. **As a separate sidecar in the gateway pod.** Same pod, separate container, separate language.

We chose option 3. The trade-offs:

| | In-gateway | In-model | Sidecar |
|---|---|---|---|
| Language match for Silero | bad (Go) | good (Python) | good (Python) |
| Per-stream isolation | shared process | per-model | per-stream-pod |
| Model independence | yes | **no** — every model carries VAD | yes |
| Operational complexity | low | low | medium |
| GPU coupling | none | bad — VAD on the GPU node | none |

The deciding factor was **model independence**. Putting VAD inside the model container would have forced every `VoiceModel` image to bundle a VAD runtime, and any VAD upgrade would mean rebuilding every model image. The sidecar pattern decouples them cleanly: the gateway pod owns VAD, the model pods own STT, and they communicate over loopback gRPC.

The cost is one extra container per gateway replica and one extra hop in the audio path (sub-millisecond on loopback). Worth it.

## What we'd revisit

If we ever move VAD onto a GPU (e.g. for a much larger VAD model), the sidecar pattern becomes the wrong shape — at that point the gateway pod and the VAD pod need different node selectors and the sidecar collapses into a separate Deployment. We're not there.

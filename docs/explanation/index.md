# Explanation

> Understanding-oriented. Read these for context, perspective, and the *why*.

If [Tutorial](../tutorial/index.md) and [How-to](../how-to/index.md) tell you how to **use** Vox, and [Reference](../reference/index.md) tells you **what** the interfaces are, this section tells you **why** they look the way they do.

## Contents

- [Architecture overview](architecture.md) — all components, how they fit together, end-to-end request flow
- [Pipeline design](pipeline-design.md) — how STT → diarize → summarize is orchestrated, event log, graceful degradation
- [Why a VAD sidecar](why-vad-sidecar.md) — and not in-process VAD, and not a separate service
- [Streaming design](streaming-design.md) — why WebSocket, why 20 ms PCM frames, why partial-vs-final
- [Operator design](operator-design.md) — why a CRD instead of Helm values, where the reconciler boundary is
- [Lessons learned](lessons-learned.md) — concrete mistakes we made and what they taught us
- [ADRs](adrs.md) — formal architecture decision records, dated and immutable

# Tutorial

> Learning-oriented. The goal of this section is to teach you Vox by **doing** something concrete.

If you've just landed here, start with **[First transcription in 5 minutes](first-transcription.md)**. It walks you from a fresh `git clone` to a live transcription against a running cluster, with nothing assumed.

The tutorial is deliberately narrow. It does **not** explain why things work the way they do — that's what [Explanation](../explanation/index.md) is for. It does **not** cover every option — that's what [How-to guides](../how-to/index.md) and [Reference](../reference/index.md) are for. It just gets you to a working result, fast.

## What you'll need

- A GCP project with billing enabled
- `gcloud`, `kubectl`, `terraform`, and `helm` installed locally
- A microphone (for the streaming step)
- About 5 minutes of attention and ~€0.20 of GKE credits

## What you'll have at the end

A two-node GKE cluster running the gateway, VAD sidecar, and a `whisper-tiny` `VoiceModel`, with the `vox` CLI streaming partial transcripts from your microphone to your terminal in real time.

# Python SDK

This page is generated from docstrings in the `vox` package by [`mkdocstrings`](https://mkdocstrings.github.io/). Update the docstrings, not this page.

!!! note "Enable once the SDK exists"
    Replace the placeholder blocks below with `mkdocstrings` invocations when your Python package is in place. The plugin is already configured in `mkdocs.yml` with `paths: [sdk]` — pointing at `sdk/vox/`.

<!-- Once sdk/vox/ exists with docstrings, replace these placeholders:

## `vox.client`

::: vox.client
    options:
      show_root_heading: false
      heading_level: 3
      members_order: source

## `vox.types`

::: vox.types
    options:
      show_root_heading: false
      heading_level: 3
      members_order: source
-->

## `vox.client` *(placeholder)*

The `VoxClient` class wraps the gateway HTTP and WebSocket endpoints. Methods:

- `transcribe(path, *, model, language=None) -> TranscriptionResult`
- `stream_file(path, *, model, language=None) -> AsyncIterator[Partial]`
- `stream_microphone(*, model, device=None) -> AsyncIterator[Partial]`
- `health() -> HealthReport`

## `vox.types` *(placeholder)*

`TranscriptionResult`, `Segment`, `Partial`, `HealthReport` dataclasses. See [HTTP API](http-api.md) and [WebSocket API](websocket-api.md) for the underlying schemas.

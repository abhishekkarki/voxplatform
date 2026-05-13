# How-to guides

> Task-oriented. Each guide answers a specific question of the form *"How do I X?"*

These pages assume you've already worked through the [Tutorial](../tutorial/index.md) and have a running cluster. They're meant to be skimmed and copy-pasted, not read in order.

## Available guides

- [Deploy a VoiceModel](deploy-voicemodel.md) — apply a new model variant and watch the operator reconcile it
- [Run the eval harness](eval-harness.md) — compute WER against a held-out dataset, locally or against the live cluster
- [Scale the cluster up and down](scale-cluster.md) — the scale-to-zero workflow for cost control
- [Use the Python SDK](python-sdk.md) — call the gateway from your own code, batch or streaming

!!! note "Looking for something else?"
    If your question is conceptual (*"why does it work this way?"*), check [Explanation](../explanation/index.md). If you need exact signatures or schemas, see [Reference](../reference/index.md).

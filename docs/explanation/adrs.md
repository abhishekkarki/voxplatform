# Architecture Decision Records

ADRs capture significant architectural decisions, the context behind them, and the consequences. Each ADR is dated and treated as immutable — superseded ADRs stay; we add new ones rather than editing old ones.

Format: [Michael Nygard's classic template](https://github.com/joelparkerhenderson/architecture-decision-record/blob/main/locales/en/templates/decision-record-template-by-michael-nygard/index.md).

## Index

| # | Title | Status |
|---|-------|--------|
| 0001 | Use GKE rather than Cloud Run for the data plane | Accepted |
| 0002 | VAD as a sidecar in the gateway pod | Accepted |
| 0003 | WebSocket for the streaming protocol | Accepted |
| 0004 | Single `VoiceModel` CRD rather than per-engine CRDs | Accepted |
| 0005 | `jiwer` v4 as the WER source of truth | Accepted |

## Read the ADRs

The ADR files live in `docs/adr/` in the repo. To embed them on this page so the canonical copy stays in one place, add `include-markdown` blocks. The syntax (curly braces shown as entities so this very page doesn't try to execute the example) is:

```text
&#123;%
  include-markdown "../adr/0001-gke-not-cloud-run.md"
  heading-offset=1
%&#125;
```

Repeat the block for each ADR. Place these blocks where you want each ADR's content to appear on this page.

!!! tip "Keeping ADRs out of the auto-nav"
    `docs/adr/` is listed under `not_in_nav` in `mkdocs.yml` so the individual ADR files don't show up as orphaned pages. They're still part of the docs tree, which is what lets `include-markdown` reach them.

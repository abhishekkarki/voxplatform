# Architecture - the four pillars

```
┌─────────────────────────────────────────────────────────┐
│                    voxctl CLI (Go)                      │
│              Developer's entry point                    │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│              Control plane (Go operator)                │
│                                                         │
│  CRDs:  VoiceModel │ InferencePipeline │ EvalRun        │
│                                                         │
│  Reconciles CRDs into K8s Deployments, Services,        │
│  ServiceMonitors, Argo Workflows                        │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│                Data plane (per request)                 │
│                                                         │
│  Gateway (Go, gRPC/WS) → VAD → STT → Diarize → LLM      │
│       │                                                 │
│       |--> Event log (append-only, per request)         │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│                 Observability                           │
│                                                         │
│  OTel traces │ Prometheus metrics │ Grafana dashboards  │
│  Per-model: latency, throughput, CPU/GPU-sec, cost      │
└─────────────────────────────────────────────────────────┘

```


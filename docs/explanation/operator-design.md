# Operator design

> Stub. Pull the operator deep-dive from `voxplatform-docs-v2.docx` into here, keeping it as *why* rather than *how* or *what*.

Outline:

- Why a CRD instead of Helm values
- Why `VoiceModel` and not `WhisperModel` — naming for extensibility
- Where the reconciler boundary is (Deployment + Service yes; HPA, PDB no — at least not yet)
- Owner references and the cleanup story
- The `updateStatus` two-cycle gotcha and how we eventually fixed it
- What we deliberately did **not** put in the operator

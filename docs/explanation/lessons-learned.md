# Lessons learned

> The 11 hard-won lessons from `voxplatform-docs-v2.docx`. Each one is a story worth telling — keep them short, concrete, and self-contained.

Recommended one-section-per-lesson structure: **What happened → What we tried → What we learned**.

## Topics to cover

1. ARM vs. x86 image mismatch on Apple Silicon (the `exec format error` story)
2. Mutable tags in Artifact Registry — why `:latest` cost us a day
3. Resource requests vs. limits on GKE, and why the model pods wouldn't schedule
4. Corporate proxy ate our GCS pre-signed URLs
5. The auth flow we got wrong (and the one we ended up with)
6. Terraform typos that survive `terraform plan`
7. Why `pip install` corrupts venvs and `uv pip install` doesn't
8. WebSocket `Hijacker` and Go's `net/http` graceful-shutdown interaction
9. `jiwer` v4 breaking changes from v2
10. `controller-runtime` API churn between minor versions
11. `envtest` platforms — what works on `darwin/arm64` and what doesn't

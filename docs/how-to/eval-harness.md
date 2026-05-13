# Run the eval harness

The eval harness computes Word Error Rate (WER) using `jiwer` v4 against a held-out dataset of `(audio, reference_transcript)` pairs.

## Local run

```bash
cd eval
uv pip install -r requirements.txt
python -m vox_eval run \
  --dataset datasets/librispeech-test-clean-100.jsonl \
  --endpoint http://localhost:8080 \
  --model whisper-tiny \
  --output results/local-$(date +%Y%m%d).json
```

## Against the live cluster

```bash
GATEWAY=$(kubectl get svc vox-gateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
python -m vox_eval run \
  --dataset datasets/librispeech-test-clean-100.jsonl \
  --endpoint "http://${GATEWAY}" \
  --model whisper-tiny
```

## In CI

The eval job runs on every PR against `main` over a 100-sample subset. The full 2620-sample run is nightly. See `.github/workflows/eval.yml`.

## See also

- [Lessons learned](../explanation/lessons-learned.md) — what changed between jiwer v2 and v4, and other gotchas

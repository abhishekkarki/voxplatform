"""Evaluation runner.

Orchestrates the full eval pipeline:
  1. Load dataset (audio + ground truth)
  2. Transcribe each sample via the VoxPlatform SDK
  3. Compute per-sample and aggregate WER
  4. Compare against baseline threshold
  5. Generate report

Example:
    from vox_eval.runner import EvalRunner

    runner = EvalRunner(gateway_url="http://localhost:8080")
    report = runner.run("eval/datasets/librispeech-mini")
    print(f"Corpus WER: {report.corpus_wer:.3f}")
"""

from __future__ import annotations

import json
import sys
import time
from dataclasses import asdict
from pathlib import Path

from vox_eval.dataset import EvalDataset, load_dataset
from vox_eval.metrics import (
    AggregateMetrics,
    SampleMetrics,
    aggregate,
    compute_wer,
)

# Import the SDK — installed from clients/python
sys.path.insert(0, str(Path(__file__).parent.parent.parent / "clients" / "python"))

try:
    from voxplatform import VoxClient, VoxError
except ImportError:
    raise ImportError(
        "voxplatform SDK not found. Install it first:\n"
        "  pip install -e clients/python"
    )


class EvalRunner:
    """Runs evaluation against a VoxPlatform gateway.

    Args:
        gateway_url: URL of the gateway service.
        model: Model to use for transcription. None = gateway default.
        timeout: Per-request timeout in seconds.
        wer_threshold: Maximum acceptable corpus WER. If the evaluated
                       WER exceeds this, the eval is marked as failed.
                       Default 0.3 (30%) is reasonable for small.en on
                       clean speech.
    """

    def __init__(
        self,
        gateway_url: str = "http://localhost:8080",
        model: str | None = None,
        timeout: float = 120.0,
        wer_threshold: float = 0.3,
    ):
        self.gateway_url = gateway_url
        self.model = model
        self.timeout = timeout
        self.wer_threshold = wer_threshold

    def run(
        self,
        dataset_path: str | Path,
        output_dir: str | Path | None = None,
    ) -> EvalReport:
        """Run evaluation on a dataset.

        Args:
            dataset_path: Path to dataset directory with manifest.csv.
            output_dir: Where to save the JSON report. Defaults to
                        eval/results/.

        Returns:
            EvalReport with metrics and pass/fail status.
        """
        # 1. Load dataset
        print(f"Loading dataset from {dataset_path}...")
        dataset = load_dataset(dataset_path)
        print(f"  {dataset.size} samples loaded")

        # 2. Transcribe each sample
        print(f"\nTranscribing via {self.gateway_url}...")
        client = VoxClient(
            base_url=self.gateway_url,
            timeout=self.timeout,
        )

        sample_metrics: list[SampleMetrics] = []
        errors: list[dict] = []

        try:
            for i, sample in enumerate(dataset.samples):
                progress = f"[{i + 1}/{dataset.size}]"

                try:
                    start = time.time()
                    result = client.transcribe(
                        sample.audio_path,
                        model=self.model,
                    )
                    elapsed = time.time() - start

                    # Compute WER
                    wer, subs, ins, dels = compute_wer(
                        sample.ground_truth,
                        result.text,
                    )

                    metric = SampleMetrics(
                        sample_id=sample.sample_id,
                        ground_truth=sample.ground_truth,
                        prediction=result.text,
                        wer=wer,
                        substitutions=subs,
                        insertions=ins,
                        deletions=dels,
                        processing_time=elapsed,
                    )
                    sample_metrics.append(metric)

                    # Show per-sample result
                    status = "\u2705" if wer < self.wer_threshold else "\u274C"
                    print(f"  {progress} {status} {sample.sample_id}: WER={wer:.3f} ({elapsed:.1f}s)")

                except VoxError as e:
                    print(f"  {progress} \u274C {sample.sample_id}: ERROR [{e.code}] {e.message}")
                    errors.append({
                        "sample_id": sample.sample_id,
                        "error_code": e.code,
                        "error_message": e.message,
                        "request_id": e.request_id,
                    })

                except Exception as e:
                    print(f"  {progress} \u274C {sample.sample_id}: ERROR {e}")
                    errors.append({
                        "sample_id": sample.sample_id,
                        "error_code": "client_error",
                        "error_message": str(e),
                        "request_id": "unknown",
                    })
        finally:
            client.close()

        # 3. Aggregate metrics
        if not sample_metrics:
            raise RuntimeError(
                f"All {dataset.size} samples failed. "
                f"Is the gateway running at {self.gateway_url}?"
            )

        metrics = aggregate(dataset.name, sample_metrics)

        # 4. Build report
        passed = metrics.corpus_wer <= self.wer_threshold
        report = EvalReport(
            metrics=metrics,
            errors=errors,
            passed=passed,
            wer_threshold=self.wer_threshold,
            gateway_url=self.gateway_url,
            model=self.model or "default",
        )

        # 5. Print summary
        print(report.summary())

        # 6. Save report
        if output_dir:
            report.save(output_dir)

        return report


class EvalReport:
    """Evaluation report with metrics, errors, and pass/fail status.

    The report is both human-readable (summary()) and machine-readable
    (to_dict() / save()). CI pipelines can parse the JSON output to
    gate deployments.
    """

    def __init__(
        self,
        metrics: AggregateMetrics,
        errors: list[dict],
        passed: bool,
        wer_threshold: float,
        gateway_url: str,
        model: str,
    ):
        self.metrics = metrics
        self.errors = errors
        self.passed = passed
        self.wer_threshold = wer_threshold
        self.gateway_url = gateway_url
        self.model = model

    def summary(self) -> str:
        """Human-readable summary printed to stdout."""
        m = self.metrics
        status = "\u2705 PASSED" if self.passed else "\u274C FAILED"

        lines = [
            "",
            "=" * 50,
            f"  Eval Report: {m.dataset_name}",
            "=" * 50,
            f"  Status:         {status}",
            f"  WER Threshold:  {self.wer_threshold:.1%}",
            f"  Corpus WER:     {m.corpus_wer:.3f} ({m.corpus_wer:.1%})",
            f"  Mean WER:       {m.mean_wer:.3f} ({m.mean_wer:.1%})",
            f"  Median WER:     {m.median_wer:.3f}",
            f"  Min / Max WER:  {m.min_wer:.3f} / {m.max_wer:.3f}",
            "",
            f"  Samples:        {m.total_samples}",
            f"  Errors:         {len(self.errors)}",
            f"  Ref Words:      {m.total_reference_words}",
            f"  Substitutions:  {m.total_substitutions}",
            f"  Insertions:     {m.total_insertions}",
            f"  Deletions:      {m.total_deletions}",
            "",
            f"  Avg Time:       {m.mean_processing_time:.2f}s per sample",
            f"  Model:          {self.model}",
            f"  Gateway:        {self.gateway_url}",
            "=" * 50,
        ]

        # Show worst samples for debugging
        worst = sorted(m.samples, key=lambda s: s.wer, reverse=True)[:3]
        if worst and worst[0].wer > 0:
            lines.append("  Worst samples:")
            for s in worst:
                lines.append(f"    {s.sample_id}: WER={s.wer:.3f}")
                lines.append(f"      truth: {s.ground_truth[:60]}")
                lines.append(f"      pred:  {s.prediction[:60]}")
            lines.append("=" * 50)

        return "\n".join(lines)

    def to_dict(self) -> dict:
        """Machine-readable dict for JSON serialization."""
        m = self.metrics
        return {
            "passed": self.passed,
            "wer_threshold": self.wer_threshold,
            "corpus_wer": round(m.corpus_wer, 4),
            "mean_wer": round(m.mean_wer, 4),
            "median_wer": round(m.median_wer, 4),
            "total_samples": m.total_samples,
            "total_reference_words": m.total_reference_words,
            "substitutions": m.total_substitutions,
            "insertions": m.total_insertions,
            "deletions": m.total_deletions,
            "mean_processing_time": round(m.mean_processing_time, 3),
            "model": self.model,
            "gateway_url": self.gateway_url,
            "dataset": m.dataset_name,
            "errors": self.errors,
            "samples": [
                {
                    "sample_id": s.sample_id,
                    "wer": round(s.wer, 4),
                    "ground_truth": s.ground_truth,
                    "prediction": s.prediction,
                    "processing_time": round(s.processing_time, 3),
                }
                for s in m.samples
            ],
        }

    def save(self, output_dir: str | Path) -> Path:
        """Save report as JSON."""
        output_dir = Path(output_dir)
        output_dir.mkdir(parents=True, exist_ok=True)

        timestamp = time.strftime("%Y%m%d-%H%M%S")
        filename = f"eval-{self.metrics.dataset_name}-{timestamp}.json"
        output_path = output_dir / filename

        with open(output_path, "w") as f:
            json.dump(self.to_dict(), f, indent=2)

        print(f"\nReport saved: {output_path}")
        return output_path
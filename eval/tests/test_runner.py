"""Tests for EvalReport saving.

These tests run locally with no network calls — they build a report
directly from metrics rather than transcribing real audio.
"""

from __future__ import annotations

import json

from vox_eval.metrics import SampleMetrics, aggregate
from vox_eval.runner import EvalReport


def _make_report(passed: bool = True) -> EvalReport:
    sample = SampleMetrics(
        sample_id="sample1",
        ground_truth="hello world",
        prediction="hello world" if passed else "goodbye moon entirely",
        wer=0.0 if passed else 1.5,
        substitutions=0 if passed else 2,
        insertions=0 if passed else 1,
        deletions=0,
        processing_time=0.1,
    )
    metrics = aggregate("unit-test", [sample])
    return EvalReport(
        metrics=metrics,
        errors=[],
        passed=passed,
        wer_threshold=0.3,
        gateway_url="http://localhost:8080",
        model="test-model",
    )


class TestSave:
    """save() writes a timestamped file into a directory."""

    def test_save_creates_timestamped_file_in_dir(self, tmp_path):
        report = _make_report()
        output_path = report.save(tmp_path)

        assert output_path.parent == tmp_path
        assert output_path.name.startswith("eval-unit-test-")
        assert output_path.suffix == ".json"
        assert output_path.exists()


class TestSaveAs:
    """save_as() writes to an exact, predictable path — the EvalRun/Argo contract."""

    def test_save_as_writes_exact_path(self, tmp_path):
        report = _make_report()
        target = tmp_path / "nested" / "report.json"

        result_path = report.save_as(target)

        assert result_path == target
        assert target.exists()

    def test_save_as_content_matches_to_dict(self, tmp_path):
        report = _make_report(passed=False)
        target = tmp_path / "report.json"

        report.save_as(target)

        with open(target) as f:
            data = json.load(f)

        assert data == report.to_dict()
        assert data["passed"] is False

    def test_save_as_creates_parent_dirs(self, tmp_path):
        report = _make_report()
        target = tmp_path / "a" / "b" / "c" / "report.json"

        report.save_as(target)

        assert target.exists()

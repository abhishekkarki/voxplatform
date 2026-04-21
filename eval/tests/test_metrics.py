"""Tests for WER metrics computation.

These tests run locally with no network calls.
They validate the core metric that gates deployments.
"""

from __future__ import annotations

import pytest

from vox_eval.metrics import SampleMetrics, aggregate, compute_wer


class TestComputeWER:
    """Test WER computation with various scenarios."""

    def test_perfect_match(self):
        wer, s, i, d = compute_wer("hello world", "hello world")
        assert wer == 0.0
        assert s == 0
        assert i == 0
        assert d == 0

    def test_case_insensitive(self):
        """WER should be case-insensitive — 'Hello' == 'hello'."""
        wer, _, _, _ = compute_wer("Hello World", "hello world")
        assert wer == 0.0

    def test_punctuation_ignored(self):
        """Punctuation differences are not errors."""
        wer, _, _, _ = compute_wer("Hello, world!", "hello world")
        assert wer == 0.0

    def test_single_substitution(self):
        wer, s, i, d = compute_wer("the cat sat", "the cat sit")
        assert s == 1
        assert i == 0
        assert d == 0
        assert abs(wer - 1 / 3) < 0.01  # 1 error out of 3 words

    def test_insertion(self):
        """Extra words in prediction count as insertions."""
        wer, s, i, d = compute_wer("the cat", "the big cat")
        assert i >= 1

    def test_deletion(self):
        """Missing words in prediction count as deletions."""
        wer, s, i, d = compute_wer("the big cat", "the cat")
        assert d >= 1

    def test_completely_wrong(self):
        wer, _, _, _ = compute_wer("hello world", "foo bar")
        assert wer == 1.0  # Every word is a substitution

    def test_empty_prediction(self):
        """Empty prediction = every reference word is deleted."""
        wer, s, i, d = compute_wer("hello world", "")
        assert wer == 1.0
        assert d == 2

    def test_empty_reference(self):
        """Empty reference with prediction = WER 1.0."""
        wer, _, _, _ = compute_wer("", "hello")
        assert wer == 1.0

    def test_both_empty(self):
        wer, _, _, _ = compute_wer("", "")
        assert wer == 0.0

    def test_contractions_expanded(self):
        """'don't' should match 'do not'."""
        wer, _, _, _ = compute_wer("I don't know", "I do not know")
        assert wer == 0.0

    def test_multiple_spaces_ignored(self):
        wer, _, _, _ = compute_wer("hello   world", "hello world")
        assert wer == 0.0

    def test_real_world_example(self):
        """A realistic STT scenario."""
        reference = "the quick brown fox jumps over the lazy dog"
        hypothesis = "the quick brown fox jumped over a lazy dog"
        # 2 errors: jumps→jumped, the→a
        wer, s, i, d = compute_wer(reference, hypothesis)
        assert 0.1 < wer < 0.4  # Reasonable range


class TestAggregate:
    """Test aggregate metric computation."""

    def _make_sample(
        self, sample_id: str, truth: str, pred: str, time: float = 1.0
    ) -> SampleMetrics:
        wer, s, i, d = compute_wer(truth, pred)
        return SampleMetrics(
            sample_id=sample_id,
            ground_truth=truth,
            prediction=pred,
            wer=wer,
            substitutions=s,
            insertions=i,
            deletions=d,
            processing_time=time,
        )

    def test_single_perfect_sample(self):
        sample = self._make_sample("s1", "hello world", "hello world")
        result = aggregate("test", [sample])

        assert result.corpus_wer == 0.0
        assert result.mean_wer == 0.0
        assert result.total_samples == 1

    def test_multiple_samples(self):
        samples = [
            self._make_sample("s1", "hello world", "hello world", time=1.0),
            self._make_sample("s2", "the cat sat", "the cat sit", time=2.0),
        ]
        result = aggregate("test", samples)

        assert result.total_samples == 2
        assert result.mean_wer > 0  # One sample has errors
        assert result.mean_processing_time == 1.5

    def test_median_odd_count(self):
        samples = [
            self._make_sample("s1", "a", "a"),        # WER 0.0
            self._make_sample("s2", "a b", "a c"),     # WER 0.5
            self._make_sample("s3", "a b c", "x y z"), # WER 1.0
        ]
        result = aggregate("test", samples)
        assert abs(result.median_wer - 0.5) < 0.01

    def test_median_even_count(self):
        samples = [
            self._make_sample("s1", "a", "a"),        # WER 0.0
            self._make_sample("s2", "a b", "a c"),     # WER 0.5
            self._make_sample("s3", "a b", "a c"),     # WER 0.5
            self._make_sample("s4", "a b c", "x y z"), # WER 1.0
        ]
        result = aggregate("test", samples)
        assert abs(result.median_wer - 0.5) < 0.01

    def test_empty_samples_raises(self):
        with pytest.raises(ValueError, match="No samples"):
            aggregate("test", [])

    def test_worst_sample_identifiable(self):
        samples = [
            self._make_sample("good", "hello world", "hello world"),
            self._make_sample("bad", "the cat sat", "foo bar baz"),
        ]
        result = aggregate("test", samples)

        worst = max(result.samples, key=lambda s: s.wer)
        assert worst.sample_id == "bad"
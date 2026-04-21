"""Speech recognition metrics.

WER (Word Error Rate) is the standard metric for evaluating
speech-to-text accuracy. It measures the minimum edit distance
between the predicted and reference transcripts at the word level.

WER = (Substitutions + Insertions + Deletions) / Total Reference Words

A WER of 0.0 means perfect transcription.
A WER of 0.1 means 10% of words were wrong.
A WER > 1.0 is possible if the model hallucinates extra words.

Example:
    Reference: "the cat sat on the mat"
    Predicted: "the cat sit on a mat"
    Substitutions: 2 (sat→sit, the→a)
    WER = 2/6 = 0.333
"""

from __future__ import annotations

from dataclasses import dataclass

import jiwer


@dataclass
class SampleMetrics:
    """Metrics for a single audio sample."""

    sample_id: str
    ground_truth: str
    prediction: str
    wer: float
    substitutions: int
    insertions: int
    deletions: int
    processing_time: float  # seconds


@dataclass
class AggregateMetrics:
    """Aggregate metrics across all samples in a dataset."""

    dataset_name: str
    total_samples: int
    mean_wer: float
    median_wer: float
    min_wer: float
    max_wer: float
    total_substitutions: int
    total_insertions: int
    total_deletions: int
    total_reference_words: int
    corpus_wer: float  # WER computed across all samples as one corpus
    mean_processing_time: float
    samples: list[SampleMetrics]

    @property
    def passed(self) -> bool:
        """A simple pass/fail — override with threshold in runner."""
        return self.corpus_wer < 1.0  # Sanity check only


# Standard text normalization transforms for WER computation.
# These handle common differences that aren't real errors:
# - case differences ("The" vs "the")
# - extra whitespace
# - punctuation ("Hello," vs "Hello")
_TRANSFORMS = jiwer.Compose([
    jiwer.ExpandCommonEnglishContractions(),  # "don't" → "do not"
    jiwer.RemoveMultipleSpaces(),
    jiwer.Strip(),
    jiwer.RemovePunctuation(),
    jiwer.ToLowerCase(),
])


def compute_wer(
    ground_truth: str,
    prediction: str,
) -> tuple[float, int, int, int]:
    """Compute WER between a reference and hypothesis.

    Applies standard normalization (lowercase, remove punctuation,
    expand contractions) before comparison.

    Args:
        ground_truth: The correct transcription.
        prediction: What the model produced.

    Returns:
        Tuple of (wer, substitutions, insertions, deletions).

    Example:
        wer, s, i, d = compute_wer("hello world", "hello word")
        # wer=0.5, s=1, i=0, d=0
    """
    # Handle edge cases
    if not ground_truth.strip():
        # If reference is empty, any prediction is 100% wrong
        return (1.0 if prediction.strip() else 0.0), 0, 0, 0

    if not prediction.strip():
        # If prediction is empty, every word in reference is a deletion
        word_count = len(ground_truth.split())
        return 1.0, 0, 0, word_count

    # Compute WER with detailed breakdown
    output = jiwer.process_words(
        ground_truth,
        prediction,
        reference_transform=_TRANSFORMS,
        hypothesis_transform=_TRANSFORMS,
    )

    return (
        output.wer,
        output.substitutions,
        output.insertions,
        output.deletions,
    )


def aggregate(
    dataset_name: str,
    sample_metrics: list[SampleMetrics],
) -> AggregateMetrics:
    """Compute aggregate statistics across all samples.

    Two WER numbers are computed:
    - mean_wer: average of per-sample WERs (each sample weighted equally)
    - corpus_wer: WER computed across all samples as one corpus
      (longer samples have more weight)

    Corpus WER is the standard for academic papers.
    Mean WER is more intuitive for debugging.
    """
    if not sample_metrics:
        raise ValueError("No samples to aggregate")

    wers = [s.wer for s in sample_metrics]
    sorted_wers = sorted(wers)

    # Corpus-level WER: treat all samples as one big transcript
    all_references = [s.ground_truth for s in sample_metrics]
    all_predictions = [s.prediction for s in sample_metrics]

    corpus_output = jiwer.process_words(
        all_references,
        all_predictions,
        reference_transform=_TRANSFORMS,
        hypothesis_transform=_TRANSFORMS,
    )

    # Median
    n = len(sorted_wers)
    if n % 2 == 0:
        median = (sorted_wers[n // 2 - 1] + sorted_wers[n // 2]) / 2
    else:
        median = sorted_wers[n // 2]

    total_ref_words = sum(
        len(s.ground_truth.split()) for s in sample_metrics
    )

    return AggregateMetrics(
        dataset_name=dataset_name,
        total_samples=len(sample_metrics),
        mean_wer=sum(wers) / len(wers),
        median_wer=median,
        min_wer=min(wers),
        max_wer=max(wers),
        total_substitutions=corpus_output.substitutions,
        total_insertions=corpus_output.insertions,
        total_deletions=corpus_output.deletions,
        total_reference_words=total_ref_words,
        corpus_wer=corpus_output.wer,
        mean_processing_time=sum(s.processing_time for s in sample_metrics) / len(sample_metrics),
        samples=sample_metrics,
    )
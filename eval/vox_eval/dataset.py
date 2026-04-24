"""Dataset loading for evaluation.

A dataset is a directory containing:
  - manifest.csv: maps audio filenames to ground truth text
  - audio files referenced in the manifest

Example manifest.csv:
    audio,text
    sample1.wav,"Hello world"
    sample2.wav,"The quick brown fox"

This is deliberately simple — no custom format, no database,
just CSV + audio files. Anyone can create a test dataset in 5 minutes.
"""

from __future__ import annotations

import csv
from dataclasses import dataclass
from pathlib import Path


@dataclass
class EvalSample:
    """A single evaluation sample: audio file + expected transcription."""

    audio_path: Path
    ground_truth: str
    sample_id: str  # Derived from filename, used in reports


@dataclass
class EvalDataset:
    """A collection of evaluation samples loaded from a manifest."""

    name: str
    samples: list[EvalSample]
    base_dir: Path

    @property
    def size(self) -> int:
        return len(self.samples)


def load_dataset(dataset_dir: str | Path) -> EvalDataset:
    """Load a dataset from a directory containing manifest.csv and audio files.

    Args:
        dataset_dir: Path to the dataset directory.

    Returns:
        EvalDataset with all samples loaded.

    Raises:
        FileNotFoundError: If the directory or manifest doesn't exist.
        ValueError: If the manifest is malformed or audio files are missing.

    Example:
        dataset = load_dataset("eval/datasets/librispeech-mini")
        print(f"Loaded {dataset.size} samples")
        for sample in dataset.samples:
            print(f"  {sample.sample_id}: {sample.ground_truth[:50]}...")
    """
    base_dir = Path(dataset_dir)

    if not base_dir.exists():
        raise FileNotFoundError(f"Dataset directory not found: {base_dir}")

    manifest_path = base_dir / "manifest.csv"
    if not manifest_path.exists():
        raise FileNotFoundError(
            f"manifest.csv not found in {base_dir}. "
            f"Create a CSV with columns: audio,text"
        )

    samples = []
    missing_files = []

    with open(manifest_path, newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)

        # Validate headers
        if reader.fieldnames is None:
            raise ValueError(f"Empty manifest: {manifest_path}")

        required = {"audio", "text"}
        actual = set(reader.fieldnames)
        if not required.issubset(actual):
            raise ValueError(
                f"Manifest missing required columns. "
                f"Expected: {required}, got: {actual}"
            )

        for i, row in enumerate(reader):
            audio_filename = row["audio"].strip()
            ground_truth = row["text"].strip()

            if not audio_filename or not ground_truth:
                raise ValueError(
                    f"Empty audio or text at manifest row {i + 2}"
                )

            audio_path = base_dir / audio_filename

            if not audio_path.exists():
                missing_files.append(audio_filename)
                continue

            # Sample ID is the filename without extension
            sample_id = Path(audio_filename).stem

            samples.append(EvalSample(
                audio_path=audio_path,
                ground_truth=ground_truth,
                sample_id=sample_id,
            ))

    if missing_files:
        raise ValueError(
            f"Audio files not found in {base_dir}: "
            + ", ".join(missing_files[:5])
            + (f" (and {len(missing_files) - 5} more)" if len(missing_files) > 5 else "")
        )

    if not samples:
        raise ValueError(f"No valid samples in manifest: {manifest_path}")

    # Dataset name is the directory name
    name = base_dir.name

    return EvalDataset(name=name, samples=samples, base_dir=base_dir)
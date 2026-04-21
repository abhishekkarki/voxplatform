"""Tests for dataset loading."""

from __future__ import annotations

from pathlib import Path

import pytest

from vox_eval.dataset import load_dataset


@pytest.fixture
def valid_dataset(tmp_path: Path) -> Path:
    """Create a valid dataset directory."""
    audio1 = tmp_path / "sample1.wav"
    audio2 = tmp_path / "sample2.wav"
    audio1.write_bytes(b"fake audio 1")
    audio2.write_bytes(b"fake audio 2")

    manifest = tmp_path / "manifest.csv"
    manifest.write_text(
        'audio,text\n'
        'sample1.wav,"hello world"\n'
        'sample2.wav,"the quick brown fox"\n'
    )
    return tmp_path


def test_load_valid_dataset(valid_dataset: Path):
    dataset = load_dataset(valid_dataset)

    assert dataset.size == 2
    assert dataset.name == valid_dataset.name
    assert dataset.samples[0].sample_id == "sample1"
    assert dataset.samples[0].ground_truth == "hello world"
    assert dataset.samples[1].ground_truth == "the quick brown fox"


def test_missing_directory():
    with pytest.raises(FileNotFoundError, match="not found"):
        load_dataset("/nonexistent/path")


def test_missing_manifest(tmp_path: Path):
    with pytest.raises(FileNotFoundError, match="manifest.csv"):
        load_dataset(tmp_path)


def test_missing_columns(tmp_path: Path):
    manifest = tmp_path / "manifest.csv"
    manifest.write_text("filename,transcript\ntest.wav,hello\n")

    with pytest.raises(ValueError, match="missing required columns"):
        load_dataset(tmp_path)


def test_missing_audio_file(tmp_path: Path):
    manifest = tmp_path / "manifest.csv"
    manifest.write_text('audio,text\nmissing.wav,"hello"\n')

    with pytest.raises(ValueError, match="Audio files not found"):
        load_dataset(tmp_path)


def test_empty_manifest(tmp_path: Path):
    manifest = tmp_path / "manifest.csv"
    manifest.write_text("audio,text\n")

    with pytest.raises(ValueError, match="No valid samples"):
        load_dataset(tmp_path)


def test_empty_text_field(tmp_path: Path):
    audio = tmp_path / "test.wav"
    audio.write_bytes(b"data")

    manifest = tmp_path / "manifest.csv"
    manifest.write_text('audio,text\ntest.wav,""\n')

    with pytest.raises(ValueError, match="Empty audio or text"):
        load_dataset(tmp_path)
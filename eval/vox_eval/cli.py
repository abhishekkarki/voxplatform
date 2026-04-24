"""VoxPlatform Evaluation CLI.

Usage:
    vox-eval run eval/datasets/librispeech-mini
    vox-eval run eval/datasets/librispeech-mini --threshold 0.2
    vox-eval run eval/datasets/librispeech-mini --url http://gateway:8080 --json
"""

from __future__ import annotations

import argparse
import json
import os
import sys

from vox_eval.runner import EvalRunner


def main() -> None:
    parser = argparse.ArgumentParser(
        prog="vox-eval",
        description="VoxPlatform evaluation harness — automated STT accuracy testing",
    )

    subparsers = parser.add_subparsers(dest="command", required=True)

    # -- run --
    run_parser = subparsers.add_parser("run", help="Run evaluation on a dataset")
    run_parser.add_argument(
        "dataset",
        help="Path to dataset directory containing manifest.csv",
    )
    run_parser.add_argument(
        "--url",
        default=os.environ.get("VOX_GATEWAY_URL", "http://localhost:8080"),
        help="Gateway URL (default: $VOX_GATEWAY_URL or http://localhost:8080)",
    )
    run_parser.add_argument(
        "-m", "--model",
        help="Model to use (default: gateway default)",
    )
    run_parser.add_argument(
        "--threshold",
        type=float,
        default=0.3,
        help="WER threshold — eval fails if corpus WER exceeds this (default: 0.3)",
    )
    run_parser.add_argument(
        "--output",
        default="eval/results",
        help="Output directory for JSON report (default: eval/results)",
    )
    run_parser.add_argument(
        "--json",
        action="store_true",
        dest="json_output",
        help="Output JSON report to stdout instead of human-readable summary",
    )
    run_parser.add_argument(
        "--timeout",
        type=float,
        default=120.0,
        help="Per-request timeout in seconds (default: 120)",
    )

    args = parser.parse_args()

    if args.command == "run":
        cmd_run(args)


def cmd_run(args: argparse.Namespace) -> None:
    runner = EvalRunner(
        gateway_url=args.url,
        model=args.model,
        timeout=args.timeout,
        wer_threshold=args.threshold,
    )

    try:
        report = runner.run(
            dataset_path=args.dataset,
            output_dir=args.output if not args.json_output else None,
        )
    except FileNotFoundError as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
    except RuntimeError as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)

    if args.json_output:
        print(json.dumps(report.to_dict(), indent=2))

    # Exit code: 0 = passed, 1 = failed
    # CI pipelines use exit codes to gate deployments
    sys.exit(0 if report.passed else 1)


if __name__ == "__main__":
    main()
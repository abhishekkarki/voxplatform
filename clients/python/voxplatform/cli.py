"""VoxPlatform CLI.

Usage:
    vox health                          # Check gateway health
    vox ready                           # Check if gateway can serve traffic
    vox models                          # List available models
    vox transcribe audio.wav            # Transcribe an audio file
    vox transcribe audio.wav -m model   # Use a specific model
    vox transcribe audio.wav --json     # Output raw JSON

The gateway URL defaults to http://localhost:8080.
Set VOX_GATEWAY_URL environment variable to change it.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time

from voxplatform.client import VoxClient
from voxplatform.models import VoxError


def main() -> None:
    parser = argparse.ArgumentParser(
        prog="vox",
        description="VoxPlatform CLI — interact with the voice inference gateway",
    )
    parser.add_argument(
        "--url",
        default=os.environ.get("VOX_GATEWAY_URL", "http://localhost:8080"),
        help="Gateway URL (default: $VOX_GATEWAY_URL or http://localhost:8080)",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=120.0,
        help="Request timeout in seconds (default: 120)",
    )

    subparsers = parser.add_subparsers(dest="command", required=True)

    # -- health --
    subparsers.add_parser("health", help="Check gateway liveness")

    # -- ready --
    subparsers.add_parser("ready", help="Check if gateway can serve traffic")

    # -- models --
    subparsers.add_parser("models", help="List available models")

    # -- transcribe --
    transcribe_parser = subparsers.add_parser("transcribe", help="Transcribe an audio file")
    transcribe_parser.add_argument("file", help="Path to audio file")
    transcribe_parser.add_argument(
        "-m", "--model",
        help="Model to use (default: gateway default)",
    )
    transcribe_parser.add_argument(
        "-l", "--language",
        help="Language code (e.g., 'en'). Auto-detected if omitted.",
    )
    transcribe_parser.add_argument(
        "--json",
        action="store_true",
        dest="json_output",
        help="Output raw JSON response",
    )

    args = parser.parse_args()

    # Create client
    client = VoxClient(base_url=args.url, timeout=args.timeout)

    try:
        if args.command == "health":
            cmd_health(client)
        elif args.command == "ready":
            cmd_ready(client)
        elif args.command == "models":
            cmd_models(client)
        elif args.command == "transcribe":
            cmd_transcribe(client, args)
    except VoxError as e:
        print(f"Error [{e.code}]: {e.message}", file=sys.stderr)
        print(f"Request ID: {e.request_id}", file=sys.stderr)
        sys.exit(1)
    except ConnectionError:
        print(f"Cannot connect to gateway at {args.url}", file=sys.stderr)
        print("Is the gateway running? Check VOX_GATEWAY_URL.", file=sys.stderr)
        sys.exit(1)
    finally:
        client.close()


def cmd_health(client: VoxClient) -> None:
    status = client.health()
    print(f"Gateway: {status.status}")


def cmd_ready(client: VoxClient) -> None:
    status = client.ready()
    print(f"Gateway: {status.status}")
    if status.reason:
        print(f"Reason: {status.reason}")


def cmd_models(client: VoxClient) -> None:
    models = client.models()
    if not models:
        print("No models available")
        return

    for m in models:
        print(f"  {m.id}  [{m.type}]  {m.state}")


def cmd_transcribe(client: VoxClient, args: argparse.Namespace) -> None:
    # Show progress since transcription can be slow
    print(f"Transcribing: {args.file}", file=sys.stderr)
    start = time.time()

    result = client.transcribe(
        args.file,
        model=args.model,
        language=args.language,
    )

    elapsed = time.time() - start

    if args.json_output:
        # Raw JSON output — useful for piping to jq or other tools
        print(json.dumps(result.model_dump(by_alias=True), indent=2))
    else:
        # Human-readable output
        print(f"\n{result.text}\n")
        print(f"---", file=sys.stderr)
        print(f"Model:      {result.model}", file=sys.stderr)
        print(f"Time:       {result.processing_time:.2f}s (server) / {elapsed:.2f}s (total)", file=sys.stderr)
        print(f"Request ID: {result.request_id}", file=sys.stderr)


if __name__ == "__main__":
    main()
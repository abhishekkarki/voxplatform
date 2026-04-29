"""VoxPlatform CLI.

Usage:
    vox health                          # Check gateway health
    vox ready                           # Check if gateway can serve traffic
    vox models                          # List available models
    vox transcribe audio.wav            # Transcribe an audio file
    vox record                          # Stream from mic, live transcription
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
    transcribe_parser.add_argument("-m", "--model", help="Model to use")
    transcribe_parser.add_argument("-l", "--language", help="Language code")
    transcribe_parser.add_argument("--json", action="store_true", dest="json_output", help="Output raw JSON")

    # -- record --
    record_parser = subparsers.add_parser("record", help="Stream from microphone, live transcription")
    record_parser.add_argument("--duration", type=int, default=0, help="Max recording duration in seconds (0=unlimited)")

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
        elif args.command == "record":
            cmd_record(args)
    except VoxError as e:
        print(f"Error [{e.code}]: {e.message}", file=sys.stderr)
        print(f"Request ID: {e.request_id}", file=sys.stderr)
        sys.exit(1)
    except ConnectionError:
        print(f"Cannot connect to gateway at {args.url}", file=sys.stderr)
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
    print(f"Transcribing: {args.file}", file=sys.stderr)
    start = time.time()

    result = client.transcribe(args.file, model=args.model, language=args.language)
    elapsed = time.time() - start

    if args.json_output:
        print(json.dumps(result.model_dump(by_alias=True), indent=2))
    else:
        print(f"\n{result.text}\n")
        print(f"---", file=sys.stderr)
        print(f"Model:      {result.model}", file=sys.stderr)
        print(f"Time:       {result.processing_time:.2f}s (server) / {elapsed:.2f}s (total)", file=sys.stderr)
        print(f"Request ID: {result.request_id}", file=sys.stderr)


def cmd_record(args: argparse.Namespace) -> None:
    """Stream audio from microphone over WebSocket.

    Opens a WebSocket connection to the gateway's /v1/audio/stream endpoint,
    captures audio from the default microphone, sends 20ms frames,
    and prints partial transcripts as they arrive.
    """
    try:
        import sounddevice as sd
    except ImportError:
        print("sounddevice not installed. Run: pip install 'voxplatform[audio]'", file=sys.stderr)
        sys.exit(1)

    try:
        from websockets.sync.client import connect
    except ImportError:
        print("websockets not installed. Run: pip install websockets", file=sys.stderr)
        sys.exit(1)

    import numpy as np

    # Convert HTTP URL to WebSocket URL
    ws_url = args.url.replace("http://", "ws://").replace("https://", "wss://")
    ws_url = f"{ws_url}/v1/audio/stream"

    sample_rate = 16000
    frame_duration_ms = 20
    frame_samples = sample_rate * frame_duration_ms // 1000  # 320 samples

    print(f"Connecting to {ws_url}...", file=sys.stderr)

    with connect(ws_url) as ws:
        print("Connected. Speak now (Ctrl+C to stop)...\n", file=sys.stderr)

        # Audio callback — streams PCM frames to WebSocket
        def audio_callback(indata, frames, time_info, status):
            if status:
                print(f"Audio status: {status}", file=sys.stderr)
            # Convert float32 → int16 PCM bytes
            pcm = (indata[:, 0] * 32767).astype(np.int16).tobytes()
            try:
                ws.send(pcm)
            except Exception:
                pass

        # Receive loop — prints transcripts from the gateway
        import threading

        def receive_loop():
            try:
                while True:
                    msg = ws.recv()
                    data = json.loads(msg)

                    if data["type"] == "final":
                        print(f"  {data['text']}")
                        sys.stdout.flush()
                    elif data["type"] == "error":
                        print(f"  [ERROR] {data['text']}", file=sys.stderr)
            except Exception:
                pass

        receiver = threading.Thread(target=receive_loop, daemon=True)
        receiver.start()

        # Start recording from microphone
        try:
            with sd.InputStream(
                samplerate=sample_rate,
                channels=1,
                dtype="float32",
                blocksize=frame_samples,
                callback=audio_callback,
            ):
                if args.duration > 0:
                    time.sleep(args.duration)
                else:
                    print("Press Ctrl+C to stop recording.\n", file=sys.stderr)
                    while True:
                        time.sleep(0.1)
        except KeyboardInterrupt:
            print("\nStopping...", file=sys.stderr)

        # Signal end of session
        ws.send("close")
        time.sleep(1)  # Wait for final transcript
        print("\nSession ended.", file=sys.stderr)


if __name__ == "__main__":
    main()
#!/usr/bin/env python3
"""Compare source/output audiobook speech with Whisper windows.

This is a corruption detector, not an objective audio-quality measurement. It
transcribes aligned windows from both files and reports windows whose speech
content differs materially. The source file is never modified.
"""

from __future__ import annotations

import argparse
import difflib
import json
import re
import subprocess
import tempfile
import time
from pathlib import Path


def duration(path: Path) -> float:
    result = subprocess.run(
        ["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", str(path)],
        check=True, capture_output=True, text=True,
    )
    return float(result.stdout.strip())


def extract(path: Path, start: float, seconds: float, out: Path) -> None:
    subprocess.run(
        ["ffmpeg", "-y", "-v", "error", "-ss", str(start), "-t", str(seconds), "-i", str(path),
         "-vn", "-ac", "1", "-ar", "16000", str(out)],
        check=True,
    )


def normalize(text: str) -> str:
    return re.sub(r"[^a-z0-9 ]+", " ", text.lower()).strip()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--model", default="large-v3-turbo")
    parser.add_argument("--device", default="mps")
    parser.add_argument("--window-seconds", type=float, default=45)
    parser.add_argument("--interval-seconds", type=float, default=300)
    parser.add_argument("--start-offset-seconds", type=float, default=30)
    parser.add_argument("--min-similarity", type=float, default=0.65)
    parser.add_argument("--json", type=Path, required=True)
    args = parser.parse_args()

    import whisper

    source_duration = duration(args.source)
    output_duration = duration(args.output)
    if abs(source_duration - output_duration) > 2.0:
        raise SystemExit(f"duration mismatch: {source_duration:.3f} vs {output_duration:.3f}")

    model = whisper.load_model(args.model, device=args.device)
    first = min(args.start_offset_seconds, max(0, source_duration - args.window_seconds))
    starts = list(range(int(first), max(1, int(source_duration - args.window_seconds)), int(args.interval_seconds)))
    windows = []
    started = time.time()
    with tempfile.TemporaryDirectory(prefix="sonder-whisper-") as temp:
        temp_path = Path(temp)
        for start in starts:
            src_wav = temp_path / "source.wav"
            out_wav = temp_path / "output.wav"
            extract(args.source, start, args.window_seconds, src_wav)
            extract(args.output, start, args.window_seconds, out_wav)
            source_text = model.transcribe(str(src_wav), language="en", fp16=False, temperature=0,
                                           condition_on_previous_text=False)["text"].strip()
            output_text = model.transcribe(str(out_wav), language="en", fp16=False, temperature=0,
                                           condition_on_previous_text=False)["text"].strip()
            source_norm, output_norm = normalize(source_text), normalize(output_text)
            similarity = difflib.SequenceMatcher(None, source_norm, output_norm).ratio()
            windows.append({
                "startSeconds": start,
                "sourceText": source_text,
                "outputText": output_text,
                "similarity": round(similarity, 4),
                "passed": bool(source_norm and output_norm and similarity >= args.min_similarity),
            })

    report = {
        "model": args.model,
        "device": args.device,
        "source": str(args.source),
        "output": str(args.output),
        "sourceDurationSeconds": source_duration,
        "outputDurationSeconds": output_duration,
        "windowSeconds": args.window_seconds,
        "intervalSeconds": args.interval_seconds,
        "minSimilarity": args.min_similarity,
        "windows": windows,
        "passed": all(w["passed"] for w in windows),
        "elapsedSeconds": round(time.time() - started, 1),
    }
    args.json.parent.mkdir(parents=True, exist_ok=True)
    args.json.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({"passed": report["passed"], "windows": len(windows), "failed": sum(not w["passed"] for w in windows), "elapsedSeconds": report["elapsedSeconds"], "report": str(args.json)}))
    return 0 if report["passed"] else 2


if __name__ == "__main__":
    raise SystemExit(main())

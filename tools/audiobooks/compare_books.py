#!/usr/bin/env python3
"""Compare the audio content of two audiobook files that a layout plan flagged
as competing copies.

Two folders claiming the same canonical path is a *suspicion*, not a finding.
Equal byte size is not proof of equal content, and two different editions of one
book are genuinely different files that both should be kept. This tool produces
the evidence needed to choose, without touching either file.

It is read-only: files are opened for reading only, hashed, and never moved,
renamed, or modified.

The fingerprint combines:
  - container duration and stream layout, from ffprobe
  - size
  - SHA-256 of the head and tail windows of the file, plus a sampled digest
    over the whole file when --full is given

Head+tail is enough to prove *difference* cheaply. It can only prove *identity*
when combined with equal size and equal duration, which is why the verdict
wording distinguishes the two rather than claiming "identical".
"""

from __future__ import annotations

import argparse
import hashlib
import json
import pathlib
import subprocess
import sys

FFPROBE = "/opt/homebrew/bin/ffprobe"
FFMPEG = "/opt/homebrew/bin/ffmpeg"
WINDOW = 8 * 1024 * 1024  # 8 MiB head and tail

# Seconds of audio sampled at each probe point. Decoding to raw PCM is what
# makes the comparison answer the real question ("is this the same
# recording?") rather than the container question ("are these bytes equal?"),
# which a retag pass changes without touching the audio at all.
AUDIO_SAMPLE_SECONDS = 20
AUDIO_SAMPLE_POINTS = 5


def probe_fingerprint(path: pathlib.Path) -> dict:
    try:
        out = subprocess.run(
            [FFPROBE, "-v", "quiet", "-print_format", "json",
             "-show_format", "-show_streams", str(path)],
            capture_output=True, timeout=300, check=True).stdout
        d = json.loads(out)
    except (subprocess.CalledProcessError, subprocess.TimeoutExpired, OSError,
            json.JSONDecodeError) as exc:
        return {"error": str(exc)}
    fmt = d.get("format", {})
    audio = [s for s in d.get("streams", []) if s.get("codec_type") == "audio"]
    a0 = audio[0] if audio else {}
    return {
        "duration_seconds": round(float(fmt.get("duration", 0) or 0), 3),
        "container_bitrate": int(fmt.get("bit_rate", 0) or 0),
        "audio_streams": len(audio),
        "audio_codec": a0.get("codec_name", ""),
        "audio_channels": a0.get("channels", 0),
        "sample_rate": a0.get("sample_rate", ""),
        "tags": {k.lower().lstrip("©"): v for k, v in (fmt.get("tags") or {}).items()
                 if k.lower() not in ("encoder", "major_brand", "minor_version",
                                       "compatible_brands")},
    }


def file_fingerprint(path: pathlib.Path, full: bool) -> dict:
    size = path.stat().st_size
    h = hashlib.sha256()
    try:
        with path.open("rb") as f:
            head = f.read(WINDOW)
            h.update(head)
            head_hash = h.hexdigest()
            f.seek(max(0, size - WINDOW))
            h.update(f.read(WINDOW))
            if full:
                f.seek(0)
                for chunk in iter(lambda: f.read(8 * 1024 * 1024), b""):
                    h.update(chunk)
    except PermissionError as exc:
        # Some files in this library are mode 0600 owned by another uid from a
        # previous copy. Report that rather than failing the whole comparison.
        return {"size_bytes": size, "error": f"permission_denied: {exc.strerror}"}
    return {
        "size_bytes": size,
        "head_tail_sha256": h.hexdigest(),
        "head_sha256": head_hash,
        "full_hash": h.hexdigest() if full else None,
    }


def audio_fingerprint(path: pathlib.Path, duration: float) -> dict:
    """Hash the decoded audio, sampled at several points across the file.

    Container-level hashes cannot distinguish "the same recording, re-tagged"
    from "a different recording", because rewriting metadata changes the file
    bytes without changing a single sample. Decoding short windows to raw PCM
    and hashing those answers the question that actually matters.
    """
    if duration <= 0:
        return {"error": "unknown duration"}
    # Sample at the start, the middle, and toward the end, skipping the
    # very edges where silence and encoding padding live.
    points = []
    for i in range(AUDIO_SAMPLE_POINTS):
        frac = (i + 0.5) / AUDIO_SAMPLE_POINTS
        points.append(max(0.0, duration * frac - AUDIO_SAMPLE_SECONDS / 2))

    h = hashlib.sha256()
    for start in points:
        cmd = [FFMPEG, "-v", "quiet", "-nostdin", "-ss", f"{start:.3f}",
               "-t", str(AUDIO_SAMPLE_SECONDS), "-i", str(path),
               "-map", "0:a:0", "-f", "s16le", "-acodec", "pcm_s16le",
               "-ar", "8000", "-ac", "1", "-"]
        try:
            pcm = subprocess.run(cmd, capture_output=True, timeout=600, check=True).stdout
        except (subprocess.CalledProcessError, subprocess.TimeoutExpired, OSError) as exc:
            return {"error": f"audio decode failed at {start:.0f}s: {exc}"}
        if not pcm:
            return {"error": f"no audio decoded at {start:.0f}s"}
        h.update(pcm)
    return {
        "sample_points_seconds": [round(p, 1) for p in points],
        "sample_seconds_each": AUDIO_SAMPLE_SECONDS,
        "decoded_pcm_sha256": h.hexdigest(),
    }


def compare(a: pathlib.Path, b: pathlib.Path, full: bool, with_audio: bool) -> dict:
    fa, fb = file_fingerprint(a, full), file_fingerprint(b, full)
    pa, pb = probe_fingerprint(a), probe_fingerprint(b)

    unreadable = [str(p) for p, f in ((a, fa), (b, fb)) if f.get("error")]
    same_size = (not unreadable) and fa["size_bytes"] == fb["size_bytes"]
    same_head_tail = ((not unreadable)
                      and fa.get("head_tail_sha256") == fb.get("head_tail_sha256"))
    same_duration = (pa.get("duration_seconds") == pb.get("duration_seconds")
                     and pa.get("duration_seconds", 0) > 0)
    same_audio = (pa.get("audio_codec") == pb.get("audio_codec")
                  and pa.get("audio_channels") == pb.get("audio_channels")
                  and pa.get("sample_rate") == pb.get("sample_rate"))

    aaf = baf = {}
    if with_audio and not unreadable:
        aaf = audio_fingerprint(a, pa.get("duration_seconds", 0.0))
        baf = audio_fingerprint(b, pb.get("duration_seconds", 0.0))
    same_recording = (not aaf.get("error") and not baf.get("error")
                      and bool(aaf) and bool(baf)
                      and aaf.get("decoded_pcm_sha256") == baf.get("decoded_pcm_sha256"))

    if unreadable:
        verdict = "not_comparable"
        detail = ("Could not read the file content: "
                  + ", ".join(unreadable) + ". The size below is still "
                  "recorded, but equal size is not proof of equal content.")
    elif with_audio and same_recording:
        if same_size and same_head_tail:
            verdict = "identical_file"
            detail = "Byte-identical files: the same recording, same container."
        else:
            verdict = "same_recording_different_container"
            detail = ("The decoded audio is identical at every sampled point, "
                      "so this is the same recording in a different container "
                      "or with different tags. Keep one copy; the retagged one "
                      "is usually the better one because it carries credits.")
    elif with_audio and (aaf.get("error") or baf.get("error")):
        verdict = "container_only_comparison"
        detail = ("Audio could not be decoded for comparison ("
                  + (aaf.get("error") or baf.get("error")) + "). Falling back "
                  "to container evidence, which a retag pass can change.")
    elif same_head_tail and same_size:
        verdict = "identical_file"
        detail = "Byte-identical files: the same recording, same container."
    elif same_duration and same_audio and not same_size:
        verdict = "different_content_same_book"
        detail = ("Same duration and audio format but different bytes, and the "
                  "audio was not compared. Likely the same recording re-tagged, "
                  "or a different encode. Re-run with --audio to decide.")
    elif same_head_tail and not same_size:
        verdict = "prefix_identical"
        detail = "Head and tail match but sizes differ; the files were edited."
    else:
        verdict = "different_books"
        detail = "Different audio content. These are not copies of each other."

    return {
        "a": {"path": str(a), "file": fa, "probe": pa, "audio": aaf},
        "b": {"path": str(b), "file": fb, "probe": pb, "audio": baf},
        "unreadable": unreadable,
        "same_size": same_size,
        "same_head_tail": same_head_tail,
        "same_duration": same_duration,
        "same_audio_format": same_audio,
        "same_recording_audio": same_recording if with_audio else None,
        "verdict": verdict,
        "detail": detail,
    }


def main() -> int:
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("plan", type=pathlib.Path)
    p.add_argument("--out", required=True, type=pathlib.Path)
    p.add_argument("--full", action="store_true",
                   help="also hash the whole file (slower; reads every byte "
                        "over the network)")
    p.add_argument("--audio", action="store_true", default=True,
                   help="decode sampled audio and hash the PCM (default on; "
                        "this is what distinguishes a re-tagged copy from a "
                        "different recording)")
    p.add_argument("--no-audio", dest="audio", action="store_false",
                   help="skip audio decoding and use container evidence only")
    a = p.parse_args()

    plan = json.loads(a.plan.read_text())
    # Group every folder that claims a canonical path, however many there are.
    by_canonical: dict[str, list[dict]] = {}
    for f in plan["folders"]:
        if f.get("canonical_path"):
            by_canonical.setdefault(f["canonical_path"], []).append(f)

    groups = {c: fs for c, fs in by_canonical.items() if len(fs) > 1}
    results = []
    for canonical, folders in sorted(groups.items()):
        print(f"\n=== {canonical}  ({len(folders)} folders) ===", file=sys.stderr)
        # Compare the first (and only) audio file in each folder. A flagged
        # canonical collision always involves single-file folders, so take
        # files[0] defensively rather than assuming.
        files = [pathlib.Path(fs["files"][0]["source"]) for fs in folders]
        for i in range(len(files)):
            for j in range(i + 1, len(files)):
                r = compare(files[i], files[j], a.full, a.audio)
                r["canonical_path"] = canonical
                r["folders"] = [fs["relative_folder"] for fs in folders]
                results.append(r)
                print(f"  {files[i].name}\n   vs {files[j].name}\n"
                      f"   -> {r['verdict']}: {r['detail']}", file=sys.stderr)

    report = {
        "policy": ("Read-only comparison. Nothing was renamed, moved, or "
                   "modified. Equal size is not proof of equal content: a "
                   "retag pass changes container bytes without touching the "
                   "audio. The decisive test is the decoded-PCM digest."),
        "full_hash": a.full,
        "audio_compared": a.audio,
        "groups_compared": len(groups),
        "pairs_compared": len(results),
        "verdict_counts": {v: sum(1 for r in results if r["verdict"] == v)
                           for v in {r["verdict"] for r in results}},
        "comparisons": results,
    }
    a.out.parent.mkdir(parents=True, exist_ok=True)
    a.out.write_text(json.dumps(report, indent=2))
    print(f"\n{json.dumps(report['verdict_counts'], indent=2)}", file=sys.stderr)
    print(f"wrote {a.out}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())

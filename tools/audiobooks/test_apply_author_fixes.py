"""Tests for the manifest executor.

The dangerous properties are the ones worth pinning: it must refuse a merge, it
must not duplicate media, and it must put a book back if the post-move
verification fails. Everything here runs against a temporary tree.
"""

import json
import os
import pathlib

import pytest

from apply_author_fixes import dir_stats, main


def make_book(root: pathlib.Path, author: str, book: str, n=1, size=1024) -> pathlib.Path:
    d = root / author / book
    d.mkdir(parents=True)
    for i in range(n):
        (d / f"{book} - part {i + 1}.m4b").write_bytes(b"\0" * size)
    return d


def entry(**kw) -> dict:
    base = {"action": "move_and_retag", "reason": "test",
            "blocker": None, "source_dir": "A/B", "target_dir": "C/B",
            "author_from": "A", "author_to": "C", "book": "B", "files": 1,
            "evidence": {}, "target_exists": False, "typo_of": None,
            "safe_to_move": True}
    base.update(kw)
    return base


def write_manifest(tmp_path: pathlib.Path, entries: list[dict]) -> pathlib.Path:
    p = tmp_path / "manifest.json"
    p.write_text(json.dumps({"entries": entries}, indent=2))
    return p


def run(manifest, root, *extra) -> int:
    import sys
    argv = ["apply_author_fixes.py", "--manifest", str(manifest),
            "--root", str(root), *extra]
    old = sys.argv
    sys.argv = argv
    try:
        return main()
    finally:
        sys.argv = old


# --- preflight refusals --------------------------------------------------


def test_existing_destination_is_refused_as_a_merge(tmp_path):
    root = tmp_path / "lib"
    make_book(root, "A", "B")
    make_book(root, "C", "B")  # already here
    m = write_manifest(tmp_path, [entry()])
    assert run(m, root, "--apply", "--receipt", str(tmp_path / "r.json")) == 0
    r = json.loads((tmp_path / "r.json").read_text())
    assert r["moved"] == []
    assert "merge" in r["refused"][0]["reason"]
    # Both copies are untouched.
    assert (root / "A" / "B").is_dir() and (root / "C" / "B").is_dir()


def test_manifest_hold_is_respected_even_if_the_path_is_free(tmp_path):
    root = tmp_path / "lib"
    make_book(root, "A", "B")
    m = write_manifest(tmp_path, [entry(safe_to_move=False,
                                        blocker="one char from 'Cee'")])
    run(m, root, "--apply", "--receipt", str(tmp_path / "r.json"))
    r = json.loads((tmp_path / "r.json").read_text())
    assert r["moved"] == []
    assert r["refused"][0]["reason"] == "one char from 'Cee'"
    assert (root / "A" / "B").is_dir()


def test_missing_source_is_refused(tmp_path):
    root = tmp_path / "lib"
    (root / "A").mkdir(parents=True)
    m = write_manifest(tmp_path, [entry()])
    run(m, root, "--apply", "--receipt", str(tmp_path / "r.json"))
    r = json.loads((tmp_path / "r.json").read_text())
    assert r["moved"] == []
    assert "missing" in r["refused"][0]["reason"]


# --- the happy path ------------------------------------------------------


def test_move_preserves_every_byte_and_creates_no_duplicate(tmp_path):
    root = tmp_path / "lib"
    src = make_book(root, "A", "B", n=3, size=4096)
    before = dir_stats(src)
    m = write_manifest(tmp_path, [entry()])
    assert run(m, root, "--apply", "--receipt", str(tmp_path / "r.json")) == 0

    dst = root / "C" / "B"
    assert not src.exists(), "the source must be gone, or the book is duplicated"
    assert dir_stats(dst) == before
    r = json.loads((tmp_path / "r.json").read_text())
    assert r["moved"][0]["verified_files"] == 3
    assert r["moved"][0]["verified_bytes"] == 3 * 4096
    # And the bytes are literally the same file, not a re-encode.
    assert sorted(f.name for f in dst.iterdir()) == [
        "B - part 1.m4b", "B - part 2.m4b", "B - part 3.m4b"]


def test_plan_mode_changes_nothing(tmp_path):
    root = tmp_path / "lib"
    make_book(root, "A", "B")
    m = write_manifest(tmp_path, [entry()])
    assert run(m, root) == 0
    assert (root / "A" / "B").is_dir()
    assert not (root / "C").exists()


def test_limit_stops_early_and_says_so(tmp_path):
    root = tmp_path / "lib"
    make_book(root, "A", "B")
    make_book(root, "A", "C")
    m = write_manifest(tmp_path, [entry(book="B"), entry(book="C")])
    run(m, root, "--apply", "--limit", "1", "--receipt", str(tmp_path / "r.json"))
    r = json.loads((tmp_path / "r.json").read_text())
    assert len(r["moved"]) == 1
    assert r["failed"][0]["error"] == "limit_reached"


def test_rolled_back_when_verification_fails(tmp_path, monkeypatch):
    # If the move lands but the post-move check fails, the book goes back where
    # it was. A half-applied move is worse than no move.
    root = tmp_path / "lib"
    src = make_book(root, "A", "B", n=2, size=2048)
    m = write_manifest(tmp_path, [entry()])

    import apply_author_fixes
    real = apply_author_fixes.dir_stats
    calls = {"n": 0}

    def flaky(p):
        calls["n"] += 1
        # The first call is the pre-move snapshot, the second is the post-move
        # check; report a different size the second time.
        return (99, 99) if calls["n"] == 2 else real(p)

    monkeypatch.setattr(apply_author_fixes, "dir_stats", flaky)
    assert run(m, root, "--apply", "--receipt", str(tmp_path / "r.json")) == 1
    monkeypatch.undo()

    assert src.is_dir(), "the book must be back at its original path"
    assert not (root / "C").exists()
    r = json.loads((tmp_path / "r.json").read_text())
    assert r["rolled_back"][0]["error"].startswith("post-move check failed")
    assert r["moved"] == []


def test_creates_the_author_folder_when_absent(tmp_path):
    root = tmp_path / "lib"
    make_book(root, "A", "B")
    m = write_manifest(tmp_path, [entry(author_to="New Author")])
    run(m, root, "--apply", "--receipt", str(tmp_path / "r.json"))
    assert (root / "New Author" / "B").is_dir()


def test_resource_files_are_not_counted_as_media(tmp_path):
    root = tmp_path / "lib"
    d = make_book(root, "A", "B", n=2)
    (d / "._B - part 1.m4b").write_bytes(b"\0" * 999)  # AppleDouble
    assert dir_stats(d) == (2, 2048)

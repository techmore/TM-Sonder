#!/usr/bin/env python3
"""Fetch missing audiobook covers (stage locally, no NAS writes).

Reads missing_covers from m4b_work/cleanup-report.json, tries Wikipedia
then Open Library for each, stages verified images under
m4b_work/migration/covers/candidates/, records results in
m4b_work/migration/covers/fetch-results.json.

  python3 tools/audiobooks/fetch_covers.py [--limit N]

Pacing (~1 req/1.2s) is respected; every network op is guarded so one
failure can't kill the run. Never invents art: strict title-overlap +
author-surname checks, failures recorded as no-match.
"""
import json
import os
import re
import sys
import time
import urllib.parse
import urllib.request
import urllib.error

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)),
                                "..", ".."))
import poster_fetch as pf

ROOT = "/Users/seandolbec/Projects/TM-Sonder"
REPORT = os.path.join(ROOT, "m4b_work", "cleanup-report.json")
CAND_DIR = os.path.join(ROOT, "m4b_work", "migration", "covers", "candidates")
RESULTS = os.path.join(ROOT, "m4b_work", "migration", "covers", "fetch-results.json")


def slug(author, book):
    s = f"{author}-{book}".lower()
    s = re.sub(r"[^a-z0-9]+", "-", s).strip("-")
    return s[:100] or "cover"


def save_image(url, dest_base):
    src = url.split("?")[0]
    ext = ".png" if ".png" in src.lower() else ".jpg"
    for cand in (dest_base + ".jpg", dest_base + ".png"):
        if os.path.exists(cand):
            return cand  # never overwrite / re-download
    path = dest_base + ext
    req = urllib.request.Request(src, headers={"User-Agent": pf.IMG_UA})
    with urllib.request.urlopen(req, timeout=40) as r:
        data = r.read()
    ok = (len(data) >= 5000 and
          (data[:3] == b"\xff\xd8\xff" or data[:8] == b"\x89PNG\r\n\x1a\n"))
    if not ok:
        return None
    tmp = path + ".tmp"
    with open(tmp, "wb") as f:
        f.write(data)
    os.rename(tmp, path)
    return path


def ol_fallback(clean, author):
    q = urllib.parse.quote(f"{clean} {author}")
    url = (f"https://openlibrary.org/search.json?q={q}"
           "&fields=title,author_name,cover_i,key&limit=5")
    req = urllib.request.Request(url, headers={"User-Agent": pf.API_UA})
    with urllib.request.urlopen(req, timeout=20) as r:
        res = json.loads(r.read().decode("utf-8", "replace"))
    time.sleep(1.2)
    surname = author.split()[-1].lower()
    for doc in res.get("docs", []):
        tov = pf.overlap(clean, doc.get("title", ""))
        amatch = any(surname in a.lower() for a in doc.get("author_name", []))
        if tov >= 0.75 or (tov >= 0.6 and amatch):
            ci = doc.get("cover_i")
            if ci:
                return f"https://covers.openlibrary.org/b/id/{ci}-L.jpg"
    return None


def main():
    limit = int(sys.argv[sys.argv.index("--limit") + 1]) if "--limit" in sys.argv else 10 ** 9
    os.makedirs(CAND_DIR, exist_ok=True)
    rep = json.load(open(REPORT))
    targets = []
    for author, row in rep["authors"].items():
        if not isinstance(row, dict):
            continue
        for b in row.get("missing_covers", []):
            if isinstance(b, str):
                targets.append((author, b))
    targets = targets[:limit]
    print(f"cover-fetch targets: {len(targets)}", flush=True)

    prior = {}
    if os.path.exists(RESULTS):
        try:
            for row in json.load(open(RESULTS)):
                prior[(row["author"], row["book"])] = row
        except ValueError:
            pass

    out = []
    for i, (author, book) in enumerate(targets, 1):
        key = (author, book)
        if key in prior and prior[key].get("status") == "staged":
            out.append(prior[key])
            print(f"{i}/{len(targets)} SKIP-staged {author} / {book}", flush=True)
            continue
        dest = os.path.join(CAND_DIR, slug(author, book))
        status, path = "no-match", None
        try:
            url = pf.get_image_for(book, log=lambda *a: None)
            if url:
                path = save_image(url, dest)
                status = "staged" if path else "no-image"
            if status != "staged" and author not in ("Various",):
                try:
                    url = ol_fallback(pf.clean_title(book), author)
                    if url:
                        path = save_image(url, dest)
                        if path:
                            status = "staged"
                except Exception as e:
                    print(f"  OL err {author}/{book}: {e}", flush=True)
        except Exception as e:
            status = "error"
            print(f"  ERR {author}/{book}: {e}", flush=True)
        row = {"author": author, "book": book, "status": status, "cover": path}
        out.append(row)
        print(f"{i}/{len(targets)} {status} {author} / {book}"
              + (f" -> {os.path.basename(path)}" if path else ""), flush=True)
        if i % 10 == 0:
            json.dump(out, open(RESULTS, "w"), indent=1)
        time.sleep(0.5)
    json.dump(out, open(RESULTS, "w"), indent=1)
    from collections import Counter
    print("FETCH DONE:", dict(Counter(r["status"] for r in out)), flush=True)


if __name__ == "__main__":
    main()

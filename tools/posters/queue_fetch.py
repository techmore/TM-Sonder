#!/usr/bin/env python3
"""Fetch Wikipedia infobox posters for queued movies/shows (internet only).

Resumable: skips staged entries, retries retry-later/error ones. All output
lands in m4b_work/poster-queue/ (local disk), so this runs fine while the NAS
is disconnected. Heavy throttling just means entries stay retry-later.
"""
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime

ROOT = "/Users/seandolbec/Projects/TM-Sonder/m4b_work/poster-queue"
UA = {"User-Agent": "sonder-poster-queue/1.0 (contact: local)"}
LAST_RUN = os.path.join(ROOT, ".last-fetch")
MIN_INTERVAL = 3 * 3600  # one retry sweep every few hours is plenty polite
BETWEEN_CALLS = 12  # seconds between API calls; well under bot etiquette
THROTTLE = {"fails": 0}


def enrich_running():
    try:
        import urllib.request as _r
        c = json.load(open(os.path.expanduser("~/.config/sonder/server.json")))
        base = f"http://127.0.0.1:{c.get('port', 8097)}"
        q = f"?token={c.get('pairingToken', '')}"
        st = json.load(_r.urlopen(base + "/api/status" + q, timeout=15))
        return bool(st.get("enriching"))
    except Exception:
        return False


def clean(t):
    t = re.sub(r"\[.*?\]", "", t)
    t = re.sub(r"MgB iNFo", "", t, flags=re.I)
    return re.sub(r"\s+", " ", t).strip(" -.")


THROTTLE = {"fails": 0}


def get(u, tries=3):
    """Polite fetch: 12s baseline, honors Retry-After, exponential backoff.
    After 5 consecutive hard blocks the whole run parks itself (circuit
    breaker) instead of extending the IP block."""
    for a in range(tries):
        try:
            time.sleep(BETWEEN_CALLS)
            req = urllib.request.urlopen(
                urllib.request.Request(u, headers=UA), timeout=30)
            THROTTLE["fails"] = 0
            return json.load(req)
        except urllib.error.HTTPError as e:
            if e.code in (429, 403) and a < tries - 1:
                ra = e.headers.get("Retry-After")
                wait = int(ra) + 5 if ra and str(ra).isdigit() else 300 * (a + 1)
                print(f"  throttled ({e.code}), waiting {wait}s", flush=True)
                time.sleep(wait)
                continue
            THROTTLE["fails"] += 1
            raise


def api(p):
    return "https://en.wikipedia.org/w/api.php?" + urllib.parse.urlencode(p)


def plausible(qtitle, page):
    f = lambda s: (set(re.sub(r"[^a-z0-9 ]", "",
                              s.lower().split("(")[0]).split())
                   - {"the", "a", "an", "of", "and"})
    qk, pk = f(qtitle), f(page)
    return len(qk & pk) >= max(1, len(qk) - 1)


def main():
    if "--force" not in sys.argv:
        try:
            age = time.time() - os.path.getmtime(LAST_RUN)
            if age < MIN_INTERVAL:
                print(f"fetch ran {age / 3600:.1f}h ago, cooling down",
                      flush=True)
                return 0
        except OSError:
            pass
    open(LAST_RUN, "w").write(datetime.now().isoformat())
    if enrich_running():
        print("server enrich running, yielding (retry next sweep)",
              flush=True)
        return 0
    q = json.load(open(os.path.join(ROOT, "queue.json")))
    covdir = os.path.join(ROOT, "covers")
    os.makedirs(covdir, exist_ok=True)
    res_path = os.path.join(ROOT, "fetch-results.json")
    res = json.load(open(res_path)) if os.path.exists(res_path) else {}
    targets = [m for m in q["movies"]
               if not m["title"].startswith("Ghibli Batch")]
    targets += [{"title": s["title"],
                 "year": s["years"][0] if s["years"] else 0}
                for s in q["shows"]]
    for m in targets:
        key = f"{m['title']} ({m.get('year', 0)})"
        if res.get(key, {}).get("status") == "staged":
            continue
        t = clean(m["title"])
        print(f"TRY {t} ({m.get('year', 0)})", flush=True)
        try:
            s = f"{t} film {m.get('year', 0)}" if m.get("year") else f"{t} film"
            hits = [h["title"] for h in get(api(
                {"action": "query", "list": "search", "srsearch": s,
                 "format": "json", "srlimit": 5})
            ).get("query", {}).get("search", [])]
            got = None
            for h in hits:
                if not plausible(t, h):
                    continue
                wt = get(api({"action": "parse", "page": h, "prop": "wikitext",
                              "format": "json"}))["parse"]["wikitext"]["*"]
                im = re.search(r"\|\s*image\s*=\s*([^\n|]+)", wt)
                if not im:
                    continue
                fn = im.group(1).strip()
                ii = get(api({"action": "query", "titles": "File:" + fn,
                              "prop": "imageinfo", "iiprop": "url|size",
                              "format": "json"}))["query"]["pages"]
                for p in ii.values():
                    for info in p.get("imageinfo", []):
                        url = info["url"].split("?")[0]
                        if not url.lower().endswith(
                                (".jpg", ".jpeg", ".png", ".webp")):
                            continue
                        out = re.sub(r"\W+", "-", t.lower()).strip("-") + ".jpg"
                        urllib.request.urlretrieve(
                            url, os.path.join(covdir, out))
                        got = {"status": "staged", "page": h, "file": fn,
                               "cover": os.path.join(covdir, out),
                               "size": [info.get("width"),
                                        info.get("height")]}
                        break
                    if got:
                        break
                if got:
                    break
            res[key] = got or {"status": "no-match", "tried": hits}
            print(f"  -> {res[key]['status']} {res[key].get('page', '')}",
                  flush=True)
        except Exception as e:
            res[key] = {"status": "retry-later", "err": str(e)[:150]}
            print(f"  ERROR {e}", flush=True)
            if THROTTLE["fails"] >= 5:
                print("CIRCUIT-BREAKER: too many blocks, parking run",
                      flush=True)
                break
        json.dump(res, open(res_path, "w"), indent=1)
    print("FETCH-DONE", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())

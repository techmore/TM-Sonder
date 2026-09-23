#!/usr/bin/env python3
"""Fetch posters through ego-browser (space 16), ~1 title/minute.

Resumes fetch-results.json like queue_fetch.py. Writes staged covers into
m4b_work/poster-queue/covers/. Run in background; each title takes ~60s.
"""
import json
import os
import re
import subprocess
import sys
import time

ROOT = "/Users/seandolbec/Projects/TM-Sonder/m4b_work/poster-queue"
SPACE = 16
PACE = 60

EXTRA = [  # WG newcomer covers (books -> no film suffix)
    ("Distrust That Particular Flavor", 2012, "book"),
    ("Alien III", 2021, "audio drama"),
    ("All Tomorrow's Parties", 1999, "novel"),
]


def clean(t):
    t = re.sub(r"\[.*?\]", "", t)
    t = re.sub(r"MgB iNFo", "", t, flags=re.I)
    return re.sub(r"\s+", " ", t).strip(" -.")


def slug(t):
    return re.sub(r"\W+", "-", t.lower()).strip("-")


JS = """const task = await taskSpace(%d);
const page = task.page("p1");
const want = %s;
await page.goto("https://en.wikipedia.org/wiki/Special:Search?search=" + encodeURIComponent(want.q) + "&go=Go", { timeout: 30000 });
let state = await page.evaluate((want) => {
  const norm = (s) => (s || "").toLowerCase().replace(/[^a-z0-9 ]/g, " ").split(/\\s+/).filter(w => w && !["the","a","an","of","and","film","book","novel","audio","drama"].includes(w) && !/^\\d+$/.test(w));
  const qw = norm(want.title);
  const title = document.title.replace(/ - Wikipedia$/, "");
  const tw = norm(title.split("(")[0]);
  const overlap = qw.filter(w => tw.includes(w)).length;
  const ok = overlap >= Math.max(1, qw.length - 1);
  const img = document.querySelector("table.infobox img");
  let fileHref = null;
  if (img && img.closest("a")) fileHref = img.closest("a").href;
  const firstHit = document.querySelector(".mw-search-result-heading a");
  return { title, ok, fileHref, firstHit: firstHit ? firstHit.href : null };
}, want);
if (!state.fileHref && state.firstHit) {
  await page.goto(state.firstHit, { timeout: 30000 });
  await page.waitForLoadState({ timeout: 15000 }).catch(() => {});
  state = await page.evaluate((want) => {
    const norm = (s) => (s || "").toLowerCase().replace(/[^a-z0-9 ]/g, " ").split(/\\s+/).filter(w => w && !["the","a","an","of","and","film","book","novel","audio","drama"].includes(w) && !/^\\d+$/.test(w));
    const qw = norm(want.title);
    const title = document.title.replace(/ - Wikipedia$/, "");
    const tw = norm(title.split("(")[0]);
    const overlap = qw.filter(w => tw.includes(w)).length;
    const img = document.querySelector("table.infobox img");
    const href = img && img.closest("a") ? img.closest("a").href : null;
    return { title, ok: overlap >= Math.max(1, qw.length - 1) && !!href, fileHref: href, via: "search-fallback" };
  }, want);
}
console.log("STATE " + JSON.stringify(state));
if (state.ok && state.fileHref) {
  await page.goto(state.fileHref, { timeout: 30000 });
  const orig = await page.evaluate(() => {
    const a = [...document.querySelectorAll("a")].find(x => /Original file/.test(x.textContent));
    return a ? a.href : null;
  });
  console.log("ORIG " + orig);
  if (orig) {
    const r = await page.fetch(orig, { saveAs: %s, timeout: 60000 });
    console.log("SAVED ok=" + r.ok + " status=" + r.status);
  }
}
"""


def fetch_one(query, dest):
    if os.path.exists(dest):
        return "cached"
    js = JS % (SPACE, json.dumps({"title": query, "q": query}), json.dumps(dest))
    p = subprocess.run(["ego-browser", "nodejs", "-e", js],
                       capture_output=True, text=True, timeout=300)
    out = p.stdout + p.stderr
    m = re.search(r"^SAVED ok=(\w+) status=(\d+)", out, re.M)
    if m and m.group(1) == "true":
        return "staged"
    m2 = re.search(r"^STATE (\{.*\})", out, re.M)
    return "no-infobox" + ((" " + m2.group(1)[:120]) if m2 else "")


def main():
    q = json.load(open(os.path.join(ROOT, "queue.json")))
    res = json.load(open(os.path.join(ROOT, "fetch-results.json")))
    jobs = []
    for m in q["movies"]:
        if m["title"].startswith("Ghibli Batch"):
            continue
        t = clean(m["title"])
        key = f"{m['title']} ({m.get('year', 0)})"
        if res.get(key, {}).get("status") == "staged":
            continue
        kind = "film"
        qq = f"{t} {kind} {m.get('year', 0)}" if m.get("year") else f"{t} {kind}"
        jobs.append((key, qq, t, "movie"))
    for title, year, kind in EXTRA:
        key = f"WG:{title}"
        if res.get(key, {}).get("status") == "staged":
            continue
        jobs.append((key, f"{title} {kind}", title, "book"))
    print(f"ego-fetch jobs: {len(jobs)}", flush=True)
    for i, (key, query, title, kind) in enumerate(jobs):
        print(f"[{i + 1}/{len(jobs)}] TRY {query}", flush=True)
        dest = os.path.join(ROOT, "covers", slug(title) + "-ego.jpg")
        try:
            r = fetch_one(query, dest)
        except Exception as e:
            r = f"error {e}"[:150]
        print(f"  -> {r}", flush=True)
        if r == "staged":
            res[key] = {"status": "staged", "via": "ego-browser",
                        "cover": dest, "query": query}
        elif r == "cached":
            res[key] = {"status": "staged", "via": "ego-browser",
                        "cover": dest, "query": query}
        else:
            res[key] = {"status": "retry-later", "err": r,
                        "query": query}
        json.dump(res, open(os.path.join(ROOT, "fetch-results.json"), "w"),
                  indent=1)
        if i < len(jobs) - 1:
            time.sleep(PACE)
    print("EGO-FETCH-DONE", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())

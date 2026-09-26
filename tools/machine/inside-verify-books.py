import json, os, urllib.request, collections

base = "http://127.0.0.1:8097"
lib = json.loads(urllib.request.urlopen(f"{base}/api/library", timeout=60).read())
items = lib["items"]
print("total items:", len(items))
for k in ("audiobook", "movie", "tvShow", "ebook"):
    print(f"  {k:10} {sum(1 for i in items if i.get('kind') == k)}")

ab = [i for i in items if i.get("kind") == "audiobook"]
groups = collections.defaultdict(list)
for i in ab:
    if i.get("bookGroupID"):
        groups[i["bookGroupID"]].append(i)
print("\nbooks with >1 file:", sum(1 for g in groups.values() if len(g) > 1))
for g, parts in groups.items():
    if len(parts) < 2:
        continue
    parts.sort(key=lambda p: p.get("bookPartIndex") or 0)
    total = sum(p.get("durationSeconds") or 0 for p in parts)
    print(f"  {parts[0].get('title')!r}: {len(parts)} files, "
          f"book total {total:.1f}s, head part {parts[0].get('durationSeconds'):.1f}s")

# The audiobook browser endpoint already collapses into books.
cat = json.loads(urllib.request.urlopen(f"{base}/api/audiobooks", timeout=60).read())
rows = cat["items"]
print(f"\n/api/audiobooks rows: {len(rows)}")
for r in rows:
    if r.get("partCount"):
        print(f"  {r.get('title')!r}: partCount={r['partCount']} "
              f"duration={r.get('durationSeconds'):.1f}s "
              f"parts with progress="
              f"{[round(p.get('progressSeconds', 0), 1) for p in r.get('parts', [])]}")

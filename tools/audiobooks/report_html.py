#!/usr/bin/env python3
"""Build a self-contained HTML report for reviewing unattributed audiobooks.

Merges four read-only sources into one page you can actually work through:

  standardize-plan.json   the folder, its path, and why it was flagged
  tag_audit output        what the file's own container tags say
  author-curation.json    the curated attribution proposal and its reason
  ffprobe (live)          duration, size, bitrate, streams

Nothing is applied. The page exists so a human can decide, and exports the
decisions as a move plan for a later, explicit apply step.
"""

from __future__ import annotations

import argparse
import html
import json
import pathlib
import re
import subprocess
import sys
import unicodedata

FFPROBE = "/opt/homebrew/bin/ffprobe"


def probe(path: pathlib.Path) -> dict:
    try:
        out = subprocess.run(
            [FFPROBE, "-v", "quiet", "-print_format", "json",
             "-show_format", "-show_streams", str(path)],
            capture_output=True, timeout=300, check=True).stdout
        d = json.loads(out)
    except Exception as exc:  # noqa: BLE001 - report must not die on one file
        return {"error": str(exc)}
    fmt = d.get("format", {})
    audio = [s for s in d.get("streams", []) if s.get("codec_type") == "audio"]
    a0 = audio[0] if audio else {}
    tags = {k.lower().lstrip("©"): v for k, v in (fmt.get("tags") or {}).items()}
    return {
        "duration_seconds": round(float(fmt.get("duration", 0) or 0), 1),
        "bitrate": int(fmt.get("bit_rate", 0) or 0),
        "codec": a0.get("codec_name", ""),
        "channels": a0.get("channels", 0),
        "sample_rate": a0.get("sample_rate", ""),
        "narrator_comment": tags.get("comment", ""),
        "tag_artist": tags.get("artist", ""),
        "tag_album_artist": tags.get("album_artist", ""),
        "tag_album": tags.get("album", ""),
        "tag_date": tags.get("date", ""),
        "tag_genre": tags.get("genre", ""),
    }


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--plan", required=True, type=pathlib.Path)
    ap.add_argument("--curation", required=True, type=pathlib.Path)
    ap.add_argument("--out", required=True, type=pathlib.Path)
    ap.add_argument("--reason", default="unknown_author_folder")
    a = ap.parse_args()

    plan = json.loads(a.plan.read_text())
    curation = json.loads(a.curation.read_text())
    by_book: dict[str, dict] = {e["book"]: e for e in curation.get("entries", [])}

    rows = []
    for f in plan["folders"]:
        if a.reason not in f["reasons"]:
            continue
        cur = by_book.get(f["book"], {})
        entry = {
            "book": f["book"],
            "folder": f["relative_folder"],
            "author_folder": f["author"],
            "files": [],
            "proposed_author": cur.get("author", ""),
            "proposed_narrator": cur.get("narrator", ""),
            "confidence": cur.get("confidence", "none"),
            "needs_verification": bool(cur.get("needs-verification", True)),
            "reason": cur.get("reason", ""),
            "group": f["group"],
            "file_count": len(f["files"]),
        }
        for fe in f["files"]:
            p = pathlib.Path(fe["source"])
            entry["files"].append({
                "path": str(p),
                "size_bytes": fe["size_bytes"],
                "probe": probe(p),
            })
        rows.append(entry)

    rows.sort(key=lambda r: (r["confidence"] != "high", r["confidence"],
                             r["book"].lower()))

    by_conf: dict[str, int] = {}
    for r in rows:
        by_conf[r["confidence"]] = by_conf.get(r["confidence"], 0) + 1
    needs_verify = sum(1 for r in rows if r["needs_verification"])

    a.out.parent.mkdir(parents=True, exist_ok=True)
    a.out.write_text(render(rows, by_conf, needs_verify, a.reason))
    print(f"wrote {a.out}  ({len(rows)} folders)", file=sys.stderr)
    return 0


def badge(conf: str) -> str:
    return f'<span class="badge b-{html.escape(conf)}">{html.escape(conf)}</span>'


def render(rows: list[dict], by_conf: dict, needs_verify: int, reason: str) -> str:
    data = json.dumps(rows, ensure_ascii=False)
    trs = []
    for r in rows:
        files_html = []
        for f in r["files"]:
            pr = f["probe"]
            dur = pr.get("duration_seconds", 0) or 0
            dur_s = f"{int(dur // 3600)}:{int(dur % 3600 // 60):02d}:{int(dur % 60):02d}"
            size_mb = f["size_bytes"] / 1e6
            audio = f"{pr.get('codec','?')} {pr.get('channels','?')}ch " \
                    f"{pr.get('sample_rate','?')}Hz {pr.get('bitrate',0)//1000}kbps" \
                if not pr.get("error") else f"<span class=err>{html.escape(pr['error'])}</span>"
            tags = []
            for label, key in (("artist", "tag_artist"), ("album_artist", "tag_album_artist"),
                               ("album", "tag_album"), ("date", "tag_date"),
                               ("genre", "tag_genre"), ("comment", "narrator_comment")):
                v = pr.get(key) or ""
                if v:
                    tags.append(f"<span class=tag><b>{label}</b> {html.escape(str(v)[:120])}</span>")
            files_html.append(
                f"<div class=file><code class=path>{html.escape(f['path'])}</code>"
                f"<div class=meta>{size_mb:.1f} MB &middot; {dur_s} &middot; {audio}</div>"
                f"<div class=tags>{''.join(tags) or '<span class=dim>no container tags</span>'}</div>"
                f"</div>")
        author_display = (f'<b class=author>{html.escape(r["proposed_author"])}</b>'
                          if r["proposed_author"] else '<span class=unident>UNIDENTIFIED</span>')
        narr = (f'<div class=narr>{html.escape(r["proposed_narrator"])}</div>'
                if r["proposed_narrator"] else "")
        trs.append(f"""
<tr data-conf="{html.escape(r['confidence'])}" data-verify="{'1' if r['needs_verification'] else '0'}"
    data-search="{html.escape((r['book'] + ' ' + r['folder'] + ' ' + r['proposed_author']).lower())}">
  <td class=sel><input type=checkbox class=keep></td>
  <td class=book>
    <div class=title>{html.escape(r['book'])}</div>
    <div class=folder>{html.escape(r['folder'])}</div>
    {'<div class=grp>folder holds ' + str(r['file_count']) + ' files</div>' if r['file_count'] > 1 else ''}
  </td>
  <td>{author_display}{narr}{badge(r['confidence'])}
      {'<div class=verify>needs verification</div>' if r['needs_verification'] else ''}
      <div class=reason>{html.escape(r['reason'])}</div></td>
  <td class=files>{''.join(files_html)}</td>
</tr>""")

    conf_cards = "".join(
        f'<button class="chip" data-filter="conf" data-value="{html.escape(c)}">'
        f'{html.escape(c)} <b>{n}</b></button>'
        for c, n in sorted(by_conf.items()))

    return f"""<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<title>Unattributed audiobooks — review</title>
<style>
 :root {{ --bg:#12151a; --panel:#1a1f27; --line:#2a323d; --fg:#e6ebf2; --dim:#8b98a9;
          --acc:#5aa9e6; --good:#5ec27e; --warn:#e2a45a; --bad:#e2686b; }}
 * {{ box-sizing:border-box }}
 body {{ margin:0; background:var(--bg); color:var(--fg);
   font:14px/1.5 ui-sans-serif,-apple-system,"Segoe UI",Roboto,sans-serif }}
 header {{ position:sticky; top:0; z-index:10; background:var(--panel);
   border-bottom:1px solid var(--line); padding:14px 20px }}
 h1 {{ margin:0 0 4px; font-size:17px }}
 .sub {{ color:var(--dim); font-size:12.5px }}
 .bar {{ display:flex; gap:8px; flex-wrap:wrap; align-items:center; margin-top:10px }}
 input[type=search] {{ flex:1 1 260px; min-width:180px; padding:7px 10px; border-radius:7px;
   border:1px solid var(--line); background:#0e1116; color:var(--fg); font:inherit }}
 .chip {{ padding:6px 11px; border-radius:999px; border:1px solid var(--line);
   background:#0e1116; color:var(--dim); cursor:pointer; font:inherit; font-size:12.5px }}
 .chip b {{ color:var(--fg) }}
 .chip.on {{ border-color:var(--acc); color:var(--fg); background:#16222e }}
 .warnbtn {{ border-color:var(--warn) }} .warnbtn.on {{ background:#2a2113 }}
 main {{ padding:16px 20px 60px }}
 table {{ width:100%; border-collapse:collapse; align-items:top }}
 th {{ text-align:left; font-size:11.5px; text-transform:uppercase; letter-spacing:.5px;
   color:var(--dim); padding:8px 10px; border-bottom:1px solid var(--line) }}
 td {{ padding:11px 10px; border-bottom:1px solid var(--line); vertical-align:top }}
 tr:hover {{ background:#161b22 }}
 .sel {{ width:26px }} .book {{ width:24% }} td:nth-child(3) {{ width:22% }}
 .title {{ font-weight:600; font-size:14.5px }}
 .folder {{ color:var(--dim); font-family:ui-monospace,Menlo,monospace; font-size:11.5px;
   word-break:break-all; margin-top:2px }}
 .grp {{ color:var(--warn); font-size:11.5px; margin-top:3px }}
 .author {{ color:var(--good) }} .unident {{ color:var(--bad); font-weight:600 }}
 .narr {{ color:var(--dim); font-size:12.5px; margin-top:2px }}
 .reason {{ color:var(--dim); font-size:12px; margin-top:6px }}
 .verify {{ color:var(--warn); font-size:11.5px; margin-top:4px }}
 .badge {{ display:inline-block; padding:1px 8px; border-radius:999px; font-size:11px;
   border:1px solid var(--line); margin-top:6px }}
 .b-high {{ color:var(--good); border-color:var(--good) }}
 .b-medium {{ color:var(--warn); border-color:var(--warn) }}
 .b-low,.b-none {{ color:var(--bad); border-color:var(--bad) }}
 .file {{ border-left:2px solid var(--line); padding-left:9px; margin-bottom:9px }}
 .path {{ font-size:11px; color:var(--acc); word-break:break-all }}
 .meta {{ color:var(--dim); font-size:11.5px; margin-top:2px }}
 .tag {{ display:inline-block; background:#0e1116; border:1px solid var(--line);
   border-radius:5px; padding:1px 6px; margin:3px 4px 0 0; font-size:11px }}
 .tag b {{ color:var(--dim); font-weight:600 }}
 .dim {{ color:var(--dim) }} .err {{ color:var(--bad) }}
 footer {{ position:fixed; bottom:0; left:0; right:0; background:var(--panel);
   border-top:1px solid var(--line); padding:10px 20px; display:flex; gap:10px;
   align-items:center; font-size:12.5px }}
 button.act {{ padding:7px 13px; border-radius:7px; border:1px solid var(--line);
   background:#0e1116; color:var(--fg); cursor:pointer; font:inherit }}
 button.act.primary {{ border-color:var(--acc); color:var(--acc) }}
 #status {{ color:var(--dim) }}
 .note {{ max-width:900px; color:var(--dim); font-size:12.5px; margin:0 0 16px;
   border-left:2px solid var(--warn); padding-left:12px }}
</style></head><body>
<header>
  <h1>Unattributed audiobooks — review</h1>
  <div class="sub">reason <code>{html.escape(reason)}</code> &middot;
    {len(rows)} folders &middot; {needs_verify} need verification &middot;
    read-only: nothing is moved by this page</div>
  <div class="bar">
    <input type="search" id="q" placeholder="Search book, folder, or proposed author…  (press /)">
    {conf_cards}
    <button class="chip warnbtn" data-filter="verify" data-value="1">needs verification</button>
    <button class="chip" id="clear">clear filters</button>
  </div>
</header>
<main>
  <p class="note">The automated lookup was tried and rejected: it proposed
  <b>Stephen King</b> for <i>Cycle of the Werewolf</i> (Philip K. Dick) and
  &ldquo;Publius Vergilius Maro&rdquo; for <i>The Aeneid</i>, and a cross-check
  against the ebook library confirmed 2 of 45. Tick a row to accept its
  proposal, then export a move plan. Nothing is applied here.</p>
  <table>
    <thead><tr><th></th><th>Book folder</th><th>Proposal</th><th>File &amp; embedded tags</th></tr></thead>
    <tbody id="rows">{''.join(trs)}</tbody>
  </table>
</main>
<footer>
  <span id="status">no rows selected</span>
  <button class="act" id="all">select all visible</button>
  <button class="act primary" id="exp">export move plan</button>
  <button class="act" id="csv">copy as TSV</button>
</footer>
<script>
const ROWS = {data};
const state = {{ q:'', conf:null, verify:false }};
const body = document.getElementById('rows');
const q = document.getElementById('q');

function matches(tr){{
  if (state.q && !tr.dataset.search.includes(state.q)) return false;
  if (state.conf && tr.dataset.conf !== state.conf) return false;
  if (state.verify && tr.dataset.verify !== '1') return false;
  return true;
}}
function apply(){{
  let shown = 0;
  body.querySelectorAll('tr').forEach(tr=>{{
    const ok = matches(tr); tr.style.display = ok ? '' : 'none'; if (ok) shown++;
  }});
  document.getElementById('status').textContent =
    shown + ' shown \\u00b7 ' + body.querySelectorAll('.keep:checked').length + ' selected';
}}
q.addEventListener('input', ()=>{{ state.q = q.value.trim().toLowerCase(); apply(); }});
document.addEventListener('keydown', e=>{{
  if (e.key === '/' && document.activeElement !== q) {{ e.preventDefault(); q.focus(); }}
  if (e.key === 'Escape') {{ q.value=''; state.q=''; state.conf=null; state.verify=false;
    document.querySelectorAll('.chip').forEach(c=>c.classList.remove('on')); apply(); }}
}});
document.querySelectorAll('.chip[data-filter]').forEach(c=>{{
  c.addEventListener('click', ()=>{{
    const f=c.dataset.filter;
    if (f==='conf') {{ state.conf = state.conf===c.dataset.value ? null : c.dataset.value; }}
    else {{ state.verify = !state.verify; }}
    document.querySelectorAll('.chip[data-filter]').forEach(x=>
      x.classList.toggle('on', f==='conf' ? x.dataset.value===state.conf && state.conf
                                          : state.verify));
    apply();
  }});
}});
document.getElementById('clear').addEventListener('click', ()=>{{
  state.q=''; state.conf=null; state.verify=false; q.value='';
  document.querySelectorAll('.chip').forEach(c=>c.classList.remove('on')); apply();
}});
body.addEventListener('change', e=>{{ if (e.target.classList.contains('keep')) apply(); }});
document.getElementById('all').addEventListener('click', ()=>{{
  body.querySelectorAll('tr').forEach(tr=>{{
    if (tr.style.display !== 'none') tr.querySelector('.keep').checked = true; }});
  apply();
}});
document.getElementById('exp').addEventListener('click', ()=>{{
  const picked = [];
  body.querySelectorAll('tr').forEach(tr=>{{
    if (!tr.querySelector('.keep').checked) return;
    const i = [...body.querySelectorAll('tr')].indexOf(tr);
    const r = ROWS[i];
    if (r && r.proposed_author) picked.push({{
      book: r.book, folder: r.folder, from: r.author_folder,
      to: r.proposed_author, narrator: r.proposed_narrator,
      confidence: r.confidence, reason: r.reason }});
  }});
  const blob = new Blob([JSON.stringify({{
    policy: 'Review output only. Not applied. Move each folder from <from> to <to>.',
    generated: new Date().toISOString(), moves: picked }}, null, 2)],
    {{type:'application/json'}});
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob); a.download = 'author-move-plan.json'; a.click();
}});
document.getElementById('csv').addEventListener('click', async ()=>{{
  const out = [['book','from','to','narrator','confidence','path','duration_s','tags']]
    .concat(ROWS.map(r=>{{
      const f = r.files[0], p = f.probe;
      const tags = p.narrator_comment ? p.narrator_comment.slice(0,60) : '';
      return [r.book, r.author_folder, r.proposed_author, r.proposed_narrator,
              r.confidence, f.path, p.duration_seconds||0, tags];
    }}));
  const tsv = out.map(r=>r.map(c=>`"${{String(c).replace(/"/g,'""')}}"`).join('\\t')).join('\\n');
  await navigator.clipboard.writeText(tsv);
  const s=document.getElementById('status'); const old=s.textContent;
  s.textContent='TSV copied to clipboard'; setTimeout(()=>s.textContent=old,1600);
}});
apply();
</script></body></html>"""


if __name__ == "__main__":
    sys.exit(main())

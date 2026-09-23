#!/usr/bin/env python3
"""Refresh audit summary and exception queues from an inventory."""
import argparse,collections,json,pathlib

def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('inventory',type=pathlib.Path);ap.add_argument('--covers',type=pathlib.Path);ap.add_argument('--video-checks',type=pathlib.Path);a=ap.parse_args();base=a.inventory.parent;rows=json.loads(a.inventory.read_text());covers=json.loads(a.covers.read_text()) if a.covers else {}
    if a.video_checks:
        checks={}
        for line in a.video_checks.read_text().splitlines():
            try:r=json.loads(line);checks[r['path']]=r
            except (KeyError,ValueError):pass
        for row in rows:
            check=checks.get(row['path'])
            if check and not row.get('error'):
                if check.get('error') and 'video' not in row:
                    row['video_check_error']=check['error'];row['issues']=list(set(row.get('issues',[]))|{'video_check_failed'})
                elif check.get('error'):
                    row.pop('video_check_error',None)
                else:
                    row.pop('video_check_error',None);row['video']=check['video'];row['issues']=[i for i in row.get('issues',[]) if i not in ('non_cover_video_stream','video_check_failed')]
                    if row['video']:row['issues'].append('non_cover_video_stream')
        a.inventory.write_text(json.dumps(rows,indent=2))
        with (base/'inventory.jsonl').open('a') as f:
            for row in rows:f.write(json.dumps(row)+'\n')
    summary={'files':len(rows),'GiB':round(sum(r['size'] for r in rows)/2**30,2),'hours':round(sum(r.get('duration',0) for r in rows)/3600,2),'containers':dict(collections.Counter(r.get('container','ERROR') for r in rows)),'codecs':dict(collections.Counter(s['codec_name'] for r in rows for s in r.get('audio',[]))),'issues':dict(collections.Counter(i for r in rows for i in r.get('issues',[]))),'probe_errors':sum('error' in r for r in rows),'embedded_covers':sum(r.get('embedded_cover',False) for r in rows),'sidecar_covers':sum(bool(r.get('sidecar_covers')) for r in rows),'video_checked':sum('video' in r and not r.get('video_check_error') for r in rows)}
    queue=[{'source':r['path'],'title':r.get('tags',{}).get('title',pathlib.Path(r['path']).stem),'author':r.get('tags',{}).get('artist',pathlib.Path(r['path']).parent.parent.name),'status':'verified_cover_staged' if r['path'] in covers else 'unresolved','cover':covers.get(r['path'])} for r in rows if 'missing_cover' in r.get('issues',[])]
    summary['missing_cover_queue']=dict(collections.Counter(r['status'] for r in queue))
    (base/'summary.json').write_text(json.dumps(summary,indent=2));(base/'missing-covers.json').write_text(json.dumps(queue,indent=2));(base/'probe-errors.json').write_text(json.dumps([r for r in rows if 'error' in r or r.get('video_check_error')],indent=2));print(json.dumps(summary,indent=2))
if __name__=='__main__':main()

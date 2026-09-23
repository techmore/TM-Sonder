#!/usr/bin/env python3
"""Read-only, resumable audiobook container/codec/chapter/artwork inventory."""
import argparse, concurrent.futures, collections, json, os, pathlib, subprocess
AUDIO = {'.m4b','.m4a','.mp3','.opus','.ogg','.flac','.wav','.aac','.mp4','.wma','.aax','.aa'}
IMAGES = {'.jpg','.jpeg','.png','.webp','.gif'}
def probe(path):
    p=subprocess.run(['ffprobe','-v','error','-show_format','-show_streams','-show_chapters','-of','json',str(path)],capture_output=True,text=True,timeout=90)
    if p.returncode and 'Permission denied' in p.stderr:
        # Retry a transient NAS open failure only after a normal read succeeds.
        with pathlib.Path(path).open('rb') as f:f.read(1)
        p=subprocess.run(['ffprobe','-v','error','-show_format','-show_streams','-show_chapters','-of','json',str(path)],capture_output=True,text=True,timeout=90)
    if p.returncode: raise ValueError(p.stderr[-2000:])
    return json.loads(p.stdout)
def inspect(item):
    path,root=item; stat=path.stat()
    row={'path':str(path),'relative_path':str(path.relative_to(root)),'root':str(root),'size':stat.st_size,'mtime_ns':stat.st_mtime_ns}
    try:
        data=probe(path); fmt=data.get('format',{}); streams=data.get('streams',[])
        audio=[s for s in streams if s.get('codec_type')=='audio']
        row.update(container=fmt.get('format_name'),duration=float(fmt.get('duration',0)),tags=fmt.get('tags',{}),audio=[{k:s.get(k) for k in ['codec_name','profile','channels','sample_rate','bit_rate']} for s in audio],chapters=data.get('chapters',[]),embedded_cover=any(s.get('disposition',{}).get('attached_pic') for s in streams),sidecar_covers=sorted(str(p) for p in path.parent.iterdir() if p.suffix.lower() in IMAGES and not p.name.startswith('.')))
        row['video']=[{k:s.get(k) for k in ['codec_name','width','height','duration']} for s in streams if s.get('codec_type')=='video' and not s.get('disposition',{}).get('attached_pic')]
        row['issues']=[]
        if row['video']:row['issues'].append('non_cover_video_stream')
        if not audio or row['duration']<=0:row['issues'].append('invalid_audio')
        if path.suffix.lower()=='.m4b' and 'mp4' not in (row['container'] or '').split(','):row['issues'].append('m4b_container_mismatch')
        if not row['embedded_cover'] and not row['sidecar_covers']:row['issues'].append('missing_cover')
        if not row['chapters']:row['issues'].append('no_chapters')
    except Exception as e:row['error']=str(e)
    return row

def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('roots',nargs='+',type=pathlib.Path);ap.add_argument('--out',required=True,type=pathlib.Path);ap.add_argument('--workers',type=int,default=4);args=ap.parse_args()
    args.out.mkdir(parents=True,exist_ok=True); cache=args.out/'inventory.jsonl'; prior={}
    if cache.exists():
        for line in cache.read_text().splitlines():
            try:r=json.loads(line);prior[r['path']]=r
            except (ValueError,KeyError):pass
    items=[]; rows=[]; errors=[]; seen=set()
    for root in args.roots:
        root=root.resolve()
        if not root.is_dir():raise SystemExit(f'Root unavailable: {root}')
        walked=0
        for directory,dirs,files in os.walk(root,onerror=lambda e:errors.append(str(e))):
            walked+=1
            if walked%250==0:print(f'Enumerated {walked} directories under {root}',flush=True)
            dirs[:]=[d for d in dirs if not d.startswith('.')]
            for name in files:
                p=pathlib.Path(directory)/name
                if name.startswith('.') or p.suffix.lower() not in AUDIO or str(p) in seen:continue
                seen.add(str(p));s=p.stat();old=prior.get(str(p))
                if old and 'video' in old and not old.get('error') and old['size']==s.st_size and old['mtime_ns']==s.st_mtime_ns:
                    old['sidecar_covers']=sorted(str(x) for x in p.parent.iterdir() if x.suffix.lower() in IMAGES and not x.name.startswith('.'))
                    old['issues']=[x for x in old['issues'] if x!='missing_cover']
                    if not old['embedded_cover'] and not old['sidecar_covers']:old['issues'].append('missing_cover')
                    rows.append(old)
                else:items.append((p,root))
    print(f'{len(items)} files to probe, {len(rows)} cached',flush=True)
    with cache.open('a') as f, concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as pool:
        for i,row in enumerate(pool.map(inspect,items),1):
            f.write(json.dumps(row)+'\n');f.flush();rows.append(row)
            if i%25==0:print(f'Probed {i}/{len(items)}',flush=True)
    rows.sort(key=lambda r:r['path']);(args.out/'inventory.json').write_text(json.dumps(rows,indent=2))
    summary={'files':len(rows),'GiB':round(sum(r['size'] for r in rows)/2**30,2),'hours':round(sum(r.get('duration',0) for r in rows)/3600,2),'containers':dict(collections.Counter(r.get('container','ERROR') for r in rows)),'codecs':dict(collections.Counter(a['codec_name'] for r in rows for a in r.get('audio',[]))),'issues':dict(collections.Counter(i for r in rows for i in r.get('issues',[]))),'probe_errors':sum('error' in r for r in rows),'walk_errors':errors,'embedded_covers':sum(r.get('embedded_cover',False) for r in rows),'sidecar_covers':sum(bool(r.get('sidecar_covers')) for r in rows)}
    (args.out/'summary.json').write_text(json.dumps(summary,indent=2));print(json.dumps(summary,indent=2))
if __name__=='__main__':main()

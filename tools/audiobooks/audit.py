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

def migration_plan(rows):
    """Attach non-destructive canonical-path and review metadata.

    This is deliberately a plan, not a move operation. The standard target is
    Author/Book/file; wrapper directories and working files are quarantined
    from the active library but remain in the inventory for rollback/review.
    """
    grouped = collections.defaultdict(list)
    for row in rows:
        rel = pathlib.PurePosixPath(str(row.get('relative_path', '')).replace('\\', '/'))
        parts = [p for p in rel.parts if p not in ('', '.', '..')]
        lower_parts = [p.lower() for p in parts]
        filename = parts[-1] if parts else ''
        book_parts = parts[-3:-1] if len(parts) >= 3 else ([parts[-2]] if len(parts) == 2 else [])
        author = book_parts[0] if len(book_parts) == 2 else ''
        book = book_parts[-1] if book_parts else ''
        book_key = '/'.join(p for p in book_parts if p).lower() or filename.lower()
        grouped[book_key].append(row)
        lower_name = filename.lower()
        reason = None
        if any(p.startswith('_inbox') or p.startswith('test-') or p in {'m4b forge compact'} or p.startswith('compact-m4b-') for p in lower_parts[:-1]):
            reason = 'inbox_or_wrapper'
        elif any(token in lower_name for token in ('.sonder-retag.', '.wcqr')) or lower_name.endswith(('.partial', '.tmp', '.download')):
            reason = 'working_file'
        if author and book:
            canonical = f'{author}/{book}/{filename}'
        elif book:
            canonical = f'{book}/{filename}'
        else:
            canonical = filename
        row['book_key'] = book_key
        row['canonical_relative_path'] = canonical
        row['canonical_path'] = str(pathlib.Path(row.get('root', '.')) / pathlib.Path(*canonical.split('/')))
        row['migration_action'] = 'quarantine' if reason else 'pending'
        row['migration_reason'] = reason or ''
    for book_key, book_rows in grouped.items():
        for row in book_rows:
            row['book_file_count'] = len(book_rows)
            if row['migration_action'] == 'pending' and len(book_rows) > 1:
                row['migration_action'] = 'review_multipart'
                row['migration_reason'] = 'multiple_files_in_book_folder'
    return grouped

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
    migration_plan(rows)
    rows.sort(key=lambda r:r['path']);(args.out/'inventory.json').write_text(json.dumps(rows,indent=2))
    manifest = {'version': 1, 'target_layout': 'Author/Book/file', 'rows': rows}
    (args.out/'migration-manifest.json').write_text(json.dumps(manifest, indent=2))
    summary={'files':len(rows),'GiB':round(sum(r['size'] for r in rows)/2**30,2),'hours':round(sum(r.get('duration',0) for r in rows)/3600,2),'containers':dict(collections.Counter(r.get('container','ERROR') for r in rows)),'codecs':dict(collections.Counter(a['codec_name'] for r in rows for a in r.get('audio',[]))),'issues':dict(collections.Counter(i for r in rows for i in r.get('issues',[]))),'migration_actions':dict(collections.Counter(r.get('migration_action') for r in rows)),'multipart_books':sum(1 for n in {r.get('book_key') for r in rows} if n and sum(1 for x in rows if x.get('book_key') == n) > 1),'probe_errors':sum('error' in r for r in rows),'walk_errors':errors,'embedded_covers':sum(r.get('embedded_cover',False) for r in rows),'sidecar_covers':sum(bool(r.get('sidecar_covers')) for r in rows)}
    (args.out/'summary.json').write_text(json.dumps(summary,indent=2));print(json.dumps(summary,indent=2))
if __name__=='__main__':main()

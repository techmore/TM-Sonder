#!/usr/bin/env python3
"""Run a bounded staging batch from a reviewed plan; dry-run by default."""
import argparse,json,pathlib,shutil,time
from convert import convert,sha256

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('plan',type=pathlib.Path);p.add_argument('--output-root',type=pathlib.Path,required=True);p.add_argument('--limit',type=int,default=1);p.add_argument('--execute',action='store_true');args=p.parse_args()
    if args.limit<1:raise SystemExit('--limit must be positive')
    jobs=[j for j in json.loads(args.plan.read_text())['jobs'] if j['action']=='pilot_candidate'];output=args.output_root.resolve();completed=0
    if args.execute:output.mkdir(parents=True,exist_ok=True)
    for job in jobs:
        source=pathlib.Path(job['source']);root=pathlib.Path(job['conversion_root']);target=output/source.relative_to(root).with_suffix('.m4b');receipt=target.with_suffix('.m4b.receipt.json')
        cover=job.get('verified_cover')
        expected_cover=str(pathlib.Path(cover).resolve()) if cover else 'source embedded image'
        if target.exists():
            if receipt.exists():
                previous=json.loads(receipt.read_text());stat=source.stat()
                if previous.get('source')==str(source.resolve()) and previous.get('source_size')==stat.st_size and previous.get('source_mtime_ns')==stat.st_mtime_ns and previous.get('output_size')==target.stat().st_size and previous.get('bitrate_kbps')==job['target_kbps'] and previous.get('cover_source')==expected_cover:
                    if not args.execute or previous.get('output_sha256')==sha256(target):continue
            raise SystemExit(f'Existing output does not have a matching verified receipt: {target}')
        cover=job.get('verified_cover')
        if not job.get('embedded_cover') and not cover:
            print(json.dumps({'source':str(source),'skipped':'Needs a visually verified sidecar cover mapping'}));continue
        print(json.dumps({'source':str(source),'target':str(target),'bitrate_kbps':job['target_kbps'],'execute':args.execute}),flush=True)
        if args.execute:
            needed=max(job['source_bytes']*2,job['estimated_output_bytes']*3)+128*1024**2
            if shutil.disk_usage(output).free<needed:raise SystemExit(f'Insufficient staging space; need at least {needed} free bytes')
            record={'source':str(source),'started_at':time.time()}
            try:
                r=convert(source,root,output,job['target_kbps'],pathlib.Path(cover) if cover else None);record.update(status='verified_staged',output=r['output'])
            except Exception as e:
                record.update(status='failed',error=str(e))
                with (output/'batch.jsonl').open('a') as f:f.write(json.dumps(record)+'\n')
                raise
            with (output/'batch.jsonl').open('a') as f:f.write(json.dumps(record)+'\n')
        completed+=1
        if completed>=args.limit:break
    print(json.dumps({'selected':completed,'execute':args.execute}))
if __name__=='__main__':main()

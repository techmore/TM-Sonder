#!/usr/bin/env python3
"""Supplement an older audit with non-cover video detection; resumable."""
import argparse,concurrent.futures,json,pathlib,subprocess

def check(row):
    command=['ffprobe','-v','error','-analyzeduration','1','-probesize','32','-fpsprobesize','0','-show_entries','stream=codec_type,codec_name,width,height,duration:stream_disposition=attached_pic','-of','json',row['path']]
    try:
        p=subprocess.run(command,capture_output=True,text=True,timeout=90)
        if p.returncode and 'Permission denied' in p.stderr:
            with open(row['path'],'rb') as f:f.read(1)
            p=subprocess.run(command,capture_output=True,text=True,timeout=90)
        if p.returncode:raise ValueError(p.stderr[-1000:])
        streams=json.loads(p.stdout)['streams']
        return {'path':row['path'],'video':[{k:s.get(k) for k in ['codec_name','width','height','duration']} for s in streams if s.get('codec_type')=='video' and not s.get('disposition',{}).get('attached_pic')]}
    except Exception as e:return {'path':row['path'],'error':str(e)}

def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('inventory',type=pathlib.Path);ap.add_argument('--out',type=pathlib.Path,required=True);ap.add_argument('--workers',type=int,default=8);args=ap.parse_args();done={}
    if args.out.exists():
        for line in args.out.read_text().splitlines():
            try:r=json.loads(line);done[r['path']]=r
            except (ValueError,KeyError):pass
    rows=[r for r in json.loads(args.inventory.read_text()) if not r.get('error') and 'video' not in r and ('error' in done.get(r['path'],{}) or r['path'] not in done)]
    print(f'{len(rows)} stream checks remaining',flush=True)
    with args.out.open('a') as f,concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as pool:
        futures=[pool.submit(check,row) for row in rows]
        for i,future in enumerate(concurrent.futures.as_completed(futures),1):
            r=future.result()
            f.write(json.dumps(r)+'\n');f.flush()
            if i%100==0:print('Checked',i,flush=True)
if __name__=='__main__':main()

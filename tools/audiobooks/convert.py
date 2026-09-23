#!/usr/bin/env python3
"""Stage one audiobook as Opus/MP4 .m4b; never replace or delete originals."""
import argparse, hashlib, json, os, pathlib, re, subprocess, tempfile
from audit import probe

def run(cmd):
    p=subprocess.run(cmd,capture_output=True,text=True)
    if p.returncode:raise RuntimeError(p.stderr[-4000:])
    return p

def sha256(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for chunk in iter(lambda:f.read(1024*1024),b''):h.update(chunk)
    return h.hexdigest()

def convert(source, root, output, bitrate, cover=None):
    source=source.resolve();root=root.resolve();output=output.resolve()
    if output==root or root in output.parents:raise ValueError('Output must be outside the source library')
    relative=source.relative_to(root).with_suffix('.m4b');target=output/relative
    if target.exists() or target.with_suffix('.m4b.receipt.json').exists():raise FileExistsError(f'Output or receipt exists: {target}')
    before=source.stat();original=probe(source);audios=[s for s in original['streams'] if s['codec_type']=='audio']
    if len(audios)!=1:raise ValueError('Expected exactly one audio stream; review manually')
    if int(audios[0]['channels'])>2:raise ValueError('Multichannel source needs an explicit channel policy')
    if audios[0]['codec_name']=='opus':raise ValueError('Already Opus; avoid another lossy encode')
    if any(s.get('codec_type')=='video' and not s.get('disposition',{}).get('attached_pic') for s in original['streams']):raise ValueError('Non-cover video present; review before removing video')
    duration=float(original['format']['duration'])
    embedded=next((s['index'] for s in original['streams'] if s.get('disposition',{}).get('attached_pic')),None)
    if cover:
        cover=cover.resolve()
        if not cover.is_file():raise ValueError('Cover is not a file')
    if embedded is None and cover is None:raise ValueError('Cover required: supply a verified image with --cover')
    target.parent.mkdir(parents=True,exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='.opus-stage-',dir=target.parent) as tmp:
        tmp=pathlib.Path(tmp);candidate=tmp/'book.m4b'
        audio_only=tmp/'audio.m4b'
        command=['ffmpeg','-hide_banner','-v','error','-nostdin','-n','-i',str(source),'-map','0:a:0','-map_metadata','0','-map_chapters','0','-vn','-c:a','libopus','-b:a',f'{bitrate}k','-vbr','on','-application','audio','-compression_level','10','-f','mp4',str(audio_only)]
        run(command)
        # Encode separately: some FFmpeg builds terminate audio early when
        # an attached picture is mapped during encoding.
        image=tmp/'cover.jpg'
        run(['ffmpeg','-hide_banner','-v','error','-nostdin','-n','-i',str(cover or source),'-map',('0:v:0' if cover else f'0:{embedded}'),'-frames:v','1',str(image)])
        remux=['ffmpeg','-hide_banner','-v','error','-nostdin','-n','-i',str(audio_only),'-i',str(image),'-map','0:a:0','-map','1:v:0','-map_metadata','0','-map_chapters','0','-c','copy','-disposition:v:0','attached_pic','-f','mp4','-movflags','+faststart',str(candidate)]
        run(remux)
        actual=probe(candidate);streams=actual['streams'];audio=[s for s in streams if s['codec_type']=='audio']
        if len(audio)!=1 or audio[0]['codec_name']!='opus' or 'mp4' not in actual['format']['format_name'].split(','):raise ValueError('Output codec/container verification failed')
        if int(audio[0]['channels'])!=int(audios[0]['channels']):raise ValueError('Channel count changed')
        if abs(float(actual['format']['duration'])-duration)>0.25:raise ValueError('Duration changed by more than 250 ms')
        source_audio_duration=float(audios[0].get('duration',duration))
        if abs(float(audio[0].get('duration',0))-source_audio_duration)>0.25:raise ValueError('Audio stream duration changed by more than 250 ms')
        if not any(s.get('disposition',{}).get('attached_pic') for s in streams):raise ValueError('Output cover missing')
        oldchap=original.get('chapters',[]);newchap=actual.get('chapters',[])
        if len(oldchap)!=len(newchap):raise ValueError('Chapter count changed')
        for old,new in zip(oldchap,newchap):
            if old.get('tags',{}).get('title')!=new.get('tags',{}).get('title'):raise ValueError('Chapter title changed')
            keys=('start_time','end_time')
            if abs(float(old['end_time'])-float(old['start_time']))<0.01:
                # Zero-length trailing chapters get their end stretched to
                # EOF by the MP4 muxer; compare start only.
                keys=('start_time',)
            drift=max(abs(float(old[k])-float(new[k])) for k in keys)
            if drift>1.0:raise ValueError(f'Chapter timing changed (max drift {drift:.2f}s)')
        oldtags={k.lower():v for k,v in original['format'].get('tags',{}).items()};newtags={k.lower():v for k,v in actual['format'].get('tags',{}).items()}
        def norm(k,v):
            # Container remux normalizes track/disc ("01/01" -> "1"); compare
            # the leading track number only.
            if k in ('track','disc'):
                m=re.match(r'\d+',str(v or ''))
                return m.group(0).lstrip('0') or '0' if m else ''
            return v
        for k in ('title','artist','album','album_artist','composer','date','genre','comment','track','disc'):
            if k in oldtags and norm(k,newtags.get(k))!=norm(k,oldtags[k]):raise ValueError(f'Metadata field changed: {k}')
        run(['ffmpeg','-hide_banner','-v','error','-xerror','-nostdin','-i',str(candidate),'-map','0:a:0','-f','null','-'])
        after=source.stat()
        if (before.st_size,before.st_mtime_ns)!=(after.st_size,after.st_mtime_ns):raise ValueError('Source changed during conversion')
        receipt={'source':str(source),'output':str(target),'source_size':before.st_size,'output_size':candidate.stat().st_size,'source_mtime_ns':before.st_mtime_ns,'source_sha256':sha256(source),'output_sha256':sha256(candidate),'bitrate_kbps':bitrate,'duration':duration,'chapters':len(newchap),'embedded_cover':True,'cover_source':str(cover) if cover else 'source embedded image','full_audio_decode':'passed','command':command,'remux_command':remux,'playback_acceptance':'pending web/iOS device testing'}
        # Hard-link publication refuses an existing destination, including concurrent runs.
        os.link(candidate,target)
        receipt_path=target.with_suffix('.m4b.receipt.json')
        with receipt_path.open('x') as f:json.dump(receipt,f,indent=2)
    return receipt

def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('source',type=pathlib.Path);ap.add_argument('--root',type=pathlib.Path,required=True);ap.add_argument('--output-root',type=pathlib.Path,required=True);ap.add_argument('--bitrate',type=int,choices=[24,32,40,48,64],default=32);ap.add_argument('--cover',type=pathlib.Path);args=ap.parse_args()
    print(json.dumps(convert(args.source,args.root,args.output_root,args.bitrate,args.cover),indent=2))
if __name__=='__main__':main()

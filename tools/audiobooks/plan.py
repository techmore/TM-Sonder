#!/usr/bin/env python3
"""Build a reviewable, non-executing Opus migration manifest from audit JSON."""
import argparse,collections,json,pathlib

def build(rows, covers=None):
    covers=covers or {}
    jobs=[]
    for r in rows:
        a=r.get('audio',[]);duration=r.get('duration',0);channels=a[0].get('channels') if len(a)==1 else None
        target=32 if channels==1 else 48
        issues=list(r.get('issues',[]))
        verified_cover=covers.get(r['path'])
        if verified_cover and pathlib.Path(verified_cover).is_file():issues=[i for i in issues if i!='missing_cover']
        else:verified_cover=None
        if r.get('error') or any(i in issues for i in ['non_cover_video_stream','video_check_failed']) or len(a)!=1 or not duration or channels not in (1,2):action='manual_review'
        elif pathlib.Path(r['path']).suffix.lower()!='.m4b':action='review_media_kind_and_book_membership'
        elif a[0]['codec_name']=='opus':action='keep_existing_opus'
        elif r['size']<=duration*target*1000/8*1.25:action='keep_low_bitrate_source'
        elif 'missing_cover' in issues:action='resolve_cover_before_conversion'
        elif 'no_chapters' in issues:action='review_chapters_before_conversion'
        else:action='pilot_candidate'
        jobs.append({'source':r['path'],'source_root':r.get('root',str(pathlib.Path(r['path']).parent)),'relative_path':r['relative_path'],'action':action,'issues':issues,'codec':a[0]['codec_name'] if len(a)==1 else None,'source_bytes':r['size'],'duration_seconds':duration,'target_kbps':target,'estimated_output_bytes':round(duration*target*1000/8),'embedded_cover':r.get('embedded_cover',False),'verified_cover':verified_cover,'sidecar_candidates':r.get('sidecar_covers',[]),'chapter_count':len(r.get('chapters',[]))})
    groups=collections.defaultdict(list)
    for job in jobs:
        parts=pathlib.Path(job['relative_path']).parts
        job['conversion_root']=job['source_root']
        if parts[:2]==('M4B Forge Compact','compact-m4b-80k'):
            parts=parts[2:]
            job['conversion_root']=str(pathlib.Path(job['source_root'])/'M4B Forge Compact'/'compact-m4b-80k')
        job['proposed_relative_path']=str(pathlib.Path(*parts))
        groups[str(pathlib.Path(*parts).parent)].append(job)
    for folder,group in groups.items():
        if len(group)>1:
            for job in group:
                job['issues'].append('multiple_files_or_versions_in_book_folder')
                if job['action']!='manual_review':job['action']='review_book_membership_and_versions'
    return {'policy':'32 kb/s mono, 48 kb/s stereo trial settings; retain originals; no production replacement until listening and player acceptance. Estimates exclude artwork/container overhead and VBR variation. File-level inventory does not establish book identity or completeness.','counts':dict(collections.Counter(j['action'] for j in jobs)),'jobs':jobs}

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('inventory',nargs='+',type=pathlib.Path)
    p.add_argument('--out',required=True,type=pathlib.Path)
    p.add_argument('--covers',type=pathlib.Path,help='JSON map: exact source path to visually verified cover path')
    p.add_argument('--overrides',type=pathlib.Path,help='JSON map: exact source path to {action, reason} manual-review override')
    a=p.parse_args()
    rows=[r for path in a.inventory for r in json.loads(path.read_text())]
    result=build(rows,json.loads(a.covers.read_text()) if a.covers else None)
    if a.overrides:
        overrides=json.loads(a.overrides.read_text())
        for job in result['jobs']:
            override=overrides.get(job['source'])
            if not override:
                continue
            action=override.get('action')
            reason=override.get('reason')
            if not action or not reason:
                raise SystemExit(f"Invalid override for {job['source']}: action and reason are required")
            job['action']=action
            job['manual_review_reason']=reason
        result['counts']=dict(collections.Counter(j['action'] for j in result['jobs']))
    a.out.parent.mkdir(parents=True,exist_ok=True)
    a.out.write_text(json.dumps(result,indent=2))
    print(json.dumps(result['counts'],indent=2))
if __name__=='__main__':main()

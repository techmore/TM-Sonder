#!/usr/bin/env python3
"""Fast non-m4b audiobook scan. Lists dirs fast, stats only non-m4b audio candidates."""
import argparse, collections, json, os, pathlib

AUDIO_NONM4B = {'.m4a', '.mp3', '.opus', '.ogg', '.oga', '.flac', '.wav',
                '.aac', '.mp4', '.wma', '.aax', '.aa', '.mka', '.aiff', '.aif'}

def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('root', type=pathlib.Path)
    ap.add_argument('--out', required=True, type=pathlib.Path)
    args = ap.parse_args()
    root = args.root.resolve()
    counts = collections.Counter()
    non_m4b = []
    n_files = 0
    n_dirs = 0
    stack = [str(root)]
    while stack:
        d = stack.pop()
        n_dirs += 1
        if n_dirs % 500 == 0:
            print(f'dirs={n_dirs} files={n_files} nonm4b={len(non_m4b)} @ {d}', flush=True)
        try:
            with os.scandir(d) as it:
                entries = list(it)
        except OSError as e:
            print(f'WALK-ERR {d}: {e}', flush=True)
            continue
        for e in entries:
            name = e.name
            if name.startswith('.') or name.startswith('._'):
                continue
            try:
                if e.is_dir(follow_symlinks=False):
                    stack.append(e.path)
                elif e.is_file(follow_symlinks=False):
                    n_files += 1
                    ext = pathlib.Path(name).suffix.lower()
                    counts[ext] += 1
                    if ext in AUDIO_NONM4B:
                        try:
                            sz = e.stat(follow_symlinks=False).st_size
                        except OSError:
                            sz = -1
                        non_m4b.append({'path': e.path, 'ext': ext, 'size': sz})
            except OSError as ex:
                print(f'ENTRY-ERR {d}/{name}: {ex}', flush=True)
    non_m4b.sort(key=lambda r: r['path'])
    result = {'root': str(root), 'total_files': n_files, 'total_dirs': n_dirs,
              'counts': dict(counts), 'non_m4b': non_m4b,
              'non_m4b_count': len(non_m4b),
              'non_m4b_bytes': sum(r['size'] for r in non_m4b if r['size'] > 0)}
    args.out.write_text(json.dumps(result, indent=2))
    print(f'DIRS={n_dirs} FILES={n_files}', flush=True)
    print('EXT_COUNTS:' + json.dumps(dict(counts)), flush=True)
    print(f'NON_M4B_COUNT={len(non_m4b)} BYTES={result["non_m4b_bytes"]}', flush=True)
    print(f'wrote {args.out}', flush=True)
    for r in non_m4b[:200]:
        print(f'{r["path"]} [{r["ext"]}] {r["size"]}', flush=True)

if __name__ == '__main__':
    main()

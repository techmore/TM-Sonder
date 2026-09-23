// Stage cover candidates from explicit Wikipedia season, film and book pages.
// Usage: node tools/wiki-portrait-audit.mjs AUDIT.json OUTPUT_DIR tvShow|movie|all
// Never writes the source media folders or activates candidates automatically.
import fs from 'node:fs/promises';
import path from 'node:path';
import crypto from 'node:crypto';

const [auditPath, output, kind = 'tvShow'] = process.argv.slice(2);
if (!auditPath || !output) throw new Error('Audit JSON and output directory required');
const audit = JSON.parse(await fs.readFile(auditPath, 'utf8'));
await fs.mkdir(output, { recursive: true });
const targets = new Map();
for (const row of audit) {
  if (!row.reason || (!row.targetID && !row.path.includes('/NAS/'))) continue;
  let folder = row.folder || path.basename(path.dirname(row.path));
  if (/^season/i.test(folder)) folder = path.basename(path.dirname(path.dirname(row.path)));
  if (!targets.has(row.targetID || folder)) targets.set(row.targetID || folder, {...row,folder});
}
const headers = { 'User-Agent': 'SonderArtworkAudit/1.0 (personal media library; conservative sequential metadata lookup)' };
let nextRequest = 0;
async function query(params) {
  await new Promise(resolve=>setTimeout(resolve,Math.max(0,nextRequest-Date.now())));
  nextRequest=Date.now()+6000;
  const url = new URL('https://en.wikipedia.org/w/api.php');
  url.search = new URLSearchParams({ action: 'query', format: 'json', ...params });
  const response = await fetch(url, { headers, signal: AbortSignal.timeout(15000) });
  if (!response.ok) throw new Error(`Wikipedia ${response.status}`);
  const result = await response.json();
  if (result.error) throw new Error(result.error.info);
  return result;
}
const report = JSON.parse(await fs.readFile(path.join(output,'report.json'),'utf8').catch(error=> {
  if(error.code==='ENOENT')return '[]';throw error;
}));
for (const [targetID, row] of targets) {
  const folder=row.folder;
  const mediaKind=row.kind || kind;
  const previous=report.findIndex(r=>(r.targetID || r.folder)===targetID);
  if (previous>=0 && (report[previous].filename || report[previous].status==='No qualifying portrait found')) continue;
  if (previous>=0) report.splice(previous,1);
  const title = (row.title || folder).replace(/\s*\(\d{4}\)\s*$/, '').trim();
  const year = row.year || folder.match(/\((\d{4})\)$/)?.[1];
  // Season pages provide show-specific key art instead of main-page logos.
  const isBook=mediaKind==='ebook'||mediaKind==='audiobook';
  const pages = mediaKind === 'tvShow' ? [`${title} (season 1)`, `${title} (series 1)`]
    : isBook ? [`${title} (novel)`,`${title} (book)`]
    : year ? [`${title} (${year} film)`, `${title} (film)`] : [`${title} (film)`];
  let best = null;
  try {
    for (const pageTitle of pages) {
      const data = await query({ generator: 'images', titles: pageTitle, redirects: '1', gimlimit: '50', prop: 'imageinfo', iiprop: 'url|size|extmetadata' });
      const images = Object.values(data.query?.pages || {}).filter(p => {
        const info = p.imageinfo?.[0];
        if (!info || !/\.(png|jpe?g)$/i.test(p.title)) return false;
        const ratio = info.width / info.height;
        if (ratio < .5 || ratio > (mediaKind==='audiobook'?1.1:.85) || info.width < 200 || info.height < (mediaKind==='audiobook'?200:300)) return false;
        const norm = s => s.toLowerCase().replace(/[^a-z0-9]/g, '');
        if (!norm(p.title).includes(norm(title))) return false;
        return /poster|season|series|dvd|cover|novel|book/i.test(p.title) && !/comic.con|cast|actor|actress|premiere|writer|director/i.test(p.title);
      });
      if (!images.length) continue;
      images.sort((a,b) => Math.abs(a.imageinfo[0].width/a.imageinfo[0].height-2/3)-Math.abs(b.imageinfo[0].width/b.imageinfo[0].height-2/3));
      const chosen=images[0],info=chosen.imageinfo[0];
      const remote=new URL(info.url);
      if (remote.protocol!=='https:' || remote.hostname!=='upload.wikimedia.org') continue;
      const response=await fetch(remote,{headers,signal:AbortSignal.timeout(15000)});
      if (!response.ok) continue;
      if (Number(response.headers.get('content-length')||0)>10*1024*1024) continue;
      const reader=response.body.getReader();const chunks=[];let size=0;
      while(true) { const {done,value}=await reader.read();if(done)break;size+=value.length;if(size>10*1024*1024){await reader.cancel();throw new Error('Image exceeds size limit');}chunks.push(value); }
      const bytes=Buffer.concat(chunks);
      const filename=crypto.createHash('sha256').update(bytes).digest('hex')+path.extname(remote.pathname).toLowerCase();
      await fs.writeFile(path.join(output,filename),bytes,{flag:'wx'}).catch(e=>{if(e.code!=='EEXIST')throw e;});
      best={folder,targetID,kind:mediaKind,ids:row.ids||[],author:row.author||'',expectedYear:year||null,originalPath:row.path,filename,width:info.width,height:info.height,
        article:`https://en.wikipedia.org/wiki/${encodeURIComponent(pageTitle.replaceAll(' ','_'))}`,
        sourcePage:info.descriptionurl,imageURL:info.url,license:info.extmetadata?.LicenseShortName?.value||'Unknown',
        licenseURL:info.extmetadata?.LicenseUrl?.value||'',credit:info.extmetadata?.Credit?.value||'',artist:info.extmetadata?.Artist?.value||'',
        note:mediaKind==='tvShow'?'Season 1 artwork used as series cover; not an all-seasons poster':isBook?'Book cover candidate; author and edition require verification':'Film poster candidate; release year requires verification',review:'pending visual review'};
      break;
    }
    report.push(best||{folder,targetID,kind:mediaKind,status:'No qualifying portrait found'});
  } catch(error) {
    report.push({folder,targetID,kind:mediaKind,status:String(error)});
    if (/429/.test(String(error))) {
      await fs.writeFile(path.join(output,'report.json'),JSON.stringify(report,null,2));
      console.error('Rate limited: stopped safely. Retry this resumable audit later.');
      break;
    }
  }
  await fs.writeFile(path.join(output,'report.json'),JSON.stringify(report,null,2));
  console.log(`${report.length}/${targets.size} ${folder}: ${best?'portrait staged':'no portrait'}`);
  await new Promise(resolve=>setTimeout(resolve,250));
}

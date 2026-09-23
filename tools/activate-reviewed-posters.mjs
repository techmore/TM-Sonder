// Activate only named, visually reviewed candidates. Source media is untouched.
// node tools/activate-reviewed-posters.mjs REVIEW_DIR DATA_DIR 'Show (year)' ...
import fs from 'node:fs/promises';
import path from 'node:path';
const [reviewDir,dataDir,...approved]=process.argv.slice(2);
if(!reviewDir||!dataDir||!approved.length)throw new Error('Review directory, data directory and explicit approved titles required');
const rows=JSON.parse(await fs.readFile(path.join(reviewDir,'report.json'),'utf8'));
const response=await fetch('http://127.0.0.1:8797/api/library');
if(!response.ok)throw new Error('Catalog unavailable');
const catalog=await response.json();
const directory=path.join(dataDir,'curated-posters');await fs.mkdir(directory,{recursive:true});
const manifestPath=path.join(directory,'manifest.json');
const previous=await fs.readFile(manifestPath,'utf8').catch(e=>{if(e.code==='ENOENT')return '{}';throw e;});
const manifest=JSON.parse(previous);
await fs.writeFile(path.join(directory,`manifest-backup-${Date.now()}.json`),previous,{flag:'wx'});
for(const title of approved) {
 const row=rows.find(row=>row.folder===title&&row.filename);
 if(!row||!/^[a-f0-9]{64}\.(png|jpg|jpeg)$/.test(row.filename))throw new Error(`No valid reviewed candidate for ${title}`);
 const keys=[...new Set(catalog.items.filter(i=>i.showGroupTitle===title).map(i=>i.showGroupID))];
 if(!keys.length)throw new Error(`No exact show group for ${title}`);
 await fs.copyFile(path.join(reviewDir,row.filename),path.join(directory,row.filename));
 for(const key of keys)manifest[key]={...row,review:'Visually reviewed and approved for personal-library display',approvedAt:new Date().toISOString()};
 console.log(`Activated ${title}: ${keys.length} show group(s)`);
}
const temporary=manifestPath+'.tmp';
await fs.writeFile(temporary,JSON.stringify(manifest,null,2));await fs.rename(temporary,manifestPath);
console.log('Restart the server to invalidate its catalog payload cache.');

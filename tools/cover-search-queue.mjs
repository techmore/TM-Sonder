// Build one resumable search queue across the live catalog and dimension audits.
// node tools/cover-search-queue.mjs OUTPUT.json [AUDIT.json ...]
import fs from 'node:fs/promises';
const [output,...audits]=process.argv.slice(2);
if(!output)throw new Error('Output path required');
const response=await fetch('http://127.0.0.1:8797/api/library');
if(!response.ok)throw new Error('Catalog unavailable');
const {items}=await response.json();
const flagged=new Map();
for(const file of audits)for(const row of JSON.parse(await fs.readFile(file,'utf8'))) {
 if(!row.reason)continue;
 for(const id of row.ids)flagged.set(id,row);
}
const groups=new Map();const summary={};
for(const item of items) {
 if(item.isPlaceholder||!['movie','documentary','tvShow','ebook','audiobook'].includes(item.kind))continue;
 const kind=item.kind;
 summary[kind]??={files:0,withoutCover:0,searchTargets:0};summary[kind].files++;
 if(!item.posterURL)summary[kind].withoutCover++;
 const issue=flagged.get(item.id);
 if(item.posterURL&&!issue)continue;
 if(item.posterURL?.includes('/artwork/curated/'))continue;
 const title=kind==='tvShow'?(item.showGroupTitle||item.showTitle||item.title):item.title;
 const key=kind+'\0'+(item.showGroupID||[title,item.year||0,item.author||item.studio||''].join('\0'));
 if(!groups.has(key)) {
  groups.set(key,{targetID:key,folder:title,title,kind,year:item.year||0,author:item.author||'',
   path:issue?.path||'',reason:!item.posterURL?'missing cover':issue.reason,ids:[],titles:[title]});
  summary[kind].searchTargets++;
 }
 groups.get(key).ids.push(item.id);
}
// Round-robin kinds so one large library cannot starve the others.
const buckets=Object.keys(summary).map(kind=>[...groups.values()].filter(x=>x.kind===kind));
const queue=[];
while(buckets.some(b=>b.length))for(const bucket of buckets)if(bucket.length)queue.push(bucket.shift());
await fs.writeFile(output,JSON.stringify(queue,null,2));
console.log(JSON.stringify({summary,totalTargets:queue.length,output},null,2));

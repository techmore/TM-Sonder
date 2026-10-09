(() => {
 "use strict";
 const $ = selector => document.querySelector(selector), {api} = window.Sonder;
 let state, loading = false, editing = false;
 const bytes = n => Number.isFinite(n) ? (n / 1e9).toLocaleString(undefined,{maximumFractionDigits:1}) + ' GB' : '—';
 const date = v => !v || v.startsWith('0001') ? 'Not checked' : new Date(v).toLocaleString();
 const element = (tag,text) => {const e=document.createElement(tag);if(text!==undefined)e.textContent=text;return e;};
 $('#libraryLink').href=api('/');
 function button(label, action) {const b=element('button',label);b.type='button';b.addEventListener('click',async()=>{b.disabled=true;try{await change(action);}finally{b.disabled=false;}});return b;}
 function rowCells(...items){const row=element('tr');for(const item of items){const td=element('td');td.append(typeof item==='string'?document.createTextNode(item):item);row.append(td);}return row;}
 function render(){
  if(!state)return;
  $('#free').textContent=bytes(state.freeBytes);$('#reserve').textContent=state.settings.minFreeGB+' GB';
  $('#queue').textContent=(state.counts?.queued||0)+' / '+((state.active&&!state.active.startsWith('check:'))?'1':'0');
  $('#saved').textContent=bytes(state.savedBytes||0);
  $('#pause').textContent=state.settings.paused?'Resume sync':'Pause sync';$('#pause').disabled=!state.ready;$('#check').disabled=!state.ready||state.settings.paused;
  $('#notice').textContent=state.settings.paused?'Sync paused. Channel checks and new downloads are paused.':state.blocked|| (state.checking?'Checking channel uploads…':state.active?'Working: '+state.active:'Watching for new uploads.');
  $('#storagePath').textContent='Storage: '+(state.storagePath||'Not configured')+' · '+state.knownArchiveIDs+' known video IDs from the existing archive';
  $('#dependencies').textContent=Object.entries(state.dependencies||{}).map(([k,v])=>k+': '+(v?'ready':'missing')).join(' · ');
  if(!editing){$('#minFree').value=state.settings.minFreeGB;$('#height').value=state.settings.maxHeight;$('#profile').value=state.settings.profile;}
  $('#empty').hidden=state.channels.length>0;
  $('#channels').replaceChildren(...state.channels.map(c=>{
   const title=element('div'),a=element('a',c.name);a.href=c.url;a.target='_blank';a.rel='noopener noreferrer';title.append(a);if(c.error)title.append(element('small',c.error));if(c.paused)title.append(element('small','Paused'));
   const interval=element('select');for(const [v,label] of [[1,'Hourly'],[24,'Daily']]){const o=element('option',label);o.value=v;interval.append(o);}interval.value=c.intervalHours;interval.addEventListener('change',()=>change({action:'channelInterval',id:c.id,intervalHours:Number(interval.value)}));
   const dates=element('div',date(c.lastCheck));dates.append(element('small','Next: '+date(c.nextCheck)));
   const actions=element('div');actions.className='yt-actions';actions.append(button(c.paused?'Resume':'Pause',{action:c.paused?'channelResume':'channelPause',id:c.id}),button('Check now',{action:'channelCheck',id:c.id}),button('Unsubscribe',{action:'channelRemove',id:c.id}));return rowCells(title,interval,dates,actions);
  }));
  const filter=$('#filter').value;
  $('#jobs').replaceChildren(...[...(state.jobs||[])].reverse().filter(j=>filter==='all'||(filter==='completed'?j.status==='completed':!['completed','skipped'].includes(j.status))).map(j=>{
   const title=element('div'),a=element('a',j.title||j.id);a.href='https://www.youtube.com/watch?v='+encodeURIComponent(j.id);a.target='_blank';a.rel='noopener noreferrer';title.append(a);title.append(element('small',j.detail||''));if(j.path)title.append(element('small',j.path));
   const status=element('span',j.status);status.className='yt-pill';const actions=element('div');actions.className='yt-actions';
   if(['failed','cancelled'].includes(j.status))actions.append(button('Retry',{action:'jobRetry',id:j.id}));if(['queued','downloading','converting'].includes(j.status))actions.append(button('Cancel',{action:'jobCancel',id:j.id}));
   return rowCells(title,status,bytes(j.bytes||0)+' / '+bytes(j.savedBytes||0),actions);
  }));
 }
 async function refresh(){if(loading||document.hidden)return;loading=true;try{const r=await fetch(api('/api/settings/youtube'),{cache:'no-store'});if(r.status===401||r.status===403)throw new Error('Sign in with the server owner account to manage subscriptions.');if(!r.ok)throw new Error('Subscription worker is unavailable.');state=await r.json();render();}catch(e){$('#notice').textContent=e.message;}finally{loading=false;}}
 async function change(payload){try{const r=await fetch(api('/api/settings/youtube'),{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});if(!r.ok){const body=await r.json().catch(()=>({}));throw new Error(body.error||body.message||'Could not save change');}await refresh();return true;}catch(e){$('#notice').textContent=e.message;return false;}}
 $('#add').addEventListener('submit',async e=>{e.preventDefault();const button=e.submitter;button.disabled=true;try{if(await change({action:'add',url:$('#url').value,intervalHours:Number($('#interval').value),backfill:Number($('#backfill').value)}))$('#url').value='';}finally{button.disabled=false;}});
 $('#policy').addEventListener('input',()=>{editing=true;});$('#policy').addEventListener('submit',async e=>{e.preventDefault();if(!state)return;const button=e.submitter;button.disabled=true;try{if(await change({action:'settings',settings:{...state.settings,minFreeGB:Number($('#minFree').value),maxHeight:Number($('#height').value),profile:$('#profile').value}})){editing=false;render();}}finally{button.disabled=false;}});
 $('#pause').addEventListener('click',()=>state&&change({action:'settings',settings:{...state.settings,paused:!state.settings.paused}}));$('#check').addEventListener('click',()=>change({action:'checkAll'}));$('#refresh').addEventListener('click',refresh);$('#filter').addEventListener('change',render);document.addEventListener('visibilitychange',refresh);void refresh();setInterval(refresh,10000);
})();

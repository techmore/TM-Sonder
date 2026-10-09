(() => {
 'use strict';
 const link=document.querySelector('#youtubeLink');
 if(!link||!window.Sonder)return;
 link.href=window.Sonder.api('/youtube');
 fetch(window.Sonder.api('/api/settings/youtube/permission'),{cache:'no-store'}).then(response=>{link.hidden=!response.ok;}).catch(()=>{});
})();

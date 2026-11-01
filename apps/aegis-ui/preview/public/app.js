const shapes={
dashboard:'<rect x="3" y="3" width="7" height="7" rx="1.4"/><rect x="14" y="3" width="7" height="11" rx="1.4"/><rect x="3" y="14" width="7" height="7" rx="1.4"/><rect x="14" y="18" width="7" height="3" rx="1"/>',
server:'<rect x="3" y="3" width="18" height="7" rx="2"/><rect x="3" y="14" width="18" height="7" rx="2"/><path d="M7 6.5h.01M7 17.5h.01M11 6.5h6M11 17.5h6"/>',
archive:'<path d="M4 8v12h16V8M3 3h18v5H3zM9 12h6"/>',
diff:'<path d="M5 3v18M19 3v18M9 7h6M12 4v6M9 16h6"/>',
shield:'<path d="M12 3 3.5 6v5c0 5 3 8 8.5 10 5.5-2 8.5-5 8.5-10V6zM12 8v5M12 16h.01"/>',
lock:'<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3M12 14v3"/>',
help:'<circle cx="12" cy="12" r="9"/><path d="M9 9a3 3 0 1 1 4 2.8c-.8.3-1 1-1 2M12 17h.01"/>',
'arrow-up-right':'<path d="M6 18 18 6M6 6h12v12"/>',
arrow:'<path d="M4 12h16M14 6l6 6-6 6"/>',
chevron:'<path d="m9 5 7 7-7 7"/>',
search:'<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/>',
bell:'<path d="M4 17h16l-2-3V9a6 6 0 0 0-12 0v5zM10 21h4"/>',
download:'<path d="M12 3v12m-5-5 5 5 5-5M4 16v5h16v-5"/>',
check:'<path d="m5 12 4 4L19 6"/>',
clock:'<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
globe:'<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c5 5 5 13 0 18-5-5-5-13 0-18"/>',
activity:'<path d="M2 12h5l3-8 4 16 3-8h5"/>',
alert:'<path d="m12 3 10 17H2zM12 9v4M12 17h.01"/>',
plus:'<path d="M12 4v16M4 12h16"/>',
swap:'<path d="M3 7h17l-4-4M21 17H4l4 4"/>',
close:'<path d="m6 6 12 12M6 18 18 6"/>',
code:'<path d="m8 6-6 6 6 6m8-12 6 6-6 6M14 3l-4 18"/>',
pin:'<path d="M19 10c0 6-7 11-7 11S5 16 5 10a7 7 0 0 1 14 0Z"/><circle cx="12" cy="10" r="2"/>'
};
const icon=(name)=>'<svg class="icon" viewBox="0 0 24 24" aria-hidden="true">'+(shapes[name]||shapes.server)+'</svg>';
const esc=(value)=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const iconize=()=>document.querySelectorAll('[data-icon]').forEach(el=>el.outerHTML=icon(el.dataset.icon));
iconize();
const response=await fetch('/demo-data.json');
if(!response.ok)throw new Error('Demo fixtures could not be loaded');
const data=await response.json();
const devices=data.devices;
const state={search:'',site:'all',inventoryRisk:'all',selectedDevice:devices[0].id,detailTab:'overview',onlyChanges:false,layout:'split',riskImpact:'all',reviewed:new Set()};
const main=document.querySelector('#main');

// Go workspace views never fall back to fixtures. Risk and local sample material remain separate.
const GO_SOURCE='Aegis Go API — Isolated Demo';
const GO_BACKUP_SOURCE='Aegis Go API — Disposable SQLite Demo';
const inventoryHTTP={status:'idle',records:[],path:'/api/go/v1/devices',httpStatus:null,meta:null,elapsed:0,error:null,sequence:0};
const deviceHTTP={status:'idle',id:null,record:null,path:null,httpStatus:null,meta:null,elapsed:0,error:null,sequence:0};
function isInventoryRecord(record){
 return record && record.source==='AEGIS_GO_ISOLATED_DEMO' &&
  ['id','display_name','hostname','management_ip','site','model','role','status','risk'].every(key=>typeof record[key]==='string'&&record[key].length>0);
}
async function requestMockAPI(url,expectedSource='Local Mock API'){
 const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),8000);
 const started=performance.now();
 try{
  const response=await fetch(url,{cache:'no-store',credentials:'omit',signal:controller.signal});
  let payload;
  try{payload=await response.json();}catch{throw Object.assign(new Error('The mock API returned an unreadable response.'),{httpStatus:response.status});}
  if(!response.ok)throw Object.assign(new Error(payload.error?.message||'The mock API could not complete this request.'),{httpStatus:response.status,meta:payload.meta});
  if(payload.meta?.synthetic!==true||payload.meta?.source!==expectedSource||typeof payload.meta?.request_id!=='string'){
   throw Object.assign(new Error('The response did not identify itself as the expected synthetic demo API.'),{httpStatus:response.status});
  }
  return{payload,httpStatus:response.status,elapsed:Math.round(performance.now()-started)};
 }finally{clearTimeout(timer);}
}
async function loadInventory(){
 const sequence=++inventoryHTTP.sequence;
 Object.assign(inventoryHTTP,{status:'loading',records:[],error:null,httpStatus:null,meta:null});render();
 try{
  const result=await requestMockAPI(inventoryHTTP.path,GO_SOURCE);
  if(sequence!==inventoryHTTP.sequence)return;
  if(!Array.isArray(result.payload.data)||!result.payload.data.every(isInventoryRecord)||
      new Set(result.payload.data.map(d=>d.id)).size!==result.payload.data.length)throw new Error('The Go demo inventory response has an invalid device list.');
  knownGoDeviceCount=result.payload.data.length;
  Object.assign(inventoryHTTP,{status:'ready',records:result.payload.data,httpStatus:result.httpStatus,meta:result.payload.meta,elapsed:result.elapsed});
 }catch(error){
  if(sequence!==inventoryHTTP.sequence)return;
  Object.assign(inventoryHTTP,{status:'error',records:[],httpStatus:error.httpStatus||null,meta:error.meta||null,
   error:error.httpStatus?error.message:'Could not reach the isolated Aegis Go API. Make sure the local preview server is running, then try again.'});
 }
 render();
}
async function loadDevice(id){
 const sequence=++deviceHTTP.sequence;
 Object.assign(deviceHTTP,{status:'loading',id,record:null,path:'/api/go/v1/devices/'+encodeURIComponent(id),error:null,httpStatus:null,meta:null});render();
 try{
  const result=await requestMockAPI(deviceHTTP.path,GO_SOURCE);
  if(sequence!==deviceHTTP.sequence)return;
  if(!isInventoryRecord(result.payload.data)||result.payload.data.id!==id)throw new Error('The Go API returned an unexpected device record.');
  Object.assign(deviceHTTP,{status:'ready',record:result.payload.data,httpStatus:result.httpStatus,meta:result.payload.meta,elapsed:result.elapsed});
 }catch(error){
  if(sequence!==deviceHTTP.sequence)return;
  Object.assign(deviceHTTP,{status:error.httpStatus===404?'not-found':'error',record:null,httpStatus:error.httpStatus||null,meta:error.meta||null,
   error:error.httpStatus?error.message:'Could not load this device from the isolated Aegis Go API. Check the local preview server and try again.'});
 }
 render();
}
function apiSource(info){
 const ready=info.status==='ready',loading=info.status==='loading';
 const source=info.source||(info.path?.startsWith('/api/go/')?(info.path.includes('/backups')?GO_BACKUP_SOURCE:GO_SOURCE):'Local Mock API');
 const status=loading?'Request in progress':info.httpStatus?'HTTP '+info.httpStatus:'Connection unavailable';
 return'<div class="api-source '+(ready?'api-ready':loading?'api-pending':'api-failed')+'" data-api-path="'+esc(info.path)+'" data-http-status="'+(info.httpStatus||'')+'"><div class="api-source-label">'+icon('server')+'<strong>'+esc(source)+'</strong><span class="api-status">'+esc(status)+'</span></div><div class="api-source-detail"><a href="'+esc(info.path)+'" target="_blank" rel="noopener">GET '+esc(info.path)+'</a>'+(ready?'<span>'+info.elapsed+' ms · synthetic response</span>':'')+(info.meta?.upstream_path?'<span>Go GET '+esc(info.meta.upstream_path)+' · disposable SQLite · ephemeral viewer</span>':'')+'</div>'+(info.meta?.request_id?'<details class="api-request-id"><summary>HTTP request details</summary><span>Response request ID: <code>'+esc(info.meta.request_id)+'</code></span></details>':'')+'</div>';
}
function apiState(info,title,description,retryAction){
 const loading=info.status==='loading';
 return apiSource(info)+'<section class="card api-state" '+(loading||info.status==='ready'?'role="status" aria-live="polite"':'role="alert"')+'>'+
 '<span class="api-state-symbol">'+(loading?'<span class="api-spinner" aria-hidden="true"></span>':icon(info.status==='not-found'?'search':info.status==='ready'?'archive':'alert'))+'</span><h2>'+esc(title)+'</h2><p>'+esc(description)+'</p>'+
 (loading?'':btn('Retry',retryAction,'primary','','arrow'))+'</section>';
}
function inventoryDevicePage(route){
 if(deviceHTTP.status==='loading'||deviceHTTP.id!==route.requestedDeviceId){
  return header('Device inventory','Loading device details…','Reading the isolated Go API and disposable demo database over HTTP.')+apiState(deviceHTTP,'Fetching device record','Contacting the isolated Aegis Go API.','retry-device');
 }
 if(deviceHTTP.status==='not-found'){
  return'<a class="back-link" href="#/devices">'+icon('arrow')+'Back to device inventory</a>'+header('Device inventory','Device not found','No synthetic device matches “'+deviceHTTP.id+'”.')+apiState(deviceHTTP,'This device is not in the isolated Go inventory',deviceHTTP.error,'retry-device');
 }
 if(deviceHTTP.status!=='ready'){
  return'<a class="back-link" href="#/devices">'+icon('arrow')+'Back to device inventory</a>'+header('Device inventory','Device details unavailable','The device record could not be loaded over HTTP.')+apiState(deviceHTTP,'Could not load device details',deviceHTTP.error||'Please try again.','retry-device');
 }
 return deviceDetail(deviceHTTP.record);
}
// Session-only snapshots are deliberately separate from immutable HTTP history.
const memorySnapshots=new Map();
const localSnapshots=id=>memorySnapshots.get(id)||[];
const historyHTTP={status:'idle',scope:null,devices:[],records:[],requests:[],sequence:0};
const comparisonHTTP={status:'idle',records:[],requests:[],left:null,right:null,sequence:0};
const configurationHTTP={status:'idle',deviceId:null,selectedId:null,records:[],record:null,requests:[],sequence:0,source:GO_BACKUP_SOURCE};
let snapshotSequence=0;
async function backupRequest(path,expectedSource='Local Mock API'){
 try{
  const result=await requestMockAPI(path,expectedSource);
  return {...result,info:{status:'ready',source:expectedSource,path,httpStatus:result.httpStatus,meta:result.payload.meta,elapsed:result.elapsed}};
 }catch(error){error.path=path;throw error;}
}
function backupFailure(error,fallbackPath,source='Local Mock API'){
 return {status:error.httpStatus===404?'not-found':'error',source,path:error.path||fallbackPath,
  httpStatus:error.httpStatus||null,meta:error.meta||null,elapsed:0,
  error:(error.httpStatus||error.path&&(error.message.startsWith('The mock API returned')||error.message.startsWith('The Go API returned')))?error.message:
   'Could not load synthetic backups from '+source+'. Check the local server, then Retry.'};
}
function httpEvidence(requests,source='Local Mock API'){
 return '<div class="api-source api-ready"><div class="api-source-label">'+icon('server')+
  '<strong>'+esc(source)+'</strong><span class="api-status">HTTP 200 · '+requests.length+' GET response'+(requests.length===1?'':'s')+
  '</span></div><details class="api-request-id"><summary>HTTP sources &amp; response request IDs</summary>'+
  requests.map(info=>'<div class="api-source-detail"><a href="'+esc(info.path)+'" target="_blank" rel="noopener">GET '+esc(info.path)+
   '</a><span>'+info.elapsed+' ms · <code>'+esc(info.meta.request_id)+'</code></span></div>').join('')+'</details></div>';
}
async function loadBackupHistory(scope='all'){
 const sequence=++historyHTTP.sequence;
 Object.assign(historyHTTP,{status:'loading',scope,source:GO_BACKUP_SOURCE,devices:[],records:[],requests:[],path:goBackupPath(scope==='all'?'core-sw-01':scope),httpStatus:null,meta:null});render();
 try{
  const inventory=await backupRequest('/api/go/v1/devices',GO_SOURCE);
  const goDevices=inventory.payload.data;
  if(!Array.isArray(goDevices)||!goDevices.every(isInventoryRecord)||new Set(goDevices.map(d=>d.id)).size!==goDevices.length)throw new Error('The Go API returned invalid inventory metadata.');
  knownGoDeviceCount=goDevices.length;
  const ids=scope==='all'?goDevices.map(d=>d.id):[scope];
  const results=await Promise.all(ids.map(async id=>{
   const result=await readGoHistory(id);
   const device=goDevices.find(d=>d.id===id);
   if(!device)throw new Error('Backup device is not in the isolated Go inventory.');
   return {...result,records:result.records.map(b=>({...b,device}))};
  }));
  if(sequence!==historyHTTP.sequence)return;
  Object.assign(historyHTTP,{status:'ready',devices:goDevices,records:results.flatMap(x=>x.records).sort((a,b)=>b.created_at.localeCompare(a.created_at)),requests:[inventory.info,...results.map(x=>x.info)]});
 }catch(error){
  if(sequence!==historyHTTP.sequence)return;
  Object.assign(historyHTTP,backupFailure(error,historyHTTP.path,GO_BACKUP_SOURCE),{devices:[],records:[],requests:[]});
 }
 render();
}
function sessionHistory(scope){
 return historyHTTP.devices.filter(d=>scope==='all'||d.id===scope).flatMap(d=>localSnapshots(d.id).map(b=>({...b,device:d})));
}
function backupHistoryBody(scope){
 if(historyHTTP.scope!==scope||historyHTTP.status==='loading'||historyHTTP.status==='idle')return apiState({...historyHTTP,status:'loading'},'Loading Go backup history…','Fetching metadata from the native BackupService and disposable SQLite.','retry-backups');
 if(historyHTTP.status!=='ready')return apiState(historyHTTP,historyHTTP.status==='not-found'?'Go backup history not found':'Go backup API unavailable',historyHTTP.error,'retry-backups');
 const local=sessionHistory(scope);
 return httpEvidence(historyHTTP.requests,GO_BACKUP_SOURCE)+'<section class="card">'+cardHead('Go backup history',historyHTTP.records.length+' seeded synthetic snapshots · native BackupService reads',demoTag())+
  (historyHTTP.records.length?backupTable(historyHTTP.records):'<div class="empty-state"><h2>No Go backups</h2><p>The Go API returned an empty history for this scope.</p>'+btn('Retry','retry-backups')+'</div>')+'</section>'+
  '<section class="card session-snapshots">'+cardHead('Browser-memory fixture copies',local.length+' local copies · never persisted to Go or Node','<span class="tiny-tag">THIS TAB ONLY</span>')+
  (local.length?backupTable(local):'<p class="session-note">Create demo backup copies a legacy synthetic fixture into this tab only. Reloading removes it. Go history stays unchanged.</p>')+'</section>';
}
const goBackupPath=id=>'/api/go/v1/devices/'+encodeURIComponent(id)+'/backups';
function isGoBackup(record,id,withConfig=false){
 return record&&record.device_id===id&&record.source==='AEGIS_GO_BACKUP_DEMO'&&record.synthetic===true&&
  ['id','label','created_at','config_type','origin'].every(k=>typeof record[k]==='string'&&record[k])&&
  /^[a-f0-9]{64}$/.test(record.content_hash)&&Number.isSafeInteger(record.size_bytes)&&record.size_bytes>=0&&
  Number.isFinite(Date.parse(record.created_at))&&(!withConfig||typeof record.config==='string'&&record.config.startsWith('! AEGIS SYNTHETIC DEMO — DISPOSABLE BACKUP SNAPSHOT\n'));
}
async function readGoHistory(id){
 const result=await backupRequest(goBackupPath(id),GO_BACKUP_SOURCE),records=result.payload.data;
 if(!Array.isArray(records)||!records.every(b=>isGoBackup(b,id))||new Set(records.map(b=>b.id)).size!==records.length)throw Object.assign(new Error('The Go API returned invalid backup metadata.'),{path:goBackupPath(id)});
 return {records,info:result.info};
}
async function readGoSnapshot(id,backupId){
 const path=goBackupPath(id)+'/'+encodeURIComponent(backupId),result=await backupRequest(path,GO_BACKUP_SOURCE);
 if(!isGoBackup(result.payload.data,id,true)||result.payload.data.id!==backupId)throw Object.assign(new Error('The Go API returned an unexpected snapshot.'),{path});
 return {record:result.payload.data,info:result.info};
}
function historyReplaceConfiguration(id,backupId){
 const route=routing();
 if(!route.detail||route.requestedDeviceId!==id||state.detailTab!=='configuration')return;
 const query=new URLSearchParams({tab:'configuration'});
 if(backupId)query.set('backup',backupId);
 history.replaceState(null,'','#/devices/'+encodeURIComponent(id)+'?'+query);
}
async function loadSavedConfiguration(id,selectedId=routing().query.get('backup')){
 const sequence=++configurationHTTP.sequence;
 Object.assign(configurationHTTP,{status:'loading',deviceId:id,selectedId,records:[],record:null,requests:[],
  path:goBackupPath(id)+(selectedId?'/'+encodeURIComponent(selectedId):''),httpStatus:null,meta:null,error:null});
 historyReplaceConfiguration(id,selectedId);render();
 try{
  const history=await readGoHistory(id);
  if(sequence!==configurationHTTP.sequence)return;
  const records=history.records.filter(b=>b.config_type==='running').sort((a,b)=>b.created_at.localeCompare(a.created_at));
  Object.assign(configurationHTTP,{records,requests:[history.info]});
  if(!selectedId&&!records.length){Object.assign(configurationHTTP,{status:'empty'});render();return;}
  const wanted=selectedId||records[0].id;
  configurationHTTP.selectedId=wanted;historyReplaceConfiguration(id,wanted);
  const snapshot=await readGoSnapshot(id,wanted);
  if(sequence!==configurationHTTP.sequence)return;
  const metadata=records.find(b=>b.id===wanted);
  if(!metadata)throw Object.assign(new Error('The requested backup is not in this device’s current saved running-config history.'),{httpStatus:404,path:snapshot.info.path,meta:snapshot.info.meta});
  if(snapshot.record.config_type!=='running'||snapshot.record.content_hash!==metadata.content_hash||
     snapshot.record.size_bytes!==metadata.size_bytes||snapshot.record.created_at!==metadata.created_at){
   throw Object.assign(new Error('The Go API returned inconsistent saved configuration metadata.'),{path:snapshot.info.path});
  }
  Object.assign(configurationHTTP,{status:'ready',record:snapshot.record,requests:[history.info,snapshot.info]});
 }catch(error){
  if(sequence!==configurationHTTP.sequence)return;
  Object.assign(configurationHTTP,backupFailure(error,configurationHTTP.path,GO_BACKUP_SOURCE),{record:null});
 }
 render();
}
function savedConfigurationBody(id){
 const info=configurationHTTP;
 const intro='<section class="saved-configuration"><div class="saved-config-heading"><div><h2>Saved running configuration</h2><p>Latest saved configuration — not live device state.</p></div>'+btn('Refresh versions','refresh-configuration','default','','activity')+'</div>';
 if(info.deviceId!==id||info.status==='idle'||info.status==='loading')return intro+apiState({...info,status:'loading',source:GO_BACKUP_SOURCE,path:goBackupPath(id)},'Loading saved configuration…','Reading this Go device’s saved history and synthetic snapshot content.','retry-configuration')+'</section>';
 if(info.status==='empty')return intro+httpEvidence(info.requests,GO_BACKUP_SOURCE)+'<section class="card api-state" role="status"><h2>No saved running configuration</h2><p>This Go device has no persisted running-config snapshots. No fixture configuration is substituted.</p>'+btn('Retry','retry-configuration')+'</section></section>';
 if(info.status!=='ready'){
  const title=info.httpStatus===401||info.httpStatus===403?'Saved configuration not authorized':info.status==='not-found'?'Saved configuration not found':'Saved configuration unavailable';
  const description=info.status==='not-found'?'This device or backup UUID is unavailable. A Go restart regenerates snapshot IDs. Retry keeps the requested UUID; Refresh versions explicitly reloads the current list.':info.error;
  return intro+apiState(info,title,description,'retry-configuration')+(info.selectedId?'<p class="saved-config-request">Requested backup: <code>'+esc(info.selectedId)+'</code></p>':'')+'</section>';
 }
 const selected=info.record,index=info.records.findIndex(b=>b.id===selected.id),other=info.records[index+1]||info.records[index-1]||selected;
 const compare='#/compare?'+new URLSearchParams({device:id,left:other.id,right:selected.id});
 const options=info.records.map((b,i)=>'<option value="'+esc(b.id)+'" '+(b.id===selected.id?'selected':'')+'>'+date(b.created_at)+' · '+time(b.created_at)+' · '+esc(b.id.slice(0,8))+(i===0?' · latest':'')+'</option>').join('');
 return intro+httpEvidence(info.requests,GO_BACKUP_SOURCE)+'<article class="card saved-config-card"><div class="saved-config-toolbar"><div class="select-group"><label for="saved-config-version">Saved version</label><select id="saved-config-version" data-control="saved-config-version">'+options+'</select></div><span class="tiny-tag">'+(index===0?'LATEST SAVED':'OLDER SAVED VERSION')+'</span><a class="btn btn-primary" href="'+esc(compare)+'">'+icon('diff')+'Compare saved versions</a></div>'+
  '<dl class="saved-config-metadata"><div><dt>Snapshot timestamp · UTC</dt><dd data-saved-timestamp="'+esc(selected.created_at)+'">'+date(selected.created_at)+' · '+time(selected.created_at)+'</dd></div><div><dt>Backup UUID</dt><dd><code id="saved-config-id">'+esc(selected.id)+'</code></dd></div><div><dt>SHA-256 · native content verification</dt><dd><code id="saved-config-sha">'+esc(selected.content_hash)+'</code></dd></div><div><dt>Saved size</dt><dd id="saved-config-size">'+selected.size_bytes+' bytes</dd></div></dl>'+
  '<div class="saved-config-caption">'+icon('lock')+'Read-only synthetic snapshot · '+esc(id)+' · native BackupService.Content</div><pre id="saved-config-content" class="config-view saved-config-content" tabindex="0" aria-label="Saved synthetic running configuration">'+esc(selected.config)+'</pre><div class="saved-config-foot">DEMO / UNVERIFIED · Saved synthetic content only. No live configuration request, connector or SSH.</div></article></section>';
}

function comparisonHeading(route){
 return header('Go configuration comparison','See what changed. Understand what matters.','Native BackupService.Diff · persisted synthetic snapshots · no device access.',
  btn('Refresh snapshots','refresh-compare','default','','activity')+demoTag());
}
async function loadComparison(route=routing()){
 const sequence=++comparisonHTTP.sequence;
 Object.assign(comparisonHTTP,{status:'loading',source:GO_BACKUP_SOURCE,devices:[],device:null,records:[],requests:[],left:null,right:null,native:null,path:'/api/go/v1/devices',httpStatus:null,meta:null});render();
 try{
  const inventory=await backupRequest('/api/go/v1/devices',GO_SOURCE),goDevices=inventory.payload.data;
  if(!Array.isArray(goDevices)||!goDevices.every(isInventoryRecord))throw new Error('The Go API returned invalid inventory metadata.');
  if(sequence!==comparisonHTTP.sequence)return;
  knownGoDeviceCount=goDevices.length;
  Object.assign(comparisonHTTP,{devices:goDevices,requests:[inventory.info]});
  if(!goDevices.length){Object.assign(comparisonHTTP,{status:'empty'});render();return;}
  const id=route.query.get('device')||(goDevices.some(d=>d.id===state.selectedDevice)?state.selectedDevice:goDevices[0].id);
  const device=goDevices.find(d=>d.id===id);
  if(!device){
   // Obtain the native 404, never replace an explicitly requested device.
   await backupRequest('/api/go/v1/devices/'+encodeURIComponent(id),GO_SOURCE);
   throw new Error('The requested device is not in the Go demo inventory.');
  }
  const history=await readGoHistory(id),records=history.records;
  if(sequence!==comparisonHTTP.sequence)return;
  Object.assign(comparisonHTTP,{device,records,path:goBackupPath(id),requests:[inventory.info,history.info]});
  if(!records.length){Object.assign(comparisonHTTP,{status:'empty'});render();return;}
  const rightId=route.query.get('right')||records[0].id;
  const rightIndex=records.findIndex(b=>b.id===rightId);
  const leftId=route.query.get('left')||(records[rightIndex+1]||records[rightIndex-1]||records[1]||records[0]).id;
  const [left,right,native]=await Promise.all([readGoSnapshot(id,leftId),readGoSnapshot(id,rightId),readGoDiff(id,leftId,rightId)]);
  if(sequence!==comparisonHTTP.sequence)return;
  if(native.record.changed!==(left.record.config!==right.record.config))throw new Error('The Go Diff and selected content did not agree.');
  Object.assign(comparisonHTTP,{status:'ready',left:left.record,right:right.record,native:native.record,
   requests:[inventory.info,history.info,left.info,right.info,native.info]});
  // Canonical links retain the real selected UUIDs, including across restarts.
  if(routing().view==='compare')historyReplaceComparison(id,leftId,rightId);
 }catch(error){
  if(sequence!==comparisonHTTP.sequence)return;
  Object.assign(comparisonHTTP,backupFailure(error,comparisonHTTP.path,GO_BACKUP_SOURCE),{left:null,right:null,native:null});
 }
 render();
}
function historyReplaceComparison(device,left,right){
 const query=new URLSearchParams({device,left,right});
 history.replaceState(null,'','#/compare?'+query);
}
async function readGoDiff(id,left,right){
 const path=goBackupPath(id)+'/'+encodeURIComponent(left)+'/diff/'+encodeURIComponent(right);
 const result=await backupRequest(path,GO_BACKUP_SOURCE),record=result.payload.data;
 if(!record||record.source!=='AEGIS_GO_DIFF_DEMO'||record.left_backup_id!==left||record.right_backup_id!==right||
    typeof record.changed!=='boolean'||typeof record.diff!=='string'||
    !Number.isSafeInteger(record.added_lines)||record.added_lines<0||!Number.isSafeInteger(record.removed_lines)||record.removed_lines<0){
  throw Object.assign(new Error('The Go API returned an unexpected Diff result.'),{path});
 }
 return {record,info:result.info};
}
async function showSnapshot(id,backupId){
 const sequence=++snapshotSequence;
 const local=localSnapshots(id).find(b=>b.id===backupId);
 modal('Synthetic snapshot',local?'<p>Browser-memory fixture copy · never persisted to Go or Node.</p><pre class="config-view">'+esc(local.config)+'</pre>':
  apiState({status:'loading',source:GO_BACKUP_SOURCE,path:goBackupPath(id)+'/'+encodeURIComponent(backupId)},'Loading snapshot…','Fetching synthetic configuration text over HTTP.','retry-snapshot'));
 if(local)return;
 try{
  const result=await readGoSnapshot(id,backupId);
  if(sequence!==snapshotSequence)return;
  modal(result.record.label,apiSource(result.info)+'<p>DEMO / UNVERIFIED · Seeded synthetic text read by native BackupService.Content with its SHA-256 verification.</p><details class="api-request-id"><summary>Native snapshot metadata</summary><p>ID: <code>'+esc(result.record.id)+'</code></p><p>SHA-256: <code>'+esc(result.record.content_hash)+'</code></p><p>'+result.record.size_bytes+' bytes · '+esc(result.record.trigger_type)+'</p></details><pre class="config-view">'+esc(result.record.config)+'</pre>');
 }catch(error){
  if(sequence!==snapshotSequence)return;
  const info=backupFailure(error,goBackupPath(id)+'/'+encodeURIComponent(backupId),GO_BACKUP_SOURCE);
  modal('Synthetic snapshot unavailable',apiState(info,info.status==='not-found'?'Snapshot not found':'Snapshot API unavailable',info.error,'retry-snapshot'));
  const retry=document.querySelector('[data-action="retry-snapshot"]');
  if(retry){retry.dataset.deviceId=id;retry.dataset.backupId=backupId;}
 }
}
function enterRoute(){
 const route=routing();
 if(route.view==='overview')void loadDashboard();
 else if(route.view==='devices'){
  if(route.detail){
   state.detailTab=['overview','configuration','backups'].includes(route.query.get('tab'))?route.query.get('tab'):'overview';
   void loadDevice(route.requestedDeviceId);
   if(state.detailTab==='backups')void loadBackupHistory(route.requestedDeviceId);
   if(state.detailTab==='configuration')void loadSavedConfiguration(route.requestedDeviceId);
  }
  else void loadInventory();
 }else if(route.view==='backups')void loadBackupHistory(route.query.get('device')||'all');
 else if(route.view==='compare')void loadComparison(route);
 else render();
}

const date=(value)=>new Intl.DateTimeFormat('en-GB',{day:'2-digit',month:'short',year:'numeric',timeZone:'UTC'}).format(new Date(value));
const time=(value)=>new Intl.DateTimeFormat('en-GB',{hour:'2-digit',minute:'2-digit',timeZone:'UTC'}).format(new Date(value))+' UTC';
const findDevice=id=>devices.find(d=>d.id===id)||devices[0];
const findingKey=(d,f)=>d.id+':'+f.id;
const badge=(value,label)=>'<span class="badge '+esc(value)+'"><i></i>'+esc(label||({high:'High impact',medium:'Medium impact',low:'Low · demo',unknown:'Unknown',healthy:'Healthy',reviewed:'Demo reviewed'}[value]||value))+'</span>';
const demoTag=()=>'<span class="tiny-tag">DEMO · UNVERIFIED</span>';
const btn=(label,action,style='primary',extra='',glyph='arrow')=>'<button class="btn btn-'+style+'" data-action="'+action+'" '+extra+'>'+icon(glyph)+esc(label)+'</button>';
const glyph=d=>'<span class="device-glyph">'+icon(d.role.includes('router')?'globe':'server')+'</span>';
const deviceLabel=d=>'<span class="device-name">'+glyph(d)+'<span>'+esc(d.display_name)+'<small>'+esc(d.role)+'</small></span></span>';
const header=(eyebrow,title,subtitle,actions='')=>'<section class="page-heading"><div><div class="eyebrow">'+esc(eyebrow)+'</div><h1>'+esc(title)+'</h1><p>'+esc(subtitle)+'</p></div><div class="heading-actions">'+actions+'</div></section>';
const cardHead=(title,sub,action='')=>'<div class="card-head"><div><h2>'+esc(title)+'</h2>'+(sub?'<div class="card-subtitle">'+esc(sub)+'</div>':'')+'</div>'+action+'</div>';
const empty=(title,description)=>'<div class="empty-state">'+icon('search')+'<h2>'+esc(title)+'</h2><p>'+esc(description)+'</p><button class="btn" data-action="reset-filters">Clear filters</button></div>';
function routing(){
 const parts=(location.hash.slice(1)||'/overview').split('?');
 const segments=parts[0].split('/').filter(Boolean);
 const view=['overview','devices','backups','compare','risk'].includes(segments[0])?segments[0]:'overview';
 const query=new URLSearchParams(parts[1]||'');
 const deviceId=segments[0]==='devices'&&segments[1]?segments[1]:query.get('device');
 if(deviceId&&devices.some(d=>d.id===deviceId))state.selectedDevice=deviceId;
 let requestedDeviceId=segments[1]||null;
 if(view==='devices'&&requestedDeviceId){try{requestedDeviceId=decodeURIComponent(requestedDeviceId);}catch{}}
 return{view,query,detail:view==='devices'&&!!segments[1],device:findDevice(state.selectedDevice),requestedDeviceId};
}
function go(view,params={}){
 const query=new URLSearchParams(params).toString();
 location.hash='/'+view+(query?'?'+query:'');
}
function compareTo(d,rightId,leftId){
 // Explicit row selections travel as IDs; contents are loaded by loadComparison().
 const records=historyHTTP.status==='ready'?historyHTTP.records.filter(b=>b.device_id===d.id):[];
 const right=records.find(b=>b.id===rightId);
 const index=records.indexOf(right);
 const left=records.find(b=>b.id===leftId)||(right?records[index+1]||records[index-1]||right:null);
 const target=new URLSearchParams({device:d.id,...(rightId?{right:rightId}:{}),...(left?{left:left.id}:{})}).toString();
 if(location.hash==='#/compare?'+target)void loadComparison();
 else go('compare',Object.fromEntries(new URLSearchParams(target)));
}
const dashboardHTTP={status:'idle',devices:[],backups:[],requests:[],sequence:0,path:'/api/go/v1/devices',source:GO_BACKUP_SOURCE};
let knownGoDeviceCount=null;
async function loadDashboard(){
 const sequence=++dashboardHTTP.sequence;
 Object.assign(dashboardHTTP,{status:'loading',devices:[],backups:[],requests:[],httpStatus:null,meta:null,error:null});knownGoDeviceCount=null;render();
 try{
  const inventory=await backupRequest('/api/go/v1/devices',GO_SOURCE),goDevices=inventory.payload.data;
  if(!Array.isArray(goDevices)||!goDevices.every(d=>isInventoryRecord(d)&&typeof d.device_group==='string')||
     new Set(goDevices.map(d=>d.id)).size!==goDevices.length)throw new Error('The Go API returned invalid dashboard inventory.');
  const histories=await Promise.all(goDevices.map(async device=>{
   const history=await readGoHistory(device.id);
   return {...history,records:history.records.map(backup=>({...backup,device}))};
  }));
  if(sequence!==dashboardHTTP.sequence)return;
  const backups=histories.flatMap(h=>h.records).sort((a,b)=>b.created_at.localeCompare(a.created_at)||a.device.display_name.localeCompare(b.device.display_name));
  if(new Set(backups.map(b=>b.id)).size!==backups.length)throw new Error('The Go API returned duplicate backup identifiers.');
  knownGoDeviceCount=goDevices.length;
  Object.assign(dashboardHTTP,{status:'ready',devices:goDevices,backups,requests:[inventory.info,...histories.map(h=>h.info)]});
 }catch(error){
  if(sequence!==dashboardHTTP.sequence)return;
  knownGoDeviceCount=null;
  Object.assign(dashboardHTTP,backupFailure(error,dashboardHTTP.path,GO_BACKUP_SOURCE),{devices:[],backups:[],requests:[]});
 }
 render();
}
function dashboardValues(){
 const records=dashboardHTTP.devices,backups=dashboardHTTP.backups;
 const covered=new Set(backups.map(b=>b.device_id));
 const groups=new Map();
 for(const d of records)groups.set(d.device_group,(groups.get(d.device_group)||0)+1);
 return {total:records.length,snapshots:backups.length,covered:records.filter(d=>covered.has(d.id)).length,
  coverage:records.length?Math.round(100*records.filter(d=>covered.has(d.id)).length/records.length):null,
  groups:[...groups].sort((a,b)=>b[1]-a[1]||a[0].localeCompare(b[0])),latest:backups[0]||null};
}
function dashboardCompareURL(backup){
 const versions=dashboardHTTP.backups.filter(b=>b.device_id===backup.device_id);
 const index=versions.findIndex(b=>b.id===backup.id),before=versions[index+1]||versions[index-1]||backup;
 return '#/compare?'+new URLSearchParams({device:backup.device_id,left:before.id,right:backup.id});
}
function dashboardStats(values){
 return '<section class="stats-grid" aria-label="Go dashboard summary">'+[
  ['devices','Go devices',values.total,'From the current Go Inventory','server'],
  ['snapshots','Saved snapshots',values.snapshots,'Persisted Go synthetic backups','archive'],
  ['coverage','Backup coverage',values.coverage===null?'—':values.coverage+'%',values.covered+' of '+values.total+' Go devices have backups','check'],
  ['groups','Device groups',values.groups.length,'Distinct device_group values','globe']
 ].map(([key,label,value,foot,glyph])=>'<article class="stat" data-dashboard-metric="'+key+'" data-value="'+esc(value)+'"><div class="stat-label">'+label+'<span class="stat-icon">'+icon(glyph)+'</span></div><div class="stat-number">'+esc(value)+'</div><div class="stat-foot">'+esc(foot)+'</div></article>').join('')+'</section>';
}
function groupDistribution(values){
 return '<article class="card group-card">'+cardHead('Device groups','Distribution from Go device_group metadata.','<span class="tiny-tag">GO METADATA</span>')+
 '<div class="group-distribution" aria-label="Go device group distribution">'+values.groups.map(([group,count])=>'<div class="group-row" data-group="'+esc(group)+'" data-count="'+count+'"><div class="group-label"><strong>'+esc(group||'Ungrouped (empty field)')+'</strong><span>'+count+' device'+(count===1?'':'s')+'</span></div><div class="group-track"><span style="width:'+(values.total?count/values.total*100:0)+'%"></span></div></div>').join('')+'</div>'+
 '<div class="chart-footer"><span>'+values.total+' devices across '+values.groups.length+' groups</span><a href="#/devices" class="text-link">Open Go Inventory '+icon('arrow')+'</a></div></article>';
}
function dashboardDeviceList(){
 return '<article class="card dashboard-devices">'+cardHead('Your Go devices','The same records used by Inventory and Backup History.')+
 '<div class="go-device-list">'+dashboardHTTP.devices.map(d=>{
  const versions=dashboardHTTP.backups.filter(b=>b.device_id===d.id);
  return '<div class="go-device-row"><span class="device-glyph">'+icon('server')+'</span><div class="go-device-facts"><a class="text-link" href="#/devices/'+encodeURIComponent(d.id)+'">'+esc(d.display_name)+'</a><small>'+esc(d.device_group||'Ungrouped')+' · '+esc(d.management_ip)+'</small></div><div class="go-device-backups"><strong>'+versions.length+'</strong><a href="#/backups?device='+encodeURIComponent(d.id)+'">Go backups '+icon('chevron')+'</a></div></div>';
 }).join('')+'</div></article>';
}
function dashboardRecent(values){
 const recent=dashboardHTTP.backups.slice(0,4);
 return '<section class="card dashboard-recent">'+cardHead('Recent Go backups','Newest persisted records from this disposable demo.','<a class="text-link" href="#/backups">All '+values.snapshots+' Go backups '+icon('arrow')+'</a>')+
 '<div class="recent-backup-list" aria-label="Recent Go backups">'+(recent.length?recent.map(b=>'<article class="recent-backup-row" data-backup-id="'+esc(b.id)+'"><div class="recent-backup-main"><span class="device-glyph">'+icon('archive')+'</span><div><button class="text-link" data-action="snapshot" data-device-id="'+esc(b.device_id)+'" data-backup-id="'+esc(b.id)+'">'+esc(b.label)+'</button><small><a href="#/devices/'+encodeURIComponent(b.device_id)+'">'+esc(b.device.display_name)+'</a> · '+esc(b.config_type)+'</small></div></div><div class="recent-backup-time">'+date(b.created_at)+'<small>'+time(b.created_at)+'</small></div><a class="btn btn-soft" href="'+esc(dashboardCompareURL(b))+'">'+icon('diff')+'Compare</a></article>').join(''):'<p class="session-note">No Go backups are stored for the current devices.</p>')+'</div></section>';
}
function overview(){
 const heading=header('One isolated Go workspace','Your devices. One clear picture.','Inventory, backups and comparison share the same disposable Go data.',
  btn('Refresh Dashboard','retry-dashboard','primary','','activity')+'<a class="btn" href="#/devices">'+icon('server')+'Go Inventory</a>');
 if(dashboardHTTP.status==='idle'||dashboardHTTP.status==='loading')return heading+apiState({...dashboardHTTP,status:'loading'},'Loading Go Dashboard…','Reading current Go Inventory and its corresponding backup histories.','retry-dashboard');
 if(dashboardHTTP.status!=='ready')return heading+apiState(dashboardHTTP,'Go Dashboard unavailable',dashboardHTTP.error,'retry-dashboard');
 const values=dashboardValues();
 const latest=values.latest;
 return heading+'<div class="go-dashboard">'+httpEvidence(dashboardHTTP.requests,GO_BACKUP_SOURCE)+dashboardStats(values)+
 (!values.total?'<section class="card api-state"><h2>No Go devices yet</h2><p>The Go Inventory returned an empty collection. No fixture records are shown.</p>'+btn('Retry','retry-dashboard')+'</section>':
 '<div class="dashboard-latest"><span>'+icon('clock')+'Latest saved snapshot · UTC</span>'+(latest?'<button class="text-link" data-action="snapshot" data-device-id="'+esc(latest.device_id)+'" data-backup-id="'+esc(latest.id)+'" data-dashboard-latest="'+esc(latest.created_at)+'">'+date(latest.created_at)+' · '+time(latest.created_at)+'</button>':'<strong>No saved snapshots</strong>')+'<span class="tiny-tag">SYNTHETIC GO DATA</span></div>'+
 '<section class="dashboard-grid">'+groupDistribution(values)+dashboardDeviceList()+'</section>'+dashboardRecent(values))+
 '<aside class="fixture-risk-note">'+icon('shield')+'<div><strong>Risk Review is a separate demonstration</strong><p>FIXTURE DEMO — UNVERIFIED — NOT LINKED TO GO DATA</p></div><a href="#/risk" class="text-link">Open fixture Risk Review '+icon('arrow')+'</a></aside></div>';
}

function inventory(){
 const heading=header('Device inventory','A home for every device.','Synthetic records from the real Go API and disposable SQLite. Live access disabled; risk is not assessed.',btn('Refresh inventory','retry-inventory','primary','','activity')+btn('Export demo JSON','export','default',inventoryHTTP.status==='ready'?'':'disabled','download'));
 if(inventoryHTTP.status==='loading'||inventoryHTTP.status==='idle')return heading+apiState(inventoryHTTP,'Loading inventory…','Fetching synthetic devices from the isolated Aegis Go API.','retry-inventory');
 if(inventoryHTTP.status==='error')return heading+apiState(inventoryHTTP,'Go Inventory API unavailable',inventoryHTTP.error,'retry-inventory');
 const inventoryRecords=inventoryHTTP.records;
 if(!inventoryRecords.length)return heading+apiState(inventoryHTTP,'No devices in the Go demo inventory','The Go API returned an empty device collection. Refresh to check again.','retry-inventory');
 const q=state.search.trim().toLowerCase();
 const rows=inventoryRecords.filter(d=>(!q||[d.display_name,d.management_ip,d.site,d.model,d.role].join(' ').toLowerCase().includes(q))&&(state.site==='all'||d.site===state.site)&&(state.inventoryRisk==='all'||d.risk===state.inventoryRisk));
 return heading+apiSource(inventoryHTTP)+
 '<div class="filterbar"><label class="search-field">'+icon('search')+'<input id="inventory-search" placeholder="Search name, address, or device group…" aria-label="Search device inventory" value="'+esc(state.search)+'"></label><select id="site-filter" aria-label="Filter by device group" data-control="site"><option value="all">All device groups</option>'+[...new Set(inventoryRecords.map(d=>d.site))].map(s=>'<option '+(state.site===s?'selected':'')+'>'+esc(s)+'</option>').join('')+'</select><select aria-label="Filter by impact" data-control="inventory-risk"><option value="all">All impact levels</option>'+['high','medium','unknown','low'].map(v=>'<option value="'+v+'" '+(state.inventoryRisk===v?'selected':'')+'>'+v[0].toUpperCase()+v.slice(1)+'</option>').join('')+'</select><span class="filter-summary">'+rows.length+' of '+inventoryRecords.length+' devices · DEMO</span></div><section class="card">'+(rows.length?'<div class="table-scroll"><table aria-label="Device inventory"><thead><tr><th>Device</th><th>Management address</th><th>Device group</th><th>Demo status</th><th>Potential impact</th><th></th></tr></thead><tbody>'+rows.map(d=>'<tr data-device="'+d.id+'" tabindex="0" aria-label="Open '+d.display_name+'"><td>'+deviceLabel(d)+'</td><td><span class="mono">'+esc(d.management_ip)+'</span><div class="card-subtitle">'+esc(d.model)+'</div></td><td>'+esc(d.site)+'</td><td>'+badge(d.status==='Healthy'?'healthy':d.risk,d.status)+'</td><td>'+badge(d.risk)+'</td><td class="arrow-cell">'+icon('chevron')+'</td></tr>').join('')+'</tbody></table></div>':empty('No matching devices','Try another name, address, device group or impact filter.'))+'</section>';
}
function backupTable(rows){
 if(!rows.length)return '<div class="empty-state">No synthetic snapshots in this section.</div>';
 return '<div class="table-scroll"><table aria-label="Configuration backup history"><thead><tr><th>Snapshot</th><th>Device</th><th>Captured at · UTC</th><th>Configuration</th><th>Source</th><th>Explore</th></tr></thead><tbody>'+rows.map(b=>
  '<tr><td><button class="text-link" data-action="snapshot" data-device-id="'+esc(b.device.id)+'" data-backup-id="'+esc(b.id)+'"><strong>'+esc(b.label)+'</strong></button><div class="card-subtitle mono">'+esc(b.id)+'</div></td><td>'+esc(b.device.display_name)+'</td><td>'+date(b.created_at)+'<div class="card-subtitle">'+time(b.created_at)+'</div></td><td>'+badge('healthy','Running · synthetic')+'</td><td>'+esc(b.origin)+(b.content_hash?'<div class="card-subtitle mono">SHA-256 '+esc(b.content_hash.slice(0,12))+'…</div>':'')+'</td><td><button class="text-link" data-action="snapshot" data-device-id="'+esc(b.device.id)+'" data-backup-id="'+esc(b.id)+'">View</button><span class="muted"> · </span>'+
  (b.browser_memory?'<span class="small muted">Local copy · excluded from Go comparison</span>':'<button class="text-link" data-action="compare" data-device-id="'+esc(b.device.id)+'" data-right="'+esc(b.id)+'">Compare</button>')+'</td></tr>').join('')+'</tbody></table></div>';
}
function backupsPage(route){
 const selected=route.query.get('device')||'all';
 const target=historyHTTP.devices.find(d=>d.id===(selected==='all'?state.selectedDevice:selected))||(selected==='all'?historyHTTP.devices[0]:null);
 return header('Go configuration backups','Your demo history, backed by Go.','Only the four Go demo devices. Metadata in disposable SQLite; synthetic content read by BackupService.',
  btn('Refresh history','retry-backups','default','','activity')+btn('Create demo backup','create-backup','primary','data-device-id="'+esc(target?.id||'')+'" '+(historyHTTP.status==='ready'&&target?'':'disabled'),'plus'))+
  '<div class="filterbar"><select data-control="backups-device" aria-label="Filter backups by device" '+(historyHTTP.devices.length?'':'disabled')+'><option value="all">All Go demo devices</option>'+
  historyHTTP.devices.map(d=>'<option value="'+esc(d.id)+'" '+(d.id===selected?'selected':'')+'>'+esc(d.display_name)+'</option>').join('')+
  '</select><span class="filter-summary">'+historyHTTP.devices.length+' Go devices · DEMO / UNVERIFIED · read-only</span></div>'+backupHistoryBody(selected);
}
function deviceDetail(d){
 const facts=[['Hostname',d.hostname],['Management address',d.management_ip],['Device group',d.device_group],['Platform',d.platform],['Software version',d.version],['Serial number',d.serial],['Connection','Disabled · no live access'],['Data source',GO_SOURCE],['Vendor / protocol',d.vendor+' / '+d.protocol],['Device operations','Disabled · demo record']];
 let body;
 if(state.detailTab==='configuration')body=savedConfigurationBody(d.id);
 else if(state.detailTab==='backups')body='<div class="backup-actions">'+btn('Refresh history','retry-backups','default','','activity')+btn('Create demo backup','create-backup','soft','data-device-id="'+d.id+'"','plus')+'</div>'+backupHistoryBody(d.id);
 else body='<section class="detail-grid"><article class="card">'+cardHead('Device profile','Native Go Inventory record · disposable SQLite.',demoTag())+'<div class="facts-grid">'+facts.map(([label,value])=>'<div><span class="fact-label">'+esc(label)+'</span><span class="fact-value">'+esc(value)+'</span></div>').join('')+'</div><div class="detail-note">'+icon('lock')+'Live connections, credentials, and configuration writes are disabled.</div></article><article class="card">'+cardHead('Go snapshot comparison','Persisted synthetic backups · native BackupService.Diff')+'<div class="aside-body"><p class="small muted">Choose versions from Go backup history. Browser-memory copies and configuration fixtures are separate.</p><a class="text-link" href="#/backups?device='+esc(d.id)+'">Browse Go backups '+icon('arrow')+'</a></div><div class="priority-card-footer">'+icon('shield')+'Risk Review is fixture-only — not linked to this Go profile.'+'</div></article></section>';
 return'<a class="back-link" href="#/devices">'+icon('arrow')+'Back to device inventory</a><section class="page-heading"><div class="device-heading">'+glyph(d)+'<div><div class="eyebrow">'+esc(d.site)+' / DEMO DEVICE</div><h1>'+esc(d.display_name)+'</h1><p>'+esc(d.management_ip)+' &nbsp;·&nbsp; '+esc(d.model)+' &nbsp;·&nbsp; '+badge(d.risk)+'</p></div></div><div class="heading-actions"><button class="btn" disabled title="Live device access is disabled in this demo">'+icon('lock')+'Connect to device</button>'+btn('Go comparison','compare','primary','data-device-id="'+d.id+'"','diff')+'</div></section>'+apiSource(deviceHTTP)+'<nav class="detail-tabs" aria-label="Device detail tabs">'+['overview','configuration','backups','risk'].map(t=>'<button class="'+(state.detailTab===t?'active':'')+'" data-action="detail-tab" data-tab="'+t+'" data-device-id="'+d.id+'">'+({overview:'Overview',configuration:'Configuration',backups:'Backup history',risk:'Risk review'}[t])+'</button>').join('')+'</nav>'+body;
}
function diffLines(left,right){
 const a=left.split('\n'),b=right.split('\n'),dp=Array.from({length:a.length+1},()=>new Uint16Array(b.length+1));
 for(let i=a.length-1;i>=0;i--)for(let j=b.length-1;j>=0;j--)dp[i][j]=a[i]===b[j]?1+dp[i+1][j+1]:Math.max(dp[i+1][j],dp[i][j+1]);
 const ops=[];let i=0,j=0;
 while(i<a.length||j<b.length){
  if(i<a.length&&j<b.length&&a[i]===b[j]){ops.push({kind:'same',left:a[i],right:b[j],ln:++i,rn:++j});}
  else if(i<a.length&&(j===b.length||dp[i+1][j]>=dp[i][j+1])){ops.push({kind:'removed',left:a[i],ln:++i});}
  else{ops.push({kind:'added',right:b[j],rn:++j});}
 }
 const rows=[];
 for(let k=0;k<ops.length;){
  if(ops[k].kind==='same'){rows.push(ops[k++]);continue;}
  const removed=[],added=[];
  while(k<ops.length&&ops[k].kind!=='same'){(ops[k].kind==='removed'?removed:added).push(ops[k++]);}
  for(let n=0;n<Math.max(removed.length,added.length);n++)rows.push({kind:'changed',left:removed[n]?.left,right:added[n]?.right,ln:removed[n]?.ln,rn:added[n]?.rn});
 }
 return{rows,ops,added:ops.filter(x=>x.kind==='added').length,removed:ops.filter(x=>x.kind==='removed').length};
}
function comparePage(route){
 const heading=comparisonHeading(route),info=comparisonHTTP;
 if(info.status==='loading'||info.status==='idle')return heading+'<div class="go-comparison">'+apiState({...info,status:'loading'},'Loading Go comparison…','Reading current history, both Go snapshots and the native Diff response.','retry-compare')+'</div>';
 if(info.status==='empty')return heading+httpEvidence(info.requests,GO_BACKUP_SOURCE)+'<section class="card empty-state"><h2>No Go snapshots to compare</h2><p>Refresh the current Go history to check again.</p>'+btn('Refresh snapshots','refresh-compare')+'</section>';
 if(info.status!=='ready'){
  const title=info.status==='not-found'?'Go selection not found':'Go comparison unavailable';
  const description=info.status==='not-found'?'This device or snapshot selection is not available. A Go restart regenerates snapshot UUIDs. Retry keeps these IDs; Refresh snapshots loads the current Go history.':info.error;
  return heading+'<div class="go-comparison">'+apiState(info,title,description,'retry-compare')+'<a class="back-link" href="#/backups">Open current Go backup history</a></div>';
 }
 const d=info.device,left=info.left,right=info.right,native=info.native;
 const options=selected=>info.records.map(b=>'<option value="'+esc(b.id)+'" '+(b.id===selected.id?'selected':'')+'>'+date(b.created_at)+' · '+esc(b.id.slice(0,8))+'</option>').join('');
 let content='';
 if(state.layout==='split'){
  // Existing browser alignment is presentation only. Counts come from Go below.
  const alignment=diffLines(left.config,right.config);
  const cell=(text,line,kind)=>'<div class="diff-cell '+(text===undefined?'blank':kind)+'"><span class="line-no">'+(line||'')+'</span><span class="line-sign">'+(text===undefined?'':kind==='removed'?'−':kind==='added'?'+':'')+'</span><code class="line-text">'+esc(text??'')+'</code></div>';
  content=alignment.rows.filter(row=>!state.onlyChanges||row.kind!=='same').map(row=>'<div class="diff-row">'+cell(row.left,row.ln,row.kind==='changed'?'removed':'')+cell(row.right,row.rn,row.kind==='changed'?'added':'')+'</div>').join('');
  content='<div class="diff-pane-head"><span>BEFORE · '+date(left.created_at)+'</span><span>AFTER · '+date(right.created_at)+'</span></div><div class="diff-content" aria-label="Side-by-side Go snapshot presentation">'+(content||'<div class="empty-state">No changed lines. Turn off Changes only to see context.</div>')+'</div>';
 }else{
  const raw=native.diff;
  const shown=state.onlyChanges?raw.split('\n').filter(line=>/^(--- |\+\+\+ |@@|[+-]|\\)/.test(line)).join('\n')+(raw.endsWith('\n')?'\n':''):raw;
  content='<div class="unified-caption">Native Go unified diff'+(state.onlyChanges?' · context lines hidden':' · exact response text')+'</div><pre id="native-unified" class="native-unified" aria-label="Native Go unified diff">'+esc(shown)+'</pre>';
 }
 return heading+'<div class="go-comparison">'+httpEvidence(info.requests,GO_BACKUP_SOURCE)+
 '<div class="demo-callout comparison-provenance">'+icon('help')+'DEMO / UNVERIFIED · Counts and Unified diff come from Go BackupService.Diff. Side-by-side uses browser alignment of the same fetched Go snapshots.</div>'+
 '<section class="card comparison-card"><div class="compare-toolbar"><div class="select-group compare-device-group"><label for="compare-device">Device</label><select id="compare-device" data-control="compare-device">'+info.devices.map(x=>'<option value="'+esc(x.id)+'" '+(x.id===d.id?'selected':'')+'>'+esc(x.display_name)+'</option>').join('')+'</select></div>'+
 '<div class="select-group"><label for="compare-left">Before snapshot</label><select id="compare-left" data-control="compare-version">'+options(left)+'</select></div>'+
 '<button class="btn swap" data-action="swap" aria-label="Swap snapshots">'+icon('swap')+'</button>'+
 '<div class="select-group"><label for="compare-right">After snapshot</label><select id="compare-right" data-control="compare-version">'+options(right)+'</select></div></div>'+
 '<div class="diff-heading"><div class="diff-summary" data-native-added="'+native.added_lines+'" data-native-removed="'+native.removed_lines+'"><span class="added-count">+'+native.added_lines+' added</span><span class="removed-count">−'+native.removed_lines+' removed</span><span class="tiny-tag">Native Go counts</span></div>'+
 '<div class="diff-heading-right"><label class="checkbox-label"><input type="checkbox" id="changes-only" '+(state.onlyChanges?'checked':'')+'>Changes only</label><div class="segmented" aria-label="Comparison layout"><button data-action="layout" data-layout="split" class="'+(state.layout==='split'?'active':'')+'">Side by side</button><button data-action="layout" data-layout="unified" class="'+(state.layout==='unified'?'active':'')+'">Unified</button></div></div></div>'+
 (!native.changed?'<div class="no-change-note" role="status">'+icon('check')+'Native Go reports no changes between these selections.</div>':'')+
 content+'<details class="comparison-selection"><summary>Selected Go snapshot IDs</summary><p>Before: <code>'+esc(left.id)+'</code></p><p>After: <code>'+esc(right.id)+'</code></p></details>'+
 '<div class="comparison-foot"><span>'+icon('lock')+'Read-only · Go-backed synthetic snapshots</span>'+demoTag()+'</div></section></div>';
}

function riskPage(route){
 const selected=route.query.get('device')||'all';
 const chosen=selected==='all'?devices:[findDevice(selected)];
 const findings=chosen.flatMap(d=>d.findings.map(f=>({...f,device:d})));
 const filtered=findings.filter(f=>state.riskImpact==='all'||f.impact===state.riskImpact);
 const pending=findings.filter(f=>!state.reviewed.has(findingKey(f.device,f))).length;
 const scope=selected==='all'?'Across the sample workspace':findDevice(selected).display_name;
 const rows=filtered.map(f=>{
  const done=state.reviewed.has(findingKey(f.device,f));
  return'<article class="finding '+(done?'review-complete':'')+'"><div class="finding-top"><span class="finding-symbol">'+icon(f.kind==='uncertainty'?'help':'shield')+'</span><h3>'+esc(f.title)+'</h3>'+badge(done?'reviewed':f.impact)+'</div><p>'+esc(f.description)+'</p><div class="finding-meta"><span>'+esc(f.device.display_name)+' · '+esc(f.kind)+' · '+esc(f.priority)+' review priority</span><button class="text-link" data-action="review-finding" data-device-id="'+f.device.id+'" data-finding-id="'+f.id+'">'+(done?'Undo demo review':'Mark demo reviewed')+'</button></div><details><summary>Explore this sample finding</summary><p>'+esc(f.detail)+'</p><button class="text-link" data-action="compare" data-device-id="'+f.device.id+'">Open Go comparison (seeded devices only) '+icon('arrow')+'</button></details></article>';
 }).join('');
 return header('Change-risk review','A second look, with context.','FIXTURE DEMO — UNVERIFIED — NOT LINKED TO GO DATA',demoTag())+'<div class="demo-callout">'+icon('alert')+'DEMO / UNVERIFIED · No verified risk API is connected. Unknown impact is not low impact. Reviewing a sample does not approve or deploy a change.</div><div class="risk-layout"><section><div class="risk-summary"><div class="risk-score">'+icon('shield')+'</div><div><h2>'+pending+' sample findings to explore</h2><p>'+esc(scope)+'<br>Potential impact is advisory. All results shown here are synthetic.</p></div></div><div class="filterbar"><select data-control="risk-device" aria-label="Filter risk findings by device"><option value="all">All devices</option>'+devices.filter(d=>d.findings.length).map(d=>'<option value="'+d.id+'" '+(d.id===selected?'selected':'')+'>'+esc(d.display_name)+'</option>').join('')+'</select><select data-control="risk-impact" aria-label="Filter findings by impact"><option value="all">All impact levels</option>'+['high','medium','unknown'].map(v=>'<option value="'+v+'" '+(v===state.riskImpact?'selected':'')+'>'+v[0].toUpperCase()+v.slice(1)+'</option>').join('')+'</select><span class="filter-summary">'+filtered.length+' demo findings</span></div><div class="findings">'+(rows||'<div class="card">'+empty('No synthetic findings in this view','This does not mean the device is secure. Choose another sample or impact filter.')+'</div>')+'</div></section><aside class="risk-aside"><article class="card">'+cardHead('Review context','A sample, not an approval.')+'<div class="aside-body"><div class="aside-row"><span>Evidence source</span><strong>DEMO fixtures</strong></div><div class="aside-row"><span>Verified API</span><strong>Not connected</strong></div><div class="aside-row"><span>Scope</span><strong>Browser session</strong></div><div class="aside-row"><span>Device access</span><strong>Disabled</strong></div>'+btn('Mark demo reviewed','review-all','soft','data-device-id="'+selected+'"','check')+'</div></article><article class="card">'+cardHead('Keep the distinction clear','')+'<div class="aside-body"><p class="small muted" style="line-height:1.9">A demo review tracks your exploration here. It creates no authorization, audit evidence, or accepted security result.</p><div class="aside-row"><span>Phase E</span><strong>Deferred / not accepted</strong></div><div class="aside-row"><span>Production release</span><strong>Blocked</strong></div></div></article></aside></div>';
}
function render(){
 const active=document.activeElement;
 const focus=active?.id?{id:active.id,start:active.selectionStart,end:active.selectionEnd}:null;
 const route=routing();
 document.querySelectorAll('[data-nav]').forEach(a=>a.classList.toggle('active',a.dataset.nav===route.view));
 document.querySelector('#breadcrumb').textContent=route.detail?(deviceHTTP.status==='ready'&&deviceHTTP.id===route.requestedDeviceId?deviceHTTP.record.display_name:'Device details'):({overview:'Overview',devices:'Device inventory',backups:'Configuration backups',compare:'Compare configurations',risk:'Risk reviews'}[route.view]);
 document.querySelector('#inventory-count').textContent=knownGoDeviceCount===null?'—':knownGoDeviceCount;
 main.setAttribute('aria-busy',String((route.view==='overview'&&dashboardHTTP.status==='loading')||(route.view==='devices'&&(route.detail?deviceHTTP.status==='loading'||state.detailTab==='backups'&&historyHTTP.status==='loading'||state.detailTab==='configuration'&&configurationHTTP.status==='loading':inventoryHTTP.status==='loading'))||route.view==='backups'&&historyHTTP.status==='loading'||route.view==='compare'&&comparisonHTTP.status==='loading'));
 main.innerHTML=route.detail?inventoryDevicePage(route):({overview:overview,devices:inventory,backups:()=>backupsPage(route),compare:()=>comparePage(route),risk:()=>riskPage(route)}[route.view])();
 iconize();
 if(focus){const next=document.getElementById(focus.id);if(next){next.focus({preventScroll:true});if(typeof focus.start==='number'&&next.setSelectionRange)try{next.setSelectionRange(focus.start,focus.end);}catch{}}}
 document.title='Aegis · '+document.querySelector('#breadcrumb').textContent;
}
let toastTimer;
function toast(message){const el=document.querySelector('#toast');el.textContent=message;el.classList.add('visible');clearTimeout(toastTimer);toastTimer=setTimeout(()=>el.classList.remove('visible'),4200);}
let modalFocus;
function closeModal(){snapshotSequence++;document.querySelector('#modal-root').innerHTML='';modalFocus?.focus?.({preventScroll:true});}
function modal(title,body){
 if(!document.querySelector('#modal-root [role="dialog"]'))modalFocus=document.activeElement;
 document.querySelector('#modal-root').innerHTML='<div class="modal-backdrop"><section class="modal" role="dialog" aria-modal="true" aria-labelledby="modal-title"><div class="modal-head"><h2 id="modal-title">'+esc(title)+'</h2><button id="modal-close" class="icon-button" data-action="close-modal" aria-label="Close dialog">'+icon('close')+'</button></div>'+body+'</section></div>';
 document.querySelector('#modal-close').focus();
}
document.addEventListener('click',event=>{
 if(event.target.classList.contains('modal-backdrop'))return closeModal();
 const action=event.target.closest('[data-action]');
 if(action){
  const name=action.dataset.action,d=findDevice(action.dataset.deviceId||state.selectedDevice);
  if(name==='inventory')go('devices');
  if(name==='retry-dashboard')void loadDashboard();
  if(name==='retry-inventory')void loadInventory();
  if(name==='retry-device')void loadDevice(routing().requestedDeviceId);
  if(name==='retry-configuration')void loadSavedConfiguration(routing().requestedDeviceId,configurationHTTP.selectedId);
  if(name==='refresh-configuration')void loadSavedConfiguration(routing().requestedDeviceId,null);
  if(name==='retry-backups'){const route=routing();void loadBackupHistory(route.detail?route.requestedDeviceId:route.query.get('device')||'all');}
  if(name==='retry-compare')void loadComparison();
  if(name==='refresh-compare'){const requested=routing().query.get('device');const id=comparisonHTTP.devices.some(d=>d.id===requested)?requested:comparisonHTTP.device?.id;const target='/compare'+(id?'?'+new URLSearchParams({device:id}):'');if(location.hash==='#'+target)void loadComparison();else location.hash=target;}
  if(name==='retry-snapshot')void showSnapshot(action.dataset.deviceId,action.dataset.backupId);
  if(name==='about')modal('A safe place to explore.','<p>Aegis brings network inventory, configuration history, comparison and change review into one workspace.</p><p>Inventory and device profiles use the existing Aegis Go API, native authentication/RBAC and a disposable SQLite database through a same-origin adapter. Backup history, snapshot viewing and the device Configuration tab use Go BackupService, disposable SQLite metadata and seeded synthetic files. Comparison uses native Go BackupService.Diff and the same Go snapshot contents; browser alignment is presentation only. Dashboard uses the same Go Inventory and backup histories. Risk Review is a separate fixture demonstration; local copies stay in browser memory. No production backend, device credentials, SSH, production databases or device changes are connected.</p><dl><dt>Phase E</dt><dd>DEFERRED / NOT ACCEPTED</dd><dt>Historical E1</dt><dd>Go cases passed; Controller failed</dd><dt>Production release</dt><dd>BLOCKED</dd><dt>Phase F / Task #7</dt><dd>Not authorized</dd></dl>'+btn('Back to exploring','close-modal'));
  if(name==='activity')go('backups');
  if(name==='close-modal')closeModal();
  if(name==='layout'){state.layout=action.dataset.layout;render();}
  if(name==='reset-filters'){state.search='';state.site='all';state.inventoryRisk='all';state.riskImpact='all';document.querySelector('#global-search').value='';render();}
  if(name==='detail-tab'){
   const id=routing().requestedDeviceId,tab=action.dataset.tab;
   if(tab==='risk')go('risk',{device:id});
   else go('devices/'+encodeURIComponent(id),{tab});
  }
  if(name==='compare')compareTo({id:action.dataset.deviceId||state.selectedDevice},action.dataset.right);
  if(name==='risk')go('risk',{device:d.id});
  if(name==='swap'){const l=document.querySelector('#compare-left').value,r=document.querySelector('#compare-right').value;go('compare',{device:comparisonHTTP.device.id,left:r,right:l});}
  if(name==='snapshot')void showSnapshot(action.dataset.deviceId,action.dataset.backupId);
  if(name==='create-backup'){
   const local=localSnapshots(d.id);
   memorySnapshots.set(d.id,[{id:d.id+'-demo-'+Date.now(),device_id:d.id,source:'SYNTHETIC_DEMO',synthetic:true,browser_memory:true,
    label:'Local demo copy '+String(local.length+1).padStart(2,'0'),created_at:new Date().toISOString(),config_type:'running',
    origin:'Browser memory · fixture copy · NOT persisted',config:d.backups[0].config},...local]);
   render();toast('Synthetic copy created in this tab only. No API write; reload discards it.');
  }
  if(name==='review-finding'){const k=d.id+':'+action.dataset.findingId;if(state.reviewed.has(k))state.reviewed.delete(k);else state.reviewed.add(k);render();toast('Demo review updated in this browser only. No approval was created.');}
  if(name==='review-all'){const chosen=action.dataset.deviceId==='all'?devices:[d];chosen.forEach(x=>x.findings.forEach(f=>state.reviewed.add(findingKey(x,f))));render();toast('Sample findings marked reviewed. No real change was approved.');}
  if(name==='export'){if(inventoryHTTP.status!=='ready')return toast('Load the Go demo inventory before exporting.');const blob=new Blob([JSON.stringify({mode:'SYNTHETIC_DEMO',source:GO_SOURCE,devices:inventoryHTTP.records},null,2)],{type:'application/json'});const url=URL.createObjectURL(blob);const a=document.createElement('a');a.href=url;a.download='aegis-synthetic-device-inventory.json';a.click();setTimeout(()=>URL.revokeObjectURL(url),1000);toast('Exported synthetic inventory only.');}
  return;
 }
 const link=event.target.closest('[data-route]');
 if(link){location.hash=link.dataset.route;return;}
 const device=event.target.closest('[data-device]');
 if(device){state.detailTab='overview';go('devices/'+device.dataset.device);return;}
 const comparison=event.target.closest('[data-compare]');
 if(comparison)compareTo(findDevice(comparison.dataset.compare),comparison.dataset.right);
});
document.addEventListener('input',event=>{
 if(event.target.id==='inventory-search'){state.search=event.target.value;render();}
 if(event.target.id==='global-search'){state.search=event.target.value;state.site='all';state.inventoryRisk='all';if(routing().view!=='devices'||routing().detail)go('devices');else render();}
});
document.addEventListener('change',event=>{
 const el=event.target,control=el.dataset.control;
 if(control==='saved-config-version')void loadSavedConfiguration(routing().requestedDeviceId,el.value);
 if(control==='site'){state.site=el.value;render();}
 if(control==='inventory-risk'){state.inventoryRisk=el.value;render();}
 if(control==='backups-device')go('backups',el.value==='all'?{}:{device:el.value});
 if(control==='compare-device'){state.selectedDevice=el.value;go('compare',{device:el.value});}
 if(control==='compare-version')go('compare',{device:comparisonHTTP.device.id,left:document.querySelector('#compare-left').value,right:document.querySelector('#compare-right').value});
 if(control==='risk-device')go('risk',el.value==='all'?{}:{device:el.value});
 if(control==='risk-impact'){state.riskImpact=el.value;render();}
 if(el.id==='changes-only'){state.onlyChanges=el.checked;render();}
});
document.addEventListener('keydown',event=>{
 const dialog=document.querySelector('[role="dialog"]');
 if(event.key==='Escape'){closeModal();return;}
 if(dialog&&event.key==='Tab'){
  const nodes=dialog.querySelectorAll('button,a,input,select,textarea,[tabindex="0"]'),first=nodes[0],last=nodes[nodes.length-1];
  if(event.shiftKey&&document.activeElement===first){event.preventDefault();last.focus();}
  else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first.focus();}
 }
 if(event.key==='/'&&!dialog&&!['INPUT','TEXTAREA','SELECT'].includes(document.activeElement?.tagName)){event.preventDefault();document.querySelector('#global-search').focus();}
 if((event.key==='Enter'||event.key===' ')&&event.target.matches('[data-device],[data-compare]')){event.preventDefault();event.target.click();}
});
addEventListener('hashchange',()=>{enterRoute();scrollTo(0,0);});
enterRoute();


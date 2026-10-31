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
const state={search:'',site:'all',inventoryRisk:'all',selectedDevice:devices[0].id,detailTab:'overview',chartDays:7,onlyChanges:false,layout:'split',riskImpact:'all',reviewed:new Set()};
const main=document.querySelector('#main');

// HTTP views never fall back to fixture records. Dashboard/risk remain fixture demos.
const GO_SOURCE='Aegis Go API — Isolated Demo';
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
 const source=info.path?.startsWith('/api/go/')?GO_SOURCE:'Local Mock API';
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
 const legacy=devices.find(d=>d.id===deviceHTTP.record.id);
 return deviceDetail({...deviceHTTP.record,backups:legacy?.backups||[],findings:legacy?.findings||[]});
}
// Session-only snapshots are deliberately separate from immutable HTTP history.
const memorySnapshots=new Map();
const localSnapshots=id=>memorySnapshots.get(id)||[];
const historyHTTP={status:'idle',scope:null,records:[],requests:[],sequence:0};
const comparisonHTTP={status:'idle',records:[],requests:[],left:null,right:null,sequence:0};
let snapshotSequence=0;
const backupPath=id=>'/api/mock/v1/devices/'+encodeURIComponent(id)+'/backups';
function isBackup(record,id,withConfig=false){
 return record&&record.device_id===id&&record.source==='SYNTHETIC_DEMO'&&record.synthetic===true&&
  ['id','label','created_at','config_type','origin'].every(key=>typeof record[key]==='string'&&record[key].length>0)&&
  Number.isFinite(Date.parse(record.created_at))&&(!withConfig||
   (typeof record.config==='string'&&record.config.startsWith('! AEGIS SYNTHETIC DEMO — NOT A DEVICE CONFIGURATION\n')));
}
async function backupRequest(path){
 try{
  const result=await requestMockAPI(path);
  return {...result,info:{status:'ready',path,httpStatus:result.httpStatus,meta:result.payload.meta,elapsed:result.elapsed}};
 }catch(error){error.path=path;throw error;}
}
async function readHistory(id){
 const result=await backupRequest(backupPath(id)),records=result.payload.data;
 if(!Array.isArray(records)||!records.every(b=>isBackup(b,id))||new Set(records.map(b=>b.id)).size!==records.length){
  throw Object.assign(new Error('The mock API returned an invalid synthetic backup history.'),{path:backupPath(id)});
 }
 return {records,info:result.info};
}
async function readSnapshot(id,backupId){
 const path=backupPath(id)+'/'+encodeURIComponent(backupId),result=await backupRequest(path);
 if(!isBackup(result.payload.data,id,true)||result.payload.data.id!==backupId){
  throw Object.assign(new Error('The mock API returned an unexpected synthetic snapshot.'),{path});
 }
 return {record:result.payload.data,info:result.info};
}
function backupFailure(error,fallbackPath){
 return {status:error.httpStatus===404?'not-found':'error',path:error.path||fallbackPath,
  httpStatus:error.httpStatus||null,meta:error.meta||null,elapsed:0,
  error:(error.httpStatus||error.path&&error.message.startsWith('The mock API returned'))?error.message:
   'Could not load synthetic backups from the Local Mock API. Check the local preview server, then Retry.'};
}
function httpEvidence(requests){
 return '<div class="api-source api-ready"><div class="api-source-label">'+icon('server')+
  '<strong>Local Mock API</strong><span class="api-status">HTTP 200 · '+requests.length+' GET response'+(requests.length===1?'':'s')+
  '</span></div><details class="api-request-id"><summary>HTTP sources &amp; response request IDs</summary>'+
  requests.map(info=>'<div class="api-source-detail"><a href="'+esc(info.path)+'" target="_blank" rel="noopener">GET '+esc(info.path)+
   '</a><span>'+info.elapsed+' ms · <code>'+esc(info.meta.request_id)+'</code></span></div>').join('')+'</details></div>';
}
async function loadBackupHistory(scope='all'){
 const sequence=++historyHTTP.sequence;
 Object.assign(historyHTTP,{status:'loading',scope,records:[],requests:[],path:backupPath(scope==='all'?devices[0].id:scope),httpStatus:null,meta:null});render();
 try{
  const ids=scope==='all'?devices.map(d=>d.id):[scope];
  const results=await Promise.all(ids.map(async id=>{
   const result=await readHistory(id);
   return {...result,records:result.records.map(b=>({...b,device:devices.find(d=>d.id===id)||{id,display_name:id}}))};
  }));
  if(sequence!==historyHTTP.sequence)return;
  Object.assign(historyHTTP,{status:'ready',records:results.flatMap(x=>x.records).sort((a,b)=>b.created_at.localeCompare(a.created_at)),requests:results.map(x=>x.info)});
 }catch(error){
  if(sequence!==historyHTTP.sequence)return;
  Object.assign(historyHTTP,backupFailure(error,historyHTTP.path),{records:[],requests:[]});
 }
 render();
}
function sessionHistory(scope){
 return devices.filter(d=>scope==='all'||d.id===scope).flatMap(d=>localSnapshots(d.id).map(b=>({...b,device:d})));
}
function backupHistoryBody(scope){
 if(historyHTTP.scope!==scope||historyHTTP.status==='loading'||historyHTTP.status==='idle'){
  return apiState({...historyHTTP,status:'loading'},'Loading backup history…','Fetching synthetic snapshot metadata over HTTP.','retry-backups');
 }
 if(historyHTTP.status!=='ready')return apiState(historyHTTP,historyHTTP.status==='not-found'?'Backup history not found':'Backup API unavailable',historyHTTP.error,'retry-backups');
 const local=sessionHistory(scope);
 return httpEvidence(historyHTTP.requests)+'<section class="card">'+
  cardHead('HTTP backup history',historyHTTP.records.length+' synthetic snapshots · server fixture is read-only',demoTag())+
  (historyHTTP.records.length?backupTable(historyHTTP.records):'<div class="empty-state"><h2>No HTTP snapshots</h2><p>The API returned an empty backup history for this device.</p>'+btn('Retry','retry-backups')+'</div>')+'</section>'+
  '<section class="card session-snapshots">'+cardHead('Browser-memory snapshots',local.length+' local copies · never sent to or persisted by the API','<span class="tiny-tag">THIS TAB ONLY</span>')+
  (local.length?backupTable(local):'<p class="session-note">Create demo backup makes a synthetic copy in this tab only. Reloading removes it. The HTTP backup history stays unchanged.</p>')+'</section>';
}
function comparisonHeading(route){
 return header('Configuration comparison','See what changed. Understand what matters.','Synthetic snapshots fetched over HTTP; differences calculated in this browser.',btn('Review demo impact','risk','primary','data-device-id="'+route.device.id+'"','shield'));
}
async function loadComparison(route=routing()){
 const sequence=++comparisonHTTP.sequence,id=route.query.get('device')||route.device.id;
 Object.assign(comparisonHTTP,{status:'loading',records:[],requests:[],left:null,right:null,path:backupPath(id),httpStatus:null,meta:null});render();
 try{
  const history=await readHistory(id);
  if(sequence!==comparisonHTTP.sequence)return;
  const records=[...localSnapshots(id),...history.records];
  Object.assign(comparisonHTTP,{records,requests:[history.info]});
  if(!records.length){Object.assign(comparisonHTTP,{status:'empty'});render();return;}
  const leftId=route.query.get('left')||records[1]?.id||records[0].id;
  const rightId=route.query.get('right')||records[0].id;
  const selected=await Promise.all([leftId,rightId].map(async backupId=>{
   const local=localSnapshots(id).find(b=>b.id===backupId);
   return local?{record:local,info:null}:readSnapshot(id,backupId);
  }));
  if(sequence!==comparisonHTTP.sequence)return;
  Object.assign(comparisonHTTP,{status:'ready',left:selected[0].record,right:selected[1].record,
   requests:[history.info,...selected.map(x=>x.info).filter(Boolean)]});
 }catch(error){
  if(sequence!==comparisonHTTP.sequence)return;
  Object.assign(comparisonHTTP,backupFailure(error,comparisonHTTP.path),{left:null,right:null});
 }
 render();
}
async function showSnapshot(id,backupId){
 const sequence=++snapshotSequence;
 const local=localSnapshots(id).find(b=>b.id===backupId);
 modal('Synthetic snapshot',local?'<p>Browser-memory copy · not persisted by the API.</p><pre class="config-view">'+esc(local.config)+'</pre>':
  apiState({status:'loading',path:backupPath(id)+'/'+encodeURIComponent(backupId)},'Loading snapshot…','Fetching synthetic configuration text over HTTP.','retry-snapshot'));
 if(local)return;
 try{
  const result=await readSnapshot(id,backupId);
  if(sequence!==snapshotSequence)return;
  modal(result.record.label+' · HTTP snapshot',apiSource(result.info)+'<p>DEMO / UNVERIFIED · Entirely synthetic configuration. Read-only HTTP response.</p><pre class="config-view">'+esc(result.record.config)+'</pre>');
 }catch(error){
  if(sequence!==snapshotSequence)return;
  const info=backupFailure(error,backupPath(id)+'/'+encodeURIComponent(backupId));
  modal('Synthetic snapshot unavailable',apiState(info,info.status==='not-found'?'Snapshot not found':'Snapshot API unavailable',info.error,'retry-snapshot'));
  const retry=document.querySelector('[data-action="retry-snapshot"]');
  if(retry){retry.dataset.deviceId=id;retry.dataset.backupId=backupId;}
 }
}
function enterRoute(){
 const route=routing();
 if(route.view==='devices'){
  if(route.detail){void loadDevice(route.requestedDeviceId);if(state.detailTab==='backups')void loadBackupHistory(route.requestedDeviceId);}
  else void loadInventory();
 }else if(route.view==='backups')void loadBackupHistory(route.query.get('device')||'all');
 else if(route.view==='compare')void loadComparison(route);
 else render();
}

const date=(value)=>new Intl.DateTimeFormat('en-GB',{day:'2-digit',month:'short',year:'numeric',timeZone:'UTC'}).format(new Date(value));
const time=(value)=>new Intl.DateTimeFormat('en-GB',{hour:'2-digit',minute:'2-digit',timeZone:'UTC'}).format(new Date(value))+' UTC';
const findDevice=id=>devices.find(d=>d.id===id)||devices[0];
const allBackups=()=>devices.flatMap(d=>[...localSnapshots(d.id),...d.backups].map(b=>({...b,device:d}))).sort((a,b)=>b.created_at.localeCompare(a.created_at));
const findingKey=(d,f)=>d.id+':'+f.id;
const pendingDevices=()=>devices.filter(d=>d.findings.some(f=>!state.reviewed.has(findingKey(d,f))));
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
 const records=[...localSnapshots(d.id),...(historyHTTP.status==='ready'?historyHTTP.records.filter(b=>b.device_id===d.id):[])];
 const right=records.find(b=>b.id===rightId);
 const index=records.indexOf(right);
 const left=records.find(b=>b.id===leftId)||(right?records[index+1]||records.at(-1):null);
 const target=new URLSearchParams({device:d.id,...(rightId?{right:rightId}:{}),...(left?{left:left.id}:{})}).toString();
 if(location.hash==='/compare?'+target)void loadComparison();
 else go('compare',Object.fromEntries(new URLSearchParams(target)));
}
function stats(){
 const pending=pendingDevices().length;
 return'<section class="stats-grid" aria-label="Demo workspace summary">'+[
 ['Managed devices',devices.length,'Across 3 synthetic sites','server',false],
 ['Saved snapshots',allBackups().length,'Synthetic configuration backups','archive',false],
 ['Reviews to explore',pending,pending?'Prewritten demo findings':'All demo findings reviewed','shield',true],
 ['Demo backup coverage','100<small>%</small>','12 of 12 sample devices','check',false]
 ].map(([label,number,foot,symbol,accent])=>'<article class="stat '+(accent?'stat-accent':'')+'"><div class="stat-label">'+label+'<span class="stat-icon">'+icon(symbol)+'</span></div><div class="stat-number">'+number+'</div><div class="stat-foot"><i class="mini-dot"></i>'+foot+'</div></article>').join('')+'</section>';
}
function chart(){
 const days=state.chartDays;
 const end=new Date('2026-10-28T00:00:00Z');
 const counts=Array.from({length:days},(_,i)=>{
  const at=new Date(end);at.setUTCDate(at.getUTCDate()-(days-1-i));
  const key=at.toISOString().slice(0,10);
  return{day:at,count:allBackups().filter(b=>b.created_at.startsWith(key)).length};
 });
 const max=Math.max(4,...counts.map(x=>x.count));const width=550/days;const total=counts.reduce((s,x)=>s+x.count,0);
 let svg='<svg class="chart-svg" viewBox="0 0 640 180" role="img" aria-label="Synthetic backup activity over '+days+' days">';
 for(let i=0;i<4;i++){const y=20+i*40;svg+='<line x1="39" y1="'+y+'" x2="622" y2="'+y+'" class="chart-grid"/><text x="8" y="'+(y+3)+'" class="chart-label">'+Math.round(max*(1-i/3))+'</text>';}
 counts.forEach((row,i)=>{const h=row.count/max*115,x=50+i*width+(width-Math.min(26,width*.55))/2;
 svg+='<rect x="'+x+'" y="'+(140-h)+'" width="'+Math.min(26,width*.55)+'" height="'+Math.max(h,2)+'" rx="3" fill="'+(i===days-1?'#58784c':'#c7d7af')+'"><title>'+date(row.day)+': '+row.count+' synthetic backups</title></rect>';
 if(days===7||i%6===0||i===days-1)svg+='<text x="'+(50+i*width+width/2)+'" y="164" text-anchor="middle" class="chart-label">'+new Intl.DateTimeFormat('en',{month:'short',day:'numeric',timeZone:'UTC'}).format(row.day)+'</text>';
 });svg+='</svg>';
 return'<article class="card">'+cardHead('Backup activity','A little visibility goes a long way.','<div class="segmented" aria-label="Activity period"><button data-action="chart" data-days="7" class="'+(days===7?'active':'')+'">7 days</button><button data-action="chart" data-days="30" class="'+(days===30?'active':'')+'">30 days</button></div>')+'<div class="chart-wrap"><div class="chart-summary"><strong>'+total+'</strong><span>demo snapshots in this period</span></div>'+svg+'<div class="chart-legend"><span><i class="legend-dot"></i>Saved snapshots</span><span><i class="legend-dot dark"></i>Latest sample day</span></div></div><div class="chart-footer"><span>All activity is synthetic</span><a href="#/backups" class="text-link">Explore backup history '+icon('arrow')+'</a></div></article>';
}
function topology(){
 const nodes=[{x:175,y:85,id:'core-sw-01',name:'CORE-SW-01',core:true},{x:65,y:44,id:'access-sw-01',name:'Northstar HQ'},{x:285,y:44,id:'edge-rtr-01',name:'Harbor DC'},{x:65,y:140,id:'access-sw-04',name:'Cedar Branch'},{x:285,y:140,id:'core-sw-02',name:'CORE-SW-02'}];
 let svg='<svg class="topology-svg" viewBox="0 0 350 195" role="img" aria-label="Interactive synthetic network map">';
 svg+='<defs><pattern id="map-grid" width="14" height="14" patternUnits="userSpaceOnUse"><circle cx="1" cy="1" r=".6" fill="#dce8cd"/></pattern></defs><rect width="350" height="195" fill="url(#map-grid)"/>';
 for(const n of nodes.slice(1))svg+='<path d="M175 85 Q175 '+n.y+' '+n.x+' '+n.y+'" fill="none" stroke="#b7c9a2" stroke-width="1.2" stroke-dasharray="4 4"/>';
 for(const n of nodes)svg+='<g class="topo-node" role="link" tabindex="0" data-device="'+n.id+'" aria-label="Open '+n.id+'"><rect class="node-box" x="'+(n.x-20)+'" y="'+(n.y-17)+'" width="40" height="34" rx="9" fill="'+(n.core?'#466b3e':'#ffffff')+'" stroke="'+(n.core?'#466b3e':'#d6e2c9')+'"/><g transform="translate('+(n.x-10)+' '+(n.y-10)+') scale(.84)" stroke="'+(n.core?'#e5f4d1':'#8ba773')+'" fill="none" stroke-width="1.5" stroke-linecap="round">'+shapes.server+'</g><circle cx="'+(n.x+17)+'" cy="'+(n.y-14)+'" r="4" fill="'+(n.core?'#d3ac65':'#9dbb7b')+'" stroke="white" stroke-width="2"/><text x="'+n.x+'" y="'+(n.y+31)+'" text-anchor="middle" class="topology-label">'+n.name+'</text></g>';
 svg+='</svg>';
 return'<article class="card map-card">'+cardHead('Your network, at a glance','Select a node to explore.','<span class="tiny-tag">DEMO TOPOLOGY</span>')+svg+'<div class="map-footer"><span><i class="mini-dot"></i> 3 synthetic sites</span><span>No live discovery</span></div></article>';
}
function overview(){
 const pending=pendingDevices();
 const recent=devices.filter(d=>d.risk!=='low');
 const table='<table aria-label="Recent demo changes"><thead><tr><th>Device</th><th>Configuration change</th><th>Potential impact</th><th>Snapshot</th><th></th></tr></thead><tbody>'+recent.map(d=>'<tr tabindex="0" data-compare="'+d.id+'"><td>'+deviceLabel(d)+'</td><td>'+esc(d.change_summary)+'</td><td>'+badge(d.risk)+'</td><td>'+time(d.backups[0].created_at)+'</td><td class="arrow-cell">'+icon('chevron')+'</td></tr>').join('')+'</tbody></table>';
 const priority=pending.slice(0,3).map(d=>'<button class="priority-item" data-route="/risk?device='+d.id+'"><span class="priority-symbol">'+icon(d.risk==='unknown'?'help':'shield')+'</span><span><strong>'+esc(d.findings[0].title)+'</strong><p>'+esc(d.display_name)+' · '+esc(d.site)+'</p>'+badge(d.risk)+'</span>'+icon('chevron')+'</button>').join('');
 return header('A little clarity. A lot of confidence.','Your network. In focus.','Explore your devices, understand changes, and make room for better decisions.','<span class="muted small">Sample · 28 Oct 2026</span>'+btn('View inventory','inventory','primary','','arrow'))+stats()+'<section class="dashboard-grid">'+chart()+topology()+'</section><section class="dashboard-grid"><article class="card table-card">'+cardHead('Recent configuration changes','A synthetic change queue, ready to explore.',demoTag())+'<div class="table-scroll">'+table+'</div></article><article class="card">'+cardHead('Needs a closer look',pending.length+' demo devices in the review queue','<a href="#/risk" class="text-link">View all '+icon('arrow')+'</a>')+'<div class="priority-list">'+(priority||'<div class="empty-state">All demo findings reviewed.</div>')+'</div><div class="priority-card-footer">'+icon('help')+'Sample findings, not a real security assessment.</div></article></section><div class="card activity-strip"><span>'+icon('archive')+'Latest demo snapshot · '+esc(devices[0].display_name)+'</span><span>'+icon('lock')+'Live device connections disabled</span><button class="text-link" data-action="activity">View demo activity '+icon('arrow')+'</button></div>';
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
 if(!rows.length)return empty('No demo snapshots','Try selecting another sample device.');
 return'<div class="table-scroll"><table aria-label="Configuration backup history"><thead><tr><th>Snapshot</th><th>Device</th><th>Captured at · UTC</th><th>Configuration</th><th>Source</th><th>Explore</th></tr></thead><tbody>'+rows.map(b=>'<tr data-compare="'+b.device.id+'" data-right="'+b.id+'" tabindex="0"><td><strong>'+esc(b.label)+'</strong><div class="card-subtitle mono">'+esc(b.id.slice(-12))+'</div></td><td>'+esc(b.device.display_name)+'</td><td>'+date(b.created_at)+'<div class="card-subtitle">'+time(b.created_at)+'</div></td><td>'+badge('healthy','Running · demo')+'</td><td>'+esc(b.origin)+'</td><td><button class="text-link" data-action="snapshot" data-device-id="'+b.device.id+'" data-backup-id="'+b.id+'">View</button><span class="muted"> &nbsp;·&nbsp; </span><button class="text-link" data-action="compare" data-device-id="'+b.device.id+'" data-right="'+b.id+'">Compare '+icon('arrow')+'</button></td></tr>').join('')+'</tbody></table></div>';
}
function backupsPage(route){
 const selected=route.query.get('device')||'all';
 return header('Configuration backups','A little history. A clearer picture.','Read-only synthetic history from HTTP. Local demo copies stay in this browser.',
  btn('Refresh history','retry-backups','default','','activity')+btn('Create demo backup','create-backup','primary','data-device-id="'+(selected==='all'?state.selectedDevice:selected)+'"','plus'))+
  '<div class="filterbar"><select data-control="backups-device" aria-label="Filter backups by device"><option value="all">All devices</option>'+
  devices.map(d=>'<option value="'+d.id+'" '+(d.id===selected?'selected':'')+'>'+esc(d.display_name)+'</option>').join('')+
  '</select><span class="filter-summary">DEMO / UNVERIFIED · no server writes</span></div>'+backupHistoryBody(selected);
}
function deviceDetail(d){
 const facts=[['Hostname',d.hostname],['Management address',d.management_ip],['Device group',d.device_group],['Platform',d.platform],['Software version',d.version],['Serial number',d.serial],['Connection','Disabled · no live access'],['Data source',GO_SOURCE],['Vendor / protocol',d.vendor+' / '+d.protocol],['Device operations','Disabled · demo record']];
 let body;
 if(state.detailTab==='configuration')body='<section class="card">'+cardHead('Node DEMO configuration · separate from Go','Static synthetic fixture text. Go configuration reads are not connected.',demoTag())+'<pre class="config-view">'+esc(d.backups[0]?.config||'No synthetic configuration snapshot is available.')+'</pre></section>';
 else if(state.detailTab==='backups')body='<div class="backup-actions">'+btn('Refresh history','retry-backups','default','','activity')+btn('Create demo backup','create-backup','soft','data-device-id="'+d.id+'"','plus')+'</div>'+backupHistoryBody(d.id);
 else body='<section class="detail-grid"><article class="card">'+cardHead('Device profile','A sample record aligned with the inventory API.',demoTag())+'<div class="facts-grid">'+facts.map(([label,value])=>'<div><span class="fact-label">'+esc(label)+'</span><span class="fact-value">'+esc(value)+'</span></div>').join('')+'</div><div class="detail-note">'+icon('lock')+'Live connections, credentials, and configuration writes are disabled.</div></article><article class="card">'+cardHead('Demo configuration timeline','Separate Node synthetic backup flow · not backed by Go.')+'<div class="priority-list">'+d.backups.slice(0,3).map((b,i)=>'<button class="priority-item" data-action="compare" data-device-id="'+d.id+'" data-right="'+b.id+'"><span class="device-glyph">'+icon('archive')+'</span><span><strong>'+esc(b.label)+(i===0?' · latest':'')+'</strong><p>'+date(b.created_at)+' · '+time(b.created_at)+'</p><span class="small muted">Synthetic running configuration</span></span>'+icon('chevron')+'</button>').join('')+'</div><div class="priority-card-footer">'+icon('shield')+d.findings.length+' prewritten demo findings · '+badge(d.risk)+'</div></article></section>';
 return'<a class="back-link" href="#/devices">'+icon('arrow')+'Back to device inventory</a><section class="page-heading"><div class="device-heading">'+glyph(d)+'<div><div class="eyebrow">'+esc(d.site)+' / DEMO DEVICE</div><h1>'+esc(d.display_name)+'</h1><p>'+esc(d.management_ip)+' &nbsp;·&nbsp; '+esc(d.model)+' &nbsp;·&nbsp; '+badge(d.risk)+'</p></div></div><div class="heading-actions"><button class="btn" disabled title="Live device access is disabled in this demo">'+icon('lock')+'Connect to device</button>'+btn('Compare snapshots','compare','primary','data-device-id="'+d.id+'"','diff')+'</div></section>'+apiSource(deviceHTTP)+'<nav class="detail-tabs" aria-label="Device detail tabs">'+['overview','configuration','backups','risk'].map(t=>'<button class="'+(state.detailTab===t?'active':'')+'" data-action="detail-tab" data-tab="'+t+'" data-device-id="'+d.id+'">'+({overview:'Overview',configuration:'Configuration',backups:'Backup history',risk:'Risk review'}[t])+'</button>').join('')+'</nav>'+body;
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
 const d=route.device;
 const heading=comparisonHeading(route);
 if(comparisonHTTP.status==='loading'||comparisonHTTP.status==='idle')return heading+apiState({...comparisonHTTP,status:'loading'},'Loading comparison…','Fetching history and the selected synthetic snapshots over HTTP.','retry-compare');
 if(comparisonHTTP.status==='empty')return heading+httpEvidence(comparisonHTTP.requests)+'<section class="card empty-state"><h2>No snapshots to compare</h2><p>This device has no synthetic backup history.</p>'+btn('Retry','retry-compare')+'</section>';
 if(comparisonHTTP.status!=='ready')return heading+apiState(comparisonHTTP,comparisonHTTP.status==='not-found'?'Snapshot not found':'Comparison API unavailable',comparisonHTTP.error,'retry-compare')+'<a class="back-link" href="#/backups">Choose a snapshot from backup history</a>';
 const left=comparisonHTTP.left,right=comparisonHTTP.right;
 const diff=diffLines(left.config,right.config);
 const options=selected=>comparisonHTTP.records.map(b=>'<option value="'+esc(b.id)+'" '+(b.id===selected.id?'selected':'')+'>'+esc(b.label)+(b.browser_memory?' · BROWSER MEMORY':' · HTTP')+' · '+date(b.created_at)+'</option>').join('');
 let lines='';
 if(state.layout==='split')lines=diff.rows.filter(r=>!state.onlyChanges||r.kind!=='same').map(r=>'<div class="diff-row"><div class="diff-cell '+(r.left===undefined?'blank':r.kind==='changed'?'removed':'')+'"><span class="line-no">'+(r.ln||'')+'</span><span class="line-sign">'+(r.left!==undefined&&r.kind==='changed'?'−':'')+'</span>'+esc(r.left??'')+'</div><div class="diff-cell '+(r.right===undefined?'blank':r.kind==='changed'?'added':'')+'"><span class="line-no">'+(r.rn||'')+'</span><span class="line-sign">'+(r.right!==undefined&&r.kind==='changed'?'+':'')+'</span>'+esc(r.right??'')+'</div></div>').join('');
 else lines=diff.ops.filter(r=>!state.onlyChanges||r.kind!=='same').map(r=>'<div class="unified-row '+r.kind+'"><span class="line-no">'+(r.ln||r.rn||'')+'</span><span class="line-sign">'+(r.kind==='removed'?'−':r.kind==='added'?'+':' ')+'</span>'+esc(r.left??r.right)+'</div>').join('');
 return heading+httpEvidence(comparisonHTTP.requests)+([left,right].some(b=>b.browser_memory)?'<div class="demo-callout">'+icon('archive')+'This comparison includes a browser-memory copy. It was not fetched from or persisted by the API.</div>':'')+'<div class="demo-callout">'+icon('help')+'DEMO · All configuration text is synthetic. Highlighted changes are calculated from these sample snapshots, not a live device.</div><section class="card"><div class="compare-toolbar"><div class="select-group"><label for="compare-device">Device</label><select id="compare-device" data-control="compare-device">'+devices.map(x=>'<option value="'+x.id+'" '+(x.id===d.id?'selected':'')+'>'+esc(x.display_name)+'</option>').join('')+'</select></div><div class="select-group"><label for="compare-left">Before snapshot</label><select id="compare-left" data-control="compare-version">'+options(left)+'</select></div><button class="btn swap" data-action="swap" aria-label="Swap snapshots">'+icon('swap')+'</button><div class="select-group"><label for="compare-right">After snapshot</label><select id="compare-right" data-control="compare-version">'+options(right)+'</select></div></div><div class="diff-heading"><div class="diff-summary"><span class="added-count">+'+diff.added+' added</span><span class="removed-count">−'+diff.removed+' removed</span><span>'+esc(d.display_name)+'</span></div><div class="diff-heading-right"><label class="checkbox-label"><input type="checkbox" id="changes-only" '+(state.onlyChanges?'checked':'')+'>Changes only</label><div class="segmented" aria-label="Comparison layout"><button data-action="layout" data-layout="split" class="'+(state.layout==='split'?'active':'')+'">Side by side</button><button data-action="layout" data-layout="unified" class="'+(state.layout==='unified'?'active':'')+'">Unified</button></div></div></div>'+(state.layout==='split'?'<div class="diff-pane-head"><span>'+esc(left.label)+' · BEFORE</span><span>'+esc(right.label)+' · AFTER</span></div>':'')+'<div class="diff-content" aria-label="Configuration difference">'+(lines||'<div class="empty-state">These snapshots are identical. Turn off Changes only to see context.</div>')+'</div><div class="comparison-foot"><span>'+icon('lock')+'Read-only comparison · Nothing is applied to a device</span>'+demoTag()+'</div></section>';
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
  return'<article class="finding '+(done?'review-complete':'')+'"><div class="finding-top"><span class="finding-symbol">'+icon(f.kind==='uncertainty'?'help':'shield')+'</span><h3>'+esc(f.title)+'</h3>'+badge(done?'reviewed':f.impact)+'</div><p>'+esc(f.description)+'</p><div class="finding-meta"><span>'+esc(f.device.display_name)+' · '+esc(f.kind)+' · '+esc(f.priority)+' review priority</span><button class="text-link" data-action="review-finding" data-device-id="'+f.device.id+'" data-finding-id="'+f.id+'">'+(done?'Undo demo review':'Mark demo reviewed')+'</button></div><details><summary>Explore this sample finding</summary><p>'+esc(f.detail)+'</p><button class="text-link" data-action="compare" data-device-id="'+f.device.id+'">Open synthetic configuration comparison '+icon('arrow')+'</button></details></article>';
 }).join('');
 return header('Change-risk review','A second look, with context.','Prewritten sample findings illustrate a review workflow. They are not real security assessments.',demoTag())+'<div class="demo-callout">'+icon('alert')+'DEMO / UNVERIFIED · No verified risk API is connected. Unknown impact is not low impact. Reviewing a sample does not approve or deploy a change.</div><div class="risk-layout"><section><div class="risk-summary"><div class="risk-score">'+icon('shield')+'</div><div><h2>'+pending+' sample findings to explore</h2><p>'+esc(scope)+'<br>Potential impact is advisory. All results shown here are synthetic.</p></div></div><div class="filterbar"><select data-control="risk-device" aria-label="Filter risk findings by device"><option value="all">All devices</option>'+devices.filter(d=>d.findings.length).map(d=>'<option value="'+d.id+'" '+(d.id===selected?'selected':'')+'>'+esc(d.display_name)+'</option>').join('')+'</select><select data-control="risk-impact" aria-label="Filter findings by impact"><option value="all">All impact levels</option>'+['high','medium','unknown'].map(v=>'<option value="'+v+'" '+(v===state.riskImpact?'selected':'')+'>'+v[0].toUpperCase()+v.slice(1)+'</option>').join('')+'</select><span class="filter-summary">'+filtered.length+' demo findings</span></div><div class="findings">'+(rows||'<div class="card">'+empty('No synthetic findings in this view','This does not mean the device is secure. Choose another sample or impact filter.')+'</div>')+'</div></section><aside class="risk-aside"><article class="card">'+cardHead('Review context','A sample, not an approval.')+'<div class="aside-body"><div class="aside-row"><span>Evidence source</span><strong>DEMO fixtures</strong></div><div class="aside-row"><span>Verified API</span><strong>Not connected</strong></div><div class="aside-row"><span>Scope</span><strong>Browser session</strong></div><div class="aside-row"><span>Device access</span><strong>Disabled</strong></div>'+btn('Mark demo reviewed','review-all','soft','data-device-id="'+selected+'"','check')+'</div></article><article class="card">'+cardHead('Keep the distinction clear','')+'<div class="aside-body"><p class="small muted" style="line-height:1.9">A demo review tracks your exploration here. It creates no authorization, audit evidence, or accepted security result.</p><div class="aside-row"><span>Phase E</span><strong>Deferred / not accepted</strong></div><div class="aside-row"><span>Production release</span><strong>Blocked</strong></div></div></article></aside></div>';
}
function render(){
 const active=document.activeElement;
 const focus=active?.id?{id:active.id,start:active.selectionStart,end:active.selectionEnd}:null;
 const route=routing();
 document.querySelectorAll('[data-nav]').forEach(a=>a.classList.toggle('active',a.dataset.nav===route.view));
 document.querySelector('#breadcrumb').textContent=route.detail?(deviceHTTP.status==='ready'&&deviceHTTP.id===route.requestedDeviceId?deviceHTTP.record.display_name:'Device details'):({overview:'Overview',devices:'Device inventory',backups:'Configuration backups',compare:'Compare configurations',risk:'Risk reviews'}[route.view]);
 document.querySelector('#review-count').textContent=pendingDevices().length;
 document.querySelector('.nav-count').textContent=inventoryHTTP.status==='ready'?inventoryHTTP.records.length:'—';
 main.setAttribute('aria-busy',String((route.view==='devices'&&(route.detail?deviceHTTP.status==='loading'||state.detailTab==='backups'&&historyHTTP.status==='loading':inventoryHTTP.status==='loading'))||route.view==='backups'&&historyHTTP.status==='loading'||route.view==='compare'&&comparisonHTTP.status==='loading'));
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
  if(name==='retry-inventory')void loadInventory();
  if(name==='retry-device')void loadDevice(routing().requestedDeviceId);
  if(name==='retry-backups'){const route=routing();void loadBackupHistory(route.detail?route.requestedDeviceId:route.query.get('device')||'all');}
  if(name==='retry-compare')void loadComparison();
  if(name==='retry-snapshot')void showSnapshot(action.dataset.deviceId,action.dataset.backupId);
  if(name==='about')modal('A safe place to explore.','<p>Aegis brings network inventory, configuration history, comparison and change review into one workspace.</p><p>Inventory and device profiles use the existing Aegis Go API, native authentication/RBAC and a disposable SQLite database through a same-origin adapter. Backup history and comparison remain on the Node synthetic HTTP API. Dashboard and risk views retain fixtures; local snapshot copies stay in browser memory. No production backend, device credentials, SSH, production databases or device changes are connected.</p><dl><dt>Phase E</dt><dd>DEFERRED / NOT ACCEPTED</dd><dt>Historical E1</dt><dd>Go cases passed; Controller failed</dd><dt>Production release</dt><dd>BLOCKED</dd><dt>Phase F / Task #7</dt><dd>Not authorized</dd></dl>'+btn('Back to exploring','close-modal'));
  if(name==='activity')modal('Demo activity','<p>A sample activity stream. These are fictional events.</p>'+allBackups().slice(0,4).map(b=>'<div class="activity-modal-row">'+icon('archive')+' '+esc(b.device.display_name)+' · '+esc(b.label)+'<small>'+date(b.created_at)+' · '+time(b.created_at)+' · '+esc(b.origin)+'</small></div>').join(''));
  if(name==='close-modal')closeModal();
  if(name==='chart'){state.chartDays=Number(action.dataset.days);render();}
  if(name==='layout'){state.layout=action.dataset.layout;render();}
  if(name==='reset-filters'){state.search='';state.site='all';state.inventoryRisk='all';state.riskImpact='all';document.querySelector('#global-search').value='';render();}
  if(name==='detail-tab'){if(action.dataset.tab==='risk'){go('risk',{device:d.id});}else{state.detailTab=action.dataset.tab;if(state.detailTab==='backups')void loadBackupHistory(d.id);else render();}}
  if(name==='compare')compareTo(d,action.dataset.right);
  if(name==='risk')go('risk',{device:d.id});
  if(name==='swap'){const l=document.querySelector('#compare-left').value,r=document.querySelector('#compare-right').value;go('compare',{device:d.id,left:r,right:l});}
  if(name==='snapshot')void showSnapshot(d.id,action.dataset.backupId);
  if(name==='create-backup'){
   const local=localSnapshots(d.id);
   memorySnapshots.set(d.id,[{id:d.id+'-demo-'+Date.now(),device_id:d.id,source:'SYNTHETIC_DEMO',synthetic:true,browser_memory:true,
    label:'Local demo copy '+String(local.length+1).padStart(2,'0'),created_at:new Date().toISOString(),config_type:'running',
    origin:'Browser memory · NOT persisted',config:d.backups[0].config},...local]);
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
 if(control==='site'){state.site=el.value;render();}
 if(control==='inventory-risk'){state.inventoryRisk=el.value;render();}
 if(control==='backups-device')go('backups',el.value==='all'?{}:{device:el.value});
 if(control==='compare-device'){state.selectedDevice=el.value;compareTo(findDevice(el.value));}
 if(control==='compare-version')go('compare',{device:state.selectedDevice,left:document.querySelector('#compare-left').value,right:document.querySelector('#compare-right').value});
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


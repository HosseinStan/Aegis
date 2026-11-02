export const INVENTORY_SOURCE='Aegis Go API — Isolated Demo';
export const BACKUP_SOURCE='Aegis Go API — Disposable SQLite Demo';
export const previewOrigin=__AEGIS_PREVIEW_ORIGIN__;
export const previewLink=(route='/overview')=>previewOrigin+'/#'+route;

export class DemoAPIError extends Error {
  constructor(message,{status=null,requestId=null,path=null}={}){
    super(message);this.status=status;this.requestId=requestId;this.path=path;
  }
}
async function read(path,source,signal){
  let response,payload;
  const started=performance.now();
  try{
    response=await fetch(path,{method:'GET',cache:'no-store',credentials:'omit',signal:AbortSignal.any([signal,AbortSignal.timeout(8000)])});
    payload=await response.json();
  }catch(error){
    if(signal.aborted)throw error;
    throw new DemoAPIError('The isolated Go API is unavailable. Check the local demo and Retry. No fixture fallback is used.',{path,status:response?.status});
  }
  const evidence={path,status:response.status,requestId:payload.meta?.request_id||payload.error?.request_id,source,elapsed:Math.round(performance.now()-started),upstream:payload.meta?.upstream_path};
  if(!response.ok)throw new DemoAPIError(payload.error?.message||'The Go API could not complete this request.',{...evidence});
  if(payload.meta?.synthetic!==true||payload.meta?.source!==source||typeof evidence.requestId!=='string'){
    throw new DemoAPIError('The response did not identify the expected isolated synthetic Go demo.',{...evidence});
  }
  return {data:payload.data,evidence};
}
function isDevice(d){
  return d&&d.source==='AEGIS_GO_ISOLATED_DEMO'&&d.enabled===false&&d.vendor==='mock'&&d.protocol==='mock'&&
    ['id','display_name','hostname','management_ip','device_group','platform'].every(key=>typeof d[key]==='string');
}
export async function getDevices(signal){
  const result=await read('/api/go/v1/devices',INVENTORY_SOURCE,signal);
  if(!Array.isArray(result.data)||!result.data.every(isDevice)||new Set(result.data.map(d=>d.id)).size!==result.data.length){
    throw new DemoAPIError('The Go demo returned an unexpected device collection.',result.evidence);
  }
  return {devices:result.data,requests:[result.evidence]};
}
export async function getDevice(id,signal){
  const result=await read('/api/go/v1/devices/'+encodeURIComponent(id),INVENTORY_SOURCE,signal);
  if(!isDevice(result.data)||result.data.id!==id)throw new DemoAPIError('The Go demo returned an unexpected device profile.',result.evidence);
  return {device:result.data,requests:[result.evidence]};
}
function isSavedBackup(b,deviceId){
  return b&&b.device_id===deviceId&&b.source==='AEGIS_GO_BACKUP_DEMO'&&b.synthetic===true&&b.config_type==='running'&&b.trigger_type==='demo_seed'&&
    typeof b.id==='string'&&/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i.test(b.id)&&
    typeof b.content_hash==='string'&&/^[a-f0-9]{64}$/.test(b.content_hash)&&Number.isSafeInteger(b.size_bytes)&&b.size_bytes>=0&&
    ['created_at','retrieved_at'].every(key=>typeof b[key]==='string'&&Number.isFinite(Date.parse(b[key])));
}
export async function getBackupHistory(deviceId,signal){
  const result=await read('/api/go/v1/devices/'+encodeURIComponent(deviceId)+'/backups',BACKUP_SOURCE,signal);
  if(!Array.isArray(result.data)||!result.data.every(b=>isSavedBackup(b,deviceId))||new Set(result.data.map(b=>b.id)).size!==result.data.length){
    throw new DemoAPIError('The Go demo returned unexpected backup metadata.',result.evidence);
  }
  return {backups:[...result.data].sort((a,b)=>b.created_at.localeCompare(a.created_at)),requests:[result.evidence]};
}
async function getHistory(device,signal){
  const history=await getBackupHistory(device.id,signal);
  return {backups:history.backups.map(backup=>({...backup,device})),request:history.requests[0]};
}
export async function getSavedConfiguration(deviceId,requestedId,signal){
  let wanted=requestedId||null;
  try{
    const history=await getBackupHistory(deviceId,signal),versions=history.backups;
    if(!wanted&&!versions.length)return {deviceId,versions,selected:null,requests:history.requests};
    wanted=wanted||versions[0].id;
    const path='/api/go/v1/devices/'+encodeURIComponent(deviceId)+'/backups/'+encodeURIComponent(wanted);
    const content=await read(path,BACKUP_SOURCE,signal),selected=content.data;
    if(!isSavedBackup(selected,deviceId)||selected.id!==wanted||typeof selected.config!=='string'||
       !selected.config.startsWith('! AEGIS SYNTHETIC DEMO — DISPOSABLE BACKUP SNAPSHOT\n')){
      throw new DemoAPIError('The Go API returned an unexpected synthetic saved configuration.',content.evidence);
    }
    const metadata=versions.find(backup=>backup.id===wanted);
    if(!metadata)throw new DemoAPIError('This UUID is not in the device’s current saved running-configuration history.',{...content.evidence,status:null});
    const immutable=['id','device_id','source','synthetic','config_type','trigger_type','created_at','retrieved_at','content_hash','size_bytes'];
    if(immutable.some(key=>metadata[key]!==selected[key]))throw new DemoAPIError('Saved content metadata does not match the Go history record.',content.evidence);
    const bytes=new TextEncoder().encode(selected.config);
    const digest=await crypto.subtle.digest('SHA-256',bytes);
    const sha256=Array.from(new Uint8Array(digest),byte=>byte.toString(16).padStart(2,'0')).join('');
    if(bytes.byteLength!==selected.size_bytes||sha256!==selected.content_hash)throw new DemoAPIError('Saved configuration bytes do not match the reported size and SHA-256.',content.evidence);
    return {deviceId,versions,selected,requests:[...history.requests,content.evidence]};
  }catch(error){
    if(error&&typeof error==='object')error.selectedId=wanted;
    throw error;
  }
}
export async function getDashboard(signal){
  const inventory=await getDevices(signal);
  const histories=await Promise.all(inventory.devices.map(device=>getHistory(device,signal)));
  const backups=histories.flatMap(h=>h.backups).sort((a,b)=>b.created_at.localeCompare(a.created_at)||a.device.display_name.localeCompare(b.device.display_name));
  if(new Set(backups.map(b=>b.id)).size!==backups.length)throw new DemoAPIError('The Go demo returned duplicate backup identifiers.');
  const covered=new Set(backups.map(b=>b.device_id));
  const groups=new Map();
  for(const device of inventory.devices)groups.set(device.device_group,(groups.get(device.device_group)||0)+1);
  return {devices:inventory.devices,backups,requests:[...inventory.requests,...histories.map(h=>h.request)],
    coverage:inventory.devices.length?Math.round(100*covered.size/inventory.devices.length):null,covered:covered.size,
    groups:[...groups].sort((a,b)=>b[1]-a[1]||a[0].localeCompare(b[0])),latest:backups[0]||null};
}
export const savedLink=b=>'#/devices/'+encodeURIComponent(b.device_id)+'?'+new URLSearchParams({tab:'configuration',backup:b.id});
export function compareLink(backup,backups){
  const versions=backups.filter(b=>b.device_id===backup.device_id),index=versions.findIndex(b=>b.id===backup.id);
  const other=versions[index+1]||versions[index-1]||backup;
  return previewLink('/compare?'+new URLSearchParams({device:backup.device_id,left:other.id,right:backup.id}));
}
export const formatDate=value=>new Intl.DateTimeFormat('en-GB',{day:'2-digit',month:'short',year:'numeric',timeZone:'UTC'}).format(new Date(value));
export const formatTime=value=>new Intl.DateTimeFormat('en-GB',{hour:'2-digit',minute:'2-digit',timeZone:'UTC'}).format(new Date(value))+' UTC';

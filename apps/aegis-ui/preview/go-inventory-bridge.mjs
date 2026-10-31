import {randomUUID} from 'node:crypto';
import {isIP} from 'node:net';

export const GO_SOURCE='Aegis Go API — Isolated Demo';
export const GO_BACKUP_SOURCE='Aegis Go API — Disposable SQLite Demo';
const prefix='/api/go/v1/devices';
const isDemoIP=ip=>isIP(ip)===4&&/^(192\.0\.2|198\.51\.100|203\.0\.113)\./.test(ip);

// Presentation-only adapter. All records come through native Go auth/RBAC,
// DeviceService and SQLite; this module never reads the fixture or the database.
function present(record){
 if(!record||!['id','display_name','hostname','management_ip','device_group'].every(k=>typeof record[k]==='string')||
    !isDemoIP(record.management_ip)||record.vendor!=='mock'||record.platform!=='synthetic'||record.protocol!=='mock'||record.enabled!==false){
  throw new Error('The Go API did not return the expected disabled synthetic inventory.');
 }
 return {id:record.id,display_name:record.display_name,hostname:record.hostname,management_ip:record.management_ip,
  management_port:record.management_port,vendor:record.vendor,platform:record.platform,protocol:record.protocol,
  device_group:record.device_group,description:record.description,enabled:record.enabled,
  created_at:record.created_at,updated_at:record.updated_at,
  site:record.device_group,role:'Synthetic inventory record',model:'Not supplied by Inventory API',
  version:'Not supplied by Inventory API',serial:'Not supplied by Inventory API',
  status:'Demo · disabled',risk:'unknown',last_seen:'No live connection',source:'AEGIS_GO_ISOLATED_DEMO'};
}

function presentBackup(record,deviceID,config){
 if(!record||record.device_id!==deviceID||record.config_type!=='running'||record.trigger_type!=='demo_seed'||
    !['id','created_at','retrieved_at','content_hash'].every(key=>typeof record[key]==='string'&&record[key])||
    !/^[a-f0-9]{64}$/.test(record.content_hash)||!Number.isSafeInteger(record.size_bytes)||record.size_bytes<0||
    !Number.isFinite(Date.parse(record.created_at))||!Number.isFinite(Date.parse(record.retrieved_at))||
    (config!==undefined&&(typeof config!=='string'||!config.startsWith('! AEGIS SYNTHETIC DEMO — DISPOSABLE BACKUP SNAPSHOT\n')))){
  throw new Error('Unexpected synthetic Go backup response');
 }
 return {id:record.id,device_id:record.device_id,config_type:record.config_type,
  created_at:record.created_at,retrieved_at:record.retrieved_at,content_hash:record.content_hash,size_bytes:record.size_bytes,
  trigger_type:record.trigger_type,label:'Go snapshot '+record.id.slice(0,8),origin:'Go '+record.trigger_type+' · read-only',
  source:'AEGIS_GO_BACKUP_DEMO',synthetic:true,...(config===undefined?{}:{config})};
}

export function createGoInventoryBridge(){
 let target=null,identity=null,token=null,expires=0,loginPending=null,generation=0;
 function disconnect(){generation++;target=null;identity=null;token=null;expires=0;loginPending=null;}
 function connect(url,credentials){
  const parsed=new URL(url);
  if(parsed.protocol!=='http:'||parsed.hostname!=='127.0.0.1'||!parsed.port||parsed.username||parsed.password||
     parsed.pathname!=='/'||parsed.search||parsed.hash)throw new Error('The Go demo must use an exact IPv4 loopback origin.');
  disconnect();target=parsed.origin;identity=credentials;
 }
 async function upstream(path,options={}){
  if(!target)throw new Error('Go demo unavailable');
  const response=await fetch(target+path,{...options,redirect:'error',credentials:'omit',signal:AbortSignal.timeout(6000)});
  if(response.headers.get('x-aegis-demo-mode')!=='mock-inventory-v1')throw new Error('Unexpected Go demo identity');
  const payload=await response.json();
  return {response,payload};
 }
 async function authenticate(){
  if(token&&Date.now()<expires)return token;
  if(loginPending)return loginPending;
  const current=generation;
  const pending=(async()=>{
   const {response,payload}=await upstream('/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(identity)});
   if(!response.ok||typeof payload.data?.tokens?.access_token!=='string')throw new Error('Isolated viewer authentication failed');
   if(current!==generation)throw new Error('Go demo changed');
   token=payload.data.tokens.access_token;
   expires=Date.now()+Math.min(payload.data.tokens.expires_in||60,600)*1000-10000;
   return token;
  })();
  loginPending=pending;
  try{return await pending;}finally{if(loginPending===pending)loginPending=null;}
 }
 async function authenticated(path){
  const current=generation,access=await authenticate();
  const result=await upstream(path,{headers:{Authorization:'Bearer '+access,'X-Request-ID':randomUUID()}});
  if(current!==generation)throw new Error('Go demo changed');
  if(result.response.status===401){token=null;expires=0;}
  return result;
 }
 async function probe(){
  const {response,payload}=await authenticated('/api/v1/auth/me');
  const permissions=payload.data?.permissions||[];
  if(!response.ok||!permissions.includes('devices.read')||!permissions.includes('backups.read')||
     permissions.includes('backups.create')||permissions.includes('devices.manage')||
     permissions.includes('credentials.manage')||permissions.includes('system.manage'))throw new Error('Unexpected demo viewer grants');
  return {authentication:'verified',identity:'ephemeral viewer + demo backup reader',devicesRead:true,backupsRead:true,deviceWrites:false,backupWrites:false};
 }
 async function handle(req,res,pathname){
  const match=new RegExp('^'+prefix+'(?:/([a-zA-Z0-9-]+)(?:/(backups)(?:/([a-zA-Z0-9-]+))?)?)?$').exec(pathname);
  const backups=Boolean(match?.[2]),id=match?.[1]||null,backupID=match?.[3]||null;
  let meta={source:backups?GO_BACKUP_SOURCE:GO_SOURCE,mode:'DEMO',synthetic:true,request_id:randomUUID(),storage:backups?'disposable SQLite metadata + synthetic snapshot files':'disposable SQLite'};
  const json=(status,body)=>{
   res.writeHead(status,{'Content-Type':'application/json; charset=utf-8','X-Aegis-Data-Source':backups?'aegis-go-disposable-sqlite-demo':'aegis-go-isolated-demo',
    'X-Request-ID':meta.request_id,...(status===405?{Allow:'GET'}:{})});
   res.end(JSON.stringify({...body,meta}));
  };
  if(req.method!=='GET')return json(405,{error:{code:'READ_ONLY_DEMO',message:'This adapter supports Inventory and backup List/Content GET requests only.'}});
  if(!match)return json(404,{error:{code:'ENDPOINT_NOT_FOUND',message:'Only Inventory and backup List/Content reads are connected to Go.'}});
  if(!target)return json(503,{error:{code:'GO_API_UNAVAILABLE',message:'The isolated Go API is unavailable. Start it, then Retry. No Node fixture fallback is used.'}});
  const path='/api/v1/devices'+(id?'/'+id:'')+(backups?'/backups'+(backupID?'/'+backupID:'?limit=100'):id?'':'?limit=100');
  try{
   const {response,payload}=await authenticated(path);
   const requestID=payload.meta?.request_id||payload.error?.request_id;
   if(typeof requestID!=='string'||requestID!==response.headers.get('x-request-id'))throw new Error('Missing Go request identity');
   meta={...meta,request_id:requestID,upstream_path:path,...(payload.meta?.pagination?{pagination:payload.meta.pagination}:{})};
   if(!response.ok)return json(response.status,{error:{code:payload.error?.code||'GO_API_ERROR',message:payload.error?.message||'The Go API could not complete this request.'}});
   const records=backups?(backupID?presentBackup(payload.data?.backup,id,payload.data?.config):
    Array.isArray(payload.data)?payload.data.map(record=>presentBackup(record,id)):null):
    id?present(payload.data):Array.isArray(payload.data)?payload.data.map(present):null;
   if(records===null||backupID&&records.id!==backupID||!backups&&id&&records.id!==id)throw new Error('Unexpected Go response');
   if(backupID&&typeof records.config!=='string')throw new Error('Missing Go snapshot content');
   return json(200,{data:records});
  }catch{
   return json(503,{error:{code:'GO_API_UNAVAILABLE',message:'The isolated Go API could not be reached or verified. Start it, then Retry. No Node fixture fallback is used.'}});
  }
 }
 return {connect,disconnect,handle,probe,configured:()=>target!==null};
}

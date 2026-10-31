import http from 'node:http';
import { readFile, writeFile } from 'node:fs/promises';
import { randomUUID } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import {createGoInventoryBridge,GO_SOURCE} from './go-inventory-bridge.mjs';
const root = path.dirname(fileURLToPath(import.meta.url));
const publicRoot = path.join(root, 'public');
const files = {'/':'index.html','/index.html':'index.html','/app.js':'app.js','/styles.css':'styles.css','/demo-data.json':'demo-data.json','/favicon.svg':'favicon.svg'};
const types = {'.html':'text/html; charset=utf-8','.js':'text/javascript; charset=utf-8','.css':'text/css; charset=utf-8','.json':'application/json; charset=utf-8','.svg':'image/svg+xml'};
const fixture = JSON.parse(await readFile(path.join(publicRoot, 'demo-data.json'), 'utf8'));
if (fixture.mode !== 'DEMO' || !Array.isArray(fixture.devices) ||
    fixture.devices.some(device => device.source !== 'SYNTHETIC_DEMO')) {
  throw new Error('Only the reviewed synthetic DEMO fixture can supply the mock API.');
}
// Inventory endpoints expose metadata only. Snapshot text has its own GET endpoint.
const mockDevices = fixture.devices.map(({backups, findings, ...device}) => ({
  ...device, backup_count: backups.length, finding_count: findings.length
}));
const mockBackups = new Map(fixture.devices.map(device => [device.id, device.backups.map(backup => {
  if (typeof backup.config !== 'string' || !backup.config.startsWith('! AEGIS SYNTHETIC DEMO — NOT A DEVICE CONFIGURATION\n')) {
    throw new Error('Every mock snapshot must carry the reviewed synthetic configuration banner.');
  }
  return {...backup,device_id:device.id,source:'SYNTHETIC_DEMO',synthetic:true};
})]));
export async function startPreview({port=0,goInventory=createGoInventoryBridge()}={}){
if (!Number.isInteger(port) || port < 0 || port > 65535) throw new Error('Invalid localhost port.');
const server = http.createServer(async (req,res)=>{
  res.setHeader('Cache-Control','no-store');
  res.setHeader('X-Content-Type-Options','nosniff');
  res.setHeader('Content-Security-Policy',"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'");
  if(!/^((127\.0\.0\.1)|(localhost)):\d+$/.test(req.headers.host||'')){res.writeHead(403);return res.end('Local preview only');}
  const pathname = new URL(req.url, 'http://127.0.0.1').pathname;
  if(pathname==='/api/go/v1/devices'||pathname.startsWith('/api/go/v1/devices/')){
    return goInventory.handle(req,res,pathname);
  }
  if (pathname === '/api/mock/v1/devices' || pathname.startsWith('/api/mock/v1/devices/')) {
    const meta = {source:'Local Mock API',mode:'DEMO',synthetic:true,request_id:randomUUID()};
    const json = (status, body) => {
      res.writeHead(status, {'Content-Type':'application/json; charset=utf-8',
        'X-Aegis-Data-Source':'Local Mock API','X-Request-ID':meta.request_id,
        ...(status === 405 ? {'Allow':'GET'} : {})});
      res.end(JSON.stringify({...body,meta}));
    };
    if (req.method !== 'GET') return json(405,{error:{code:'METHOD_NOT_ALLOWED',message:'The mock API supports GET only.'}});
    if (pathname === '/api/mock/v1/devices') return json(200,{data:mockDevices});
    const parts = pathname.slice('/api/mock/v1/devices/'.length).split('/');
    if (!(parts.length === 1 || (parts[1] === 'backups' && [2,3].includes(parts.length))) || parts.some(part => !part)) {
      return json(404,{error:{code:'ENDPOINT_NOT_FOUND',message:'This mock API endpoint was not found.'}});
    }
    let id, backupId;
    try { id = decodeURIComponent(parts[0]); backupId = parts[2] === undefined ? undefined : decodeURIComponent(parts[2]); }
    catch { return json(400,{error:{code:'INVALID_DEVICE_ID',message:'The device identifier is not valid.'}}); }
    const device = mockDevices.find(candidate => candidate.id === id);
    if (!device) return json(404,{error:{code:'DEVICE_NOT_FOUND',message:'This synthetic device was not found.'}});
    if (parts[1] === 'backups') {
      const backups = mockBackups.get(id);
      if (backupId === undefined) return json(200,{data:backups.map(({config,...metadata}) => metadata)});
      const backup = backups.find(candidate => candidate.id === backupId);
      if (!backup) return json(404,{error:{code:'BACKUP_NOT_FOUND',message:'This synthetic snapshot was not found for this device.'}});
      return json(200,{data:backup});
    }
    return json(200,{data:device});
  }
  if(!['GET','HEAD'].includes(req.method)){res.writeHead(405,{'Allow':'GET, HEAD'});return res.end('Preview is read-only');}
  if(pathname === '/healthz'){res.writeHead(200,{'Content-Type':'application/json'});return res.end(JSON.stringify({status:'ok',mode:'synthetic-demo',liveDeviceAccess:false,productionApiConnected:false,inventorySource:GO_SOURCE,goApiConfigured:goInventory.configured()}));}
  const name=files[pathname];
  if(!name){res.writeHead(404);return res.end('Not found');}
  try{const body=await readFile(path.join(publicRoot,name));res.writeHead(200,{'Content-Type':types[path.extname(name)]});res.end(req.method==='HEAD'?undefined:body);}
  catch{res.writeHead(500);res.end('Preview asset unavailable');}
});
await new Promise((resolve,reject)=>{
 server.once('error',reject);
 server.listen(port,'127.0.0.1',resolve);
});
const boundPort=server.address().port;
const info={url:'http://127.0.0.1:'+boundPort,pid:process.pid,host:'127.0.0.1',port:boundPort,mode:'DEMO',inventorySource:'Aegis Go API — Isolated Demo',startedAt:new Date().toISOString()};
try{await writeFile(path.join(root,'runtime.json'),JSON.stringify(info,null,2));}
catch(error){server.close();throw error;}
return {server,info};
}

if(process.argv[1]&&path.resolve(process.argv[1])===fileURLToPath(import.meta.url)){
 const args=process.argv.slice(2);
 if(args.length>1||(args.length&&!/^--port=\d+$/.test(args[0])))throw new Error('Usage: node server.mjs [--port=0..65535]');
 const {info}=await startPreview({port:args.length?Number(args[0].slice(7)):0});
 console.log(info.url);
}

// CI-only end-to-end smoke of an isolated synthetic Go -> Node -> Vue flow.
// Builds/launches cmd/inventory-demo only; never starts cmd/server or SSH.
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {once} from 'node:events';
import {mkdtemp,rm,stat} from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';

const thisDir=path.dirname(fileURLToPath(import.meta.url));
const vueRoot=path.resolve(thisDir,'..');
const repoRoot=path.resolve(vueRoot,'../../..');
const runner=path.join(repoRoot,'apps/aegis-ui/preview/run-go-demo.mjs');
const binary=process.env.AEGIS_DEMO_BINARY;
assert(binary&&path.isAbsolute(binary)&&path.basename(binary)==='aegis-inventory-demo','Select the dedicated absolute Go mock-demo binary');
assert((await stat(binary)).isFile(),'Mock-only demo binary must exist');
const temp=await mkdtemp(path.join(os.tmpdir(),'aegis-synthetic-ci-'));
let child, vite;

function waitForSupervisor(proc){
 return new Promise((resolve,reject)=>{
   let go,preview,settled=false;
   const lines=createInterface({input:proc.stdout});
   const finish=(error)=>{
     if(settled)return;
     settled=true;clearTimeout(timer);lines.removeAllListeners('line');proc.removeListener('exit',onExit);proc.stdout.resume();
     if(error)reject(error);
     else resolve({go,preview});
   };
   const onExit=()=>finish(new Error('Isolated Go/Node supervisor exited before synthetic readiness'));
   const timer=setTimeout(()=>finish(new Error('Timed out waiting for synthetic Go/Node readiness')),40000);
   proc.once('exit',onExit);
   lines.on('line',line=>{
     try {
       const payload=JSON.parse(line);
       if(payload.connector==='mock'&&payload.devices===4&&payload.backups===12&&payload.goApi)go=payload;
       if(payload.preview&&payload.goApi)preview=payload;
       if(go&&preview)finish();
     }catch { /* Non-JSON supervisor informational line. */ }
   });
 });
}
async function getJson(origin,resource){
 const response=await fetch(origin+resource,{signal:AbortSignal.timeout(10000)});
 assert.equal(response.status,200,resource+' must be HTTP 200');
 return response.json();
}
try {
 child=spawn(process.execPath,[runner,'--go-binary='+binary,'--run-parent='+temp,'--port=0'],{
   cwd:vueRoot,env:process.env,stdio:['pipe','pipe','pipe'],windowsHide:true,shell:false
 });
 child.stderr.resume();
 const {go,preview}=await waitForSupervisor(child);
 assert.equal(go.connector,'mock');
 assert.equal(go.devices,4);assert.equal(go.backups,12);
 assert.equal(go.authentication,'verified');
 const previewOrigin=new URL(preview.preview).origin;
 const goOrigin=new URL(go.goApi).origin;
 assert(previewOrigin.startsWith('http://127.0.0.1:'));
 assert(goOrigin.startsWith('http://127.0.0.1:'));
 assert.equal(new URL(preview.goApi).origin,goOrigin);
 const health=await getJson(previewOrigin,'/healthz');
 assert.equal(health.liveDeviceAccess,false);
 assert.equal(health.productionApiConnected,false);
 const unauthorized=await fetch(goOrigin+'/api/v1/devices',{signal:AbortSignal.timeout(10000)});
 assert.equal(unauthorized.status,401,'Raw Go inventory must require authentication');
 const inv=await getJson(previewOrigin,'/api/go/v1/devices');
 assert.equal(inv.meta?.synthetic,true);
 assert.equal(inv.data?.length,4);
 assert(inv.data.every(d=>d.enabled===false&&d.protocol==='mock'&&d.vendor==='mock'));
 let backupIds=[];
 for(const device of inv.data){
   const payload=await getJson(previewOrigin,'/api/go/v1/devices/'+encodeURIComponent(device.id)+'/backups');
   assert.equal(payload.meta?.synthetic,true);
   assert.equal(payload.data?.length,3);
   backupIds.push(...payload.data.map(b=>b.id));
 }
 assert.equal(new Set(backupIds).size,12,'Persisted synthetic backup UUIDs must be unique');
 const denied=await fetch(previewOrigin+'/api/go/v1/devices',{method:'POST',signal:AbortSignal.timeout(10000)});
 assert.equal(denied.status,405,'The synthetic bridge must reject writes');

 process.env.AEGIS_PREVIEW_ORIGIN=previewOrigin; // Public origin only; no credentials.
 vite=await createServer({
   root:vueRoot,configFile:path.join(vueRoot,'vite.config.js'),configLoader:'native',
   envDir:false,logLevel:'silent',clearScreen:false,
   server:{host:'127.0.0.1',port:0,strictPort:true,open:false}
 });
 await vite.listen();
 const addr=vite.httpServer.address();
 assert.equal(addr.address,'127.0.0.1');
 const vueOrigin='http://127.0.0.1:'+addr.port;
 const html=await fetch(vueOrigin+'/',{signal:AbortSignal.timeout(10000)});
 assert.equal(html.status,200);
 assert((await html.text()).includes('/src/main.js'),'Native Vue entrypoint must be served');
 const throughVue=await getJson(vueOrigin,'/api/go/v1/devices');
 assert.deepEqual(throughVue.data.map(d=>d.id).sort(),inv.data.map(d=>d.id).sort());
 let proxiedIds=[];
 for(const d of throughVue.data){
   const payload=await getJson(vueOrigin,'/api/go/v1/devices/'+encodeURIComponent(d.id)+'/backups');
   proxiedIds.push(...payload.data.map(b=>b.id));
 }
 assert.deepEqual(proxiedIds.sort(),backupIds.sort(),'Vue and Node must read the same Go UUIDs');
 console.log('AEGIS_SYNTHETIC_E2E_PASS: Go -> Node -> Vue; 4 disabled mock devices; 12 persisted synthetic backups; unauthenticated Go=401; writes=405');
}finally{
 if(vite)await vite.close().catch(()=>{});
 if(child&&child.exitCode===null&&!child.killed){
   child.stdin.write('quit\n');
   await Promise.race([once(child,'exit'),new Promise(r=>setTimeout(r,5000))]);
   if(child.exitCode===null)child.kill('SIGTERM');
 }
 await rm(temp,{recursive:true,force:true});
}

// Development-only supervisor. Never invokes cmd/server, Go builds or a shell.
import {spawn} from 'node:child_process';
import {randomBytes} from 'node:crypto';
import {createInterface} from 'node:readline';
import {stat,mkdir} from 'node:fs/promises';
import path from 'node:path';
import {createGoInventoryBridge,GO_SOURCE} from './go-inventory-bridge.mjs';
import {startPreview} from './server.mjs';

const flags=new Map();
for(const arg of process.argv.slice(2)){
 const match=/^--(go-binary|run-parent|port)=(.+)$/.exec(arg);
 if(!match||flags.has(match[1]))throw new Error('Use --go-binary=ABS_PATH --run-parent=ABS_EXTERNAL_DIR [--port=0]');
 flags.set(match[1],match[2]);
}
const binary=flags.get('go-binary'),runParent=flags.get('run-parent');
if(!binary||!runParent||!path.isAbsolute(binary)||!path.isAbsolute(runParent))throw new Error('Explicit absolute demo binary and disposable parent paths are required.');
if(!/^aegis-inventory-demo(?:\.exe)?$/.test(path.basename(binary)))throw new Error('Build and select only the dedicated aegis-inventory-demo executable.');
if(!(await stat(binary)).isFile())throw new Error('Demo binary is missing.');
await mkdir(runParent,{recursive:true});
const port=flags.has('port')?Number(flags.get('port')):0;
if(!Number.isInteger(port)||port<0||port>65535)throw new Error('Invalid preview port.');
const bridge=createGoInventoryBridge();
let child=null,starting=false,closing=false,goInfo=null;
const credentials={username:'demo-viewer-'+randomBytes(8).toString('hex'),password:randomBytes(32).toString('base64url')};

async function startAPI(){
 if(child||starting||closing)return;
 starting=true;
 const proc=spawn(binary,[],{shell:false,windowsHide:true,stdio:['pipe','pipe','pipe'],cwd:path.dirname(binary)});
 child=proc;
 proc.stdin.on('error',()=>{}); // A startup failure must not expose private bootstrap bytes.
 proc.stderr.on('data',()=>{}); // Never echo arbitrary child output; report lifecycle failures below.
 proc.on('exit',()=>{
  if(child===proc){child=null;goInfo=null;bridge.disconnect();if(!closing)console.log('Go API stopped. Inventory will show unavailable. Use start-api, then Retry.');}
 });
 try{
  const info=await new Promise((resolve,reject)=>{
   const lines=createInterface({input:proc.stdout});
   const timer=setTimeout(()=>{lines.close();reject(new Error('Go demo startup timed out.'));},15000);
   const fail=()=>{clearTimeout(timer);lines.close();reject(new Error('Go demo did not start. Check the dedicated binary and disposable parent.'));};
   proc.once('error',fail);proc.once('exit',fail);
   lines.once('line',line=>{
    clearTimeout(timer);lines.close();proc.removeListener('error',fail);proc.removeListener('exit',fail);
    try{
     const ready=JSON.parse(line);
     if(ready.source!==GO_SOURCE||ready.connector!=='mock'||ready.pid!==proc.pid)throw new Error();
     resolve(ready);
    }catch{reject(new Error('Unexpected demo startup response.'));}
   });
   // Credentials go only through the anonymous stdin pipe, never argv/env/logs/files.
   proc.stdin.end(JSON.stringify({run_parent:runParent,...credentials})+'\n');
  });
  if(child!==proc)throw new Error('Go demo stopped during startup.');
  bridge.connect(info.url,credentials);
  const auth=await bridge.probe();
  goInfo=info;
  console.log(JSON.stringify({goApi:info.url,pid:info.pid,storage:'fresh disposable SQLite',connector:'mock',devices:info.devices,...auth}));
 }catch(error){
  bridge.disconnect();if(child===proc)proc.kill();
  throw error;
 }finally{starting=false;}
}
async function stopAPI(){
 const proc=child;
 bridge.disconnect();
 if(!proc)return;
 await new Promise(resolve=>{proc.once('exit',resolve);proc.kill();});
}
await startAPI();
const preview=await startPreview({port,goInventory:bridge});
console.log(JSON.stringify({preview:preview.info.url,inventory:'/api/go/v1/devices',goApi:goInfo.url}));
console.log('Local demo commands: stop-api, start-api, quit. Restart creates a fresh disposable database.');
let commandQueue=Promise.resolve();
const commands=createInterface({input:process.stdin});
commands.on('line',line=>{
 commandQueue=commandQueue.then(async()=>{
  if(line.trim()==='stop-api')await stopAPI();
  else if(line.trim()==='start-api')await startAPI();
  else if(line.trim()==='quit')await shutdown();
  else console.log('Commands: stop-api, start-api, quit');
 }).catch(()=>console.error('Demo command failed. Inventory remains unavailable; check the isolated runtime.'));
});
async function shutdown(){
 if(closing)return;
 closing=true;commands.close();preview.server.close();await stopAPI();process.exit();
}
process.on('SIGINT',()=>void shutdown());
process.on('SIGTERM',()=>void shutdown());
process.on('exit',()=>{if(child)child.kill();});

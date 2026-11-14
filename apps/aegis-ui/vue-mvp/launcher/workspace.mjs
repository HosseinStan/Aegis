import {readFile,writeFile,mkdir,open,unlink,rename,cp,lstat,realpath,access} from 'node:fs/promises';
import path from 'node:path';
import {createHash,randomUUID} from 'node:crypto';
import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {createRequire} from 'node:module';
import {pathToFileURL} from 'node:url';

const GO_EXE_SHA='c0ea949d000a24883f37b1dd2db794a9fb2b824fbcff11facb469615a50ca6af';
const GO_ZIP_SHA='71ae086f0137e665f855a811cc119152b6a783e0bbc1b906e709f46d80141eb0';
const SOURCE='Aegis Go API — Isolated Demo',BACKUPS='Aegis Go API — Disposable SQLite Demo';
const fail=(code,message)=>{throw new Error(code+': '+message);};
const digest=bytes=>createHash('sha256').update(bytes).digest('hex');
const exists=async name=>{try{await access(name);return true;}catch{return false;}};
const within=(parent,child)=>{const relative=path.relative(parent,child);return relative===''||!relative.startsWith('..'+path.sep)&&relative!=='..'&&!path.isAbsolute(relative);};
const json=async name=>JSON.parse(await readFile(name,'utf8'));
export function parseTools(value){
  const keys=['nodeExe','pnpmCjs','goRoot','goArchive','moduleCacheSeed','runtimeRoot'];
  if(!value||Object.keys(value).length!==keys.length||Object.keys(value).some(key=>!keys.includes(key))||keys.some(key=>typeof value[key]!=='string'||!path.isAbsolute(value[key])))fail('TOOLS_INVALID','Use only the six documented absolute tool/runtime paths; no credentials.');
  return Object.fromEntries(keys.map(key=>[key,path.resolve(value[key])]));
}
export function localOrigin(value){
  let url;try{url=new URL(value);}catch{fail('READINESS_INVALID','Expected a localhost HTTP origin.');}
  if(url.protocol!=='http:'||url.hostname!=='127.0.0.1'||!url.port||url.username||url.password||url.pathname!=='/'||url.search||url.hash)fail('READINESS_INVALID','Service must bind to an exact http://127.0.0.1:PORT origin.');
  return url.origin;
}
export function validateInventory(payload){
  const expected={'core-sw-01':'192.0.2.10','core-sw-02':'192.0.2.11','edge-rtr-01':'198.51.100.20','access-sw-01':'203.0.113.30'};
  if(payload.meta?.source!==SOURCE||payload.meta?.synthetic!==true||typeof payload.meta.request_id!=='string'||!Array.isArray(payload.data)||payload.data.length!==4||
     new Set(payload.data.map(d=>d.id)).size!==4||!payload.data.every(d=>d.source==='AEGIS_GO_ISOLATED_DEMO'&&d.enabled===false&&d.vendor==='mock'&&d.protocol==='mock'&&expected[d.id]===d.management_ip))fail('READINESS_INVALID','Expected four disabled synthetic mock devices, without fixture fallback.');
  return payload.data;
}
export function validateBackups(payload,device){
  if(payload.meta?.source!==BACKUPS||payload.meta?.synthetic!==true||typeof payload.meta.request_id!=='string'||!Array.isArray(payload.data)||payload.data.length!==3||
     !payload.data.every(b=>b.device_id===device&&b.synthetic===true&&b.source==='AEGIS_GO_BACKUP_DEMO'&&b.config_type==='running'&&b.trigger_type==='demo_seed'&&/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/i.test(b.id)&&/^[a-f0-9]{64}$/.test(b.content_hash)))fail('READINESS_INVALID','Expected three persisted synthetic snapshots per Go device.');
  return payload.data.map(b=>b.id).sort();
}
export function parseGoPackages(text){
  const result=[];let start=-1,depth=0,quoted=false,escaped=false;
  for(let i=0;i<text.length;i++){
    const ch=text[i];
    if(!quoted&&depth===0&&ch!=='{'&&!/\s/.test(ch))fail('BUILD_INPUTS_INVALID','Unexpected data outside Go package metadata.');
    if(quoted){if(escaped)escaped=false;else if(ch==='\\')escaped=true;else if(ch==='"')quoted=false;continue;}
    if(ch==='"'){quoted=true;continue;}
    if(ch==='{'){if(depth++===0)start=i;}
    if(ch==='}'&&--depth===0){result.push(JSON.parse(text.slice(start,i+1)));start=-1;}
  }
  if(quoted||depth||start!==-1||!result.length)fail('BUILD_INPUTS_INVALID','Go package metadata was incomplete.');
  return result;
}
export async function copyOfflineCache(seed,runtimeRoot,session,signal){
  const target=path.join(runtimeRoot,'module-cache'),pending=path.join(session,'module-cache.pending');
  if(!within(runtimeRoot,session)||!within(runtimeRoot,pending)||!within(runtimeRoot,target))fail('RUNTIME_PATH_UNSAFE','Cache publication must remain inside this runtime.');
  await cp(seed,pending,{recursive:true,errorOnExist:true,force:false,filter:async source=>{signal.throwIfAborted();if((await lstat(source)).isSymbolicLink())fail('CACHE_UNSAFE','Module cache seed contains a link.');return true;}});
  await rename(pending,target);
}
async function externalRoot(root){
  for(let probe=root;;probe=path.dirname(probe)){
    if(await exists(probe)){
      if((await lstat(probe)).isSymbolicLink())fail('RUNTIME_PATH_UNSAFE','Runtime path cannot contain junctions or symlinks.');
      if(await exists(path.join(probe,'.git')))fail('RUNTIME_PATH_UNSAFE','Runtime must be outside every Git checkout.');
    }
    if(path.dirname(probe)===probe)break;
  }
}
async function main(){
  // Do not move any spawn or service startup before this stdin gate: the Windows host assigns our Job first.
  const commands=createInterface({input:process.stdin,terminal:false});
  const bootstrap=await new Promise((resolve,reject)=>{
    commands.once('line',line=>{try{resolve(JSON.parse(line));}catch{reject(new Error('BOOTSTRAP_INVALID: Invalid public tools configuration.'));}});
    commands.once('close',()=>reject(new Error('BOOTSTRAP_MISSING: Start through start-local.ps1.')));
  });
  const cancellation=new AbortController(),signal=cancellation.signal;
  commands.on('line',line=>{if(line==='stop')cancellation.abort();});
  commands.on('close',()=>cancellation.abort());
  for(const event of ['SIGINT','SIGTERM','SIGBREAK'])process.on(event,()=>cancellation.abort());
  let supervisor,vite,lock,lockPath,lockNonce,sessionFile,sessionInfo,failed=false,healthTimer;
  const requestStop=()=>cancellation.abort();
  const check=()=>signal.throwIfAborted();
  async function tool(exe,args,env,label){
    check();return new Promise((resolve,reject)=>{
      const child=spawn(exe,args,{shell:false,windowsHide:true,env,stdio:['ignore','pipe','pipe'],signal});
      let output='',size=0;
      child.stdout.on('data',bytes=>{size+=bytes.length;if(size>16*1024*1024){child.kill();reject(new Error('TOOL_OUTPUT_INVALID: Unexpected tool output size.'));}else output+=bytes.toString('utf8');});
      child.stderr.resume(); // Never relay arbitrary process output or private bootstrap material.
      child.on('error',()=>reject(new Error('TOOL_FAILED: '+label+' could not start.')));
      child.on('close',code=>code===0?resolve(output.trim()):reject(new Error('TOOL_FAILED: '+label+' failed. Offline dependencies must already be available; no download was attempted.')));
    });
  }
  async function read(origin,urlPath,marker){
    check();const response=await fetch(origin+urlPath,{method:'GET',cache:'no-store',credentials:'omit',signal:AbortSignal.any([signal,AbortSignal.timeout(8000)])});
    if(!response.ok||marker&&response.headers.get('x-aegis-demo-mode')!=='mock-inventory-v1')fail('READINESS_FAILED','The isolated Go/Node demo did not become healthy.');
    return response;
  }
  try{
    if(process.platform!=='win32'||process.arch!=='x64')fail('PLATFORM_UNSUPPORTED','This verified launcher targets Windows x64.');
    const tools=parseTools(bootstrap.tools),vueRoot=await realpath(bootstrap.vueRoot),repoRoot=path.resolve(vueRoot,'../../..');
    if(path.resolve(process.execPath).toLowerCase()!==tools.nodeExe.toLowerCase())fail('TOOLS_INVALID','Node executable does not match the tools configuration.');
    const [major,minor]=process.versions.node.split('.').map(Number);
    if(major<22||major===22&&minor<13)fail('NODE_VERSION_UNSUPPORTED','Node 22.13 or newer is required. No installation is performed.');
    for(const name of ['pnpmCjs','goArchive'])if(!(await lstat(tools[name])).isFile())fail('TOOLS_MISSING','An existing '+name+' file is required.');
    const goExe=path.join(tools.goRoot,'bin/go.exe');
    if(digest(await readFile(goExe))!==GO_EXE_SHA||digest(await readFile(tools.goArchive))!==GO_ZIP_SHA||!(await readFile(path.join(tools.goRoot,'VERSION'),'utf8')).startsWith('go1.26.11\n'))fail('GO_TRUST_FAILED','Select the existing approved Go 1.26.11 Windows x64 SDK and official archive. No toolchain fallback or installation is allowed.');
    const manifest=await json(path.join(vueRoot,'package.json')),require=createRequire(path.join(vueRoot,'package.json'));
    if((await json(path.resolve(tools.pnpmCjs,'../../package.json'))).version!==manifest.packageManager.split('@').at(-1))fail('PNPM_VERSION_MISMATCH','Select the already installed pinned pnpm version.');
    for(const [pkg,version] of Object.entries({...manifest.dependencies,...manifest.devDependencies})){
      let installed;try{installed=require(pkg+'/package.json').version;}catch{fail('VUE_DEPENDENCIES_MISSING','Prepare the pinned local Vue packages from the lockfile offline before launch; automatic installation is disabled.');}
      if(installed!==version)fail('VUE_DEPENDENCIES_MISMATCH','Installed '+pkg+' does not match its exact pinned version.');
    }
    if(digest(await readFile(path.join(vueRoot,'pnpm-lock.yaml')))!==digest(await readFile(path.join(vueRoot,'node_modules/.pnpm/lock.yaml'))))fail('VUE_LOCK_MISMATCH','Installed dependencies do not match the frozen lockfile.');
    await externalRoot(tools.runtimeRoot);
    if(within(repoRoot,tools.runtimeRoot)||within(tools.runtimeRoot,repoRoot))fail('RUNTIME_PATH_UNSAFE','Use an external runtime directory separate from the source checkout.');
    const owner={schema:1,repoRoot:repoRoot.toLowerCase()};
    if(await exists(tools.runtimeRoot)){
      let actual;try{actual=await json(path.join(tools.runtimeRoot,'.aegis-local-runtime.json'));}catch{fail('RUNTIME_NOT_OWNED','Existing runtime directory has no matching launcher ownership marker. Choose a fresh external directory.');}
      if(JSON.stringify(actual)!==JSON.stringify(owner))fail('RUNTIME_NOT_OWNED','This runtime belongs to another checkout. Choose a fresh directory.');
    }else{await mkdir(tools.runtimeRoot,{recursive:true});await writeFile(path.join(tools.runtimeRoot,'.aegis-local-runtime.json'),JSON.stringify(owner),{flag:'wx'});}
    lockPath=path.join(tools.runtimeRoot,'.launcher.lock');lockNonce=randomUUID();
    try{lock=await open(lockPath,'wx');await lock.writeFile(lockNonce);}catch{fail('SESSION_LOCKED','Another session owns this runtime, or an interrupted session left a lock. No process was stopped. Use a fresh runtime or inspect the stale lock manually.');}
    const session=path.join(tools.runtimeRoot,'sessions',randomUUID());await mkdir(session,{recursive:true});
    sessionFile=path.join(session,'launch.json');
    sessionInfo={state:'starting',controllerPid:process.pid,session};
    const moduleCache=path.join(tools.runtimeRoot,'module-cache'),cacheReady=path.join(tools.runtimeRoot,'module-cache.ready');
    const baseEnv={...process.env,NODE_OPTIONS:'',COREPACK_ENABLE_NETWORK:'0',npm_config_offline:'true',npm_config_manage_package_manager_versions:'false'};
    if((await tool(process.execPath,[tools.pnpmCjs,'--version'],baseEnv,'Pinned pnpm validation'))!==manifest.packageManager.split('@').at(-1))fail('PNPM_VERSION_MISMATCH','Installed pnpm did not report the pinned version.');
    if(!(await exists(cacheReady))){
      if(await exists(moduleCache))fail('CACHE_INCOMPLETE','A previous cache preparation did not finish. Use a fresh external runtime.');
      console.log('Preparing a private copy of existing offline Go dependencies…');
      await copyOfflineCache(tools.moduleCacheSeed,tools.runtimeRoot,session,signal);
      await writeFile(cacheReady,'copied existing dependencies; no downloads\n',{flag:'wx'});
    }
    for(const dir of ['go-build','gopath'])await mkdir(path.join(tools.runtimeRoot,dir),{recursive:true});
    await mkdir(path.join(session,'tmp'));
    const goEnv={...baseEnv,GOENV:'off',GOWORK:'off',GOROOT:tools.goRoot,GOTOOLCHAIN:'local',GOPROXY:'off',GOFLAGS:'-mod=readonly',CGO_ENABLED:'0',GOOS:'windows',GOARCH:'amd64',GOMODCACHE:moduleCache,GOCACHE:path.join(tools.runtimeRoot,'go-build'),GOPATH:path.join(tools.runtimeRoot,'gopath'),GOTMPDIR:path.join(session,'tmp')};
    if(await tool(goExe,['version'],goEnv,'Go version validation')!=='go version go1.26.11 windows/amd64')fail('GO_VERSION_MISMATCH','Only Go 1.26.11 is allowed.');
    const apiRoot=path.join(repoRoot,'apps/aegis-api');
    // cwd is fixed to this clone's API, never a protected checkout or production entrypoint.
    const originalCwd=process.cwd();process.chdir(apiRoot);
    let key,binary,reused=false;
    try{
      const packages=parseGoPackages(await tool(goExe,['list','-deps','-json','./cmd/inventory-demo'],goEnv,'Offline Go dependency/source listing'));
      const inputs=[path.join(apiRoot,'go.mod'),path.join(apiRoot,'go.sum')];
      for(const pkg of packages.filter(pkg=>pkg.Module?.Main)){
        if(!within(apiRoot,pkg.Dir))fail('BUILD_INPUTS_UNSAFE','Main-module build inputs must remain in this isolated API checkout.');
        for(const type of ['GoFiles','CgoFiles','EmbedFiles','SFiles','SysoFiles','CFiles','CXXFiles','HFiles'])for(const file of pkg[type]||[])inputs.push(path.join(pkg.Dir,file));
      }
      const fingerprint=[];for(const file of [...new Set(inputs)].sort())fingerprint.push([path.relative(apiRoot,file),digest(await readFile(file))]);
      key=digest(JSON.stringify({go:GO_EXE_SHA,target:'./cmd/inventory-demo',flags:['-trimpath','CGO_ENABLED=0','GOOS=windows','GOARCH=amd64'],fingerprint}));
      const binaryDir=path.join(tools.runtimeRoot,'bin',key);binary=path.join(binaryDir,'aegis-inventory-demo.exe');
      if(await exists(binaryDir)){
        const receipt=await json(path.join(binaryDir,'build.json'));
        if(receipt.key!==key||receipt.sha256!==digest(await readFile(binary)))fail('BINARY_CACHE_INVALID','Cached demo binary failed verification. It was not restored or overwritten. Use a fresh runtime.');
        reused=true;console.log('Reusing the verified dedicated Go demo binary.');
      }else{
        console.log('Building the dedicated mock-only Go demo offline…');
        await tool(goExe,['mod','verify'],goEnv,'Offline Go module verification');
        const pending=path.join(session,'binary');await mkdir(pending);
        const candidate=path.join(pending,'aegis-inventory-demo.exe');
        await tool(goExe,['build','-trimpath','-o',candidate,'./cmd/inventory-demo'],goEnv,'Offline dedicated Go demo build');
        await writeFile(path.join(pending,'build.json'),JSON.stringify({key,sha256:digest(await readFile(candidate))}));
        if(!within(tools.runtimeRoot,pending)||!within(tools.runtimeRoot,binaryDir))fail('RUNTIME_PATH_UNSAFE','Binary publication must remain inside this runtime.');
        await mkdir(path.dirname(binaryDir),{recursive:true});await rename(pending,binaryDir);
      }
    }finally{process.chdir(originalCwd);}
    check();
    let goReady,previewReady;
    const ready=new Promise((resolve,reject)=>{
      supervisor=spawn(process.execPath,[path.join(repoRoot,'apps/aegis-ui/preview/run-go-demo.mjs'),'--go-binary='+binary,'--run-parent='+path.join(session,'data'),'--port=0'],{shell:false,windowsHide:true,env:baseEnv,stdio:['pipe','pipe','pipe']});
      supervisor.stderr.resume();supervisor.stdin.on('error',()=>{});
      const timeout=setTimeout(()=>reject(new Error('SUPERVISOR_TIMEOUT: The existing Go/Node supervisor did not report readiness.')),25000);
      const lines=createInterface({input:supervisor.stdout});
      lines.on('line',line=>{
        if(line.startsWith('Go API stopped.')){if(!signal.aborted){failed=true;console.error('GO_STOPPED: The dedicated Go API stopped; this session is closing without fallback.');}requestStop();}
        let value;try{value=JSON.parse(line);}catch{return;}
        if(value.goApi&&value.connector==='mock')goReady=value;
        if(value.preview){previewReady=value;clearTimeout(timeout);resolve();}
      });
      supervisor.once('error',()=>{clearTimeout(timeout);reject(new Error('SUPERVISOR_FAILED: The existing supervisor could not start.'));});
      supervisor.once('exit',()=>{clearTimeout(timeout);if(!signal.aborted){failed=true;console.error('SUPERVISOR_EXITED: The isolated Go/Node supervisor stopped.');}requestStop();reject(new Error('SUPERVISOR_EXITED: The isolated Go/Node supervisor stopped.'));});
      signal.addEventListener('abort',()=>{clearTimeout(timeout);reject(signal.reason);},{once:true});
    });
    await ready;check();
    if(!goReady||goReady.devices!==4||goReady.backups!==12||goReady.authentication!=='verified'||!goReady.devicesRead||!goReady.backupsRead||goReady.deviceWrites!==false||goReady.backupWrites!==false)fail('READINESS_INVALID','Unexpected mock-only authentication or dataset readiness response.');
    const nodeOrigin=localOrigin(previewReady.preview),goOrigin=localOrigin(goReady.goApi);
    if(localOrigin(previewReady.goApi)!==goOrigin)fail('READINESS_INVALID','Supervisor service origins did not agree.');
    await read(goOrigin,'/readyz',true);
    const devices=validateInventory(await (await read(nodeOrigin,'/api/go/v1/devices')).json());
    const ids=[];for(const device of devices)ids.push(...validateBackups(await (await read(nodeOrigin,'/api/go/v1/devices/'+device.id+'/backups')).json(),device.id));
    if(new Set(ids).size!==12)fail('READINESS_INVALID','The twelve Go snapshots must have unique actual UUIDs.');
    for(const key of Object.keys(process.env))if(key.startsWith('VITE_'))delete process.env[key];
    process.env.AEGIS_PREVIEW_ORIGIN=nodeOrigin; // The only public backend value passed to Vite.
    const {createServer}=await import(pathToFileURL(require.resolve('vite')).href);
    vite=await createServer({root:vueRoot,configFile:path.join(vueRoot,'vite.config.js'),configLoader:'native',envDir:false,cacheDir:path.join(tools.runtimeRoot,'vite-cache'),logLevel:'silent',clearScreen:false,server:{host:'127.0.0.1',port:0,strictPort:true,open:false}});
    await vite.listen();
    const address=vite.httpServer.address();if(!address||address.address!=='127.0.0.1')fail('VITE_BIND_FAILED','Vite did not bind to loopback.');
    const vueOrigin=localOrigin('http://127.0.0.1:'+address.port);
    if(!(await (await read(vueOrigin,'/')).text()).includes('/src/main.js'))fail('VITE_NOT_READY','Vue entrypoint did not load.');
    validateInventory(await (await read(vueOrigin,'/api/go/v1/devices')).json());
    const sharedIds=[];for(const device of devices)sharedIds.push(...validateBackups(await (await read(vueOrigin,'/api/go/v1/devices/'+device.id+'/backups')).json(),device.id));
    if(JSON.stringify(ids.sort())!==JSON.stringify(sharedIds.sort()))fail('DATASET_MISMATCH','Vue and Node did not read the same Go snapshot UUIDs.');
    sessionInfo={...sessionInfo,state:'ready',supervisorPid:supervisor.pid,goPid:goReady.pid,goOrigin,nodeOrigin,vueOrigin,binaryReused:reused,binaryKey:key};
    await writeFile(sessionFile,JSON.stringify(sessionInfo,null,2));
    console.log('\nAEGIS LOCAL DEMO — READY\n\nVue workspace: '+vueOrigin+'/#/overview\nFull preview:   '+nodeOrigin+'/#/overview\n\nGo API: Healthy\nConnector: Mock only\nDataset: Disposable synthetic SQLite\nDevice access: Disabled\n\nPress Ctrl+C or Q to stop this session.');
    healthTimer=setInterval(()=>void read(goOrigin,'/readyz',true).catch(()=>{if(!signal.aborted){failed=true;console.error('GO_UNAVAILABLE: This session is stopping; no fixture fallback.');requestStop();}}),5000);
    if(!signal.aborted)await new Promise(resolve=>signal.addEventListener('abort',resolve,{once:true}));
  }catch(error){
    if(!signal.aborted){failed=true;console.error('AEGIS LOCAL DEMO — NOT READY\n'+(error.message?.includes(': ')?error.message:'STARTUP_FAILED: Check the documented local tools, dependencies and external runtime paths.'));}
  }finally{
    clearInterval(healthTimer);cancellation.abort();
    if(vite)await vite.close().catch(()=>{});
    if(supervisor&&supervisor.exitCode===null){
      const exited=new Promise(resolve=>supervisor.once('exit',resolve));
      supervisor.stdin.end('quit\n');
      await Promise.race([exited,new Promise(resolve=>setTimeout(resolve,5000))]);
      // The enclosing Windows Job reaps any remaining owned descendants on controller exit.
    }
    if(sessionFile&&sessionInfo)await writeFile(sessionFile,JSON.stringify({...sessionInfo,state:'stopped',failed},null,2)).catch(()=>{});
    if(lock){await lock.close();if(await readFile(lockPath,'utf8').catch(()=>'')===lockNonce)await unlink(lockPath);}
    commands.close();
    console.log(failed?'AEGIS LOCAL DEMO — FAILED':'AEGIS LOCAL DEMO — STOPPED');
    process.exit(failed?1:0);
  }
}
if(process.argv[1]&&import.meta.url===pathToFileURL(path.resolve(process.argv[1])).href)await main();

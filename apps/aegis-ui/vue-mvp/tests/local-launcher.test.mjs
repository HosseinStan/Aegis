import {test} from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import os from 'node:os';
import {mkdtemp,mkdir,writeFile,readFile,access,rm} from 'node:fs/promises';
import {parseTools,localOrigin,validateInventory,validateBackups,parseGoPackages,copyOfflineCache} from '../launcher/workspace.mjs';
import {samePnpmLockContent} from '../launcher/lockfile-integrity.mjs';

const root=path.resolve('local-launcher-unit-only');
const tools=Object.fromEntries(['nodeExe','pnpmCjs','goRoot','goArchive','moduleCacheSeed','runtimeRoot'].map(key=>[key,path.join(root,key)]));
test('tools configuration accepts only explicit absolute paths, with no credential fields',()=>{
  assert.equal(parseTools(tools).nodeExe,tools.nodeExe);
  assert.throws(()=>parseTools({...tools,password:'must-never-be-accepted'}),/TOOLS_INVALID/);
  assert.throws(()=>parseTools({...tools,goRoot:'relative/sdk'}),/TOOLS_INVALID/);
});
test('reported origins must be exact loopback HTTP endpoints',()=>{
  assert.equal(localOrigin('http://127.0.0.1:12345'),'http://127.0.0.1:12345');
  for(const value of ['http://0.0.0.0:1234','https://127.0.0.1:1234','http://user:secret@127.0.0.1:1234','http://127.0.0.1:1234/api','http://example.com:1234'])assert.throws(()=>localOrigin(value),/READINESS_INVALID/);
});
const devices=[['core-sw-01','192.0.2.10'],['core-sw-02','192.0.2.11'],['edge-rtr-01','198.51.100.20'],['access-sw-01','203.0.113.30']].map(([id,management_ip])=>({id,management_ip,enabled:false,vendor:'mock',protocol:'mock',source:'AEGIS_GO_ISOLATED_DEMO'}));
const inventory={meta:{source:'Aegis Go API — Isolated Demo',synthetic:true,request_id:'unit'},data:devices};
test('readiness inspects actual four disabled mock records, rather than summary counts',()=>{
  assert.equal(validateInventory(inventory).length,4);
  for(const data of [devices.slice(0,3),[devices[0],devices[0],devices[2],devices[3]],devices.map(d=>({...d,enabled:true})),devices.map(d=>({...d,vendor:'cisco'})),devices.map(d=>({...d,management_ip:'10.0.0.1'}))])assert.throws(()=>validateInventory({...inventory,data}),/READINESS_INVALID/);
  assert.throws(()=>validateInventory({...inventory,meta:{...inventory.meta,source:'Local Mock API'}}),/READINESS_INVALID/);
});
test('snapshot readiness requires three owned synthetic persisted records',()=>{
  const payload={meta:{source:'Aegis Go API — Disposable SQLite Demo',synthetic:true,request_id:'unit'},data:[1,2,3].map(n=>({id:'00000000-0000-4000-8000-'+String(n).padStart(12,'0'),device_id:'core-sw-01',synthetic:true,source:'AEGIS_GO_BACKUP_DEMO',config_type:'running',trigger_type:'demo_seed',content_hash:'a'.repeat(64)}))};
  assert.equal(validateBackups(payload,'core-sw-01').length,3);
  assert.throws(()=>validateBackups(payload,'core-sw-02'),/READINESS_INVALID/);
  assert.throws(()=>validateBackups({...payload,data:[]},'core-sw-01'),/READINESS_INVALID/);
  assert.throws(()=>validateBackups({...payload,data:payload.data.map(b=>({...b,synthetic:false}))},'core-sw-01'),/READINESS_INVALID/);
});
test('Go metadata stream parsing handles nested braces and escaped strings, rejects partial streams',()=>{
  const values=[{Dir:'x',Text:'{quote"}',Module:{Main:true}},{Dir:'y',GoFiles:['a.go']}];
  assert.deepEqual(parseGoPackages(values.map(v=>JSON.stringify(v,null,2)).join('\n')),values);
  for(const text of ['', '{"Dir":', 'garbage {}', '{} trailing'])assert.throws(()=>parseGoPackages(text),/BUILD_INPUTS_INVALID/);
});
test('cancelled cache preparation publishes nothing; a later session can prepare it safely',async()=>{
  const temporary=await mkdtemp(path.join(os.tmpdir(),'aegis-launcher-test-'));
  try{
    const seed=path.join(temporary,'seed'),runtime=path.join(temporary,'runtime'),first=path.join(runtime,'first'),next=path.join(runtime,'next');
    await mkdir(seed);await mkdir(first,{recursive:true});await mkdir(next);await writeFile(path.join(seed,'source.txt'),'existing offline dependency');
    const stopped=new AbortController();stopped.abort();
    await assert.rejects(copyOfflineCache(seed,runtime,first,stopped.signal),{name:'AbortError'});
    await assert.rejects(access(path.join(runtime,'module-cache')));
    await copyOfflineCache(seed,runtime,next,new AbortController().signal);
    assert.equal(await readFile(path.join(runtime,'module-cache/source.txt'),'utf8'),'existing offline dependency');
    assert.equal(await readFile(path.join(seed,'source.txt'),'utf8'),'existing offline dependency');
  }finally{
    if(path.dirname(temporary)!==path.resolve(os.tmpdir())||!path.basename(temporary).startsWith('aegis-launcher-test-'))throw new Error('Unsafe temporary cleanup path');
    await rm(temporary,{recursive:true});
  }
});
test('cache publication rejects a destination outside the owned runtime before writing',async()=>{
  const controller=new AbortController();
  await assert.rejects(copyOfflineCache('unused',path.join(root,'runtime'),path.join(root,'elsewhere'),controller.signal),/RUNTIME_PATH_UNSAFE/);
});

test('pnpm lockfile integrity allows only cross-platform line endings, not dependency drift',async()=>{
  const source=await readFile(path.resolve(import.meta.dirname,'../pnpm-lock.yaml'));
  const text=source.toString('utf8');
  assert(!text.includes('\uFEFF'),'The tracked lockfile must not contain a BOM');
  assert.equal(samePnpmLockContent(source,source),true);
  const unix=text.replace(/\r\n/g,'\n');
  const windows=Buffer.from(unix.replace(/\n/g,'\r\n'),'utf8');
  assert.equal(samePnpmLockContent(source,windows),true,'CRLF on Windows must not invalidate a frozen install');
  assert.equal(samePnpmLockContent(source,Buffer.from(unix.replace(/\n$/,''))),true,'One final newline may differ');
  const tampered=unix.replace('specifier: 3.5.44','specifier: 3.5.45');
  assert.notEqual(tampered,unix,'test must change an actual version');
  assert.equal(samePnpmLockContent(source,Buffer.from(tampered)),false,'Dependency version drift must fail');
  assert.equal(samePnpmLockContent(source,Buffer.from(unix+'\nextra: true\n')),false,'Additional YAML entries must fail');
  assert.equal(samePnpmLockContent(source,Buffer.from('\uFEFF'+unix)),false,'Unexpected BOM must fail');
  assert.equal(samePnpmLockContent(source,Buffer.from(unix.replace(/\n/,'\r'))),false,'Lone CR must fail');
  assert.equal(samePnpmLockContent(source,Buffer.from([0xff])),false,'Invalid UTF-8 must fail');
});

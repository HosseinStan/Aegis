import {test,afterEach} from 'node:test';
import assert from 'node:assert/strict';

globalThis.__AEGIS_PREVIEW_ORIGIN__='http://127.0.0.1:1234';
const {getBackupWorkspace,getDashboard,savedLink,compareLink,INVENTORY_SOURCE,BACKUP_SOURCE}=await import('../src/api/go-demo.js');
const realFetch=globalThis.fetch;
afterEach(()=>{globalThis.fetch=realFetch;});
const devices=['core-sw-01','core-sw-02','edge-rtr-01','access-sw-01'].map((id,index)=>({id,display_name:id.toUpperCase(),hostname:id,management_ip:'192.0.2.'+(index+1),device_group:index<2?'hq':id,platform:'synthetic',source:'AEGIS_GO_ISOLATED_DEMO',enabled:false,vendor:'mock',protocol:'mock'}));
const backups=devices.flatMap((device,index)=>[1,2,3].map(version=>({id:'00000000-0000-4000-8000-'+String(index*3+version).padStart(12,'0'),device_id:device.id,source:'AEGIS_GO_BACKUP_DEMO',synthetic:true,config_type:'running',trigger_type:'demo_seed',created_at:'2026-11-0'+version+'T12:00:00Z',retrieved_at:'2026-11-0'+version+'T12:00:00Z',content_hash:'a'.repeat(64),size_bytes:123+version})));
const signal=()=>new AbortController().signal;
function mock({inventory=devices,records=backups,failDevice=null,status=503,abort=false}={}){
  const calls=[];
  globalThis.fetch=async(path,options)=>{
    calls.push({path,method:options.method,signal:options.signal});
    if(abort)throw new DOMException('Aborted','AbortError');
    const history=/^\/api\/go\/v1\/devices\/([^/]+)\/backups$/.exec(path);
    const isInventory=path==='/api/go/v1/devices',profile=!isInventory&&!history;
    const failed=profile||history?.[1]===failDevice;
    const source=isInventory||profile?INVENTORY_SOURCE:BACKUP_SOURCE;
    const data=isInventory?inventory:records.filter(b=>b.device_id===history?.[1]);
    return new Response(JSON.stringify({...(failed?{error:{message:'Controlled failure'}}:{data}),meta:{source,synthetic:true,request_id:'request-'+calls.length}}),{status:profile?404:failed?status:200});
  };
  return calls;
}
test('all four Go histories aggregate 12 unchanged identities and UTC timestamps using five GETs',async()=>{
  const calls=mock(),result=await getBackupWorkspace(null,signal());
  assert.equal(result.devices.length,4);assert.equal(result.backups.length,12);assert.equal(result.requests.length,5);
  assert.deepEqual(new Set(result.backups.map(b=>b.id)),new Set(backups.map(b=>b.id)));
  for(const b of result.backups){assert.equal(b.created_at,backups.find(original=>original.id===b.id).created_at);assert.equal(b.device.id,b.device_id);}
  assert.ok(result.backups[0].created_at>result.backups.at(-1).created_at);
  assert.equal(calls.length,5);assert.ok(calls.every(call=>call.method==='GET'&&!call.path.endsWith('/diff')));
});
test('filter requests only its own three records and keeps all four device options',async()=>{
  const calls=mock(),result=await getBackupWorkspace(devices[0].id,signal());
  assert.equal(result.devices.length,4);assert.equal(result.backups.length,3);assert.ok(result.backups.every(b=>b.device_id===devices[0].id));
  assert.deepEqual(calls.map(c=>c.path),['/api/go/v1/devices','/api/go/v1/devices/core-sw-01/backups']);
});
test('unknown device preserves native profile 404 and does not substitute all histories',async()=>{
  const calls=mock();await assert.rejects(getBackupWorkspace('missing',signal()),error=>error.status===404&&error.path.endsWith('/missing'));
  assert.equal(calls.length,2);
});
test('empty inventory and empty saved histories stay empty',async()=>{
  const calls=mock({inventory:[]});assert.equal((await getBackupWorkspace(null,signal())).backups.length,0);assert.equal(calls.length,1);
  mock({records:[]});assert.equal((await getBackupWorkspace(devices[0].id,signal())).backups.length,0);
});
for(const status of [401,403,404,503])test('history HTTP '+status+' rejects the entire collection with evidence, never a partial success',async()=>{
  mock({failDevice:devices[1].id,status});await assert.rejects(getBackupWorkspace(null,signal()),error=>error.status===status&&typeof error.requestId==='string'&&error.path.endsWith('/core-sw-02/backups'));
});
test('metadata from another source and repeated IDs fail closed',async()=>{
  mock({records:backups.map(b=>({...b,source:'FIXTURE'}))});await assert.rejects(getBackupWorkspace(null,signal()),/unexpected backup metadata/);
  mock({records:[...backups,backups[0]]});await assert.rejects(getBackupWorkspace(null,signal()),/unexpected backup metadata/);
});
test('cancelled requests propagate AbortError and the cancellation signal',async()=>{
  const controller=new AbortController();controller.abort();const calls=mock({abort:true});
  await assert.rejects(getBackupWorkspace(null,controller.signal),{name:'AbortError'});assert.equal(calls[0].signal.aborted,true);
});
test('View links preserve exact Vue UUID; Compare uses only same-device saved IDs',async()=>{
  mock();const result=await getBackupWorkspace(null,signal()),chosen=result.backups[0];
  const view=new URL('http://127.0.0.1/'+savedLink(chosen));
  assert.ok(view.hash.includes('/devices/'+chosen.device_id+'?tab=configuration&backup='+chosen.id));
  const compare=new URL(compareLink(chosen,result.backups)),query=new URLSearchParams(compare.hash.split('?')[1]);
  assert.equal(compare.origin,'http://127.0.0.1:1234');assert.equal(query.get('device'),chosen.device_id);assert.equal(query.get('right'),chosen.id);
  assert.ok(result.backups.some(b=>b.device_id===chosen.device_id&&b.id===query.get('left')));
});
test('Dashboard still derives 4 / 12 / 100% / 3 from the shared collection',async()=>{
  mock();const result=await getDashboard(signal());assert.equal(result.devices.length,4);assert.equal(result.backups.length,12);assert.equal(result.coverage,100);assert.equal(result.groups.length,3);
});

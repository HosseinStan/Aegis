import {test,afterEach} from 'node:test';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {effectScope} from 'vue';
import {useGoResource} from '../src/composables/useGoResource.js';
import {presentNativeDiff} from '../src/api/native-diff.js';

globalThis.__AEGIS_PREVIEW_ORIGIN__='http://127.0.0.1:1';
const {getConfigurationComparison,INVENTORY_SOURCE,BACKUP_SOURCE}=await import('../src/api/go-demo.js');
const realFetch=globalThis.fetch;
afterEach(()=>{globalThis.fetch=realFetch;});
const device={id:'core-sw-01',display_name:'CORE-SW-01',hostname:'CORE-SW-01',management_ip:'192.0.2.10',device_group:'hq',platform:'synthetic',source:'AEGIS_GO_ISOLATED_DEMO',enabled:false,vendor:'mock',protocol:'mock'};
const oldID='00000000-0000-4000-8000-000000000002',newID='00000000-0000-4000-8000-000000000003';
const banner='! AEGIS SYNTHETIC DEMO — DISPOSABLE BACKUP SNAPSHOT';
function snapshot(id,revision){
  const config=banner+'\nhostname CORE-SW-01\n! synthetic revision '+revision+'\n description Synthetic snapshot 0'+revision+'\nend\n';
  return {id,device_id:device.id,source:'AEGIS_GO_BACKUP_DEMO',synthetic:true,config_type:'running',trigger_type:'demo_seed',created_at:'2026-11-0'+revision+'T00:00:00Z',retrieved_at:'2026-11-0'+revision+'T00:00:00Z',content_hash:createHash('sha256').update(config).digest('hex'),size_bytes:Buffer.byteLength(config),config};
}
const left=snapshot(oldID,2),right=snapshot(newID,3);
const unified='--- '+oldID+'\n+++ '+newID+'\n@@ -1,6 +1,6 @@\n '+banner+'\n hostname CORE-SW-01\n-! synthetic revision 2\n+! synthetic revision 3\n- description Synthetic snapshot 02\n+ description Synthetic snapshot 03\n end\n \n';
const native={left_backup_id:oldID,right_backup_id:newID,source:'AEGIS_GO_DIFF_DEMO',changed:true,added_lines:2,removed_lines:2,diff:unified};
const metadata=({config,...record})=>record;
const selection={deviceId:device.id,leftId:oldID,rightId:newID},signal=()=>new AbortController().signal;
function mock({records=[metadata(right),metadata(left)],contentChange={},diffChange={},failPath=null,status=404,inventory=[device]}={}){
  const calls=[];
  globalThis.fetch=async(path,options)=>{
    calls.push({path,method:options.method});
    const base='/api/go/v1/devices',isInventory=path===base,isHistory=path===base+'/'+device.id+'/backups',isDiff=path.includes('/diff/');
    const id=path.split('/').at(-1),record=id===oldID?left:id===newID?right:null;
    const identical=isDiff&&path.endsWith('/'+oldID+'/diff/'+oldID);
    const diff=identical?{...native,right_backup_id:oldID,changed:false,added_lines:0,removed_lines:0,diff:''}:native;
    const data=isInventory?inventory:isHistory?records:isDiff?{...diff,...diffChange}:record?{...record,...contentChange}:null;
    const failed=path===failPath||data===null;
    return new Response(JSON.stringify({...(failed?{error:{message:'Controlled API failure'}}:{data}),meta:{source:isInventory?INVENTORY_SOURCE:BACKUP_SOURCE,synthetic:true,request_id:'request-'+calls.length}}),{status:failed?status:200});
  };
  return calls;
}
test('selected exact content is hash checked and native +2/-2 is kept with exact Unified text',async()=>{
  const calls=mock(),result=await getConfigurationComparison(selection,signal());
  assert.equal(result.left.id,oldID);assert.equal(result.right.id,newID);assert.deepEqual(result.native,native);
  assert.equal(result.rows.filter(row=>row.changed).length,2);assert.equal(result.requests.length,5);
  assert.ok(calls.every(call=>call.method==='GET'));assert.ok(calls.at(-1).path.endsWith('/'+oldID+'/diff/'+newID));
});
test('default selections use current revision 2 before revision 3',async()=>{
  mock();const result=await getConfigurationComparison({},signal());assert.equal(result.left.id,oldID);assert.equal(result.right.id,newID);
});
test('identical selection uses one content GET and preserves native zero counts',async()=>{
  const calls=mock(),result=await getConfigurationComparison({...selection,rightId:oldID},signal());
  assert.equal(result.native.changed,false);assert.equal(result.native.added_lines,0);assert.equal(result.native.removed_lines,0);assert.equal(result.native.diff,'');assert.equal(result.requests.length,4);assert.equal(calls.filter(c=>c.path.endsWith('/'+oldID)).length,2);
});
test('empty device inventory and empty history are explicit empty results',async()=>{
  mock({inventory:[]});assert.equal((await getConfigurationComparison({},signal())).device,null);
  mock({records:[]});assert.equal((await getConfigurationComparison({deviceId:device.id},signal())).left,null);
});
for(const status of [401,403,404,503])test('HTTP '+status+' preserves explicit UUIDs and evidence without fallback',async()=>{
  mock({failPath:'/api/go/v1/devices/core-sw-01/backups/'+oldID,status});
  await assert.rejects(getConfigurationComparison(selection,signal()),error=>error.status===status&&error.selection.leftId===oldID&&error.selection.rightId===newID&&typeof error.requestId==='string');
});
test('cross-device content and tampered bytes are rejected before native Diff is requested',async()=>{
  let calls=mock({contentChange:{device_id:'core-sw-02'}});await assert.rejects(getConfigurationComparison(selection,signal()),/unexpected synthetic/);assert.ok(!calls.some(c=>c.path.includes('/diff/')));
  calls=mock({contentChange:{config:left.config+'tampered'}});await assert.rejects(getConfigurationComparison(selection,signal()),/size and SHA-256/);assert.ok(!calls.some(c=>c.path.includes('/diff/')));
});
test('unlisted successful content does not satisfy current-history membership',async()=>{
  mock({records:[metadata(right)]});await assert.rejects(getConfigurationComparison(selection,signal()),/current saved running-configuration history/);
});
test('content metadata must agree with the selected history',async()=>{
  mock({records:[metadata(right),{...metadata(left),size_bytes:left.size_bytes+1}]});await assert.rejects(getConfigurationComparison(selection,signal()),/metadata does not match/);
});
for(const [label,change] of [
  ['wrong source',{source:'FIXTURE'}],['wrong left UUID',{left_backup_id:newID}],['wrong right UUID',{right_backup_id:oldID}],
  ['wrong changed flag',{changed:false}],['negative count',{added_lines:-1}],['wrong count',{removed_lines:99}],
  ['bad Unified text',{diff:'not a native patch'}],['wrong patch content',{diff:unified.replace('-! synthetic revision 2','-tampered')}]
])test('rejects native '+label,async()=>{
  mock({diffChange:change});await assert.rejects(getConfigurationComparison(selection,signal()),/Native Go Diff is inconsistent/);
});
test('native patch presentation preserves unchanged lines, line numbers and paired changes',()=>{
  const rows=presentNativeDiff(native,left,right);assert.equal(rows[0].left.number,1);assert.equal(rows[0].left.text,banner);
  const changed=rows.filter(row=>row.changed);assert.equal(changed[0].left.text,'! synthetic revision 2');assert.equal(changed[0].right.text,'! synthetic revision 3');assert.equal(changed[1].left.number,4);assert.equal(changed[1].right.kind,'added');
});
test('out-of-order old responses cannot overwrite newer comparison state; disposal aborts',async()=>{
  const pending=[],scope=effectScope();const resource=scope.run(()=>useGoResource((name,signal)=>new Promise(resolve=>pending.push({name,signal,resolve}))));
  try{
    const old=resource.load('old'),current=resource.load('current');assert.equal(pending[0].signal.aborted,true);
    pending[1].resolve({id:'current'});await current;pending[0].resolve({id:'old'});await old;
    assert.equal(resource.data.value.id,'current');assert.equal(resource.status.value,'ready');
  }finally{scope.stop();}
  assert.equal(pending[1].signal.aborted,true);
});

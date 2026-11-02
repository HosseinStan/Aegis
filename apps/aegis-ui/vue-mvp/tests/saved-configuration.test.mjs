import {test,afterEach} from 'node:test';
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';

globalThis.__AEGIS_PREVIEW_ORIGIN__='http://127.0.0.1:1';
const {getSavedConfiguration,BACKUP_SOURCE}=await import('../src/api/go-demo.js');
const realFetch=globalThis.fetch;
afterEach(()=>{globalThis.fetch=realFetch;});
const device='core-sw-01',old='00000000-0000-4000-8000-000000000001',latest='00000000-0000-4000-8000-000000000002';
const historyPath='/api/go/v1/devices/'+device+'/backups';
function backup(id,day){
  const config='! AEGIS SYNTHETIC DEMO — DISPOSABLE BACKUP SNAPSHOT\nhostname TEST-ONLY\n! synthetic revision '+day+'\n';
  return {id,device_id:device,config_type:'running',trigger_type:'demo_seed',source:'AEGIS_GO_BACKUP_DEMO',synthetic:true,
    created_at:'2026-11-0'+day+'T00:00:00Z',retrieved_at:'2026-11-0'+day+'T00:00:00Z',
    content_hash:createHash('sha256').update(config).digest('hex'),size_bytes:Buffer.byteLength(config),config};
}
const earlier=backup(old,1),newest=backup(latest,2);
const metadata=({config,...record})=>record;
function mock(history,content,{status=200,contentStatus=200,source=BACKUP_SOURCE}={}){
  const calls=[];
  globalThis.fetch=async(path,options)=>{
    calls.push({path,method:options.method});
    const isHistory=path===historyPath,httpStatus=isHistory?status:contentStatus;
    const body=httpStatus===200?{data:isHistory?history:content}:{error:{message:'Controlled API failure'}};
    return new Response(JSON.stringify({...body,meta:{source,synthetic:true,request_id:'unit-request'}}),{status:httpStatus,headers:{'Content-Type':'application/json'}});
  };
  return calls;
}
const signal=()=>new AbortController().signal;
test('latest version comes from sorted Go history; only GET reads are made',async()=>{
  const calls=mock([metadata(earlier),metadata(newest)],newest);
  const result=await getSavedConfiguration(device,null,signal());
  assert.equal(result.selected.id,latest);assert.equal(result.versions[0].id,latest);assert.equal(result.requests.length,2);
  assert.deepEqual(calls,[{path:historyPath,method:'GET'},{path:historyPath+'/'+latest,method:'GET'}]);
});
test('explicit older UUID remains the selected version',async()=>{
  mock([metadata(newest),metadata(earlier)],earlier);
  assert.equal((await getSavedConfiguration(device,old,signal())).selected.id,old);
});
test('empty history without a selection does not request substitute content',async()=>{
  const calls=mock([],null);const result=await getSavedConfiguration(device,null,signal());
  assert.equal(result.selected,null);assert.equal(calls.length,1);
});
test('unauthorized history preserves status, request ID and requested UUID',async()=>{
  mock([],null,{status:401});
  await assert.rejects(getSavedConfiguration(device,old,signal()),error=>error.status===401&&error.requestId==='unit-request'&&error.selectedId===old);
});
test('stale UUID gets 404 and never resolves to the latest content',async()=>{
  const calls=mock([metadata(newest)],null,{contentStatus:404});
  await assert.rejects(getSavedConfiguration(device,old,signal()),error=>error.status===404&&error.selectedId===old);
  assert.equal(calls[1].path,historyPath+'/'+old);
});
for(const [label,change] of [['cross-device',{device_id:'core-sw-02'}],['wrong selected UUID',{id:latest}]]){
  test('rejects '+label+' content',async()=>{
    mock([metadata(earlier)],{...earlier,...change});
    await assert.rejects(getSavedConfiguration(device,old,signal()),/unexpected synthetic/);
  });
}
test('content metadata must agree with the history record',async()=>{
  mock([{...metadata(earlier),size_bytes:earlier.size_bytes+1}],earlier);
  await assert.rejects(getSavedConfiguration(device,old,signal()),/metadata does not match/);
});
test('an unlisted 200 response is rejected without inventing a native HTTP 404',async()=>{
  mock([metadata(newest)],earlier);
  await assert.rejects(getSavedConfiguration(device,old,signal()),error=>error.status===null&&error.selectedId===old&&error.message.includes('current saved running-configuration history'));
});
test('matching metadata cannot conceal tampered configuration bytes',async()=>{
  mock([metadata(earlier)],{...earlier,config:earlier.config+'tampered\n'});
  await assert.rejects(getSavedConfiguration(device,old,signal()),/size and SHA-256/);
});
test('rejects a fixture source marker',async()=>{
  mock([metadata(earlier)],earlier,{source:'Local Mock API'});
  await assert.rejects(getSavedConfiguration(device,old,signal()),/expected isolated synthetic Go demo/);
});
test('requires the saved synthetic configuration banner',async()=>{
  const record={...earlier,config:'not a saved synthetic snapshot'};
  mock([metadata(record)],record);
  await assert.rejects(getSavedConfiguration(device,old,signal()),/unexpected synthetic/);
});

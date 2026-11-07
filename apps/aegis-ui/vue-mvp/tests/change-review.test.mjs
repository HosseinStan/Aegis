import {test} from 'node:test';
import assert from 'node:assert/strict';
import {effectScope,reactive} from 'vue';
import {useDemoViewed} from '../src/composables/useDemoViewed.js';

globalThis.__AEGIS_PREVIEW_ORIGIN__='http://127.0.0.1:1';
const {comparisonLink}=await import('../src/api/go-demo.js');
const pair={deviceId:'core-sw-01',leftId:'00000000-0000-4000-8000-000000000002',rightId:'00000000-0000-4000-8000-000000000003'};
function component(){
  const scope=effectScope(),selection=reactive({...pair,layout:'side'});
  return {scope,selection,viewed:scope.run(()=>useDemoViewed(selection))};
}
test('review deep links retain exact device and ordered snapshot IDs',()=>{
  assert.equal(comparisonLink({},'review'),'#/review');
  assert.equal(comparisonLink({deviceId:pair.deviceId},'review'),'#/review?device=core-sw-01');
  const route=comparisonLink(pair,'review'),query=new URLSearchParams(route.split('?')[1]);
  assert.ok(route.startsWith('#/review?'));assert.equal(query.get('device'),pair.deviceId);assert.equal(query.get('left'),pair.leftId);assert.equal(query.get('right'),pair.rightId);
});
test('Compare and Review links change only the screen, never the selected evidence',()=>{
  assert.equal(comparisonLink(pair,'review').replace('#/review?','#/compare?'),comparisonLink(pair));
});
test('Viewed is local memory and produces no HTTP request',()=>{
  const realFetch=globalThis.fetch;let requests=0;
  globalThis.fetch=()=>{requests++;throw new Error('No review write is permitted');};
  const state=component();
  try{assert.equal(state.viewed.value,false);state.viewed.value=true;assert.equal(state.viewed.value,true);assert.equal(requests,0);}
  finally{state.scope.stop();globalThis.fetch=realFetch;}
});
for(const [key,value] of [['deviceId','core-sw-02'],['leftId',pair.rightId],['rightId',pair.leftId]])test('Viewed resets when '+key+' changes',()=>{
  const state=component();
  try{state.viewed.value=true;state.selection[key]=value;assert.equal(state.viewed.value,false);}
  finally{state.scope.stop();}
});
test('returning to a previously viewed pair does not restore its marker',()=>{
  const state=component();
  try{state.viewed.value=true;state.selection.leftId=pair.rightId;state.selection.leftId=pair.leftId;assert.equal(state.viewed.value,false);}
  finally{state.scope.stop();}
});
test('remount/reload starts a fresh unviewed component',()=>{
  const first=component();first.viewed.value=true;first.scope.stop();const second=component();
  try{assert.equal(second.viewed.value,false);}finally{second.scope.stop();}
});
test('presentation changes retain Viewed for the unchanged exact evidence pair',()=>{
  const state=component();
  try{state.viewed.value=true;state.selection.layout='unified';assert.equal(state.viewed.value,true);}
  finally{state.scope.stop();}
});

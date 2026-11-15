<script setup>
import {computed,onMounted,ref} from 'vue';
import AppIcon from '../src/components/AppIcon.vue';
import DeviceForm from './DeviceForm.vue';
import CredentialForm from './CredentialForm.vue';
import DeviceActions from './DeviceActions.vue';

const session=ref(null),checking=ref(true),devices=ref([]),credentials=ref([]),selectedId=ref('');
const panel=ref('devices'),editing=ref(null),busy=ref(false),error=ref(''),notice=ref('');
const operator=computed(()=>session.value?.permissions.includes('devices.manage'));
const selected=computed(()=>devices.value.find(device=>device.id===selectedId.value));
const approved=computed(()=>devices.value.filter(device=>device.target_approved).length);
let workspaceRequest=0;

async function api(path,{method='GET',body}={}){
  const response=await fetch('/api/lab'+path,{
    method,cache:'no-store',credentials:'same-origin',redirect:'error',
    ...(body?{headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}:{})
  });
  if(response.status===204)return;
  const result=await response.json();
  if(!response.ok){
    if(response.status===401&&session.value)clearSession();
    throw new Error(result.error?.message||'The Lab request could not be completed.');
  }
  return result.data;
}
function clearSession(){
  workspaceRequest++;
  session.value=null;
  devices.value=[];
  credentials.value=[];
  selectedId.value='';
  panel.value='devices';
  editing.value=null;
}
async function loadWorkspace(){
  const request=++workspaceRequest;
  const [records,labels]=await Promise.all([api('/devices'),operator.value?api('/credentials?limit=100'):[]]);
  if(request!==workspaceRequest||!session.value)return;
  devices.value=records;
  credentials.value=labels;
  if(!records.some(record=>record.id===selectedId.value))selectedId.value=records[0]?.id||'';
}
async function perform(task){
  busy.value=true;error.value='';notice.value='';
  try{await task();}catch(cause){error.value=cause.message||'The local Lab is unavailable.';}
  finally{busy.value=false;}
}
onMounted(async()=>{
  try{
    session.value=await api('/session');
    await loadWorkspace();
  }catch(cause){
    if(session.value)error.value=cause.message;
  }finally{checking.value=false;}
});
function login(event){
  const form=event.currentTarget, input=new FormData(form);
  form.elements.password.value='';
  return perform(async()=>{
    session.value=await api('/session',{method:'POST',body:{username:input.get('role'),password:input.get('password')}});
    input.delete('password');
    await loadWorkspace();
  });
}
function logout(){
  return perform(async()=>{await api('/session',{method:'DELETE'});clearSession();});
}
function show(next,record=null){panel.value=next;editing.value=record;error.value='';notice.value='';}
function openDevice(record){selectedId.value=record.id;show('devices');}
function saveDevice(body){
  const id=editing.value?.id;
  return perform(async()=>{
    const record=await api(id?'/devices/'+encodeURIComponent(id):'/devices',{method:id?'PUT':'POST',body});
    panel.value='devices';editing.value=null;selectedId.value=record.id;
    await loadWorkspace();
    notice.value=id?'Device details saved.':'Device registered locally. Registration did not contact the switch.';
  });
}
function saveCredential(body){
  return perform(async()=>{
    await api('/credentials',{method:'POST',body});
    await loadWorkspace();
    notice.value='Credential encrypted and saved. It can now be assigned to a Lab device.';
  });
}
</script>

<template>
  <a href="#main" class="skip">Skip to content</a>
  <header class="site-head">
    <div class="container head-inner">
      <div class="brand"><span class="brand-mark" aria-hidden="true">✳</span><div>AEGIS<small>NETWORK WORKSPACE</small></div></div>
      <span class="lab-tag">LAB PROFILE</span>
      <div class="identity" v-if="session"><span>{{ operator?'Operator':'Read-only viewer' }}</span><button class="button inverse" :disabled="busy" @click="logout">Sign out</button></div>
    </div>
  </header>
  <main id="main" class="container main">
    <section class="hero">
      <div><div class="eyebrow">LOCAL LAB / CISCO SSH</div><h1>Lab Device Manager</h1><p>Register your Cisco switches, assign dedicated SSH credentials, and check what is required before connecting.</p></div>
      <div class="hero-status"><AppIcon name="lock"/><div><strong>Loopback only</strong><small>Independent Lab storage</small></div></div>
    </section>

    <p v-if="checking" class="message" role="status">Checking the local Lab session…</p>
    <section v-else-if="!session" class="card sign-in">
      <div class="eyebrow">LOCAL ACCESS</div><h2>Sign in to the Lab</h2><p>Use the password supplied to this local process at startup. The viewer can inspect records and saved backups. The operator can also manage devices and credentials.</p>
      <form class="form-grid" autocomplete="off" @submit.prevent="login">
        <label>Role<select name="role"><option value="operator">Lab operator</option><option value="viewer">Read-only viewer</option></select></label>
        <label>Lab password<input name="password" type="password" autocomplete="off" required></label>
        <p v-if="error" class="message danger wide" role="alert">{{ error }}</p>
        <button class="button primary" :disabled="busy">{{ busy?'Signing in…':'Sign in' }}</button>
      </form>
    </section>
    <template v-else>
      <section class="stats" aria-label="Lab summary">
        <div class="card stat"><span>Registered devices</span><strong>{{ devices.length }}</strong><small>Persistent Lab inventory</small></div>
        <div class="card stat"><span>Approved registered targets</span><strong>{{ approved }}</strong><small>Exact IP, port and host key</small></div>
        <div class="card stat"><span>Backup creation</span><strong class="small-stat">Locked</strong><small>At-rest protection pending</small></div>
      </section>
      <nav class="section-nav" aria-label="Lab sections">
        <button :aria-current="panel==='devices'?'page':null" @click="show('devices')"><AppIcon name="server"/>Devices</button>
        <button v-if="operator" :aria-current="panel==='credential'?'page':null" @click="show('credential')"><AppIcon name="lock"/>SSH credentials</button>
        <button class="button primary register" v-if="operator" @click="show('register')">+ Register device</button>
      </nav>
      <p v-if="error" class="message danger" role="alert">{{ error }} <button v-if="panel==='devices'" class="text-button" :disabled="busy" @click="perform(loadWorkspace)">Retry</button></p>
      <p v-if="notice" class="message success" role="status">{{ notice }}</p>
      <DeviceForm v-if="panel==='register'&&operator" :record="editing" :credentials="credentials" :busy="busy" @save="saveDevice" @cancel="show('devices')" @credential="show('credential')"/>
      <CredentialForm v-else-if="panel==='credential'&&operator" :credentials="credentials" :busy="busy" @save="saveCredential" @done="show('devices')"/>
      <section v-else class="device-grid">
        <aside class="card inventory">
          <header><div class="eyebrow">LAB INVENTORY</div><h2>Registered devices</h2></header>
          <p v-if="!devices.length" class="empty">No Lab devices registered.</p>
          <button v-for="device in devices" :key="device.id" type="button" class="device-option" :aria-pressed="device.id===selectedId" @click="openDevice(device)">
            <span class="device-glyph"><AppIcon name="server"/></span>
            <span><strong>{{ device.display_name }}</strong><small>{{ device.hostname||device.management_ip }} · {{ device.platform==='ios-xe'?'IOS XE':'IOS' }}</small></span>
            <span class="pill" :class="device.connection_ready?'good':'blocked'">{{ device.connection_ready?'Ready':'Blocked' }}</span>
          </button>
        </aside>
        <DeviceActions v-if="selected" :key="selected.id" :device="selected" :operator="operator" :api="api" @edit="show('register',selected)" @refresh="loadWorkspace"/>
        <article v-else class="card empty-detail"><span class="device-glyph"><AppIcon name="server"/></span><h2>{{ operator?'Register your first switch':'No device to inspect' }}</h2><p>{{ operator?'Create a dedicated SSH credential, then enter your switch details. Registration stays local and does not open an SSH connection.':'Ask the Lab operator to register a switch.' }}</p><button v-if="operator" class="button primary" @click="show('register')">Register device</button></article>
      </section>
    </template>
    <footer>AEGIS LAB · Device reads require approval · Phase E deferred · Production blocked</footer>
  </main>
</template>

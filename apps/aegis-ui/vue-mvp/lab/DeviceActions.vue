<script setup>
import {computed,ref} from 'vue';
import AppIcon from '../src/components/AppIcon.vue';
const props=defineProps({device:Object,operator:Boolean,api:Function});
const emit=defineEmits(['edit','refresh']);
const backups=ref(null),left=ref(''),right=ref(''),output=ref(null),error=ref(''),busy=ref(false);
const base=computed(()=>'/devices/'+encodeURIComponent(props.device.id));
const mayConnect=computed(()=>props.operator&&props.device.connection_ready);
const testLabel=computed(()=>{
  const test=props.device.last_connection_test;
  if(!test)return 'Not tested this session';
  return (test.code?test.code.replaceAll('_',' '):'Passed')+' · '+formatDate(test.at);
});
const formatDate=value=>new Date(value).toLocaleString('en-GB',{timeZone:'UTC'})+' UTC';
async function run(task){
  busy.value=true;output.value=null;error.value='';
  try{await task();}catch(cause){error.value=cause.message;}finally{busy.value=false;}
}
function connect(action){
  return run(async()=>{
    try{
      const result=await props.api(base.value+'/'+action,{method:'POST'});
      output.value=action==='test-ssh'?{title:'SSH test passed',text:''}:{title:'Running configuration · read from device',text:result.config};
    }finally{emit('refresh');}
  });
}
function history(){
  return run(async()=>{
    backups.value=null;
    const records=await props.api(base.value+'/backups?limit=100');
    backups.value=records||[];left.value=backups.value[1]?.id||backups.value[0]?.id||'';right.value=backups.value[0]?.id||'';
  });
}
function view(){
  return run(async()=>{
    const result=await props.api(base.value+'/backups/'+encodeURIComponent(right.value));
    output.value={title:'Saved configuration',text:result.config};
  });
}
function compare(){
  return run(async()=>{
    const result=await props.api(base.value+'/backups/'+encodeURIComponent(left.value)+'/diff/'+encodeURIComponent(right.value));
    output.value={title:'Saved configuration diff · +'+result.added_lines+' / −'+result.removed_lines,text:result.diff||'No changes.'};
  });
}
</script>

<template>
  <article class="card device-detail">
    <header class="device-header"><div><div class="eyebrow">REGISTERED LAB DEVICE</div><h2>{{ device.display_name }}</h2><p>{{ device.management_ip }} · SSH port {{ device.management_port }}</p></div><button v-if="operator" class="button" :disabled="busy" @click="emit('edit')">Edit device</button></header>
    <dl class="device-meta"><div><dt>Hostname</dt><dd>{{ device.hostname||'Unspecified' }}</dd></div><div><dt>Vendor / platform</dt><dd>Cisco / {{ device.platform==='ios-xe'?'IOS XE':'IOS' }}</dd></div><div><dt>Group</dt><dd>{{ device.device_group||'Unassigned' }}</dd></div><div v-if="device.description"><dt>Description</dt><dd>{{ device.description }}</dd></div></dl>
    <section class="readiness" aria-label="Connection readiness">
      <h3>Connection readiness</h3>
      <dl class="readiness-list">
        <div><dt>Registration</dt><dd><span class="pill good">Saved locally</span></dd></div>
        <div><dt>SSH credential</dt><dd><span class="pill" :class="device.credential_assigned?'good':'blocked'">{{ device.credential_assigned?'Assigned':'Missing' }}</span></dd></div>
        <div><dt>Target approval</dt><dd><span class="pill" :class="device.target_approved?'good':'blocked'">{{ device.target_approved?'Approved exact endpoint':'Blocked' }}</span></dd></div>
        <div><dt>SSH host key</dt><dd><span class="pill" :class="device.host_key_pinned?'good':'blocked'">{{ device.host_key_pinned?'Pinned · checked at handshake':'Missing verified pin' }}</span></dd></div>
        <div><dt>Last connection test</dt><dd>{{ testLabel }}</dd></div>
        <div><dt>Last successful backup</dt><dd>{{ device.last_backup_at?formatDate(device.last_backup_at):'None' }}</dd></div>
      </dl>
      <p v-if="!device.target_approved" class="message">Live reads require a local approval for this exact IP and port with an independently verified host key.</p>
      <p v-else-if="!device.connection_ready" class="message">An operator must save this device with its assigned credential under the approved Lab profile.</p>
      <p v-else-if="!operator" class="message">The viewer can open stored snapshots. An operator must initiate SSH.</p>
    </section>
    <section class="device-actions">
      <h3>Device reads</h3><p>These operations contact the switch. Reading its running configuration displays text without saving a snapshot.</p>
      <div class="form-actions"><button class="button primary" :disabled="!mayConnect||busy" @click="connect('test-ssh')">Test SSH</button><button class="button" :disabled="!mayConnect||busy" @click="connect('read-running')">Read running configuration</button></div>
      <h3>Local backups</h3><p>Backup creation writes to Aegis storage. It is locked until protection for sensitive content and change evidence at rest is approved.</p>
      <div class="form-actions"><button class="button" disabled>Create manual backup</button><button class="button" :disabled="busy" @click="history">View backup history</button></div>
      <p v-if="busy" class="message" role="status">Reading Lab data…</p>
      <p v-if="error" class="message danger" role="alert">{{ error }}</p>
      <template v-if="backups">
        <p v-if="!backups.length" class="message" role="status">This Lab device has no saved backups.</p>
        <div v-else class="form-grid history">
          <label>Before<select v-model="left"><option v-for="item in backups" :key="item.id" :value="item.id">{{ formatDate(item.created_at) }} · {{ item.id.slice(0,8) }}</option></select></label>
          <label>Selected saved version<select v-model="right"><option v-for="item in backups" :key="item.id" :value="item.id">{{ formatDate(item.created_at) }} · {{ item.id.slice(0,8) }}</option></select></label>
          <div class="wide form-actions"><button class="button" :disabled="busy||!right" @click="view">View saved configuration</button><button class="button" :disabled="busy||!left||!right||backups.length<2" @click="compare">Compare backups</button></div>
        </div>
      </template>
      <section v-if="output" class="output"><h3>{{ output.title }}</h3><pre v-if="output.text" tabindex="0">{{ output.text }}</pre></section>
    </section>
  </article>
</template>

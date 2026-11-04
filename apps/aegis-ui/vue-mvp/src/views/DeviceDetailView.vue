<script setup>
import {computed,watch} from 'vue';
import AppIcon from '../components/AppIcon.vue';
import ApiEvidence from '../components/ApiEvidence.vue';
import ApiState from '../components/ApiState.vue';
import SavedConfiguration from '../components/SavedConfiguration.vue';
import {getDevice,INVENTORY_SOURCE} from '../api/go-demo.js';
import {useGoResource} from '../composables/useGoResource.js';

const props=defineProps({id:{type:String,required:true},tab:{type:String,default:'overview'},backupId:{type:String,default:null}});
const emit=defineEmits(['device-title']);
const {status,data,error,load}=useGoResource(getDevice);
watch(()=>props.id,id=>load(id),{immediate:true});
watch(data,value=>emit('device-title',value?.device.display_name||'Device details'));
const facts=computed(()=>data.value?[
  ['Hostname',data.value.device.hostname],['Management address',data.value.device.management_ip],
  ['Device group',data.value.device.device_group||'Ungrouped'],['Platform',data.value.device.platform],
  ['Vendor / protocol',data.value.device.vendor+' / '+data.value.device.protocol],['Connection','Disabled · no live access'],
  ['Software / serial','Not supplied by Go Inventory API'],['Risk','Not evaluated']
]:[]);
const configuration=computed(()=>props.tab==='configuration');
const path=computed(()=>'#/devices/'+encodeURIComponent(props.id));
const links=computed(()=>[
  {label:'Backup History',description:'All saved versions for this device · native Vue',icon:'archive',href:'#/backups?'+new URLSearchParams({device:props.id}),external:false},
  {label:'Change Review',description:'Go Diff evidence · risk not evaluated · native Vue',icon:'diff',href:'#/review?'+new URLSearchParams({device:props.id}),external:false}
]);
</script>

<template>
  <a class="back-link" href="#/devices">← Back to device inventory</a>
  <ApiState v-if="status!=='ready'" :status="status==='idle'?'loading':status" :error="error" :title="status==='error'?(error?.status===404?'Device not found':'Go device profile unavailable'):'Loading Go device profile…'" description="Reading the native Go Inventory record over HTTP." @retry="load(id)"/>
  <template v-else>
    <section class="page-heading"><div class="device-heading"><span class="device-glyph"><AppIcon name="server"/></span><div><div class="eyebrow">{{ data.device.device_group }} / SYNTHETIC DEVICE</div><h1>{{ data.device.display_name }}</h1><p>{{ data.device.management_ip }} · Read-only Go demo record</p></div></div><span class="status-tag">Disabled · no live access</span></section>
    <ApiEvidence :requests="data.requests" :source="INVENTORY_SOURCE"/>
    <nav class="device-tabs" aria-label="Device detail sections"><a :href="path" :class="{active:!configuration}" :aria-current="!configuration?'page':undefined">Device profile</a><a :href="path+'?tab=configuration'" :class="{active:configuration}" :aria-current="configuration?'page':undefined">Saved Configuration</a></nav>
    <SavedConfiguration v-if="configuration" :device-id="id" :backup-id="backupId"/>
    <section v-else class="detail-grid">
      <article class="card"><header class="card-head"><div><h2>Device profile</h2><p>Native Go Inventory record · disposable SQLite.</p></div><span class="tiny-tag">DEMO / UNVERIFIED</span></header><dl class="facts-grid"><div v-for="[label,value] in facts" :key="label"><dt>{{ label }}</dt><dd>{{ value }}</dd></div></dl><div class="detail-note"><AppIcon name="lock"/>Live connections, credentials and configuration writes are disabled.</div></article>
      <article class="card"><header class="card-head"><div><h2>Continue with this device</h2><p>Saved configurations, history and Change Review stay in this Vue workspace.</p></div></header><div class="feature-links"><a :href="path+'?tab=configuration'"><AppIcon name="code"/><span>Saved Configuration<small>Read the latest saved synthetic Go snapshot here</small></span><AppIcon name="arrow"/></a><a v-for="link in links" :key="link.label" :href="link.href" :target="link.external?'_blank':undefined" :rel="link.external?'noopener noreferrer':undefined"><AppIcon :name="link.icon"/><span>{{ link.label }}<small>{{ link.description }}</small></span><AppIcon :name="link.external?'external':'arrow'"/></a></div></article>
    </section>
  </template>
</template>

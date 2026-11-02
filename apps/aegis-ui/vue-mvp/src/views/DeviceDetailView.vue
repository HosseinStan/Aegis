<script setup>
import {computed,watch} from 'vue';
import AppIcon from '../components/AppIcon.vue';
import ApiEvidence from '../components/ApiEvidence.vue';
import ApiState from '../components/ApiState.vue';
import {getDevice,INVENTORY_SOURCE,previewLink} from '../api/go-demo.js';
import {useGoResource} from '../composables/useGoResource.js';
const props=defineProps({id:{type:String,required:true}}),emit=defineEmits(['device-title']);
const {status,data,error,load}=useGoResource(getDevice);
watch(()=>props.id,id=>load(id),{immediate:true});
watch(data,value=>emit('device-title',value?.device.display_name||'Device details'));
const facts=computed(()=>data.value?[
  ['Hostname',data.value.device.hostname],['Management address',data.value.device.management_ip],
  ['Device group',data.value.device.device_group||'Ungrouped'],['Platform',data.value.device.platform],
  ['Vendor / protocol',data.value.device.vendor+' / '+data.value.device.protocol],['Connection','Disabled · no live access'],
  ['Software / serial','Not supplied by Go Inventory API'],['Risk','Not evaluated']
]:[]);
const path=computed(()=>'/devices/'+encodeURIComponent(props.id));
const links=computed(()=>[
  {label:'Saved Configuration',description:'Latest saved synthetic text, never live state',icon:'code',href:previewLink(path.value+'?tab=configuration')},
  {label:'Backup History',description:'Persisted metadata and synthetic snapshots',icon:'archive',href:previewLink('/backups?'+new URLSearchParams({device:props.id}))},
  {label:'Change Review',description:'Go Diff evidence · risk not evaluated',icon:'diff',href:previewLink('/review?'+new URLSearchParams({device:props.id}))}
]);
</script>

<template>
  <a class="back-link" href="#/devices">← Back to device inventory</a>
  <ApiState v-if="status!=='ready'" :status="status==='idle'?'loading':status" :error="error" :title="status==='error'?(error?.status===404?'Device not found':'Go device profile unavailable'):'Loading Go device profile…'" description="Reading the native Go Inventory record over HTTP." @retry="load(id)"/>
  <template v-else>
    <section class="page-heading"><div class="device-heading"><span class="device-glyph"><AppIcon name="server"/></span><div><div class="eyebrow">{{ data.device.device_group }} / SYNTHETIC DEVICE</div><h1>{{ data.device.display_name }}</h1><p>{{ data.device.management_ip }} · Read-only Go demo record</p></div></div><span class="status-tag">Disabled · no live access</span></section>
    <ApiEvidence :requests="data.requests" :source="INVENTORY_SOURCE"/>
    <section class="detail-grid"><article class="card"><header class="card-head"><div><h2>Device profile</h2><p>Native Go Inventory record · disposable SQLite.</p></div><span class="tiny-tag">DEMO / UNVERIFIED</span></header><dl class="facts-grid"><div v-for="[label,value] in facts" :key="label"><dt>{{ label }}</dt><dd>{{ value }}</dd></div></dl><div class="detail-note"><AppIcon name="lock"/>Live connections, credentials and configuration writes are disabled.</div></article><article class="card"><header class="card-head"><div><h2>Continue with this device</h2><p>These features open the existing preview.</p></div></header><div class="feature-links"><a v-for="link in links" :key="link.label" :href="link.href" target="_blank" rel="noopener noreferrer"><AppIcon :name="link.icon"/><span>{{ link.label }}<small>{{ link.description }}</small></span><AppIcon name="external"/></a></div></article></section>
  </template>
</template>

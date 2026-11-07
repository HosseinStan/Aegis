<script setup>
import {computed,onMounted,ref,watch} from 'vue';
import AppIcon from '../components/AppIcon.vue';
import ApiEvidence from '../components/ApiEvidence.vue';
import ApiState from '../components/ApiState.vue';
import {getDevices,INVENTORY_SOURCE} from '../api/go-demo.js';
import {useGoResource} from '../composables/useGoResource.js';
const props=defineProps({query:{type:String,default:''},searchRevision:Number}),emit=defineEmits(['device-count']);
const search=ref(props.query),group=ref('all');
watch([()=>props.query,()=>props.searchRevision],([query])=>{
  search.value=query;
  group.value='all';
});
const {status,data,error,load}=useGoResource(getDevices);
watch([status,data],()=>emit('device-count',status.value==='ready'?data.value.devices.length:null));
onMounted(()=>load());
const groups=computed(()=>[...new Set(data.value?.devices.map(d=>d.device_group)||[])].sort());
const records=computed(()=>data.value?.devices.filter(d=>(group.value==='all'||group.value===d.device_group)&&[d.display_name,d.management_ip,d.device_group].join(' ').toLowerCase().includes(search.value.trim().toLowerCase()))||[]);
function clearFilters(){search.value='';group.value='all';}
</script>

<template>
  <section class="page-heading"><div><div class="eyebrow">DEVICE INVENTORY</div><h1>A home for every device.</h1><p>Synthetic disabled devices from the real Go API. Live access is off; risk is not assessed.</p></div><button class="btn btn-primary" :disabled="status==='loading'" @click="load()"><AppIcon name="refresh"/>Refresh inventory</button></section>
  <div :aria-busy="status==='loading'">
    <ApiState v-if="status!=='ready'" :status="status==='idle'?'loading':status" :error="error" :title="status==='error'?'Go Inventory unavailable':'Loading Go Inventory…'" description="Requesting device records over HTTP from the isolated Go API." @retry="load()"/>
    <template v-else>
      <ApiEvidence :requests="data.requests" :source="INVENTORY_SOURCE"/>
      <ApiState v-if="!data.devices.length" status="empty" title="No devices in the Go demo" description="The API returned an empty collection. No fixture records are shown." @retry="load()"/>
      <template v-else>
        <div class="filterbar"><label class="search-field"><AppIcon name="search"/><input v-model="search" aria-label="Search device inventory" placeholder="Search name, address, or group…"></label><label class="sr-only" for="device-group">Device group</label><select id="device-group" v-model="group"><option value="all">All device groups</option><option v-for="value in groups" :key="value" :value="value">{{ value||'Ungrouped' }}</option></select><span>{{ records.length }} of {{ data.devices.length }} devices · DEMO</span></div>
        <section class="card table-card"><div v-if="records.length" class="table-scroll"><table class="inventory-table" aria-label="Go device inventory"><thead><tr><th>Device</th><th>Management address</th><th>Device group</th><th>Demo status</th><th/></tr></thead><tbody><tr v-for="device in records" :key="device.id"><td data-label="Device"><a class="device-link" :href="'#/devices/'+encodeURIComponent(device.id)"><span class="device-glyph"><AppIcon name="server"/></span><span><strong>{{ device.display_name }}</strong><small>Synthetic inventory record</small></span></a></td><td data-label="Management address"><code>{{ device.management_ip }}</code></td><td data-label="Device group">{{ device.device_group||'Ungrouped' }}</td><td data-label="Demo status"><span class="status-tag">Disabled · synthetic</span></td><td><a class="details-link" :href="'#/devices/'+encodeURIComponent(device.id)" :aria-label="'Open '+device.display_name">Open details <AppIcon name="arrow"/></a></td></tr></tbody></table></div><div v-else class="empty-state"><AppIcon name="search"/><h2>No matching devices</h2><p>Try another name, documentation address, or device group.</p><button class="btn" @click="clearFilters">Clear filters</button></div></section>
      </template>
    </template>
  </div>
</template>

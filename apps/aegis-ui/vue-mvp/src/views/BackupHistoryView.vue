<script setup>
import {computed,watch} from 'vue';
import AppIcon from '../components/AppIcon.vue';
import ApiEvidence from '../components/ApiEvidence.vue';
import ApiState from '../components/ApiState.vue';
import {getBackupWorkspace,formatDate,formatTime,savedLink,compareLink} from '../api/go-demo.js';
import {useGoResource} from '../composables/useGoResource.js';

const props=defineProps({deviceId:{type:String,default:''}}),emit=defineEmits(['device-count']);
const {status,data,error,load}=useGoResource(getBackupWorkspace);
watch(()=>props.deviceId,id=>load(id||null),{immediate:true});
watch([status,data],()=>emit('device-count',status.value==='ready'?data.value.devices.length:null));
const selectedDevice=computed(()=>data.value?.devices.find(device=>device.id===props.deviceId));
const errorTitle=computed(()=>[401,403].includes(error.value?.status)?'Backup History not authorized':error.value?.status===404?'Go device not found':'Go Backup History unavailable');
function filterDevice(event){location.hash='/backups'+(event.target.value?'?'+new URLSearchParams({device:event.target.value}):'');}
</script>

<template>
  <section class="page-heading"><div><div class="eyebrow">SAVED CONFIGURATION HISTORY</div><h1>Every snapshot. One place.</h1><p>Saved synthetic configurations from the same Go devices. Not live device state.</p></div><button class="btn btn-primary" :disabled="status==='loading'" @click="load(deviceId||null)"><AppIcon name="refresh"/>Refresh history</button></section>
  <div :aria-busy="status==='loading'">
    <ApiState v-if="status!=='ready'" :status="status==='idle'?'loading':status" :error="error" :title="status==='error'?errorTitle:'Loading Go Backup History…'" description="Reading Go Inventory and persisted backup metadata over HTTP. No fixture history is substituted." @retry="load(deviceId||null)"/>
    <p v-if="status==='error'" class="saved-request">Retry and Refresh history reload current Go history for {{ deviceId||'all devices' }}. After a Go restart, only newly returned UUIDs are listed.<a v-if="deviceId" href="#/backups"> Open all device histories →</a></p>
    <template v-if="status==='ready'">
      <ApiEvidence :requests="data.requests"/>
      <div class="history-summary"><div><strong data-backup-total>{{ data.backups.length }}</strong><span>saved synthetic snapshots</span></div><div><strong>{{ selectedDevice?1:data.devices.length }}</strong><span>{{ selectedDevice?'selected Go device':'Go devices' }}</span></div><span class="tiny-tag">READ-ONLY · DISPOSABLE SQLITE</span></div>
      <div class="filterbar history-filter"><label for="backup-device">Device history</label><select id="backup-device" :value="deviceId" @change="filterDevice"><option value="">All Go devices</option><option v-for="device in data.devices" :key="device.id" :value="device.id">{{ device.display_name }} · {{ device.device_group }}</option></select><span>{{ data.backups.length }} snapshots · {{ selectedDevice?.display_name||'all Go devices' }}</span><a v-if="deviceId" class="details-link" href="#/backups">Clear filter</a></div>
      <ApiState v-if="!data.backups.length" status="empty" title="No saved Go backups" description="The selected history is empty. No fixture records or browser-memory copies are added." @retry="load(deviceId||null)"/>
      <section v-else class="history-grid" aria-label="Go saved backup history">
        <article v-for="backup in data.backups" :key="backup.id" class="card history-card" :data-backup-id="backup.id" :data-device-id="backup.device_id">
          <header class="history-card-head"><span class="device-glyph"><AppIcon name="archive"/></span><div><a :href="'#/devices/'+encodeURIComponent(backup.device_id)">{{ backup.device.display_name }}</a><p><time :datetime="backup.created_at" :title="backup.created_at">{{ formatDate(backup.created_at) }} · {{ formatTime(backup.created_at) }}</time></p></div><span class="tiny-tag">SYNTHETIC</span></header>
          <div class="history-body"><h2>{{ backup.label||'Saved running configuration' }}</h2><dl class="history-metadata"><div><dt>Snapshot UUID</dt><dd><code>{{ backup.id }}</code></dd></div><div><dt>SHA-256 · Go metadata</dt><dd><code>{{ backup.content_hash }}</code></dd></div><div class="history-properties"><div><dt>Configuration type</dt><dd>{{ backup.config_type }}</dd></div><div><dt>Saved size</dt><dd>{{ backup.size_bytes }} bytes</dd></div><div><dt>Source</dt><dd>Go · {{ backup.trigger_type }}</dd></div></div></dl></div>
          <footer class="history-actions"><a class="btn btn-primary" :href="savedLink(backup)"><AppIcon name="code"/>View Configuration</a><a class="btn" :href="compareLink(backup,data.backups)" :aria-label="'Compare '+backup.id+' in Vue'"><AppIcon name="diff"/>Compare<AppIcon name="arrow"/></a></footer>
        </article>
      </section>
      <p class="history-footnote">DEMO / UNVERIFIED · Snapshot content opens natively in Vue. Compare opens natively in Vue with actual Go UUIDs. No creation, deletion or live device access.</p>
    </template>
  </div>
</template>

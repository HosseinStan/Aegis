<script setup>
import {computed,ref,watch} from 'vue';
import AppIcon from './AppIcon.vue';
import ApiEvidence from './ApiEvidence.vue';
import ApiState from './ApiState.vue';
import {getSavedConfiguration,formatDate,formatTime,compareLink} from '../api/go-demo.js';
import {useGoResource} from '../composables/useGoResource.js';
import {replaceHashRoute} from '../composables/useHashRoute.js';

const props=defineProps({deviceId:{type:String,required:true},backupId:{type:String,default:null}});
const {status,data,error,load}=useGoResource(getSavedConfiguration);
const requested=ref(null),activeDevice=ref(null);
const configurationURL=id=>'#/devices/'+encodeURIComponent(props.deviceId)+'?'+new URLSearchParams({tab:'configuration',...(id?{backup:id}:{})});
watch([()=>props.deviceId,()=>props.backupId],([id,backup])=>{
  const wanted=backup||null;
  if(activeDevice.value===id&&requested.value===wanted)return;
  activeDevice.value=id;requested.value=wanted;void load(id,wanted);
},{immediate:true});
watch(data,result=>{
  if(!result?.selected)return;
  requested.value=result.selected.id;replaceHashRoute(configurationURL(result.selected.id));
});
watch(error,cause=>{
  if(!cause?.selectedId)return;
  requested.value=cause.selectedId;replaceHashRoute(configurationURL(cause.selectedId));
});
function refreshVersions(){requested.value=null;replaceHashRoute(configurationURL(null));void load(props.deviceId,null);}
function selectVersion(event){location.hash=configurationURL(event.target.value);}
const errorTitle=computed(()=>[401,403].includes(error.value?.status)?'Saved configuration not authorized':error.value?.status===404?'Saved configuration not found':'Saved configuration unavailable');
const isLatest=computed(()=>data.value?.selected?.id===data.value?.versions[0]?.id);
const compareURL=computed(()=>data.value?.selected?compareLink(data.value.selected,data.value.versions):'');
</script>

<template>
  <section class="saved-configuration" :aria-busy="status==='loading'">
    <header class="saved-heading"><div><h2>Saved running configuration</h2><p>Saved configuration — not live device state.</p></div><button class="btn" :disabled="status==='loading'" @click="refreshVersions"><AppIcon name="refresh"/>Refresh versions</button></header>
    <ApiState v-if="status!=='ready'" :status="status==='idle'?'loading':status" :error="error" :title="status==='error'?errorTitle:'Loading saved configuration…'" description="Reading this device’s saved Go history and synthetic snapshot content over HTTP." @retry="load(deviceId,requested)"/>
    <p v-if="status==='error'&&requested" class="saved-request">Requested UUID: <code>{{ requested }}</code>. Retry retains this selection. Refresh versions explicitly reloads the current history after a Go restart.</p>
    <template v-if="status==='ready'">
      <ApiEvidence :requests="data.requests"/>
      <ApiState v-if="!data.selected" status="empty" title="No saved running configuration" description="No running snapshots are stored for this Go device. No fixture content is substituted." @retry="load(deviceId,requested)"/>
      <article v-else class="card saved-card">
        <div class="saved-toolbar"><div><label for="saved-version">Saved version</label><select id="saved-version" :value="data.selected.id" @change="selectVersion"><option v-for="(backup,index) in data.versions" :key="backup.id" :value="backup.id">{{ formatDate(backup.created_at) }} · {{ formatTime(backup.created_at) }} · {{ backup.id.slice(0,8) }}{{ index===0?' · latest':'' }}</option></select></div><span class="tiny-tag">{{ isLatest?'LATEST SAVED':'OLDER SAVED VERSION' }}</span><a class="btn btn-primary" :href="compareURL" target="_blank" rel="noopener noreferrer" title="Opens the existing full Node preview"><AppIcon name="diff"/>Compare saved versions <AppIcon name="external"/></a></div>
        <dl class="saved-metadata"><div><dt>Snapshot timestamp · UTC</dt><dd><time :datetime="data.selected.created_at" :title="data.selected.created_at">{{ formatDate(data.selected.created_at) }} · {{ formatTime(data.selected.created_at) }}</time></dd></div><div><dt>Snapshot UUID</dt><dd><code id="vue-saved-id">{{ data.selected.id }}</code></dd></div><div><dt>SHA-256 · verified against received bytes</dt><dd><code id="vue-saved-sha">{{ data.selected.content_hash }}</code></dd></div><div><dt>Saved byte size</dt><dd id="vue-saved-size">{{ data.selected.size_bytes }} bytes</dd></div></dl>
        <div class="saved-caption"><AppIcon name="lock"/>Read-only synthetic running-config · {{ deviceId }} · Go BackupService</div>
        <pre id="vue-saved-content" class="saved-content" tabindex="0" aria-label="Saved synthetic running configuration">{{ data.selected.config }}</pre>
        <footer class="saved-footer">DEMO / UNVERIFIED · Saved synthetic content only. No live configuration request, SSH or write operation. Compare opens the full existing preview.</footer>
      </article>
    </template>
  </section>
</template>

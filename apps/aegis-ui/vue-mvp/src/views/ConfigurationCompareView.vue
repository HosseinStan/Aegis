<script setup>
import {computed,ref,watch} from 'vue';
import AppIcon from '../components/AppIcon.vue';
import ApiEvidence from '../components/ApiEvidence.vue';
import ApiState from '../components/ApiState.vue';
import {getConfigurationComparison,formatDate,formatTime,savedLink} from '../api/go-demo.js';
import {useGoResource} from '../composables/useGoResource.js';
import {replaceHashRoute} from '../composables/useHashRoute.js';

const props=defineProps({deviceId:{default:null},leftId:{default:null},rightId:{default:null}}),emit=defineEmits(['device-count']);
const {status,data,error,load}=useGoResource(getConfigurationComparison);
const requested=ref(null),layout=ref('side'),changesOnly=ref(false);
const clean=value=>({deviceId:value.deviceId||null,leftId:value.leftId||null,rightId:value.rightId||null});
const same=(a,b)=>JSON.stringify(a)===JSON.stringify(b);
const compareURL=s=>'#/compare'+(s.deviceId?'?'+new URLSearchParams({device:s.deviceId,...(s.leftId?{left:s.leftId}:{}),...(s.rightId?{right:s.rightId}:{})}):'');
watch([()=>props.deviceId,()=>props.leftId,()=>props.rightId],()=>{
  const next=clean(props);if(same(next,requested.value))return;
  requested.value=next;void load(next);
},{immediate:true});
watch(data,result=>{
  if(!result)return;emit('device-count',result.devices.length);
  requested.value=clean({deviceId:result.device?.id,leftId:result.left?.id,rightId:result.right?.id});
  replaceHashRoute(compareURL(requested.value));
});
watch(error,cause=>{
  if(!cause?.selection)return;
  requested.value=clean(cause.selection);replaceHashRoute(compareURL(requested.value));
});
function refresh(){
  requested.value=clean({deviceId:requested.value?.deviceId});replaceHashRoute(compareURL(requested.value));void load(requested.value);
}
function choose(key,value){
  const selection=key==='deviceId'?clean({deviceId:value}):{deviceId:data.value.device.id,leftId:data.value.left.id,rightId:data.value.right.id,[key]:value};
  location.hash=compareURL(selection);
}
function swap(){location.hash=compareURL({deviceId:data.value.device.id,leftId:data.value.right.id,rightId:data.value.left.id});}
const rows=computed(()=>data.value?.rows?.filter(row=>!changesOnly.value||row.changed)||[]);
const errorTitle=computed(()=>[401,403].includes(error.value?.status)?'Comparison not authorized':error.value?.status===404?'Saved comparison not found':'Go comparison unavailable');
const versionLabel=b=>formatDate(b.created_at)+' · '+formatTime(b.created_at)+' · '+b.id.slice(0,8);
</script>

<template>
  <section class="page-heading"><div><div class="eyebrow">GO CONFIGURATION COMPARISON</div><h1>A clear view of every change.</h1><p>Saved synthetic configurations — not live device state. Risk is not evaluated.</p></div><button class="btn btn-primary" :disabled="status==='loading'" @click="refresh"><AppIcon name="refresh"/>Refresh snapshots</button></section>
  <div :aria-busy="status==='loading'">
    <ApiState v-if="status!=='ready'" :status="status==='idle'?'loading':status" :error="error" :title="status==='error'?errorTitle:'Loading Go comparison…'" description="Reading Go Inventory, current history, verified saved contents and native BackupService.Diff over HTTP." @retry="load(requested)"/>
    <div v-if="status==='error'" class="saved-request"><p>Retry keeps the requested device and snapshot UUIDs. Refresh snapshots explicitly reloads current history and chooses current versions.</p><dl class="compare-request"><div v-for="key in ['deviceId','leftId','rightId']" :key="key"><dt>{{ key }}</dt><dd><code>{{ requested?.[key]||'Not selected' }}</code></dd></div></dl><a href="#/compare">Choose from current Go devices →</a></div>
    <template v-if="status==='ready'">
      <ApiEvidence :requests="data.requests"/>
      <section class="card compare-controls">
        <div class="compare-device"><label for="compare-device">Go device</label><select id="compare-device" :value="data.device?.id||''" @change="choose('deviceId',$event.target.value)"><option v-if="!data.devices.length" value="">No Go devices</option><option v-for="device in data.devices" :key="device.id" :value="device.id">{{ device.display_name }} · {{ device.device_group }}</option></select></div>
        <template v-if="data.left&&data.right"><div><label for="compare-left">Before snapshot · UTC</label><select id="compare-left" :value="data.left.id" @change="choose('leftId',$event.target.value)"><option v-for="backup in data.versions" :key="backup.id" :value="backup.id">{{ versionLabel(backup) }}</option></select></div><button class="btn compare-swap" aria-label="Swap snapshots" @click="swap">⇄<span class="sr-only">Swap snapshots</span></button><div><label for="compare-right">After snapshot · UTC</label><select id="compare-right" :value="data.right.id" @change="choose('rightId',$event.target.value)"><option v-for="backup in data.versions" :key="backup.id" :value="backup.id">{{ versionLabel(backup) }}</option></select></div></template>
      </section>
      <ApiState v-if="!data.left||!data.right" status="empty" :title="data.device?'No saved snapshots to compare':'No Go devices to compare'" description="The current Go history is empty. No fixtures or browser-memory copies are substituted." @retry="refresh"/>
      <template v-else>
        <div class="compare-summary"><div><span class="tiny-tag">NATIVE GO DIFF</span><strong>{{ data.native.changed?'Saved changes detected':'No saved changes' }}</strong><span class="native-count added" data-native-added>+{{ data.native.added_lines }} added</span><span class="native-count removed" data-native-removed>−{{ data.native.removed_lines }} removed</span></div><p>Counts and Unified text come from Go BackupService.Diff. Side-by-side pairs lines from that same native patch; it does not calculate a second diff.</p></div>
        <section class="card compare-card"><header class="compare-toolbar"><div class="compare-layout" role="group" aria-label="Comparison layout"><button class="btn" :aria-pressed="layout==='side'" @click="layout='side'">Side by side</button><button class="btn" :aria-pressed="layout==='unified'" @click="layout='unified'">Unified</button></div><label class="compare-changes"><input v-model="changesOnly" type="checkbox" :disabled="layout==='unified'">Changes only <small>(side-by-side)</small></label></header>
          <template v-if="layout==='side'"><div class="compare-column-head"><span>BEFORE · {{ formatDate(data.left.created_at) }}</span><span>AFTER · {{ formatDate(data.right.created_at) }}</span></div><div class="compare-lines" aria-label="Side-by-side native Go diff presentation"><div v-for="(row,index) in rows" :key="index" class="compare-line" :data-changed="row.changed"><div v-for="side in ['left','right']" :key="side" :class="['compare-cell',row[side]?.kind||'',{'missing':!row[side]}]"><span class="compare-line-number">{{ row[side]?.number??'' }}</span><span class="compare-line-sign">{{ row[side]?.kind==='added'?'+':row[side]?.kind==='removed'?'−':'' }}</span><code>{{ row[side]?.text??'' }}</code></div></div></div><p v-if="!rows.length" class="compare-no-changes">No changed lines in these selected saved snapshots.</p></template>
          <template v-else><p class="compare-unified-note">Exact native Go Unified response · context is preserved.</p><pre id="vue-native-unified" class="compare-unified" tabindex="0" aria-label="Exact native Go Unified diff">{{ data.native.diff }}</pre><p v-if="!data.native.diff" class="compare-no-changes">The native Go Unified response is empty: no changes.</p></template>
        </section>
        <details class="card compare-metadata"><summary>Selected saved snapshot evidence · UUIDs, SHA-256 and byte sizes</summary><div class="compare-evidence-grid"><div v-for="[label,backup] in [['Before',data.left],['After',data.right]]" :key="label"><h2>{{ label }} · <a :href="savedLink(backup)">View Configuration →</a></h2><dl><dt>Snapshot UUID</dt><dd><code :id="label==='Before'?'vue-compare-left-id':'vue-compare-right-id'">{{ backup.id }}</code></dd><dt>Timestamp · UTC</dt><dd>{{ backup.created_at }}</dd><dt>SHA-256 · verified against received bytes</dt><dd><code>{{ backup.content_hash }}</code></dd><dt>Saved size</dt><dd>{{ backup.size_bytes }} bytes</dd></dl></div></div></details>
        <p class="history-footnote">DEMO / UNVERIFIED · RISK NOT EVALUATED · Read-only synthetic saved content. No SSH, live configuration request, approval or write operation.</p>
      </template>
    </template>
  </div>
</template>

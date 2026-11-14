<script setup>
import {computed,ref,watch} from 'vue';
import AppIcon from './AppIcon.vue';
import ApiEvidence from './ApiEvidence.vue';
import ApiState from './ApiState.vue';
import {getConfigurationComparison,formatDate,formatTime,savedLink,comparisonLink} from '../api/go-demo.js';
import {useGoResource} from '../composables/useGoResource.js';
import {replaceHashRoute} from '../composables/useHashRoute.js';
import {useDemoViewed} from '../composables/useDemoViewed.js';

const props=defineProps({deviceId:{default:null},leftId:{default:null},rightId:{default:null},view:{type:String,default:'compare'}}),emit=defineEmits(['device-count']);
const {status,data,error,load}=useGoResource(getConfigurationComparison);
const requested=ref(null),layout=ref('side'),changesOnly=ref(false);
const viewed=useDemoViewed(props);
const clean=value=>({deviceId:value.deviceId||null,leftId:value.leftId||null,rightId:value.rightId||null});
const same=(a,b)=>JSON.stringify(a)===JSON.stringify(b);
const isReview=computed(()=>props.view==='review');
const compareURL=s=>comparisonLink(s,props.view);
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
const errorTitle=computed(()=>[401,403].includes(error.value?.status)?(isReview.value?'Change Review not authorized':'Comparison not authorized'):error.value?.status===404?(isReview.value?'Saved review evidence not found':'Saved comparison not found'):(isReview.value?'Go Change Review unavailable':'Go comparison unavailable'));
const versionLabel=b=>formatDate(b.created_at)+' · '+formatTime(b.created_at)+' · '+b.id.slice(0,8);
</script>

<template>
  <section class="page-heading"><div><div class="eyebrow">{{ isReview?'GO-BACKED CHANGE REVIEW':'GO CONFIGURATION COMPARISON' }}</div><h1>{{ isReview?'Understand the saved changes.':'A clear view of every change.' }}</h1><p>Saved synthetic configurations — not live device state. Risk is not evaluated.</p></div><div class="heading-actions"><a v-if="!isReview&&status==='ready'&&data.left&&data.right" class="btn" :href="comparisonLink({deviceId:data.device.id,leftId:data.left.id,rightId:data.right.id},'review')"><AppIcon name="code"/>Open Change Review</a><button class="btn btn-primary" :disabled="status==='loading'" @click="refresh"><AppIcon name="refresh"/>Refresh snapshots</button></div></section>
  <aside v-if="isReview" class="review-boundary" aria-label="Review scope"><AppIcon name="code"/><div><strong>RISK NOT EVALUATED — DEMO / UNVERIFIED</strong><p>Saved configuration differences for evidence review. Security risk has not been evaluated; this demo provides no security conclusions or approval.</p></div></aside>
  <div :aria-busy="status==='loading'">
    <ApiState v-if="status!=='ready'" :status="status==='idle'?'loading':status" :error="error" :title="status==='error'?errorTitle:(isReview?'Loading Go Change Review…':'Loading Go comparison…')" description="Reading Go Inventory, current history, verified saved contents and native BackupService.Diff over HTTP." @retry="load(requested)"/>
    <div v-if="status==='error'" class="saved-request"><p>Retry keeps the requested device and snapshot UUIDs. Refresh snapshots explicitly reloads current history and chooses current versions.</p><dl class="compare-request"><div v-for="key in ['deviceId','leftId','rightId']" :key="key"><dt>{{ key }}</dt><dd><code>{{ requested?.[key]||'Not selected' }}</code></dd></div></dl><a :href="isReview?'#/review':'#/compare'">Choose from current Go devices →</a></div>
    <template v-if="status==='ready'">
      <ApiEvidence :requests="data.requests"/>
      <section class="card compare-controls">
        <div class="compare-device"><label for="compare-device">Go device</label><select id="compare-device" :value="data.device?.id||''" @change="choose('deviceId',$event.target.value)"><option v-if="!data.devices.length" value="">No Go devices</option><option v-for="device in data.devices" :key="device.id" :value="device.id">{{ device.display_name }} · {{ device.device_group }}</option></select></div>
        <template v-if="data.left&&data.right"><div><label for="compare-left">Before snapshot · UTC</label><select id="compare-left" :value="data.left.id" @change="choose('leftId',$event.target.value)"><option v-for="backup in data.versions" :key="backup.id" :value="backup.id">{{ versionLabel(backup) }}</option></select></div><button class="btn compare-swap" aria-label="Swap snapshots" @click="swap">⇄<span class="sr-only">Swap snapshots</span></button><div><label for="compare-right">After snapshot · UTC</label><select id="compare-right" :value="data.right.id" @change="choose('rightId',$event.target.value)"><option v-for="backup in data.versions" :key="backup.id" :value="backup.id">{{ versionLabel(backup) }}</option></select></div></template>
      </section>
      <ApiState v-if="!data.left||!data.right" status="empty" :title="data.device?(isReview?'No saved snapshots to review':'No saved snapshots to compare'):(isReview?'No Go devices to review':'No Go devices to compare')" description="The current Go history is empty. No fixtures or browser-memory copies are substituted." @retry="refresh"/>
      <template v-else>
        <section v-if="isReview" class="card review-overview" aria-label="Selected Change Review evidence">
          <header class="review-device"><span class="device-glyph"><AppIcon name="server"/></span><div><a :href="'#/devices/'+encodeURIComponent(data.device.id)">{{ data.device.display_name }}</a><p>{{ data.device.device_group||'Ungrouped' }} · {{ data.device.management_ip }} · Synthetic disabled mock device</p></div><a class="btn" :href="comparisonLink({deviceId:data.device.id,leftId:data.left.id,rightId:data.right.id})"><AppIcon name="diff"/>Open Configuration Compare</a></header>
          <div class="review-evidence"><div v-for="[label,backup] in [['Before',data.left],['After',data.right]]" :key="label"><span class="eyebrow">{{ label }} saved snapshot</span><code :id="label==='Before'?'vue-review-left-id':'vue-review-right-id'">{{ backup.id }}</code><time :datetime="backup.created_at" :title="backup.created_at">{{ formatDate(backup.created_at) }} · {{ formatTime(backup.created_at) }}</time><a :href="savedLink(backup)">View {{ label.toLowerCase() }} configuration →</a></div><div class="review-outcome"><span class="eyebrow">NATIVE GO EVIDENCE</span><h2>{{ data.native.changed?'Saved changes detected':'No saved changes' }}</h2><div><span class="native-count added" data-native-added>+{{ data.native.added_lines }} added</span><span class="native-count removed" data-native-removed>−{{ data.native.removed_lines }} removed</span></div><p>Snapshot metadata, byte size and SHA-256 verified. Change counts from Go BackupService.Diff.</p></div></div>
          <footer class="review-viewed"><label><input v-model="viewed" type="checkbox">Viewed in this demo — not approved or persisted</label><p aria-live="polite">{{ viewed?'Marked viewed for these saved versions on this page only.':'Optional local viewing marker. It resets when the device or UUID pair changes and on page reload.' }}</p></footer>
        </section>
        <div v-if="!isReview" class="compare-summary"><div><span class="tiny-tag">NATIVE GO DIFF</span><strong>{{ data.native.changed?'Saved changes detected':'No saved changes' }}</strong><span class="native-count added" data-native-added>+{{ data.native.added_lines }} added</span><span class="native-count removed" data-native-removed>−{{ data.native.removed_lines }} removed</span></div><p>Counts and Unified text come from Go BackupService.Diff. Side-by-side pairs lines from that same native patch; it does not calculate a second diff.</p></div>
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

<script setup>
import {computed,onMounted,watch} from 'vue';
import AppIcon from '../components/AppIcon.vue';
import ApiEvidence from '../components/ApiEvidence.vue';
import ApiState from '../components/ApiState.vue';
import {getDashboard,formatDate,formatTime,savedLink,compareLink} from '../api/go-demo.js';
import {useGoResource} from '../composables/useGoResource.js';
const emit=defineEmits(['device-count']);
const {status,data,error,load}=useGoResource(getDashboard);
watch([status,data],()=>emit('device-count',status.value==='ready'?data.value.devices.length:null));
onMounted(()=>load());
const metrics=computed(()=>data.value?[
  {key:'devices',label:'Go devices',value:data.value.devices.length,foot:'From the current Go Inventory',icon:'server'},
  {key:'snapshots',label:'Saved snapshots',value:data.value.backups.length,foot:'Persisted synthetic Go backups',icon:'archive'},
  {key:'coverage',label:'Backup coverage',value:data.value.coverage===null?'—':data.value.coverage+'%',foot:data.value.covered+' of '+data.value.devices.length+' Go devices have backups',icon:'check'},
  {key:'groups',label:'Device groups',value:data.value.groups.length,foot:'Actual device_group values',icon:'groups'}
]:[]);
</script>

<template>
  <section class="page-heading"><div><div class="eyebrow">ONE ISOLATED GO WORKSPACE</div><h1>Your devices. One clear picture.</h1><p>Inventory and saved history share the same disposable Go data.</p></div><div class="heading-actions"><button class="btn btn-primary" :disabled="status==='loading'" @click="load()"><AppIcon name="refresh"/>Refresh Dashboard</button><a class="btn" href="#/devices"><AppIcon name="server"/>Go Inventory</a></div></section>
  <div :aria-busy="status==='loading'">
    <ApiState v-if="status!=='ready'" :status="status==='idle'?'loading':status" :error="error" :title="status==='error'?'Go Dashboard unavailable':'Loading Go Dashboard…'" description="Reading current Go Inventory and its saved backup histories over HTTP." @retry="load()"/>
    <template v-else>
      <ApiEvidence :requests="data.requests"/>
      <section class="stats-grid" aria-label="Go dashboard summary"><article v-for="metric in metrics" :key="metric.key" class="stat" :data-dashboard-metric="metric.key" :data-value="metric.value"><div class="stat-label">{{ metric.label }}<span><AppIcon :name="metric.icon"/></span></div><strong class="stat-number">{{ metric.value }}</strong><p>{{ metric.foot }}</p></article></section>
      <ApiState v-if="!data.devices.length" status="empty" title="No Go devices yet" description="The Go API returned an empty inventory. No fixture devices are substituted." @retry="load()"/>
      <template v-else>
        <div class="latest-strip"><span><AppIcon name="clock"/>Latest saved snapshot · UTC</span><a v-if="data.latest" :href="savedLink(data.latest)">{{ formatDate(data.latest.created_at) }} · {{ formatTime(data.latest.created_at) }}</a><strong v-else>No saved snapshots</strong><span class="tiny-tag">SYNTHETIC GO DATA</span></div>
        <section class="dashboard-grid">
          <article class="card"><header class="card-head"><div><h2>Device groups</h2><p>Distribution from Go device_group metadata.</p></div><span class="tiny-tag">GO METADATA</span></header><div class="group-distribution"><div v-for="[group,count] in data.groups" :key="group" class="group-row" :data-group="group" :data-count="count"><div><strong>{{ group||'Ungrouped' }}</strong><span>{{ count }} {{ count===1?'device':'devices' }}</span></div><div class="group-track"><span :style="{width:count/data.devices.length*100+'%'}"/></div></div></div><footer class="card-foot"><span>{{ data.devices.length }} devices across {{ data.groups.length }} groups</span><a href="#/devices">Open Go Inventory <AppIcon name="arrow"/></a></footer></article>
          <article class="card"><header class="card-head"><div><h2>Your Go devices</h2><p>The same records used by the full workspace.</p></div></header><div class="go-device-list"><div v-for="device in data.devices" :key="device.id" class="go-device-row"><span class="device-glyph"><AppIcon name="server"/></span><div><a :href="'#/devices/'+encodeURIComponent(device.id)">{{ device.display_name }}</a><small>{{ device.device_group }} · {{ device.management_ip }}</small></div><span class="device-backup-count"><strong>{{ data.backups.filter(b=>b.device_id===device.id).length }}</strong><small>saved backups</small></span></div></div></article>
        </section>
        <section class="card recent-backups"><header class="card-head"><div><h2>Recent Go backups</h2><p>Current persisted UUIDs · snapshots and Compare open natively in Vue.</p></div><a href="#/backups">All {{ data.backups.length }} Go backups <AppIcon name="arrow"/></a></header><div class="recent-list"><article v-for="backup in data.backups.slice(0,4)" :key="backup.id" class="recent-row" :data-backup-id="backup.id"><div class="recent-name"><span class="device-glyph"><AppIcon name="archive"/></span><div><a :href="savedLink(backup)">{{ backup.label }}</a><small><a :href="'#/devices/'+encodeURIComponent(backup.device_id)">{{ backup.device.display_name }}</a> · {{ backup.config_type }}</small></div></div><div class="recent-time">{{ formatDate(backup.created_at) }}<small>{{ formatTime(backup.created_at) }}</small></div><a class="btn btn-soft" :href="compareLink(backup,data.backups)"><AppIcon name="diff"/>Compare <AppIcon name="arrow"/></a></article><p v-if="!data.backups.length" class="empty-note">No saved Go snapshots are available.</p></div></section>
      </template>
      <aside class="boundary-note"><AppIcon name="code"/><div><strong>Review the saved changes</strong><p>Change Review uses the same Go snapshots and native Diff. RISK NOT EVALUATED — DEMO / UNVERIFIED.</p></div><a href="#/review">Open Change Review <AppIcon name="arrow"/></a></aside>
    </template>
  </div>
</template>

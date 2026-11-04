<script setup>
import {computed,nextTick,ref,watch} from 'vue';
import AppShell from './components/AppShell.vue';
import DashboardView from './views/DashboardView.vue';
import InventoryView from './views/InventoryView.vue';
import BackupHistoryView from './views/BackupHistoryView.vue';
import ConfigurationCompareView from './views/ConfigurationCompareView.vue';
import ChangeReviewView from './views/ChangeReviewView.vue';
import DeviceDetailView from './views/DeviceDetailView.vue';
import {useHashRoute} from './composables/useHashRoute.js';
import {previewLink} from './api/go-demo.js';
const route=useHashRoute(),deviceCount=ref(null),deviceTitle=ref('Device details');
const breadcrumb=computed(()=>({dashboard:'Overview',inventory:'Device inventory',backups:'Backup History',compare:'Configuration Compare',review:'Change Review',detail:deviceTitle.value,'not-found':'Page not found'}[route.value.name]));
watch(route,async(next,previous)=>{
  const pageChanged=next.name!==previous.name||next.id!==previous.id;
  if(pageChanged)deviceTitle.value='Device details';
  if(pageChanged||next.query.get('tab')!==previous.query.get('tab')){await nextTick();document.getElementById('vue-main')?.focus({preventScroll:true});}
});
watch(breadcrumb,value=>document.title='Aegis Vue · '+value,{immediate:true});
</script>

<template>
  <AppShell :active="route.name" :breadcrumb="breadcrumb" :device-count="deviceCount">
    <DashboardView v-if="route.name==='dashboard'" @device-count="deviceCount=$event"/>
    <InventoryView v-else-if="route.name==='inventory'" :query="route.query.get('q')||''" @device-count="deviceCount=$event"/>
    <BackupHistoryView v-else-if="route.name==='backups'" :device-id="route.query.get('device')||''" @device-count="deviceCount=$event"/>
    <ConfigurationCompareView v-else-if="route.name==='compare'" :device-id="route.query.get('device')" :left-id="route.query.get('left')" :right-id="route.query.get('right')" @device-count="deviceCount=$event"/>
    <ChangeReviewView v-else-if="route.name==='review'" :device-id="route.query.get('device')" :left-id="route.query.get('left')" :right-id="route.query.get('right')" @device-count="deviceCount=$event"/>
    <DeviceDetailView v-else-if="route.name==='detail'" :id="route.id" :key="route.id" :tab="route.query.get('tab')||'overview'" :backup-id="route.query.get('backup')" @device-title="deviceTitle=$event"/>
    <section v-else class="card empty-state"><h1>Page not found</h1><p>This Vue workspace includes Dashboard, Inventory, device profiles, Saved Configuration, Backup History, Compare and Change Review.</p><a class="btn btn-primary" href="#/overview">Open Vue Dashboard</a><a class="btn" :href="previewLink()" target="_blank" rel="noopener noreferrer">Open the existing preview ↗</a></section>
  </AppShell>
</template>

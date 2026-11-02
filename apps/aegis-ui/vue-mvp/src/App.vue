<script setup>
import {computed,nextTick,ref,watch} from 'vue';
import AppShell from './components/AppShell.vue';
import DashboardView from './views/DashboardView.vue';
import InventoryView from './views/InventoryView.vue';
import DeviceDetailView from './views/DeviceDetailView.vue';
import {useHashRoute} from './composables/useHashRoute.js';
import {previewLink} from './api/go-demo.js';
const route=useHashRoute(),deviceCount=ref(null),deviceTitle=ref('Device details');
const breadcrumb=computed(()=>({dashboard:'Overview',inventory:'Device inventory',detail:deviceTitle.value,'not-found':'Page not found'}[route.value.name]));
watch(route,async()=>{deviceTitle.value='Device details';await nextTick();document.getElementById('vue-main')?.focus({preventScroll:true});});
watch(breadcrumb,value=>document.title='Aegis Vue · '+value,{immediate:true});
</script>

<template>
  <AppShell :active="route.name" :breadcrumb="breadcrumb" :device-count="deviceCount">
    <DashboardView v-if="route.name==='dashboard'" @device-count="deviceCount=$event"/>
    <InventoryView v-else-if="route.name==='inventory'" :query="route.query.get('q')||''" @device-count="deviceCount=$event"/>
    <DeviceDetailView v-else-if="route.name==='detail'" :id="route.id" :key="route.id" @device-title="deviceTitle=$event"/>
    <section v-else class="card empty-state"><h1>Page not found</h1><p>This Vue slice includes Dashboard, Inventory and basic Go device profiles.</p><a class="btn btn-primary" href="#/overview">Open Vue Dashboard</a><a class="btn" :href="previewLink()" target="_blank" rel="noopener noreferrer">Open the existing preview ↗</a></section>
  </AppShell>
</template>

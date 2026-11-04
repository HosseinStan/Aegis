<script setup>
import {onMounted,onBeforeUnmount,ref} from 'vue';
import AppIcon from './AppIcon.vue';
import {previewLink} from '../api/go-demo.js';
defineProps({active:String,breadcrumb:String,deviceCount:{default:null}});
const search=ref(''),searchInput=ref(null);
function searchDevices(){location.hash='/devices?'+new URLSearchParams({q:search.value});}
function skipToContent(){document.getElementById('vue-main')?.focus();}
function shortcut(event){
  if(event.key==='/'&&!['INPUT','SELECT','TEXTAREA'].includes(document.activeElement?.tagName)){
    event.preventDefault();searchInput.value?.focus();
  }
}
onMounted(()=>window.addEventListener('keydown',shortcut));
onBeforeUnmount(()=>window.removeEventListener('keydown',shortcut));
const remaining=[{label:'Compare configurations',route:'/compare',icon:'diff'},{label:'Change Review',route:'/review',icon:'code'}];
</script>

<template>
  <a class="skip-link" href="#vue-main" @click.prevent="skipToContent">Skip to content</a>
  <div class="shell">
    <aside class="sidebar">
      <a class="brand" href="#/overview" aria-label="Aegis overview"><span class="brand-icon">✳</span><span class="brand-word">AEGIS<small>NETWORK WORKSPACE</small></span></a>
      <div class="workspace"><span class="workspace-avatar">V</span><span>Vue workspace<small>Shared isolated Go data</small></span><i/></div>
      <div class="nav-label">VUE PREVIEW</div>
      <nav aria-label="Main navigation">
        <a href="#/overview" :class="{active:active==='dashboard'}" :aria-current="active==='dashboard'?'page':undefined" aria-label="Overview"><AppIcon name="dashboard"/><span class="nav-text">Overview</span></a>
        <a href="#/devices" :class="{active:active==='inventory'||active==='detail'}" :aria-current="active==='inventory'||active==='detail'?'page':undefined" aria-label="Device inventory"><AppIcon name="server"/><span class="nav-text">Device inventory</span><span class="nav-count">{{ deviceCount??'—' }}</span></a>
        <a href="#/backups" :class="{active:active==='backups'}" :aria-current="active==='backups'?'page':undefined" aria-label="Backup History"><AppIcon name="archive"/><span class="nav-text">Backup History</span></a>
      </nav>
      <div class="nav-label secondary-label">EXISTING PREVIEW</div>
      <nav aria-label="Full workspace in existing preview">
        <a v-for="item in remaining" :key="item.route" :href="previewLink(item.route)" target="_blank" rel="noopener noreferrer" :aria-label="item.label+' — existing preview, opens a new tab'" :title="item.label+' · existing preview'"><AppIcon :name="item.icon"/><span class="nav-text">{{ item.label }}</span><AppIcon class="external-icon" name="external"/></a>
      </nav>
      <div class="sidebar-note"><div><AppIcon name="lock"/> A contained workspace</div><p>Explore freely.<br>Live device access is off.</p><span>● LOCAL DEMO</span></div>
      <a class="full-preview" :href="previewLink()" target="_blank" rel="noopener noreferrer"><AppIcon name="external"/><span>Open full existing preview</span></a>
      <div class="sidebar-user"><span class="avatar">DM</span><span>Demo viewer<small>Read-only workspace</small></span></div>
    </aside>
    <div class="workspace-main">
      <header class="topbar">
        <div class="breadcrumbs"><span>Workspace</span><span>/</span><strong>{{ breadcrumb }}</strong></div>
        <form class="global-search" role="search" @submit.prevent="searchDevices"><AppIcon name="search"/><input ref="searchInput" v-model="search" aria-label="Find a device" placeholder="Find a device…"><kbd>/</kbd></form>
      </header>
      <div class="demo-banner"><span class="tiny-tag">VUE PREVIEW</span><span>DEMO — SYNTHETIC DATA — RISK NOT EVALUATED</span><a :href="previewLink()" target="_blank" rel="noopener noreferrer">Full existing preview <AppIcon name="external"/></a></div>
      <main id="vue-main" tabindex="-1"><slot/></main>
      <footer class="page-footer"><span>● Local preview · Synthetic data only</span><span>Phase E deferred · Production release blocked</span></footer>
    </div>
  </div>
</template>

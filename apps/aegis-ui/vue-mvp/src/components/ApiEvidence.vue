<script setup>
import AppIcon from './AppIcon.vue';
import {BACKUP_SOURCE} from '../api/go-demo.js';
defineProps({requests:{type:Array,required:true},source:{type:String,default:BACKUP_SOURCE}});
</script>

<template>
  <section class="api-evidence" aria-label="HTTP data source">
    <div class="api-label"><AppIcon name="server"/><strong>{{ source }}</strong><span class="http-status">HTTP 200 · {{ requests.length }} GET {{ requests.length===1?'response':'responses' }}</span></div>
    <details><summary>HTTP sources &amp; response request IDs</summary><div v-for="request in requests" :key="request.path" class="request-row"><a :href="request.path" target="_blank" rel="noopener noreferrer">GET {{ request.path }}</a><span>{{ request.elapsed }} ms · <code>{{ request.requestId }}</code></span><small v-if="request.upstream">Go GET {{ request.upstream }}</small></div></details>
  </section>
</template>

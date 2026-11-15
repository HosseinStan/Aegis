<script setup>
import AppIcon from './AppIcon.vue';
defineProps({status:String,error:Object,title:String,description:String});
defineEmits(['retry']);
</script>

<template>
  <section class="card api-state" :role="status==='loading'?'status':'alert'" aria-live="polite">
    <span v-if="status==='loading'" class="spinner" aria-hidden="true"/><AppIcon v-else :name="error?.status===404?'search':'alert'"/>
    <h2>{{ title }}</h2><p>{{ error?.message||description }}</p>
    <p v-if="error?.status" class="error-meta">HTTP {{ error.status }}<template v-if="error.requestId"> · Request ID: <code>{{ error.requestId }}</code></template></p>
    <button v-if="status!=='loading'" class="btn btn-primary" @click="$emit('retry')"><AppIcon name="refresh"/>Retry</button>
  </section>
</template>

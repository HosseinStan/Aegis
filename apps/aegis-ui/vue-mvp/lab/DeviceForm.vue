<script setup>
import {reactive} from 'vue';

const props=defineProps({record:Object,credentials:Array,busy:Boolean});
const emit=defineEmits(['save','cancel','credential']);
const fields=reactive({
  display_name:props.record?.display_name||'',
  hostname:props.record?.hostname||'',
  management_ip:props.record?.management_ip||'',
  management_port:props.record?.management_port||22,
  vendor:props.record?.vendor||'cisco',
  platform:props.record?.platform||'ios',
  device_group:props.record?.device_group||'',
  description:props.record?.description||'',
  credential_id:props.record?.credential_id||''
});
</script>

<template>
  <section class="card form-card" aria-labelledby="device-form-title">
    <div class="eyebrow">LOCAL INVENTORY</div>
    <h2 id="device-form-title">{{ record?'Edit Lab device':'Register a Lab device' }}</h2>
    <p>Use the actual details supplied by the Lab owner. Saving changes inventory only.</p>
    <p v-if="!credentials.length" class="message">A dedicated SSH credential is required. <button class="text-button" type="button" @click="emit('credential')">Create SSH credential</button></p>
    <form class="form-grid" @submit.prevent="emit('save',{...fields})">
      <label>Device name<input v-model.trim="fields.display_name" maxlength="100" required></label>
      <label>Hostname<input v-model.trim="fields.hostname" maxlength="253"></label>
      <label>Management IP<input v-model.trim="fields.management_ip" maxlength="64" autocapitalize="off" required><small>Literal IPv4 or IPv6 address.</small></label>
      <label>SSH port<input v-model.number="fields.management_port" type="number" min="1" max="65535" required></label>
      <label>Vendor<select v-model="fields.vendor"><option value="cisco">Cisco</option></select></label>
      <label>Platform / IOS family<select v-model="fields.platform"><option value="ios">Cisco IOS</option><option value="ios-xe">Cisco IOS XE</option></select></label>
      <label>Device group<input v-model.trim="fields.device_group" maxlength="80"></label>
      <label>Assigned SSH credential<select v-model="fields.credential_id" required><option disabled value="">Select a Lab credential</option><option v-for="item in credentials" :key="item.id" :value="item.id">{{ item.name }}</option></select></label>
      <label class="wide">Description<textarea v-model.trim="fields.description" rows="3" maxlength="600"></textarea></label>
      <div class="wide form-actions"><button class="button primary" :disabled="busy||!credentials.length">{{ busy?'Saving…':record?'Save device':'Register device' }}</button><button class="button" type="button" :disabled="busy" @click="emit('cancel')">Cancel</button></div>
    </form>
  </section>
</template>

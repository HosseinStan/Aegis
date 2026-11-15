<script setup>
import {ref} from 'vue';

defineProps({credentials:Array,busy:Boolean});
const emit=defineEmits(['save','done']);
const type=ref('password');
function submit(event){
  const form=event.currentTarget;
  const data=Object.fromEntries(new FormData(form));
  if(form.elements.secret)form.elements.secret.value='';
  const {secret,...details}=data;
  emit('save',{...details,[type.value]:secret});
}
</script>

<template>
  <section class="card form-card" aria-labelledby="credential-title">
    <div class="eyebrow">OPERATOR / ENCRYPTED STORE</div><h2 id="credential-title">SSH credentials</h2>
    <p>Create a dedicated read-only account reference for this Lab. Saved secrets cannot be retrieved from the UI.</p>
    <ul v-if="credentials.length" class="credential-list" aria-label="Saved credential labels"><li v-for="item in credentials" :key="item.id"><strong>{{ item.name }}</strong><span>{{ item.auth_type==='password'?'Password':'Private key' }}</span></li></ul>
    <form class="form-grid" autocomplete="off" @submit.prevent="submit">
      <label>Credential label<input name="name" maxlength="100" required></label>
      <label>SSH username<input name="username" maxlength="100" autocomplete="off" required></label>
      <label class="wide">Authentication method<select v-model="type" name="auth_type"><option value="password">Password</option><option value="private_key">Private key</option></select></label>
      <label v-if="type==='password'" class="wide">SSH password<input name="secret" type="password" maxlength="1024" autocomplete="new-password" required></label>
      <label v-else class="wide">SSH private key<textarea name="secret" rows="5" maxlength="32768" autocomplete="off" spellcheck="false" required></textarea><small>Supported unencrypted OpenSSH or PEM key. Passphrase-protected keys are unsupported.</small></label>
      <div class="wide form-actions"><button class="button primary" :disabled="busy">{{ busy?'Encrypting…':'Save credential' }}</button><button class="button" type="button" :disabled="busy" @click="emit('done')">Back to devices</button></div>
    </form>
  </section>
</template>

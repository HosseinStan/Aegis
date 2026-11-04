<script setup>
import SavedComparisonWorkspace from '../components/SavedComparisonWorkspace.vue';
defineProps({deviceId:{default:null},leftId:{default:null},rightId:{default:null}});
defineEmits(['device-count']);
</script>

<template>
  <SavedComparisonWorkspace view="compare" :device-id="deviceId" :left-id="leftId" :right-id="rightId" @device-count="$emit('device-count',$event)"/>
</template>

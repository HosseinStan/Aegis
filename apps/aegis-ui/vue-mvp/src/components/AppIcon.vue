<script setup>
defineProps({name:{type:String,default:'server'}});
const paths={
  dashboard:['M3 3h7v7H3zM14 3h7v11h-7zM3 14h7v7H3zM14 18h7v3h-7z'],
  server:['M5 3h14a2 2 0 0 1 2 2v3a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2zM5 14h14a2 2 0 0 1 2 2v3a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2zM7 6.5h.01M7 17.5h.01M11 6.5h6M11 17.5h6'],
  archive:['M4 8v12h16V8M3 3h18v5H3zM9 12h6'],
  diff:['M5 3v18M19 3v18M9 7h6M12 4v6M9 16h6'],
  code:['m8 6-6 6 6 6m8-12 6 6-6 6M14 3l-4 18'],
  arrow:['M4 12h16M14 6l6 6-6 6'],
  external:['M6 18 18 6M6 6h12v12'],
  search:['M17 10.5a6.5 6.5 0 1 1-13 0 6.5 6.5 0 0 1 13 0zM16 16l5 5'],
  refresh:['M20 7v5h-5M4 17v-5h5M6.1 7a7 7 0 0 1 11.6-1L20 9M4 15l2.3 3A7 7 0 0 0 18 17'],
  lock:['M5 10h14v11H5zM8 10V7a4 4 0 0 1 8 0v3M12 14v3'],
  clock:['M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0zM12 7v5l3 2'],
  check:['m5 12 4 4L19 6'],
  groups:['M12 3a3 3 0 1 1 0 6 3 3 0 0 1 0-6zM3 17a3 3 0 1 1 0 6 3 3 0 0 1 0-6zM21 17a3 3 0 1 1 0 6 3 3 0 0 1 0-6zM12 9v4M3 17v-4h18v4'],
  alert:['m12 3 10 17H2zM12 9v4M12 17h.01']
};
</script>

<template>
  <svg class="icon" viewBox="0 0 24 24" aria-hidden="true"><path v-for="(d,index) in (paths[name]||paths.server)" :key="index" :d="d"/></svg>
</template>

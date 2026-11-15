import {ref,watch} from 'vue';

// Component memory only: no storage, request, approval or audit record.
export function useDemoViewed(selection){
  const viewed=ref(false);
  watch(()=>[selection.deviceId,selection.leftId,selection.rightId],()=>viewed.value=false,{flush:'sync'});
  return viewed;
}

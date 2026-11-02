import {onScopeDispose,ref,shallowRef} from 'vue';

export function useGoResource(loader){
  const status=ref('idle'),data=shallowRef(null),error=shallowRef(null);
  let controller,sequence=0;
  async function load(...args){
    const current=++sequence;controller?.abort();controller=new AbortController();
    const signal=controller.signal;status.value='loading';data.value=null;error.value=null;
    try{
      const result=await loader(...args,signal);
      if(current===sequence){data.value=result;status.value='ready';}
    }catch(cause){
      if(current!==sequence||signal.aborted)return;
      error.value=cause;status.value='error';
    }
  }
  onScopeDispose(()=>{sequence++;controller?.abort();});
  return {status,data,error,load};
}

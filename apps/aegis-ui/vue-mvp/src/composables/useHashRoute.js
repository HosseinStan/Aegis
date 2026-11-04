import {onScopeDispose,shallowRef} from 'vue';

function parse(){
  const [pathname,search='']=(location.hash.slice(1)||'/overview').split('?');
  const segments=pathname.split('/').filter(Boolean),query=new URLSearchParams(search);
  let id=null;
  try{if(segments[0]==='devices'&&segments[1])id=decodeURIComponent(segments[1]);}catch{return {name:'not-found',query};}
  const name=segments[0]==='devices'?(id?'detail':'inventory'):segments[0]==='overview'?'dashboard':segments[0]==='backups'&&segments.length===1?'backups':'not-found';
  return {name,id,query};
}
export function useHashRoute(){
  const route=shallowRef(parse());
  const update=()=>{
    const next=parse(),previous=route.value;route.value=next;
    if(next.name!=='detail'||next.id!==previous.id||next.query.get('tab')!==previous.query.get('tab'))window.scrollTo(0,0);
  };
  window.addEventListener('hashchange',update);
  onScopeDispose(()=>window.removeEventListener('hashchange',update));
  return route;
}
export function replaceHashRoute(hash){
  const next=hash.startsWith('#')?hash:'#'+hash;
  if(location.hash===next)return;
  const oldURL=location.href;history.replaceState(null,'',next);
  window.dispatchEvent(new HashChangeEvent('hashchange',{oldURL,newURL:location.href}));
}

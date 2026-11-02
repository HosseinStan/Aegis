import {onScopeDispose,shallowRef} from 'vue';

function parse(){
  const [pathname,search='']=(location.hash.slice(1)||'/overview').split('?');
  const segments=pathname.split('/').filter(Boolean),query=new URLSearchParams(search);
  let id=null;
  try{if(segments[0]==='devices'&&segments[1])id=decodeURIComponent(segments[1]);}catch{return {name:'not-found',query};}
  const name=segments[0]==='devices'?(id?'detail':'inventory'):segments[0]==='overview'?'dashboard':'not-found';
  return {name,id,query};
}
export function useHashRoute(){
  const route=shallowRef(parse());
  const update=()=>{route.value=parse();window.scrollTo(0,0);};
  window.addEventListener('hashchange',update);
  onScopeDispose(()=>window.removeEventListener('hashchange',update));
  return route;
}

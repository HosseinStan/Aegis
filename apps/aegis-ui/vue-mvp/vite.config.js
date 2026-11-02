import {defineConfig} from 'vite';
import vue from '@vitejs/plugin-vue';

// This is only the public URL of the existing Node preview. No identity or token.
const candidate=process.env.AEGIS_PREVIEW_ORIGIN;
if(!candidate)throw new Error('Set AEGIS_PREVIEW_ORIGIN to the running isolated Node preview URL.');
const origin=new URL(candidate);
if(origin.protocol!=='http:'||origin.hostname!=='127.0.0.1'||!origin.port||origin.username||origin.password||origin.pathname!=='/'||origin.search||origin.hash){
  throw new Error('AEGIS_PREVIEW_ORIGIN must be an exact http://127.0.0.1:PORT origin.');
}
const target=origin.origin;
// Reuse the authenticated bridge, not a new API or database reader.
const proxy={
  '^/api/go/v1/devices(?:/[a-zA-Z0-9-]+)?(?:/backups)?(?:\\?.*)?$':{target,changeOrigin:true,ws:false}
};
const headers={
  'X-Content-Type-Options':'nosniff',
  'Content-Security-Policy':"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws://127.0.0.1:*; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'"
};
export default defineConfig({
  plugins:[vue()],
  define:{__AEGIS_PREVIEW_ORIGIN__:JSON.stringify(target)},
  publicDir:false,
  server:{host:'127.0.0.1',port:5173,strictPort:true,allowedHosts:['127.0.0.1','localhost'],cors:false,proxy,headers},
  preview:{host:'127.0.0.1',port:5174,strictPort:true,allowedHosts:['127.0.0.1','localhost'],cors:false,proxy,headers},
  build:{sourcemap:false}
});

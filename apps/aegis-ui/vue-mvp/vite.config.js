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
const snapshotPath='^/api/go/v1/devices/[a-zA-Z0-9-]+/backups/[a-zA-Z0-9-]+(?:\\?.*)?$';
const snapshotPattern=new RegExp(snapshotPath);
const diffPath='^/api/go/v1/devices/[a-zA-Z0-9-]+/backups/[a-zA-Z0-9-]+/diff/[a-zA-Z0-9-]+(?:\\?.*)?$';
const diffPattern=new RegExp(diffPath);
function snapshotGetOnly(server){
  server.middlewares.use((req,res,next)=>{
    const diff=diffPattern.test(req.url||'');
    if((snapshotPattern.test(req.url||'')||diff)&&req.method!=='GET'){
      res.writeHead(405,{'Allow':'GET','Content-Type':'application/json; charset=utf-8'});
      res.end(JSON.stringify({error:{code:diff?'READ_ONLY_DIFF_PROXY':'READ_ONLY_SNAPSHOT_PROXY',message:'Saved snapshot content and native Diff support GET only.'}}));return;
    }
    next();
  });
}
// Reuse the authenticated bridge, not a new API or database reader.
const proxy={
  '^/api/go/v1/devices(?:/[a-zA-Z0-9-]+)?(?:/backups)?(?:\\?.*)?$':{target,changeOrigin:true,ws:false},
  [snapshotPath]:{target,changeOrigin:true,ws:false},
  [diffPath]:{target,changeOrigin:true,ws:false}
};
const headers={
  'X-Content-Type-Options':'nosniff',
  'Content-Security-Policy':"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws://127.0.0.1:*; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'"
};
export default defineConfig({
  plugins:[vue(),{name:'aegis-saved-snapshot-get-only',configureServer:snapshotGetOnly,configurePreviewServer:snapshotGetOnly}],
  define:{__AEGIS_PREVIEW_ORIGIN__:JSON.stringify(target)},
  publicDir:false,
  server:{host:'127.0.0.1',port:5173,strictPort:true,allowedHosts:['127.0.0.1','localhost'],cors:false,proxy,headers},
  preview:{host:'127.0.0.1',port:5174,strictPort:true,allowedHosts:['127.0.0.1','localhost'],cors:false,proxy,headers},
  build:{sourcemap:false}
});

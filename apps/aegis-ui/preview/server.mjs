import http from 'node:http';
import { readFile, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const root = path.dirname(fileURLToPath(import.meta.url));
const publicRoot = path.join(root, 'public');
const files = {'/':'index.html','/index.html':'index.html','/app.js':'app.js','/styles.css':'styles.css','/demo-data.json':'demo-data.json','/favicon.svg':'favicon.svg'};
const types = {'.html':'text/html; charset=utf-8','.js':'text/javascript; charset=utf-8','.css':'text/css; charset=utf-8','.json':'application/json; charset=utf-8','.svg':'image/svg+xml'};
const server = http.createServer(async (req,res)=>{
  res.setHeader('Cache-Control','no-store');
  res.setHeader('X-Content-Type-Options','nosniff');
  res.setHeader('Content-Security-Policy',"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'");
  if(!/^((127\.0\.0\.1)|(localhost)):\d+$/.test(req.headers.host||'')){res.writeHead(403);return res.end('Local preview only');}
  if(!['GET','HEAD'].includes(req.method)){res.writeHead(405,{'Allow':'GET, HEAD'});return res.end('Preview is read-only');}
  const pathname = new URL(req.url, 'http://127.0.0.1').pathname;
  if(pathname === '/healthz'){res.writeHead(200,{'Content-Type':'application/json'});return res.end(JSON.stringify({status:'ok',mode:'synthetic-demo',liveDeviceAccess:false,realApiConnected:false}));}
  const name=files[pathname];
  if(!name){res.writeHead(404);return res.end('Not found');}
  try{const body=await readFile(path.join(publicRoot,name));res.writeHead(200,{'Content-Type':types[path.extname(name)]});res.end(req.method==='HEAD'?undefined:body);}
  catch{res.writeHead(500);res.end('Preview asset unavailable');}
});
server.listen(0,'127.0.0.1',async()=>{
 const {port}=server.address();
 const info={url:'http://127.0.0.1:'+port,pid:process.pid,host:'127.0.0.1',port,mode:'DEMO',startedAt:new Date().toISOString()};
 await writeFile(path.join(root,'runtime.json'),JSON.stringify(info,null,2));
 console.log(info.url);
});
server.on('error',error=>{console.error(error.message);process.exitCode=1;});

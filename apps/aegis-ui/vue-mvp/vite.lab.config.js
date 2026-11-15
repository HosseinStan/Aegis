import {fileURLToPath} from 'node:url';
import {defineConfig} from 'vite';
import vue from '@vitejs/plugin-vue';

// The compiled UI is served by the dedicated loopback Go Lab server.
export default defineConfig({
  root:fileURLToPath(new URL('./lab/',import.meta.url)),
  envDir:false,
  publicDir:false,
  plugins:[vue()],
  build:{sourcemap:false}
});

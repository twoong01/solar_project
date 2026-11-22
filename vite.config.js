import {defineConfig, loadEnv} from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig(({mode}) => {
  const env = loadEnv(mode, process.cwd(), '');
  const apiProxy = env.VITE_API_PROXY;
  return {
    plugins: [react()],
    base: './',
    server: {
      host: '127.0.0.1', port: 5173, strictPort: false,
      proxy: apiProxy ? {
        '/api': {target:apiProxy,changeOrigin:true},
        '/data.json': {target:apiProxy,changeOrigin:true}
      } : undefined,
    },
  };
});

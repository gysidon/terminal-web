import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import monacoEditorPluginPkg from 'vite-plugin-monaco-editor';
const monacoEditorPlugin = monacoEditorPluginPkg.default || monacoEditorPluginPkg;

export default defineConfig({
  plugins: [vue(), monacoEditorPlugin({})],
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://127.0.0.1:3000', changeOrigin: true },
      '/ws': { target: 'ws://127.0.0.1:3000', ws: true }
    }
  },
  build: {
    outDir: 'dist',
    chunkSizeWarningLimit: 1500
  }
});

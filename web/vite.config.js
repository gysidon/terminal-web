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
    chunkSizeWarningLimit: 2000,
    rollupOptions: {
      input: {
        main: 'index.html',
        splash: 'splashscreen.html'
      },
      output: {
        // 只对第三方依赖分包，且一律用带斜杠的包目录精确匹配：
        // 用 includes('monaco-editor') 会误伤 @guolao/vue-monaco-editor（首屏静态引用），
        // 把整个 monaco 拽进入口静态依赖图，懒加载直接失效。
        manualChunks(id) {
          // Vite 的 __vitePreload 辅助模块只有几百字节，Rollup 会把这类小 chunk
          // 就近并进别的 chunk；一旦并进 monaco，入口就会静态依赖 4.5MB 的 monaco。
          // 显式钉进 vendor（本来就是首屏 chunk），断掉这条意外的静态引用链。
          if (id.includes('vite/preload-helper')) return 'vendor';
          if (!id.includes('node_modules')) return;
          // monaco 包装层不指定分包，交给 Rollup 跟它的异步引用方（FileEditor）走
          if (id.includes('/@guolao/')) return;
          if (id.includes('/monaco-editor/')) return 'monaco';
          if (id.includes('/@xterm/')) return 'xterm';
          if (id.includes('/naive-ui/') || id.includes('/@vicons/')) return 'naive';
          return 'vendor';
        }
      }
    }
  }
});

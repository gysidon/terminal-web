import { createApp } from 'vue';
import App from './App.vue';
import '@xterm/xterm/css/xterm.css';

import * as monaco from 'monaco-editor';
import { loader } from '@guolao/vue-monaco-editor';

// worker 由 vite-plugin-monaco-editor 自动注入，无需手动配置 MonacoEnvironment
loader.config({ monaco });

createApp(App).mount('#app');

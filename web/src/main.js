import { createApp } from 'vue';
import App from './App.vue';

// Monaco 改为按需加载（见 components/FileEditor.vue + monaco-setup.js），
// xterm 样式随异步组件 TermPane 一起加载（见 components/TermPane.vue），
// 均不在入口处引入，避免首屏被数 MB 的第三方库拖慢。
createApp(App).mount('#app');

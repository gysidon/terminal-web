<template>
  <div class="term-wrap">
    <div ref="termEl" class="term-el"></div>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, watch } from 'vue';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { getToken } from '../api.js';
import { store } from '../store.js';

const props = defineProps({
  connId: { type: Number, required: true },
  sessionId: { type: String, required: true },
  active: { type: Boolean, default: true },
  initDir: { type: String, default: null }
});
const emit = defineEmits(['status']);

const termEl = ref(null);
let term = null;
let fit = null;
let ws = null;
let pingTimer = null;
let resizeObserver = null;
let disposed = false;
let initDirSent = false;

const XTERM_THEMES = {
  dark: {
    background: '#101014',
    foreground: '#d6d6d6',
    cursor: '#63e2b7',
    selectionBackground: '#2c4a3e',
    black: '#000000', red: '#e24b4a', green: '#63e2b7', yellow: '#efc078',
    blue: '#70a5eb', magenta: '#c397d8', cyan: '#66c7d6', white: '#d6d6d6',
    brightBlack: '#5c5c66'
  },
  light: {
    background: '#ffffff',
    foreground: '#1f2329',
    cursor: '#14b8a6',
    selectionBackground: '#bfeee8',
    black: '#000000', red: '#e24b4a', green: '#14b8a6', yellow: '#b7791f',
    blue: '#3182ce', magenta: '#b83280', cyan: '#0bc5ea', white: '#1f2329',
    brightBlack: '#6b7280'
  }
};

function applyTermStyle() {
  if (!term) return;
  term.options.fontSize = store.fontSize || 13;
  term.options.fontFamily = store.fontFamily || '"JetBrains Mono", Menlo, Consolas, monospace';
  term.options.theme = XTERM_THEMES[store.theme === 'light' ? 'light' : 'dark'];
  try { fit?.fit(); } catch {}
}

const b64encode = (s) => btoa(String.fromCharCode(...new TextEncoder().encode(s)));
const b64decode = (s) => {
  const bin = atob(s);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return bytes;
};

function connect() {
  initDirSent = false;
  emit('status', 'connecting');
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  ws = new WebSocket(`${proto}://${location.host}/ws/terminal/${props.connId}?token=${encodeURIComponent(getToken())}`);

  ws.onmessage = (ev) => {
    let msg;
    try { msg = JSON.parse(ev.data); } catch { return; }
    if (msg.type === 'data') {
      term.write(b64decode(msg.data));
    } else if (msg.type === 'connected') {
      emit('status', 'connected');
      sendResize();
      term.focus();
      if (props.initDir && !initDirSent) {
        initDirSent = true;
        const dir = props.initDir;
        const cmd = `cd '${dir.replace(/'/g, "'\\''")}'\nclear\n`;
        if (ws?.readyState === 1) ws.send(JSON.stringify({ type: 'input', data: b64encode(cmd) }));
      }
    } else if (msg.type === 'status') {
      term.write(`\x1b[90m${msg.message}\x1b[0m\r\n`);
    } else if (msg.type === 'error') {
      emit('status', 'error');
      term.write(`\r\n\x1b[31m${msg.message}\x1b[0m\r\n`);
    } else if (msg.type === 'exit') {
      emit('status', 'closed');
      term.write('\r\n\x1b[90m会话已结束，点击右上角「重连」可重新连接。\x1b[0m\r\n');
    }
  };
  ws.onclose = () => {
    if (!disposed) emit('status', 'closed');
    clearInterval(pingTimer);
  };
  ws.onerror = () => emit('status', 'error');

  clearInterval(pingTimer);
  pingTimer = setInterval(() => {
    if (ws?.readyState === 1) ws.send(JSON.stringify({ type: 'ping' }));
  }, 30000);
}

function sendResize() {
  if (ws?.readyState === 1 && term) {
    ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
  }
}

function reconnect() {
  try { ws?.close(); } catch {}
  term.reset();
  connect();
}

defineExpose({ reconnect, resize: () => { try { fit?.fit(); } catch {} } });

onMounted(() => {
  term = new Terminal({
    fontFamily: store.fontFamily || '"JetBrains Mono", Menlo, Consolas, "Courier New", monospace',
    fontSize: store.fontSize || 13,
    cursorBlink: true,
    scrollback: 5000,
    theme: XTERM_THEMES[store.theme === 'light' ? 'light' : 'dark']
  });
  fit = new FitAddon();
  term.loadAddon(fit);
  term.loadAddon(new WebLinksAddon());
  term.open(termEl.value);
  fit.fit();

  term.onData((data) => {
    if (ws?.readyState === 1) ws.send(JSON.stringify({ type: 'input', data: b64encode(data) }));
  });
  term.onResize(() => sendResize());

  resizeObserver = new ResizeObserver(() => {
    try { fit.fit(); } catch {}
  });
  resizeObserver.observe(termEl.value);

  // 设置项实时生效
  watch(() => [store.fontSize, store.fontFamily], applyTermStyle);
  watch(() => store.theme, applyTermStyle);

  // 仅当该标签页为激活态时才建立连接（刷新后恢复的非激活终端需点击才重连）
  if (props.active) connect();
  watch(
    () => props.active,
    (v) => {
      if (v && (!ws || ws.readyState !== 1)) connect();
    }
  );
});

onBeforeUnmount(() => {
  disposed = true;
  clearInterval(pingTimer);
  resizeObserver?.disconnect();
  try { ws?.close(); } catch {}
  term?.dispose();
});
</script>

<style scoped>
.term-wrap {
  flex: 1;
  min-height: 0;
  background: var(--term-bg);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 8px;
  height: 100%;
  box-sizing: border-box;
}
.term-el { width: 100%; height: 100%; }
</style>

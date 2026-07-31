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

// 未就绪时输入缓冲（避免连接建立/重连瞬间静默丢字）
const inputBuffer = [];
// 同内容 15ms 内去重，专门兜住 macOS WKWebView 把同一按键投递两次的问题
let lastInputTime = 0;
let lastInputData = '';

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

// 安全 base64 编码：逐字节拼接，避免 String.fromCharCode(...spread) 在长输入时抛 RangeError
const b64encode = (s) => {
  const bytes = new TextEncoder().encode(s);
  let bin = '';
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
  return btoa(bin);
};
const b64decode = (s) => {
  const bin = atob(s);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return bytes;
};

function rawSend(obj) {
  if (ws?.readyState === 1) {
    try { ws.send(JSON.stringify(obj)); return true; } catch { return false; }
  }
  return false;
}

function sendInput(data) {
  const now = Date.now();
  // 去重：同一内容且间隔 <15ms 视为 WKWebView 双发，丢弃第二次
  if (now - lastInputTime < 15 && data === lastInputData) return;
  lastInputTime = now;
  lastInputData = data;
  if (!rawSend({ type: 'input', data: b64encode(data) })) {
    inputBuffer.push(data); // ws 未就绪，缓存待重连后补发
  }
}

function flushInput() {
  while (inputBuffer.length) {
    const d = inputBuffer.shift();
    rawSend({ type: 'input', data: b64encode(d) });
  }
}

// 把键盘事件翻译成终端控制序列；返回 null 表示交给 xterm 原生处理（如 IME、Cmd 组合）
function keyToSequence(e) {
  const k = e.key;
  if (e.isComposing || e.metaKey) return null;

  if (!e.ctrlKey && !e.altKey) {
    if (k.length === 1) return k;
    switch (k) {
      case 'Enter': return '\r';
      case 'Backspace': return '\x7f';
      case 'Tab': return '\t';
      case 'Escape': return '\x1b';
      case 'ArrowLeft': return '\x1b[D';
      case 'ArrowRight': return '\x1b[C';
      case 'ArrowUp': return '\x1b[A';
      case 'ArrowDown': return '\x1b[B';
      case 'Home': return '\x1b[H';
      case 'End': return '\x1b[F';
      case 'Insert': return '\x1b[2~';
      case 'Delete': return '\x1b[3~';
      case 'PageUp': return '\x1b[5~';
      case 'PageDown': return '\x1b[6~';
      default:
        if (/^F([1-9]|1[0-2])$/.test(k)) {
          const n = parseInt(k.slice(1), 10);
          const fmap = { 1: '\x1bOP', 2: '\x1bOQ', 3: '\x1bOR', 4: '\x1bOS' };
          return fmap[n] || `\x1b[${n + 11}~`;
        }
        return null;
    }
  }

  if (e.ctrlKey && !e.altKey) {
    if (k.length === 1) {
      const c = k.toLowerCase().charCodeAt(0);
      if (c >= 97 && c <= 122) return String.fromCharCode(c - 96); // Ctrl+a..z
      if (k === ' ' || c === 64) return '\x00';                    // Ctrl+Space / Ctrl+@
      if (k === '[') return '\x1b';
      if (k === '\\') return '\x1c';
      if (k === ']') return '\x1d';
      if (k === '^') return '\x1e';
      if (k === '_') return '\x1f';
      if (c >= 48 && c <= 57) return String.fromCharCode(c - 48);  // Ctrl+0..9
    }
    switch (k) {
      case 'ArrowLeft': return '\x1b[1;5D';
      case 'ArrowRight': return '\x1b[1;5C';
      case 'ArrowUp': return '\x1b[1;5A';
      case 'ArrowDown': return '\x1b[1;5B';
      case 'Home': return '\x1b[1;5H';
      case 'End': return '\x1b[1;5F';
    }
    return null;
  }

  if (e.altKey && !e.ctrlKey) {
    if (k.length === 1) return '\x1b' + k; // Alt+char → ESC+char
    return null;
  }

  return null;
}

function onKeyDownCapture(e) {
  const seq = keyToSequence(e);
  if (seq === null) return; // 交给 xterm 原生处理
  e.preventDefault();
  e.stopPropagation();
  sendInput(seq);
}

function connect() {
  initDirSent = false;
  inputBuffer.length = 0;
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
      flushInput(); // 补发连接期间缓存的输入
      term.focus();
      if (props.initDir && !initDirSent) {
        initDirSent = true;
        const dir = props.initDir;
        const cmd = `cd '${dir.replace(/'/g, "'\\''")}'\nclear\n`;
        rawSend({ type: 'input', data: b64encode(cmd) });
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
    rawSend({ type: 'ping' });
  }, 30000);
}

function sendResize() {
  if (ws?.readyState === 1 && term) {
    rawSend({ type: 'resize', cols: term.cols, rows: term.rows });
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

  // 接管键盘输入（捕获阶段），阻断 WKWebView 把同一按键投递两次给 xterm textarea
  termEl.value.addEventListener('keydown', onKeyDownCapture, true);
  // 保留 xterm 原生 onData：用于粘贴 / 中文输入法组字等场景
  term.onData((data) => sendInput(data));
  term.onResize(() => sendResize());

  resizeObserver = new ResizeObserver(() => {
    try { fit.fit(); term.focus(); } catch {}
  });
  resizeObserver.observe(termEl.value);

  // 点击终端区域也确保聚焦，避免输入丢失
  termEl.value.addEventListener('pointerdown', () => term?.focus());

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
  try { termEl.value?.removeEventListener('keydown', onKeyDownCapture, true); } catch {}
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

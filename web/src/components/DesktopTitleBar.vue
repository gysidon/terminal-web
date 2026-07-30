<template>
  <div class="dtb">
    <div class="dtb-drag" @mousedown="startDrag">
      <LogoIcon :size="16" />
      <span class="dtb-title">Terminal Web</span>
    </div>
    <div class="dtb-actions">
      <button v-if="logged" class="dtb-btn dtb-settings" title="设置" @click="openSettings">
        <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33h.01a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51h.01a1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82v.01a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" /></svg>
      </button>
      <button class="dtb-btn" title="最小化" @click="minimize">
        <svg viewBox="0 0 12 12" width="12" height="12"><rect x="1.5" y="5.6" width="9" height="1.3" rx="0.65" fill="currentColor" /></svg>
      </button>
      <button class="dtb-btn" :title="maximized ? '还原' : '最大化'" @click="toggleMax">
        <svg v-if="!maximized" viewBox="0 0 12 12" width="12" height="12"><rect x="1.8" y="1.8" width="8.4" height="8.4" rx="1" fill="none" stroke="currentColor" stroke-width="1.3" /></svg>
        <svg v-else viewBox="0 0 12 12" width="12" height="12"><path d="M3.6 2.2h5.6v5.6H3.6z" fill="none" stroke="currentColor" stroke-width="1.1" /><path d="M2.2 4.6h5.6v5.6H2.2z" fill="none" stroke="currentColor" stroke-width="1.1" /></svg>
      </button>
      <button class="dtb-btn dtb-close" title="关闭" @click="close">
        <svg viewBox="0 0 12 12" width="12" height="12"><path d="M2.6 2.6l6.8 6.8M9.4 2.6l-6.8 6.8" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" /></svg>
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue';
import { store } from '../store.js';
import LogoIcon from './LogoIcon.vue';
import { getCurrentWindow } from '@tauri-apps/api/window';

defineProps({
  logged: { type: Boolean, default: false }
});

const maximized = ref(false);
let unlisten = null;

async function init() {
  try {
    const w = getCurrentWindow();
    maximized.value = await w.isMaximized();
    unlisten = await w.onResized(async () => {
      maximized.value = await w.isMaximized();
    });
  } catch {
    /* 非桌面环境（纯 Web）下忽略，按钮不会被渲染 */
  }
}

function startDrag(e) {
  // e.detail 为连击计数：双击时切换最大化/还原（startDragging 会吞掉 dblclick 事件，需在 mousedown 里判断）
  if (e.detail === 2) {
    toggleMax();
    return;
  }
  getCurrentWindow().startDragging().catch(() => {});
}
function minimize() {
  getCurrentWindow().minimize().catch(() => {});
}
function toggleMax() {
  getCurrentWindow().toggleMaximize().catch(() => {});
}
function close() {
  getCurrentWindow().close().catch(() => {});
}
function openSettings() {
  store.showSettings = true;
}

onMounted(init);
onBeforeUnmount(() => {
  if (unlisten) unlisten();
});
</script>

<style scoped>
.dtb {
  height: 38px;
  min-height: 38px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--bg-sidebar);
  border-bottom: 1px solid var(--border);
  user-select: none;
}
.dtb-drag {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  height: 100%;
  flex: 1;
  min-width: 0;
}
.dtb-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
}
.dtb-actions {
  display: flex;
  height: 100%;
}
.dtb-btn {
  width: 46px;
  height: 100%;
  border: none;
  background: transparent;
  color: var(--text);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}
.dtb-btn:hover {
  background: var(--hover);
}
.dtb-close:hover {
  background: #e81123;
  color: #fff;
}
</style>

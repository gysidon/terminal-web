<template>
  <n-config-provider :theme="naiveTheme" :theme-overrides="themeOverrides" :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider>
      <n-dialog-provider>
        <div class="app-root" :class="{ desktop: store.isDesktop }">
          <DesktopTitleBar v-if="store.isDesktop" :logged="logged" />
          <div class="app-content">
            <div v-if="booting" class="boot-loading">正在启动终端客户端…</div>
            <template v-else>
              <Setup v-if="!logged && !initialized" @setup-done="onSetupDone" />
              <Login v-else-if="!logged" @logged="onLogged" />
              <Main v-else @logout="onLogout" />
            </template>
          </div>
        </div>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup>
import { ref, computed, nextTick, onMounted, onBeforeUnmount } from 'vue';
import { NConfigProvider, NMessageProvider, NDialogProvider, darkTheme, zhCN, dateZhCN } from 'naive-ui';
import Login from './views/Login.vue';
import Main from './views/Main.vue';
import Setup from './views/Setup.vue';
import DesktopTitleBar from './components/DesktopTitleBar.vue';
import { getToken, setToken, setupStatus } from './api.js';
import { store, loadLocal, loadPublicSettings, applyTheme } from './store.js';

const booting = ref(true);
const logged = ref(false);
const initialized = ref(true);

loadLocal();

const naiveTheme = computed(() => (store.theme === 'dark' ? darkTheme : null));

const themeOverrides = computed(() => {
  const dark = store.theme === 'dark';
  return {
    common: {
      primaryColor: '#63e2b7',
      primaryColorHover: '#7fe7c4',
      primaryColorPressed: '#5acea7',
      bodyColor: dark ? '#101014' : '#f5f6f8',
      cardColor: dark ? '#18181c' : '#ffffff',
      // 亮色下用更深的薄荷绿保证对比度
      popoverColor: dark ? '#1f1f24' : '#ffffff',
      modalColor: dark ? '#18181c' : '#ffffff'
    }
  };
});

function onLogout() {
  setToken('');
  logged.value = false;
}

function onLogged() {
  logged.value = true;
}

function onUnauthorized() {
  logged.value = false;
}

function onSetupDone() {
  initialized.value = true;
  logged.value = true;
}

async function bootstrap() {
  const params = new URLSearchParams(location.search);
  const boot = params.get('boot');
  // 桌面模式标记：URL 带 ?boot= 即桌面客户端（安全/登录日志面板在 Settings 中据此隐藏）
  store.isDesktop = !!boot;
  if (boot && !getToken()) {
    try {
      const resp = await fetch('/api/desktop/session', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token: boot })
      });
      if (resp.ok) {
        const data = await resp.json();
        setToken(data.token);
        logged.value = true;
      }
    } catch (e) {
      // boot token 兑换失败则回退到登录页
    }
  }
  booting.value = false;
}

onMounted(async () => {
  await bootstrap();
  try {
    initialized.value = await setupStatus();
  } catch {
    initialized.value = true;
  }
  await loadPublicSettings();
  applyTheme();
  window.addEventListener('tw-unauthorized', onUnauthorized);
  // 桌面端：确保 Vue 完成 DOM 更新（booting=false 的分支渲染完成）后再通知 Rust
  // 关闭 splashscreen，避免 splash 关闭后用户看到 .boot-loading 或骨架屏。
  if (store.isDesktop) {
    await nextTick();
    import('@tauri-apps/api/event').then((m) => m.emit('app-ready')).catch(() => {});
  }
});
onBeforeUnmount(() => window.removeEventListener('tw-unauthorized', onUnauthorized));
</script>

<style>
.boot-loading {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-app);
  color: var(--text-sub);
  font-size: 14px;
}
.app-root {
  display: flex;
  flex-direction: column;
  height: 100%;
}
/* naive-ui 的 config-provider 会渲染一层 div，需补齐高度链，否则 .app-root 的 100% 失效 */
#app > .n-config-provider {
  height: 100%;
}
.app-content {
  flex: 1;
  min-height: 0;
}
:root,
[data-theme='dark'] {
  --bg-app: #101014;
  --bg-sidebar: #141418;
  --bg-card: #18181c;
  --bg-elevated: #1f1f24;
  --border: #26262c;
  --text: #eeeeee;
  --text-sub: #777777;
  --text-mut: #888888;
  --accent: #63e2b7;
  --term-bg: #101014;
  --hover: rgba(255, 255, 255, 0.06);
}

[data-theme='light'] {
  --bg-app: #f5f6f8;
  --bg-sidebar: #ffffff;
  --bg-card: #ffffff;
  --bg-elevated: #ffffff;
  --border: #e3e5e8;
  --text: #1f2329;
  --text-sub: #8a8f99;
  --text-mut: #6b7280;
  --accent: #14b8a6;
  --term-bg: #ffffff;
  --hover: rgba(0, 0, 0, 0.04);
}

html,
body,
#app {
  height: 100%;
  margin: 0;
}
/* 兜底：html 也铺满主题色，任何内容未绘制/透出的瞬间露出的是主题色而非 WKWebView 默认白 */
html {
  background: var(--bg-app);
}
body {
  background: var(--bg-app);
  color: var(--text);
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
}
</style>

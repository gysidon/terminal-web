<template>
  <n-config-provider :theme="naiveTheme" :theme-overrides="themeOverrides" :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider>
      <n-dialog-provider>
        <Setup v-if="!logged && !initialized" @setup-done="onSetupDone" />
        <Login v-else-if="!logged" @logged="onLogged" />
        <Main v-else @logout="onLogout" />
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue';
import { NConfigProvider, NMessageProvider, NDialogProvider, darkTheme, zhCN, dateZhCN } from 'naive-ui';
import Login from './views/Login.vue';
import Main from './views/Main.vue';
import Setup from './views/Setup.vue';
import { getToken, setToken, setupStatus } from './api.js';
import { store, loadLocal, loadPublicSettings, applyTheme } from './store.js';

const logged = ref(!!getToken());
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

onMounted(async () => {
  try {
    initialized.value = await setupStatus();
  } catch {
    initialized.value = true;
  }
  await loadPublicSettings();
  applyTheme();
  window.addEventListener('tw-unauthorized', onUnauthorized);
});
onBeforeUnmount(() => window.removeEventListener('tw-unauthorized', onUnauthorized));
</script>

<style>
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
body {
  background: var(--bg-app);
  color: var(--text);
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif;
}
</style>

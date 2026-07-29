import { reactive } from 'vue';
import { api, errMsg } from './api.js';

export const DEFAULT_FONT = '"JetBrains Mono", Menlo, Consolas, "Courier New", monospace';

// 全局 UI / 系统设置状态（单一数据源）。后端以 key-value 存储，前端同步到此处并持久化部分到 localStorage。
export const store = reactive({
  // 安全
  captchaEnabled: true,
  loginFailMax: 5,
  loginLockMinutes: 15,
  ipWhitelist: '',
  sessionTimeout: 0,
  ssoEnabled: false,
  // 外观
  theme: 'dark',
  fontSize: 13,
  fontFamily: DEFAULT_FONT
});

const LS_KEY = 'tw_settings';

function persist() {
  try {
    localStorage.setItem(LS_KEY, JSON.stringify({
      theme: store.theme,
      fontSize: store.fontSize,
      fontFamily: store.fontFamily
    }));
  } catch {}
}

export function applyTheme() {
  if (typeof document !== 'undefined') {
    document.documentElement.setAttribute('data-theme', store.theme);
  }
}

export function loadLocal() {
  try {
    const raw = localStorage.getItem(LS_KEY);
    if (raw) Object.assign(store, JSON.parse(raw));
  } catch {}
  applyTheme();
}

// 登录页/初始化阶段：只取非敏感前置设置
export async function loadPublicSettings() {
  try {
    const { data } = await api.get('/api/settings/public');
    if (data.captchaEnabled !== undefined) store.captchaEnabled = data.captchaEnabled;
    if (data.theme) store.theme = data.theme;
    if (data.fontSize) store.fontSize = data.fontSize;
    if (data.fontFamily) store.fontFamily = data.fontFamily;
  } catch {
    /* 取不到时用默认 */
  }
  applyTheme();
}

// 登录后：拉取完整设置（含安全项）
export async function loadSettings() {
  try {
    const { data } = await api.get('/api/settings');
    Object.assign(store, {
      captchaEnabled: data.captcha_enabled,
      theme: data.theme,
      fontSize: data.font_size,
      fontFamily: data.font_family,
      loginFailMax: data.login_fail_max,
      loginLockMinutes: data.login_lock_minutes,
      ipWhitelist: data.ip_whitelist,
      sessionTimeout: data.session_timeout,
      ssoEnabled: data.sso_enabled
    });
  } catch (e) {
    console.warn('加载设置失败:', errMsg(e));
  }
  applyTheme();
}

export async function saveSettings(payload) {
  const { data } = await api.put('/api/settings', payload);
  Object.assign(store, {
    captchaEnabled: data.captcha_enabled,
    theme: data.theme,
    fontSize: data.font_size,
    fontFamily: data.font_family,
    loginFailMax: data.login_fail_max,
    loginLockMinutes: data.login_lock_minutes,
    ipWhitelist: data.ip_whitelist,
    sessionTimeout: data.session_timeout,
    ssoEnabled: data.sso_enabled
  });
  persist();
  applyTheme();
  return data;
}

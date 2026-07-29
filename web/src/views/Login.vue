<template>
  <div class="login-wrap">
    <div class="login-card">
      <div class="logo">
        <LogoIcon :size="26" />
        <span class="logo-text">Terminal Web</span>
      </div>
      <div class="subtitle">SSH 终端管理平台</div>
      <n-form @keyup.enter="doLogin">
        <n-form-item label="用户名" :show-feedback="false" style="margin-bottom: 16px">
          <n-input v-model:value="username" placeholder="请输入用户名" size="large" />
        </n-form-item>
        <n-form-item label="密码" :show-feedback="false" style="margin-bottom: 16px">
          <n-input v-model:value="password" type="password" show-password-on="click" placeholder="请输入密码" size="large" />
        </n-form-item>
        <n-form-item v-if="store.captchaEnabled" label="验证码" :show-feedback="false" style="margin-bottom: 24px">
          <div class="captcha-row">
            <n-input v-model:value="captchaText" placeholder="请输入右侧字符" size="large" style="flex: 1" />
            <div class="captcha-img" v-html="captchaSvg" @click="refreshCaptcha" title="点击刷新验证码"></div>
          </div>
        </n-form-item>
        <n-button type="primary" block size="large" :loading="loading" @click="doLogin">登 录</n-button>
      </n-form>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';
import { NForm, NFormItem, NInput, NButton, useMessage } from 'naive-ui';
import { api, setToken, errMsg, getCaptcha } from '../api.js';
import { store, loadPublicSettings } from '../store.js';
import LogoIcon from '../components/LogoIcon.vue';

const emit = defineEmits(['logged']);
const message = useMessage();
const username = ref('');
const password = ref('');
const captchaId = ref('');
const captchaText = ref('');
const captchaSvg = ref('');
const loading = ref(false);

async function refreshCaptcha() {
  try {
    const data = await getCaptcha();
    captchaId.value = data.id;
    captchaSvg.value = data.svg;
    captchaText.value = '';
  } catch {
    /* 验证码获取失败不阻断登录页展示 */
  }
}

async function doLogin() {
  if (!username.value || !password.value) {
    message.warning('请输入用户名和密码');
    return;
  }
  if (store.captchaEnabled && !captchaText.value) {
    message.warning('请输入验证码');
    return;
  }
  loading.value = true;
  try {
    const payload = { username: username.value, password: password.value };
    if (store.captchaEnabled) {
      payload.captchaId = captchaId.value;
      payload.captchaText = captchaText.value;
    }
    const { data } = await api.post('/api/login', payload);
    setToken(data.token);
    emit('logged');
  } catch (err) {
    message.error(errMsg(err));
    refreshCaptcha(); // 登录失败刷新验证码
  } finally {
    loading.value = false;
  }
}

onMounted(async () => {
  await loadPublicSettings();
  if (store.captchaEnabled) refreshCaptcha();
});
</script>

<style scoped>
.login-wrap {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: radial-gradient(ellipse at 30% 20%, #1a2a25 0%, #101014 55%);
}
.login-card {
  width: 380px;
  padding: 40px 36px;
  background: #18181cee;
  border: 1px solid #2a2a30;
  border-radius: 14px;
  box-shadow: 0 12px 48px rgba(0, 0, 0, 0.5);
}
.logo {
  display: flex;
  align-items: center;
  gap: 10px;
}
.logo-dot {
  width: 26px;
  height: 26px;
  border-radius: 6px;
  object-fit: contain;
}
.logo-text {
  font-size: 22px;
  font-weight: 600;
  color: #eee;
  letter-spacing: 0.5px;
}
.subtitle {
  color: #888;
  font-size: 13px;
  margin: 8px 0 28px 24px;
}
.captcha-row {
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
}
.captcha-img {
  width: 120px;
  height: 40px;
  border-radius: 6px;
  overflow: hidden;
  cursor: pointer;
  flex-shrink: 0;
  border: 1px solid #2a2a30;
  background: #f2f2f2;
  display: flex;
  align-items: center;
  justify-content: center;
}
.captcha-img :deep(svg) {
  display: block;
  width: 120px;
  height: 40px;
}
</style>

<template>
  <div class="login-wrap">
    <div class="login-card">
      <div class="logo">
        <LogoIcon :size="26" />
        <span class="logo-text">Terminal Web</span>
      </div>
      <div class="subtitle">首次使用 · 设置管理员账号</div>
      <n-form @keyup.enter="doSetup">
        <n-form-item label="管理员用户名" :show-feedback="false" style="margin-bottom: 16px">
          <n-input v-model:value="username" placeholder="如 admin" size="large" />
        </n-form-item>
        <n-form-item label="密码" :show-feedback="false" style="margin-bottom: 16px">
          <n-input v-model:value="password" type="password" show-password-on="click" placeholder="至少 6 位" size="large" />
        </n-form-item>
        <n-form-item label="确认密码" :show-feedback="false" style="margin-bottom: 24px">
          <n-input v-model:value="confirm" type="password" show-password-on="click" placeholder="再次输入密码" size="large" />
        </n-form-item>
        <n-button type="primary" block size="large" :loading="loading" @click="doSetup">初始化并进入</n-button>
      </n-form>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue';
import { NForm, NFormItem, NInput, NButton, useMessage } from 'naive-ui';
import { setup, errMsg } from '../api.js';
import LogoIcon from '../components/LogoIcon.vue';

const emit = defineEmits(['setup-done']);
const message = useMessage();
const username = ref('');
const password = ref('');
const confirm = ref('');
const loading = ref(false);

async function doSetup() {
  if (!username.value.trim() || !password.value) {
    message.warning('请输入用户名和密码');
    return;
  }
  if (password.value.length < 6) {
    message.warning('密码至少 6 位');
    return;
  }
  if (password.value !== confirm.value) {
    message.warning('两次输入的密码不一致');
    return;
  }
  loading.value = true;
  try {
    await setup(username.value.trim(), password.value);
    message.success('初始化成功');
    emit('setup-done');
  } catch (err) {
    message.error(errMsg(err));
  } finally {
    loading.value = false;
  }
}
</script>

<style scoped>
.login-wrap {
  height: 100%;
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
</style>

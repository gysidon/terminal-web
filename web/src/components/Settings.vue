<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    title="系统设置"
    style="width: 640px; max-width: 92vw"
    :bordered="false"
    @after-enter="onOpen"
  >
    <n-tabs type="line" animated>
      <!-- 安全 -->
      <n-tab-pane name="security" tab="安全">
        <n-form label-placement="left" :label-width="130">
          <n-form-item label="登录验证码">
            <n-switch v-model:value="form.captchaEnabled">
              <template #checked>开启</template>
              <template #unchecked>关闭</template>
            </n-switch>
            <span class="hint">关闭后登录不再要求输入验证码</span>
          </n-form-item>
          <n-form-item label="失败锁定次数">
            <n-input-number v-model:value="form.loginFailMax" :min="1" :max="20" />
            <span class="hint">连续失败达到此次数后锁定账户</span>
          </n-form-item>
          <n-form-item label="锁定时间(分钟)">
            <n-input-number v-model:value="form.loginLockMinutes" :min="1" :max="1440" />
            <span class="hint">锁定后需等待该时长才能再次尝试</span>
          </n-form-item>
          <n-form-item label="IP 白名单">
            <n-input
              v-model:value="form.ipWhitelist"
              type="textarea"
              :autosize="{ minRows: 2, maxRows: 4 }"
              placeholder="留空=不限制。支持逗号分隔：192.168.1.100, 10.0.0.0/24, 172.16. (前缀)"
            />
          </n-form-item>
        </n-form>
      </n-tab-pane>

      <!-- 外观 -->
      <n-tab-pane name="appearance" tab="外观">
        <n-form label-placement="left" :label-width="130">
          <n-form-item label="主题">
            <n-radio-group v-model:value="form.theme">
              <n-radio value="dark">暗色</n-radio>
              <n-radio value="light">亮色</n-radio>
            </n-radio-group>
          </n-form-item>
          <n-form-item label="终端字体大小">
            <n-slider v-model:value="form.fontSize" :min="10" :max="24" style="width: 220px" />
            <span class="hint">{{ form.fontSize }} px</span>
          </n-form-item>
          <n-form-item label="终端字体">
            <n-select v-model:value="form.fontFamily" :options="fontOptions" style="width: 320px" />
          </n-form-item>
        </n-form>
      </n-tab-pane>

      <!-- 审计日志 -->
      <n-tab-pane name="audit" tab="登录日志">
        <div class="audit-head">
          <span class="hint">最近 {{ audit.length }} 条登录记录</span>
          <n-button size="small" secondary @click="loadAudit">刷新</n-button>
        </div>
        <n-data-table
          size="small"
          :columns="auditColumns"
          :data="audit"
          :bordered="false"
          :max-height="320"
        />
      </n-tab-pane>
    </n-tabs>

    <template #footer>
      <div style="display: flex; justify-content: flex-end; gap: 12px">
        <n-button @click="visible = false">取消</n-button>
        <n-button type="primary" :loading="saving" @click="save">保存</n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup>
import { ref, h, computed } from 'vue';
import {
  NModal, NTabs, NTabPane, NForm, NFormItem, NSwitch, NInputNumber, NInput,
  NRadioGroup, NRadio, NSlider, NSelect, NButton, NDataTable, useMessage
} from 'naive-ui';
import { store, saveSettings } from '../store.js';
import { api, errMsg } from '../api.js';
import { TerminalOutline } from '@vicons/ionicons5';

const emit = defineEmits(['update:show']);
const message = useMessage();

const props = defineProps({ show: { type: Boolean, default: false } });

const visible = computed({
  get: () => props.show,
  set: (v) => emit('update:show', v)
});

const form = ref({
  captchaEnabled: true,
  loginFailMax: 5,
  loginLockMinutes: 15,
  ipWhitelist: '',
  theme: 'dark',
  fontSize: 13,
  fontFamily: ''
});

const fontOptions = [
  { label: 'JetBrains Mono（推荐）', value: '"JetBrains Mono", Menlo, Consolas, "Courier New", monospace' },
  { label: '系统等宽（Menlo/Consolas）', value: 'Menlo, Monaco, Consolas, "Courier New", monospace' },
  { label: 'Fira Code', value: '"Fira Code", monospace' },
  { label: 'Courier New', value: '"Courier New", monospace' }
];

const audit = ref([]);
const auditColumns = [
  { title: '时间', key: 'created_at', width: 150 },
  {
    title: '结果', key: 'success', width: 70,
    render: (r) => h('span', { style: `color:${r.success ? 'var(--accent)' : '#e24b4a'}` }, r.success ? '成功' : '失败')
  },
  { title: '用户名', key: 'username', width: 110 },
  { title: 'IP', key: 'ip', width: 140 },
  { title: '详情', key: 'detail' }
];

async function loadAudit() {
  try {
    const { data } = await api.get('/api/audit', { params: { limit: 100 } });
    audit.value = data;
  } catch (e) {
    message.error(errMsg(e));
  }
}

function onOpen() {
  form.value = {
    captchaEnabled: store.captchaEnabled,
    loginFailMax: store.loginFailMax,
    loginLockMinutes: store.loginLockMinutes,
    ipWhitelist: store.ipWhitelist,
    theme: store.theme,
    fontSize: store.fontSize,
    fontFamily: store.fontFamily
  };
  loadAudit();
}

const saving = ref(false);
async function save() {
  saving.value = true;
  try {
    await saveSettings({
      captcha_enabled: form.value.captchaEnabled,
      login_fail_max: form.value.loginFailMax,
      login_lock_minutes: form.value.loginLockMinutes,
      ip_whitelist: form.value.ipWhitelist,
      theme: form.value.theme,
      font_size: form.value.fontSize,
      font_family: form.value.fontFamily
    });
    message.success('设置已保存');
    visible.value = false;
  } catch (e) {
    message.error(errMsg(e));
  } finally {
    saving.value = false;
  }
}
</script>

<style scoped>
.hint {
  color: var(--text-sub);
  font-size: 12px;
  margin-left: 10px;
}
.audit-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
</style>

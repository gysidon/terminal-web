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
          <n-form-item label="登录超时(分钟)">
            <n-input-number v-model:value="form.sessionTimeout" :min="0" :max="1440" />
            <span class="hint">无操作自动退出登录，0 = 不限制</span>
          </n-form-item>
          <n-form-item label="单点登录">
            <n-switch v-model:value="form.ssoEnabled">
              <template #checked>开启</template>
              <template #unchecked>关闭</template>
            </n-switch>
            <span class="hint">开启后新登录会使旧登录失效，需重新登录</span>
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

      <!-- 备份与恢复 -->
      <n-tab-pane name="backup" tab="备份与恢复">
        <n-alert type="warning" :show-icon="true" style="margin-bottom: 12px">
          备份文件包含可还原的 SSH 凭据（与主密钥一同打包），等同于明文凭据，请妥善保管、勿外传。
        </n-alert>
        <div class="backup-actions">
          <n-button type="primary" :loading="creating" @click="onCreate">创建备份</n-button>
          <n-button :loading="importing" @click="fileInput?.click()">导入外部备份</n-button>
          <input
            ref="fileInput"
            type="file"
            accept=".json,application/json"
            style="display: none"
            @change="onFilePicked"
          />
        </div>
        <n-data-table
          size="small"
          :columns="backupColumns"
          :data="backups"
          :bordered="false"
          :max-height="320"
          style="margin-top: 12px"
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
  NRadioGroup, NRadio, NSlider, NSelect, NButton, NDataTable, NAlert, useMessage
} from 'naive-ui';
import { store, saveSettings } from '../store.js';
import {
  api, errMsg,
  createBackup as createBackupApi, listBackups,
  downloadBackup, deleteBackup, importBackup, restoreBackup
} from '../api.js';

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
  sessionTimeout: 0,
  ssoEnabled: false,
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

// ===== 备份与恢复 =====
const backups = ref([]);
const creating = ref(false);
const importing = ref(false);
const fileInput = ref(null);

function formatSize(bytes) {
  if (!bytes) return '0 B';
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

const backupColumns = [
  { title: '文件名', key: 'filename' },
  { title: '创建时间', key: 'created_at', width: 165 },
  { title: '大小', key: 'size', width: 90, render: (r) => formatSize(r.size) },
  {
    title: '操作', key: 'actions', width: 210,
    render: (r) => h('div', { style: 'display:flex; gap:8px' }, [
      h(NButton, { size: 'small', secondary: true, onClick: () => onDownload(r) }, { default: () => '下载' }),
      h(NButton, { size: 'small', secondary: true, type: 'warning', onClick: () => onRestore(r) }, { default: () => '恢复' }),
      h(NButton, { size: 'small', secondary: true, type: 'error', onClick: () => onDelete(r) }, { default: () => '删除' })
    ])
  }
];

async function loadBackups() {
  try {
    const { data } = await listBackups();
    backups.value = data;
  } catch (e) {
    message.error(errMsg(e));
  }
}

async function onCreate() {
  creating.value = true;
  try {
    await createBackupApi();
    message.success('备份已创建');
    loadBackups();
  } catch (e) {
    message.error(errMsg(e));
  } finally {
    creating.value = false;
  }
}

function onFilePicked(e) {
  const file = e.target.files && e.target.files[0];
  if (!file) return;
  e.target.value = ''; // 允许重复选择同一文件
  if (!window.confirm('导入将清空现有连接与设置并替换为备份内容，且不可撤销，确认继续？')) return;
  importing.value = true;
  importBackup(file)
    .then(() => { message.success('导入成功，连接与设置已恢复'); loadBackups(); })
    .catch((err) => message.error(errMsg(err)))
    .finally(() => { importing.value = false; });
}

async function onDownload(r) {
  try {
    await downloadBackup(r.id, r.filename);
  } catch (e) {
    message.error(errMsg(e));
  }
}

async function onRestore(r) {
  if (!window.confirm(`将从备份「${r.filename}」恢复数据，现有连接与设置将被覆盖，确认？`)) return;
  try {
    await restoreBackup(r.id);
    message.success('恢复成功');
    loadBackups();
  } catch (e) {
    message.error(errMsg(e));
  }
}

async function onDelete(r) {
  if (!window.confirm(`确认删除备份「${r.filename}」？`)) return;
  try {
    await deleteBackup(r.id);
    message.success('已删除');
    loadBackups();
  } catch (e) {
    message.error(errMsg(e));
  }
}

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
    sessionTimeout: store.sessionTimeout,
    ssoEnabled: store.ssoEnabled,
    theme: store.theme,
    fontSize: store.fontSize,
    fontFamily: store.fontFamily
  };
  loadAudit();
  loadBackups();
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
      session_timeout: form.value.sessionTimeout,
      sso_enabled: form.value.ssoEnabled,
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
.backup-actions {
  display: flex;
  gap: 12px;
  align-items: center;
}
</style>

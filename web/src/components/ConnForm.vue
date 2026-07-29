<template>
  <n-modal
    :show="show"
    preset="card"
    :title="editing ? '编辑连接' : '新建连接'"
    style="width: 560px"
    @update:show="(v) => $emit('update:show', v)"
  >
    <n-form label-placement="left" label-width="90">
      <n-form-item label="名称" required>
        <n-input v-model:value="form.name" placeholder="如：生产环境-web01" />
      </n-form-item>
      <n-form-item label="主机" required>
        <n-input-group>
          <n-input v-model:value="form.host" placeholder="IP 或域名" style="flex: 1" />
          <n-button style="width: 34px" @click="stepPort(-1)">−</n-button>
          <n-input-number
            v-model:value="form.port"
            :min="1"
            :max="65535"
            :show-button="false"
            placeholder="22"
            style="width: 90px"
          />
          <n-button style="width: 34px" @click="stepPort(1)">+</n-button>
        </n-input-group>
      </n-form-item>
      <n-form-item label="用户名" required>
        <n-input v-model:value="form.username" placeholder="如 root" />
      </n-form-item>
      <n-form-item label="认证方式">
        <n-radio-group v-model:value="form.auth_type">
          <n-radio-button value="password">密码</n-radio-button>
          <n-radio-button value="key">私钥证书</n-radio-button>
        </n-radio-group>
      </n-form-item>
      <n-form-item v-if="form.auth_type === 'password'" label="密码">
        <n-input v-model:value="form.password" type="password" show-password-on="click"
          :placeholder="editing?.has_password ? '留空则不修改' : '请输入密码'" />
      </n-form-item>
      <template v-else>
        <n-form-item label="私钥">
          <div style="width: 100%">
            <n-input v-model:value="form.private_key" type="textarea" :rows="4"
              :placeholder="editing?.has_private_key ? '留空则不修改' : '粘贴私钥内容（-----BEGIN ... KEY-----）'" />
            <n-space size="small" align="center" style="margin-top: 8px">
              <input ref="keyInput" type="file" accept=".pem,.key,.ppk,.crt,.pub,text/*" style="display: none" @change="onKeyFile" />
              <n-button size="small" secondary @click="keyInput?.click()">选择私钥文件</n-button>
              <span v-if="keyFileName" style="font-size: 12px; color: #888">已载入：{{ keyFileName }}</span>
            </n-space>
          </div>
        </n-form-item>
        <n-form-item label="私钥口令">
          <n-input v-model:value="form.passphrase" type="password" show-password-on="click"
            :placeholder="editing?.has_passphrase ? '留空则不修改' : '私钥有 passphrase 才需填写'" />
        </n-form-item>
      </template>
      <n-form-item label="所属文件夹">
        <n-select v-model:value="form.folder_id" :options="folderOptions" clearable placeholder="不选则放在根目录" />
      </n-form-item>
      <n-form-item label="跳板机">
        <n-tree-select
          v-model:value="form.jump_id"
          :options="jumpTreeOptions"
          key-field="value"
          label-field="label"
          children-field="children"
          :render-label="renderJumpLabel"
          :default-expand-all="true"
          clearable
          placeholder="可选，先经跳板再连目标（支持多级）"
          style="width: 100%"
        />
      </n-form-item>
      <n-form-item label="备注">
        <n-input v-model:value="form.remark" placeholder="可选" />
      </n-form-item>
    </n-form>
    <template #footer>
      <n-space justify="space-between">
        <n-button :loading="testing" secondary @click="testConn">
          <template #icon><PulseOutline /></template>
          测试连接
        </n-button>
        <n-space>
          <n-button @click="$emit('update:show', false)">取消</n-button>
          <n-button type="primary" :loading="saving" @click="save">保存</n-button>
        </n-space>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup>
import { ref, computed, watch, h } from 'vue';
import {
  NModal, NForm, NFormItem, NInput, NInputNumber, NInputGroup, NRadioGroup,
  NRadioButton, NSelect, NTreeSelect, NIcon, NButton, NSpace, useMessage
} from 'naive-ui';
import { api, errMsg, testConnConfig } from '../api.js';
import { PulseOutline, FolderOutline, GlobeOutline } from '@vicons/ionicons5';

const props = defineProps({
  show: Boolean,
  editing: Object,
  prefill: Object,
  folders: { type: Array, default: () => [] },
  connections: { type: Array, default: () => [] },
  defaultFolderId: Number
});
const emit = defineEmits(['update:show', 'saved']);
const message = useMessage();
const saving = ref(false);
const testing = ref(false);
const keyInput = ref(null);
const keyFileName = ref('');
const copyFromId = ref(null);

function onKeyFile(e) {
  const f = e.target.files?.[0];
  if (!f) return;
  const reader = new FileReader();
  reader.onload = () => {
    form.value.private_key = String(reader.result);
    keyFileName.value = f.name;
  };
  reader.readAsText(f);
  e.target.value = '';
}

function stepPort(delta) {
  const cur = Number(form.value.port) || 22;
  let next = cur + delta;
  if (next < 1) next = 1;
  if (next > 65535) next = 65535;
  form.value.port = next;
}

const emptyForm = () => ({
  name: '', host: '', port: 22, username: 'root', auth_type: 'password',
  password: '', private_key: '', passphrase: '', folder_id: null, jump_id: null, remark: ''
});
const form = ref(emptyForm());

watch(
  () => props.show,
  (v) => {
    if (!v) return;
    if (props.editing) {
      const e = props.editing;
      copyFromId.value = null;
      form.value = {
        name: e.name, host: e.host, port: e.port, username: e.username,
        auth_type: e.auth_type, password: '', private_key: '', passphrase: '',
        folder_id: e.folder_id, jump_id: e.jump_id, remark: e.remark || ''
      };
    } else if (props.prefill) {
      // 复制节点：把源连接的配置预填到表单，凭据留空由服务端 copy_from 带过
      const p = props.prefill;
      copyFromId.value = p.id;
      form.value = {
        name: `${p.name}-副本`, host: p.host, port: p.port, username: p.username,
        auth_type: p.auth_type, password: '', private_key: '', passphrase: '',
        folder_id: p.folder_id, jump_id: p.jump_id, remark: p.remark || ''
      };
    } else {
      copyFromId.value = null;
      form.value = emptyForm();
      form.value.folder_id = props.defaultFolderId ?? null;
    }
  }
);

function folderLabel(f, map) {
  const parts = [f.name];
  let cur = f;
  while (cur.parent_id && map.has(cur.parent_id)) {
    cur = map.get(cur.parent_id);
    parts.unshift(cur.name);
  }
  return parts.join(' / ');
}

const folderOptions = computed(() => {
  const map = new Map(props.folders.map((f) => [f.id, f]));
  return props.folders.map((f) => ({ label: folderLabel(f, map), value: f.id }));
});

// 跳板机树形选择：文件夹为不可选父节点，连接为叶子，层级与左侧菜单一致
const jumpTreeOptions = computed(() => {
  const folderMap = new Map();
  props.folders.forEach((f) => {
    folderMap.set(f.id, { value: `f-${f.id}`, label: f.name, isFolder: true, selectable: false, children: [] });
  });
  const roots = [];
  props.folders.forEach((f) => {
    const node = folderMap.get(f.id);
    if (f.parent_id && folderMap.has(f.parent_id)) folderMap.get(f.parent_id).children.push(node);
    else roots.push(node);
  });
  props.connections
    .filter((c) => !props.editing || c.id !== props.editing.id)
    .forEach((c) => {
      const node = { value: c.id, label: c.name, isFolder: false, host: c.host };
      if (c.folder_id && folderMap.has(c.folder_id)) folderMap.get(c.folder_id).children.push(node);
      else roots.push(node);
    });
  return roots;
});

function renderJumpLabel({ option }) {
  const icon = h(
    NIcon,
    { size: 16, style: `margin-right:6px;color:${option.isFolder ? '#e0b063' : '#8a8a93'}` },
    () => h(option.isFolder ? FolderOutline : GlobeOutline)
  );
  const text = option.isFolder
    ? option.label
    : [
        option.label,
        option.host ? h('span', { style: 'color:#8a8f99;margin-left:6px;font-size:12px' }, `(${option.host})`) : null
      ];
  return h('span', { style: 'display:flex;align-items:center' }, [icon, ...(Array.isArray(text) ? text : [text])]);
}

async function save() {
  if (!form.value.name.trim() || !form.value.host.trim() || !form.value.username.trim()) {
    message.warning('请填写名称、主机和用户名');
    return;
  }
  saving.value = true;
  try {
    const body = { ...form.value };
    if (props.editing) {
      if (!body.password) delete body.password;
      if (!body.private_key) delete body.private_key;
      if (!body.passphrase) delete body.passphrase;
      await api.put(`/api/connections/${props.editing.id}`, body);
    } else {
      if (copyFromId.value) body.copy_from = copyFromId.value;
      await api.post('/api/connections', body);
    }
    message.success('已保存');
    emit('update:show', false);
    emit('saved');
  } catch (err) {
    message.error(errMsg(err));
  } finally {
    saving.value = false;
  }
}

// 用表单当前填写的值（可能尚未保存）测试真实连接
async function testConn() {
  if (!form.value.host.trim() || !form.value.username.trim()) {
    message.warning('请先填写主机和用户名');
    return;
  }
  testing.value = true;
  try {
    await testConnConfig({
      id: props.editing?.id ?? null,
      host: form.value.host,
      port: form.value.port || 22,
      username: form.value.username,
      auth_type: form.value.auth_type,
      password: form.value.password,
      private_key: form.value.private_key,
      passphrase: form.value.passphrase,
      jump_id: form.value.jump_id ?? null
    });
    message.success('连接成功');
  } catch (err) {
    message.error(errMsg(err));
  } finally {
    testing.value = false;
  }
}
</script>

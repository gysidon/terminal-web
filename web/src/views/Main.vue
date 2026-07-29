<template>
  <div class="layout">
    <div class="sidebar" :class="{ collapsed }">
      <div class="side-header">
        <div class="logo-line">
          <LogoIcon :size="20" />
          <span class="logo-text">Terminal Web</span>
        </div>
        <n-space size="small">
          <n-tooltip><template #trigger>
            <n-button quaternary circle size="small" @click="openConnForm(null)">
              <template #icon><n-icon><AddOutline /></n-icon></template>
            </n-button>
          </template>新建连接</n-tooltip>
          <n-tooltip><template #trigger>
            <n-button quaternary circle size="small" @click="createFolder(null)">
              <template #icon><n-icon><FolderOpenOutline /></n-icon></template>
            </n-button>
          </template>新建文件夹</n-tooltip>
          <n-tooltip><template #trigger>
            <n-button quaternary circle size="small" @click="loadAll">
              <template #icon><n-icon><RefreshOutline /></n-icon></template>
            </n-button>
          </template>刷新</n-tooltip>
          <n-tooltip><template #trigger>
            <n-button quaternary circle size="small" @click="collapsed = true">
              <template #icon><n-icon><ChevronBackOutline /></n-icon></template>
            </n-button>
          </template>收起侧边栏</n-tooltip>
        </n-space>
      </div>
      <div class="side-search">
        <n-input v-model:value="keyword" size="small" placeholder="搜索名称 / 主机 / 用户" clearable>
          <template #prefix><n-icon><SearchOutline /></n-icon></template>
        </n-input>
      </div>
      <div class="side-tree">
        <n-tree
          v-if="treeData.length"
          block-line
          :data="treeData"
          :expanded-keys="expandedKeys"
          :node-props="nodeProps"
          @update:expanded-keys="(k) => (expandedKeys = k)"
        />
        <div v-else class="tree-empty">
          <div class="tree-empty-ic"><n-icon size="30"><ServerOutline /></n-icon></div>
          <div class="tree-empty-title">还没有连接</div>
          <div class="tree-empty-sub">创建第一个 SSH 连接开始使用</div>
          <n-button size="small" type="primary" @click="openConnForm(null)">+ 新建连接</n-button>
        </div>
      </div>
      <div class="side-footer">
        <n-button quaternary size="small" @click="showSettings = true">
          <template #icon><n-icon><SettingsOutline /></n-icon></template>设置
        </n-button>
        <n-button quaternary size="small" @click="showPwd = true">
          <template #icon><n-icon><KeyOutline /></n-icon></template>修改密码
        </n-button>
        <n-button quaternary size="small" @click="$emit('logout')">
          <template #icon><n-icon><LogOutOutline /></n-icon></template>退出
        </n-button>
      </div>
    </div>

    <n-button v-if="collapsed" circle class="expand-btn" @click="collapsed = false">
      <template #icon><n-icon><MenuOutline /></n-icon></template>
    </n-button>

    <div class="content" :class="{ 'collapsed-gutter': collapsed }">
      <div v-if="!sessions.length" class="welcome">
        <div class="welcome-logo"><LogoIcon :size="48" /></div>
        <div class="welcome-title">欢迎使用 Terminal Web</div>
        <div class="welcome-sub">在左侧双击一个连接，即可开始 SSH 会话</div>
      </div>
      <n-tabs
        v-else
        v-model:value="activeTab"
        type="card"
        closable
        class="term-tabs"
        @close="closeSession"
      >
        <n-tab-pane
          v-for="s in sessions"
          :key="s.id"
          :name="s.id"
          :tab="() => renderTab(s)"
          display-directive="show"
        >
          <template v-if="s.type === 'sftp'">
            <SftpPane :conn-id="s.connId" :conn-name="s.name" @open-terminal="onOpenTerminalFromSftp" />
          </template>
          <template v-else-if="s.type === 'process'">
            <ProcessPane :conn-id="s.connId" :conn-name="s.name" />
          </template>
          <template v-else>
            <div class="pane-toolbar">
              <n-space size="small" align="center" class="pane-left" :wrap="true">
                <n-tag size="small" :type="s.status === 'connected' ? 'success' : s.status === 'error' ? 'error' : 'warning'" round>
                  {{ statusText(s.status) }}
                </n-tag>
                <span class="pane-host">{{ s.hostLabel }}</span>
                <div class="stat-bars" v-if="barsFor(s.connId)">
                  <div class="stat-bar" v-for="b in barsFor(s.connId)" :key="b.label">
                    <span class="stat-bar-label">{{ b.label }}</span>
                    <n-progress type="line" :percentage="ringVal(b.value)" :color="ringColor(b.value)" :height="6" :show-indicator="false" class="stat-bar-track" />
                    <span class="stat-bar-val">{{ b.text }}</span>
                  </div>
                </div>
                <span v-else class="stat-loading">统计加载中…</span>
              </n-space>
              <n-space size="small">
                <n-button size="small" secondary @click="openSftp({ id: s.connId }, s.name)">
                  <template #icon><n-icon><FolderOutline /></n-icon></template>文件管理
                </n-button>
                <n-button size="small" secondary @click="openProcess({ id: s.connId }, s.name)">
                  <template #icon><n-icon><BarChartOutline /></n-icon></template>进程管理
                </n-button>
                <n-button size="small" secondary @click="reconnect(s)">
                  <template #icon><n-icon><RefreshOutline /></n-icon></template>重连
                </n-button>
              </n-space>
            </div>
            <TermPane :ref="(el) => setPaneRef(s.id, el)" :conn-id="s.connId" :session-id="s.id" :init-dir="s.initDir" :active="activeTab === s.id" @status="(st) => (s.status = st)" />
          </template>
        </n-tab-pane>
      </n-tabs>
    </div>

    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="ctxShow"
      :x="ctxX"
      :y="ctxY"
      :options="ctxOptions"
      @select="onCtxSelect"
      @clickoutside="ctxShow = false"
    />

    <n-dropdown
      trigger="manual"
      placement="bottom-start"
      :show="tabCtxShow"
      :x="tabCtxX"
      :y="tabCtxY"
      :options="tabCtxOptions"
      @select="onTabCtxSelect"
      @clickoutside="tabCtxShow = false"
    />

    <ConnForm
      v-model:show="showConnForm"
      :editing="editingConn"
      :prefill="copySource"
      :folders="folders"
      :connections="connections"
      :default-folder-id="formDefaultFolderId"
      @saved="loadAll"
    />

    <Settings v-model:show="showSettings" />

    <n-modal v-model:show="showPwd" preset="dialog" title="修改密码" positive-text="确定" negative-text="取消" @positive-click="doChangePwd">
      <n-form>
        <n-form-item label="原密码"><n-input v-model:value="pwdOld" type="password" show-password-on="click" /></n-form-item>
        <n-form-item label="新密码（至少6位）"><n-input v-model:value="pwdNew" type="password" show-password-on="click" /></n-form-item>
      </n-form>
    </n-modal>

    <n-modal v-model:show="termModalShow" preset="card" title="终端" :bordered="false" style="width: 82vw; max-width: 1100px">
      <div class="term-modal-body">
        <TermPane
          v-if="termModalShow && termModalConnId != null"
          :conn-id="termModalConnId"
          :session-id="termModalId"
          :init-dir="termModalPath"
          :active="true"
        />
      </div>
    </n-modal>
  </div>
</template>

<script setup>
import { ref, computed, h, onMounted, onUnmounted, watch, nextTick } from 'vue';
import {
  NButton, NIcon, NInput, NTree, NTabs, NTabPane, NSpace, NTag, NDropdown,
  NTooltip, NModal, NForm, NFormItem, NProgress, useMessage, useDialog
} from 'naive-ui';
import {
  AddOutline, FolderOpenOutline, RefreshOutline, SearchOutline, KeyOutline,
  LogOutOutline, FolderOutline, ServerOutline, TerminalOutline,
  ChevronBackOutline, MenuOutline, SettingsOutline,
  PulseOutline, PencilOutline, TrashOutline, CreateOutline, BarChartOutline, CopyOutline,
  GlobeOutline, CloseOutline, ArrowBackOutline, ArrowForwardOutline
} from '@vicons/ionicons5';
import { api, errMsg, pingConn, sysInfo } from '../api.js';
import ConnForm from '../components/ConnForm.vue';
import TermPane from '../components/TermPane.vue';
import SftpPane from '../components/SftpPane.vue';
import ProcessPane from '../components/ProcessPane.vue';
import Settings from '../components/Settings.vue';
import LogoIcon from '../components/LogoIcon.vue';
import { loadSettings } from '../store.js';

defineEmits(['logout']);
const message = useMessage();
const dialog = useDialog();

const folders = ref([]);
const connections = ref([]);
const keyword = ref('');
const expandedKeys = ref([]);

const collapsed = ref(localStorage.getItem('tw_sidebar_collapsed') === '1');

const sessions = ref([]);
const activeTab = ref(null);
let sessionSeq = 0;
const paneRefs = new Map();

const showConnForm = ref(false);
const editingConn = ref(null);
const copySource = ref(null);
const formDefaultFolderId = ref(null);

const showSettings = ref(false);
const showPwd = ref(false);
const pwdOld = ref('');
const pwdNew = ref('');

// 文件管理「在终端打开」弹窗内的终端
const termModalShow = ref(false);
const termModalConnId = ref(null);
const termModalPath = ref(null);
const termModalId = ref(null);

const ctxShow = ref(false);
const ctxX = ref(0);
const ctxY = ref(0);
const ctxOptions = ref([]);
let ctxNode = null;

// 标签页右键菜单
const tabCtxShow = ref(false);
const tabCtxX = ref(0);
const tabCtxY = ref(0);
const tabCtxTargetId = ref(null);

// 连接延迟探测（左侧列表实时显示）
const latencyMap = ref({}); // connId -> ms
let latencyTimer = null;

async function fetchLatency(connId) {
  try {
    const { latency } = await pingConn(connId);
    latencyMap.value = { ...latencyMap.value, [connId]: Math.round(latency) };
  } catch {
    latencyMap.value = { ...latencyMap.value, [connId]: null };
  }
}
function refreshAllLatency() {
  connections.value.forEach((c) => fetchLatency(c.id));
}

// 终端顶部系统状态（CPU/内存/负载/各分区）轮询 —— 只刷新当前激活 tab 对应的连接
const sysInfoMap = ref({}); // connId -> { cpu, mem, load1, cores, disks:[...] }
let sysInfoTimer = null;

async function refreshActiveSysInfo() {
  const s = sessions.value.find((x) => x.id === activeTab.value);
  if (!s || s.type !== 'terminal' || s.status !== 'connected') return;
  try {
    const data = await sysInfo(s.connId);
    sysInfoMap.value = { ...sysInfoMap.value, [s.connId]: data };
  } catch {
    // 失败保持上一次的值
  }
}
function startSysInfoPolling() {
  clearInterval(sysInfoTimer);
  refreshActiveSysInfo(); // 立即拉一次
  sysInfoTimer = setInterval(refreshActiveSysInfo, 5000);
}
function barsFor(connId) {
  const i = sysInfoMap.value[connId];
  if (!i) return null;
  const arr = [
    { label: '负载', value: loadPct(connId), text: fmtLoad(i.load1) },
    { label: 'CPU', value: i.cpu, text: fmtRing(i.cpu) },
    { label: '内存', value: i.mem, text: fmtRing(i.mem) }
  ];
  (i.disks || [])
    .filter((d) => !d.mount.startsWith('/boot'))
    .forEach((d) => {
      arr.push({ label: shortMount(d.mount), value: d.percent, text: d.percent == null ? '—' : d.percent + '%' });
    });
  return arr;
}

function ringVal(v) {
  return v == null ? 0 : Math.max(0, Math.min(100, v));
}
function ringColor(v) {
  if (v == null) return '#8a8a93';
  if (v < 60) return '#63e2b7';
  if (v < 85) return '#e0b063';
  return '#f87171';
}
function fmtRing(v) {
  return v == null ? '—' : Math.round(v) + '%';
}
function fmtLoad(v) {
  return v == null ? '—' : Number(v).toFixed(2);
}
function loadPct(connId) {
  const i = sysInfoMap.value[connId];
  if (!i || i.load1 == null) return 0;
  const cores = i.cores || 1;
  return Math.max(0, Math.min(100, Math.round((i.load1 / cores) * 100)));
}
function shortMount(m) {
  if (m === '/' || !m) return '/';
  const seg = m.replace(/^\//, '').split('/');
  return seg[0] || '/';
}

// 当前已在 tab 中打开的连接集合（用于连接树图标着色：打开=绿，未打开=灰）
const openedConnIds = computed(() => new Set(sessions.value.map((s) => s.connId)));

function renderIcon(icon, color) {
  return () => h(NIcon, { color, size: 16 }, { default: () => h(icon) });
}

function renderTab(s) {
  let icon = TerminalOutline;
  let color = '#63e2b7';
  if (s.type === 'sftp') { icon = FolderOutline; color = '#e0b063'; }
  else if (s.type === 'process') { icon = BarChartOutline; color = '#a78bfa'; }
  return h(
    'span',
    { style: 'display:inline-flex;align-items:center;gap:6px;width:100%', onContextmenu: (e) => onTabCtx(e, s.id) },
    [
      h(NIcon, { size: 14, color }, { default: () => h(icon) }),
      h('span', s.name)
    ]
  );
}

const treeData = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  const conns = kw
    ? connections.value.filter(
        (c) =>
          c.name.toLowerCase().includes(kw) ||
          c.host.toLowerCase().includes(kw) ||
          c.username.toLowerCase().includes(kw)
      )
    : connections.value;

  const connNode = (c) => {
    const lat = latencyMap.value[c.id];
    const opened = openedConnIds.value.has(c.id);
    return {
      key: `c-${c.id}`,
      label: `${c.name}`,
      raw: c,
      isLeaf: true,
      prefix: renderIcon(GlobeOutline, opened ? '#63e2b7' : '#8a8a93'),
      suffix: () => {
        return lat == null
          ? h('span', { style: 'color:#666;font-size:12px' }, '—')
          : h(
              'span',
              {
                style: `color:${lat < 100 ? '#63e2b7' : lat < 300 ? '#e0b063' : '#f87171'};font-size:12px;font-variant-numeric:tabular-nums`
              },
              `${lat}ms`
            );
      }
    };
  };

  if (kw) return conns.map(connNode);

  const folderMap = new Map();
  folders.value.forEach((f) => folderMap.set(f.id, { key: `f-${f.id}`, label: f.name, prefix: renderIcon(FolderOutline, '#e0b063'), raw: f, children: [] }));
  const roots = [];
  folders.value.forEach((f) => {
    const node = folderMap.get(f.id);
    if (f.parent_id && folderMap.has(f.parent_id)) folderMap.get(f.parent_id).children.push(node);
    else roots.push(node);
  });
  conns.forEach((c) => {
    const node = connNode(c);
    if (c.folder_id && folderMap.has(c.folder_id)) folderMap.get(c.folder_id).children.push(node);
    else roots.push(node);
  });
  return roots;
});

function nodeProps({ option }) {
  return {
    onDblclick() {
      if (option.raw && option.isLeaf) openSession(option.raw);
    },
    onContextmenu(e) {
      e.preventDefault();
      ctxNode = option;
      ctxOptions.value = option.isLeaf
        ? [
            { label: '打开连接', key: 'connect', icon: renderIcon(TerminalOutline) },
            { label: '文件管理', key: 'sftp', icon: renderIcon(FolderOutline) },
            { label: '进程管理', key: 'process', icon: renderIcon(BarChartOutline) },
            { label: '连通测试', key: 'test', icon: renderIcon(PulseOutline) },
            { type: 'divider', key: 'd1' },
            { label: '复制节点', key: 'copy', icon: renderIcon(CopyOutline) },
            { label: '编辑节点', key: 'edit', icon: renderIcon(PencilOutline) },
            { label: '删除节点', key: 'del', icon: renderIcon(TrashOutline) }
          ]
        : [
            { label: '新建连接', key: 'newConn', icon: renderIcon(AddOutline) },
            { label: '新建子文件夹', key: 'newFolder', icon: renderIcon(FolderOutline) },
            { label: '重命名', key: 'rename', icon: renderIcon(CreateOutline) },
            { type: 'divider', key: 'd1' },
            { label: '删除文件夹', key: 'delFolder', icon: renderIcon(TrashOutline) }
          ];
      ctxShow.value = true;
      ctxX.value = e.clientX;
      ctxY.value = e.clientY;
    }
  };
}

async function onCtxSelect(key) {
  ctxShow.value = false;
  const raw = ctxNode?.raw;
  if (!raw) return;
  if (key === 'connect') openSession(raw);
  else if (key === 'sftp') openSftp(raw);
  else if (key === 'process') openProcess(raw);
  else if (key === 'test') {
    const msgRef = message.loading('正在测试连接...', { duration: 0 });
    try {
      await api.post(`/api/connections/${raw.id}/test`);
      msgRef.destroy();
      message.success('连接成功');
    } catch (err) {
      msgRef.destroy();
      message.error(errMsg(err));
    }
  } else if (key === 'copy') {
    copySource.value = raw;
    editingConn.value = null;
    showConnForm.value = true;
  } else if (key === 'edit') openConnForm(raw);
  else if (key === 'del') {
    dialog.warning({
      title: '删除连接',
      content: `确定删除连接「${raw.name}」吗？`,
      positiveText: '删除',
      negativeText: '取消',
      onPositiveClick: async () => {
        await api.delete(`/api/connections/${raw.id}`);
        message.success('已删除');
        loadAll();
      }
    });
  } else if (key === 'newConn') { formDefaultFolderId.value = raw.id; openConnForm(null, true); }
  else if (key === 'newFolder') createFolder(raw.id);
  else if (key === 'rename') renameFolder(raw);
  else if (key === 'delFolder') {
    dialog.warning({
      title: '删除文件夹',
      content: `确定删除文件夹「${raw.name}」吗？子文件夹将一并删除，其中的连接会移到根目录。`,
      positiveText: '删除',
      negativeText: '取消',
      onPositiveClick: async () => {
        await api.delete(`/api/folders/${raw.id}`);
        message.success('已删除');
        loadAll();
      }
    });
  }
}

function createFolder(parentId) {
  let name = '';
  dialog.create({
    title: parentId ? '新建子文件夹' : '新建文件夹',
    content: () =>
      h(NInput, {
        placeholder: '文件夹名称',
        onUpdateValue: (v) => (name = v)
      }),
    positiveText: '创建',
    negativeText: '取消',
    onPositiveClick: async () => {
      if (!name.trim()) { message.warning('请输入名称'); return false; }
      await api.post('/api/folders', { name, parent_id: parentId });
      message.success('已创建');
      loadAll();
    }
  });
}

function renameFolder(folder) {
  let name = folder.name;
  dialog.create({
    title: '重命名文件夹',
    content: () =>
      h(NInput, {
        defaultValue: folder.name,
        onUpdateValue: (v) => (name = v)
      }),
    positiveText: '保存',
    negativeText: '取消',
    onPositiveClick: async () => {
      if (!name.trim()) return false;
      await api.put(`/api/folders/${folder.id}`, { name, parent_id: folder.parent_id });
      loadAll();
    }
  });
}

function openConnForm(conn, keepFolder = false) {
  editingConn.value = conn;
  copySource.value = null;
  if (!keepFolder) formDefaultFolderId.value = conn?.folder_id ?? null;
  showConnForm.value = true;
}

function openSession(conn, initDir) {
  const id = `s${++sessionSeq}`;
  sessions.value.push({
    id,
    connId: conn.id,
    name: conn.name,
    hostLabel: `${conn.username}@${conn.host}:${conn.port}`,
    status: 'connecting',
    type: 'terminal',
    initDir: initDir || null
  });
  activeTab.value = id;
}

// 文件管理里「在终端打开」：弹窗打开一个终端并进入目标目录（不新增 tab）
function onOpenTerminalFromSftp({ connId, path }) {
  termModalConnId.value = connId;
  termModalPath.value = path;
  termModalId.value = `modal-${Date.now()}`;
  termModalShow.value = true;
}

function closeSession(id) {
  const idx = sessions.value.findIndex((s) => s.id === id);
  if (idx >= 0) sessions.value.splice(idx, 1);
  paneRefs.delete(id);
  if (activeTab.value === id) activeTab.value = sessions.value[sessions.value.length - 1]?.id ?? null;
}

// 标签页右键：关闭 / 关闭左侧 / 关闭右侧
const tabCtxOptions = computed(() => {
  const id = tabCtxTargetId.value;
  if (!id) return [];
  const idx = sessions.value.findIndex((s) => s.id === id);
  if (idx < 0) return [];
  const opts = [{ label: '关闭', key: 'close', icon: renderIcon(CloseOutline) }];
  if (idx > 0) opts.push({ label: '关闭左侧', key: 'closeLeft', icon: renderIcon(ArrowBackOutline) });
  if (idx < sessions.value.length - 1) opts.push({ label: '关闭右侧', key: 'closeRight', icon: renderIcon(ArrowForwardOutline) });
  return opts;
});

function onTabCtx(e, id) {
  e.preventDefault();
  e.stopPropagation();
  tabCtxTargetId.value = id;
  tabCtxShow.value = true;
  tabCtxX.value = e.clientX;
  tabCtxY.value = e.clientY;
}

function removeSessions(ids) {
  if (!ids.length) return;
  ids.forEach((id) => paneRefs.delete(id));
  sessions.value = sessions.value.filter((s) => !ids.includes(s.id));
  if (!sessions.value.find((s) => s.id === activeTab.value)) {
    activeTab.value = sessions.value[sessions.value.length - 1]?.id ?? null;
  }
}

function onTabCtxSelect(key) {
  tabCtxShow.value = false;
  const id = tabCtxTargetId.value;
  if (!id) return;
  const idx = sessions.value.findIndex((s) => s.id === id);
  if (idx < 0) return;
  if (key === 'close') removeSessions([id]);
  else if (key === 'closeLeft') removeSessions(sessions.value.slice(0, idx).map((s) => s.id));
  else if (key === 'closeRight') removeSessions(sessions.value.slice(idx + 1).map((s) => s.id));
}

function setPaneRef(id, el) {
  if (el) paneRefs.set(id, el);
}

function reconnect(s) {
  paneRefs.get(s.id)?.reconnect();
}

function openSftp(conn, name) {
  const id = conn.id;
  const label = name || conn.name;
  const existing = sessions.value.find((s) => s.type === 'sftp' && s.connId === id);
  if (existing) {
    activeTab.value = existing.id;
    return;
  }
  const sid = `s${++sessionSeq}`;
  sessions.value.push({ id: sid, connId: id, name: label, type: 'sftp', status: 'connected' });
  activeTab.value = sid;
}

function openProcess(conn, name) {
  const id = conn.id;
  const label = name || conn.name;
  const existing = sessions.value.find((s) => s.type === 'process' && s.connId === id);
  if (existing) {
    activeTab.value = existing.id;
    return;
  }
  const sid = `s${++sessionSeq}`;
  sessions.value.push({ id: sid, connId: id, name: label, type: 'process', status: 'connected' });
  activeTab.value = sid;
}

function statusText(st) {
  return { connecting: '连接中', connected: '已连接', error: '出错', closed: '已断开' }[st] || st;
}

async function doChangePwd() {
  try {
    await api.post('/api/change-password', { oldPassword: pwdOld.value, newPassword: pwdNew.value });
    message.success('密码已修改');
    pwdOld.value = '';
    pwdNew.value = '';
  } catch (err) {
    message.error(errMsg(err));
    return false;
  }
}

async function loadAll() {
  const [f, c] = await Promise.all([api.get('/api/folders'), api.get('/api/connections')]);
  folders.value = f.data;
  connections.value = c.data;
  if (!expandedKeys.value.length) expandedKeys.value = f.data.map((x) => `f-${x.id}`);
  refreshAllLatency();
}

// 持久化已打开的标签页，刷新后恢复（当前激活项自动连接，其余点击后再重连）
function snapshotSessions() {
  try {
    localStorage.setItem(
      'tw_sessions',
      JSON.stringify(
        sessions.value.map((s) => ({
          id: s.id,
          connId: s.connId,
          name: s.name,
          hostLabel: s.hostLabel,
          type: s.type,
          status: s.type === 'terminal' ? s.status : 'connected'
        }))
      )
    );
    localStorage.setItem('tw_active', activeTab.value || '');
  } catch {}
}

function restoreSessions() {
  try {
    const raw = localStorage.getItem('tw_sessions');
    const active = localStorage.getItem('tw_active');
    if (!raw) return;
    const saved = JSON.parse(raw);
    const connIds = new Set(connections.value.map((c) => c.id));
    const valid = saved.filter((s) => connIds.has(s.connId));
    if (!valid.length) return;
    valid.forEach((s) => {
      if (s.type === 'terminal') s.status = 'closed'; // 非激活终端在点击标签时才重连
      sessions.value.push(s);
    });
    const maxSeq = valid.reduce((m, s) => {
      const n = parseInt(String(s.id).replace(/\D/g, ''), 10);
      return Number.isNaN(n) ? m : Math.max(m, n);
    }, 0);
    sessionSeq = maxSeq;
    if (active && sessions.value.some((s) => s.id === active)) activeTab.value = active;
  } catch {}
}

watch([sessions, activeTab], snapshotSessions, { deep: true });

watch(collapsed, async (v) => {
  localStorage.setItem('tw_sidebar_collapsed', v ? '1' : '0');
  await nextTick();
  paneRefs.forEach((p) => p?.resize?.());
});

onMounted(async () => {
  await loadAll();
  await loadSettings();
  restoreSessions();
  latencyTimer = setInterval(refreshAllLatency, 30000);
  startSysInfoPolling();
});

// 切换 tab 或激活终端变为「已连接」时，立即刷新一次统计
watch(
  () => {
    const s = sessions.value.find((x) => x.id === activeTab.value);
    return s ? `${s.id}:${s.status}` : '';
  },
  () => refreshActiveSysInfo()
);

onUnmounted(() => {
  clearInterval(latencyTimer);
  clearInterval(sysInfoTimer);
});
</script>

<style scoped>
.layout { display: flex; height: 100vh; background: var(--bg-app); position: relative; }
.sidebar {
  width: 288px;
  min-width: 288px;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--border);
  background: var(--bg-sidebar);
  transition: min-width 0.22s ease, width 0.22s ease;
  overflow: hidden;
}
.sidebar.collapsed {
  min-width: 0;
  width: 0;
}
.side-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 12px 10px;
}
.logo-line { display: flex; align-items: center; gap: 8px; }
.logo-dot { width: 20px; height: 20px; border-radius: 4px; object-fit: contain; }
.logo-text { font-size: 15px; font-weight: 600; color: var(--text); }
.side-search { padding: 0 12px 10px; }
.side-tree { flex: 1; overflow: auto; padding: 0 8px; }
.side-footer {
  display: flex;
  justify-content: space-between;
  padding: 8px 12px;
  border-top: 1px solid var(--border);
}
.content { flex: 1; display: flex; flex-direction: column; min-width: 0; overflow: hidden; }
.content.collapsed-gutter { padding-left: 48px; }
.welcome {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
}
.welcome-logo {
  width: 72px; height: 72px; border-radius: 18px;
  display: flex; align-items: center; justify-content: center;
  background: linear-gradient(135deg, rgba(34,211,238,0.14), rgba(59,130,246,0.14));
  border: 1px solid var(--border);
  margin-bottom: 6px;
}
.tree-empty {
  display: flex; flex-direction: column; align-items: center; justify-content: center;
  gap: 8px; padding: 40px 16px; text-align: center;
}
.tree-empty-ic {
  width: 56px; height: 56px; border-radius: 14px;
  display: flex; align-items: center; justify-content: center;
  background: var(--bg-elevated); border: 1px solid var(--border);
  color: var(--text-mut);
}
.tree-empty-title { font-size: 14px; color: var(--text); font-weight: 500; }
.tree-empty-sub { font-size: 12px; color: var(--text-sub); margin-bottom: 4px; }
.welcome-title { font-size: 18px; color: var(--text); font-weight: 500; }
.welcome-sub { color: var(--text-sub); font-size: 13px; }
.expand-btn {
  position: absolute;
  top: 12px;
  left: 12px;
  z-index: 20;
}
.term-tabs { height: 100%; display: flex; flex-direction: column; padding: 8px 8px 0; }
.term-tabs :deep(.n-tab-pane) {
  height: 100%;
  display: flex;
  flex-direction: column;
  padding: 0 !important;
}
.term-tabs :deep(.n-tabs-pane-wrapper) { flex: 1; }
.pane-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 4px;
  flex-wrap: wrap;
}
.pane-host { color: var(--text-mut); font-size: 12px; }
.stat-bars { display: flex; align-items: center; gap: 14px; flex-wrap: wrap; }
.stat-bar { display: flex; align-items: center; gap: 6px; }
.stat-bar-label { font-size: 11px; color: var(--text-sub); white-space: nowrap; }
.stat-bar-track { width: 56px; }
.stat-bar-val { font-size: 11px; color: var(--text); font-variant-numeric: tabular-nums; min-width: 28px; }
.stat-loading { font-size: 11px; color: var(--text-sub); }
.term-modal-body { height: 64vh; display: flex; }
.term-modal-body .term-wrap { flex: 1; min-height: 0; }
</style>

<template>
  <div class="sftp-pane">
    <div
      class="sftp-root"
      :class="{ 'sftp-dragging': dragActive }"
      @dragenter="onDragEnter"
      @dragover="onDragOver"
      @dragleave="onDragLeave"
      @drop.prevent="onDrop"
      @contextmenu.prevent="onBlankContextMenu"
    >
      <div class="sftp-toolbar">
        <div class="sftp-toolbar-left">
          <n-space size="small">
            <n-button size="small" secondary :disabled="cwd === '/'" :title="cwd === '/' ? '已在根目录' : '返回上级目录'" @click="goParent">
              <template #icon><n-icon><ArrowUpOutline /></n-icon></template>
            </n-button>
            <n-input v-model:value="pathInput" size="small" placeholder="输入路径后回车" style="flex: 1; min-width: 120px" @keyup.enter="go(pathInput)" />
            <n-button size="small" secondary @click="refresh">
              <template #icon><n-icon><RefreshOutline /></n-icon></template>
            </n-button>
          </n-space>
        </div>
        <div class="sftp-toolbar-right">
          <n-space size="small">
            <n-popover trigger="hover" placement="bottom-end" :disabled="!uploads.length">
              <template #trigger>
                <span class="upload-trigger-wrap">
                  <n-upload abstract multiple :custom-request="doUpload" :show-file-list="false">
                    <n-upload-trigger #="{ handleClick }" abstract>
                      <n-button size="small" type="primary" @click="handleClick">
                        <template #icon>
                          <span v-if="uploads.length" class="upload-ring">
                            <svg class="upload-ring-svg" viewBox="0 0 24 24" width="14" height="14" aria-hidden="true">
                              <circle cx="12" cy="12" :r="RING_R" fill="none" stroke="rgba(255,255,255,0.3)" :stroke-width="RING_STROKE" />
                              <circle cx="12" cy="12" :r="RING_R" fill="none" :stroke="ringColor" :stroke-width="RING_STROKE"
                                :stroke-dasharray="RING_C"
                                :stroke-dashoffset="RING_C * (1 - uploadCompletedPercent / 100)"
                                stroke-linecap="round" transform="rotate(-90 12 12)" />
                            </svg>
                          </span>
                          <n-icon v-else><CloudUploadOutline /></n-icon>
                        </template>
                        {{ uploads.length ? `上传中 ${uploadCompleted}/${uploads.length}` : '上传' }}
                      </n-button>
                    </n-upload-trigger>
                  </n-upload>
                </span>
              </template>
              <div class="upload-pop">
                <div class="upload-pop-head">上传列表（{{ uploadCompleted }}/{{ uploads.length }}）</div>
                <div v-for="u in uploads" :key="u.id" class="upload-pop-item">
                  <span class="up-name" :title="u.name">{{ u.name }}</span>
                  <n-progress
                    type="line"
                    :percentage="u.percent"
                    :status="u.status === 'error' ? 'error' : u.status === 'paused' ? 'warning' : u.status === 'success' ? 'success' : 'info'"
                    :height="6"
                    style="flex: 1"
                  />
                  <span class="up-pct">{{ u.percent }}%</span>
                  <n-button v-if="u.status === 'uploading'" size="tiny" quaternary title="暂停" @click="pauseUpload(u)">
                    <template #icon><n-icon><PauseOutline /></n-icon></template>
                  </n-button>
                  <n-button v-else-if="u.status === 'paused'" size="tiny" quaternary title="继续" @click="resumeUpload(u)">
                    <template #icon><n-icon><PlayOutline /></n-icon></template>
                  </n-button>
                  <n-button size="tiny" quaternary title="移除" @click="removeUpload(u)">
                    <template #icon><n-icon><CloseOutline /></n-icon></template>
                  </n-button>
                </div>
              </div>
            </n-popover>
            <template v-if="checkedKeys.length">
              <n-divider vertical />
              <n-button size="small" type="error" @click="batchRemove">
                <template #icon><n-icon><TrashOutline /></n-icon></template>删除
              </n-button>
              <n-button size="small" @click="moveSelected">
                <template #icon><n-icon><MoveOutline /></n-icon></template>移动
              </n-button>
            </template>
          </n-space>
        </div>
      </div>

      <div v-if="dragActive" class="sftp-drop-mask">
        <div class="sftp-drop-inner">
          <div class="sftp-drop-icon"><n-icon size="32" color="#2f6bff"><CloudUploadOutline /></n-icon></div>
          <div class="sftp-drop-text">释放鼠标以上传文件</div>
          <div class="sftp-drop-sub">支持多文件与文件夹</div>
        </div>
      </div>

      <n-spin :show="loading" class="sftp-spin">
        <n-data-table
          size="small"
          :columns="columns"
          :data="items"
          :bordered="false"
          :max-height="tableHeight"
          :row-key="(r) => r.path"
          :checked-row-keys="checkedKeys"
          :row-props="rowProps"
          :sort-key="sortKey"
          :sort-order="sortOrder"
          @update:checked-row-keys="(k) => (checkedKeys = k)"
          @update:sorter="onSorterChange"
        />
      </n-spin>

      <div v-if="!items.length && !loading" class="sftp-empty">拖拽文件到此处即可上传</div>
    </div>

    <FileEditor v-model:show="editorShow" :conn-id="connId" :path="editorRow?.path" :name="editorRow?.name" @saved="refresh" />

    <input ref="fileInput" type="file" multiple style="display: none" @change="onFilePick" />

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
      :show="blankCtxShow"
      :x="blankCtxX"
      :y="blankCtxY"
      :options="blankCtxOptions"
      @select="onBlankCtxSelect"
      @clickoutside="blankCtxShow = false"
    />

    <n-modal v-model:show="propShow" preset="card" title="文件属性" style="width: 420px">
      <n-descriptions v-if="propRow" label-placement="left" bordered :column="1" size="small">
        <n-descriptions-item label="名称">{{ propRow.name }}</n-descriptions-item>
        <n-descriptions-item label="路径">{{ propRow.path }}</n-descriptions-item>
        <n-descriptions-item label="类型">{{ typeText(propRow.type) }}</n-descriptions-item>
        <n-descriptions-item label="大小">{{ propRow.type === 'file' ? fmtSize(propRow.size) : '-' }}</n-descriptions-item>
        <n-descriptions-item label="权限">{{ propRow.mode }}</n-descriptions-item>
        <n-descriptions-item label="修改时间">{{ fmtTime(propRow.mtime) }}</n-descriptions-item>
      </n-descriptions>
    </n-modal>

    <n-modal v-model:show="moveShow" preset="card" title="移动到" style="width: 420px">
      <n-input v-model:value="moveTarget" placeholder="目标目录（绝对路径，如 /home/user/backup）" />
      <template #footer>
        <n-space justify="end">
          <n-button size="small" @click="moveShow = false">取消</n-button>
          <n-button size="small" type="primary" @click="confirmMove">确定</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="unzipShow" preset="card" title="解压到" style="width: 420px">
      <n-input v-model:value="unzipTarget" placeholder="目标目录（绝对路径，默认当前目录）" />
      <n-input v-model:value="unzipMode" placeholder="权限（可选，如 755，递归应用到解压内容）" style="margin-top: 8px" />
      <template #footer>
        <n-space justify="end">
          <n-button size="small" @click="unzipShow = false">取消</n-button>
          <n-button size="small" type="primary" @click="confirmUnzip">解压</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup>
import { ref, computed, h, onMounted, onUnmounted, defineAsyncComponent } from 'vue';
import {
  NSpace, NTag, NButton, NIcon,
  NInput, NUpload, NUploadTrigger, NDataTable, NSpin, NProgress, NDropdown, NModal, NDivider,
  NDescriptions, NDescriptionsItem, NPopover, useMessage, useDialog
} from 'naive-ui';
import {
  RefreshOutline, CloudUploadOutline, FolderOutline,
  DocumentOutline, LinkOutline, DownloadOutline, PencilOutline, TrashOutline, DocumentTextOutline,
  AddOutline, MoveOutline, TerminalOutline, CopyOutline, ArchiveOutline, CloseOutline,
  ArrowUpOutline, PauseOutline, PlayOutline
} from '@vicons/ionicons5';
import { api, errMsg, getToken, saveBlobToDisk, sftpUnzip } from '../api.js';
// 文件编辑器按需加载：它连带 monaco 包装层，静态引入会把 monaco chunk 拽进静态依赖图
const FileEditor = defineAsyncComponent(() => import('./FileEditor.vue'));

const props = defineProps({
  connId: { type: Number, required: true },
  connName: { type: String, default: '' },
  active: { type: Boolean, default: true }
});
const emit = defineEmits(['open-terminal']);
const message = useMessage();
const dialog = useDialog();

const EDIT_MAX = 2 * 1024 * 1024; // 与后端 read 上限一致
const cwd = ref('/');
const pathInput = ref('');
const items = ref([]);
const loading = ref(false);
const uploads = ref([]);
let uploadSeq = 0;
const dragActive = ref(false);
let dragDepth = 0;
const uploadCompleted = computed(() => uploads.value.filter((u) => u.status === 'success' || u.status === 'error').length);
const uploadCompletedPercent = computed(() => (uploads.value.length ? Math.round((uploadCompleted.value / uploads.value.length) * 100) : 0));
// 上传按钮内嵌的小型圆环进度：自绘 SVG，规避 n-progress 默认 120px 宽且并无 width prop 的问题
const RING_R = 9;
const RING_STROKE = 2.5;
const RING_C = 2 * Math.PI * RING_R;
const ringColor = computed(() => {
  const list = uploads.value;
  if (!list.length) return '#fff';
  const allDone = list.every((u) => u.status === 'success' || u.status === 'error');
  if (allDone && list.some((u) => u.status === 'error')) return '#ff9a9a';
  return '#fff';
});
const tableHeight = window.innerHeight - 200;

const editorShow = ref(false);
const editorRow = ref(null);

// 行右键菜单
const ctxShow = ref(false);
const ctxX = ref(0);
const ctxY = ref(0);
const ctxOptions = ref([]);
let ctxRow = null;

// 空白处右键菜单
const blankCtxShow = ref(false);
const blankCtxX = ref(0);
const blankCtxY = ref(0);
const blankCtxOptions = [
  { label: '新建文件', key: 'newFile', icon: () => h(NIcon, null, { default: () => h(DocumentOutline) }) },
  { label: '新建目录', key: 'mkdir', icon: () => h(NIcon, null, { default: () => h(FolderOutline) }) },
  { label: '上传文件', key: 'upload', icon: () => h(NIcon, null, { default: () => h(CloudUploadOutline) }) },
  { type: 'divider', key: 'd1' },
  { label: '在终端打开', key: 'openTerm', icon: () => h(NIcon, null, { default: () => h(TerminalOutline) }) },
  { label: '刷新', key: 'refresh', icon: () => h(NIcon, null, { default: () => h(RefreshOutline) }) }
];

// 属性弹窗
const propShow = ref(false);
const propRow = ref(null);
const fileInput = ref(null);

// 选择 + 排序状态
const checkedKeys = ref([]); // 已选行 path 列表（单选/多选）
const sortKey = ref(null);
const sortOrder = ref(null);

function clearSelection() {
  checkedKeys.value = [];
}

function onSorterChange(state) {
  if (!state || !state.columnKey) return;
  if (state.order) {
    sortKey.value = state.columnKey;
    sortOrder.value = state.order;
  } else {
    // 再次点击在 asc/desc 间切换，不回到未排序
    sortKey.value = state.columnKey;
    sortOrder.value = sortOrder.value === 'ascend' ? 'descend' : 'ascend';
  }
}

function fmtSize(n) {
  if (n == null) return '';
  if (n < 1024) return n + ' B';
  if (n < 1024 ** 2) return (n / 1024).toFixed(1) + ' KB';
  if (n < 1024 ** 3) return (n / 1024 ** 2).toFixed(1) + ' MB';
  return (n / 1024 ** 3).toFixed(2) + ' GB';
}

function fmtTime(t) {
  return new Date(t).toLocaleString('zh-CN', { hour12: false });
}

const columns = [
  { type: 'selection' },
  {
    title: '名称',
    key: 'name',
    sorter: (a, b) => {
      if (a.type === 'dir' && b.type !== 'dir') return -1;
      if (a.type !== 'dir' && b.type === 'dir') return 1;
      return a.name.localeCompare(b.name, 'zh');
    },
    render(row) {
      const icon = row.type === 'dir' ? FolderOutline : row.type === 'link' ? LinkOutline : DocumentOutline;
      const color = row.type === 'dir' ? '#e0b063' : row.type === 'link' ? '#70a5eb' : '#999';
      return h(
        'div',
        { style: 'display:flex;align-items:center;gap:8px;cursor:pointer' },
        [
          h(NIcon, { color, size: 16 }, { default: () => h(icon) }),
          h('span', { style: row.type === 'dir' ? 'color:var(--text)' : 'color:var(--text-sub)' }, row.name)
        ]
      );
    }
  },
  { title: '大小', key: 'size', width: 100, sorter: (a, b) => (a.size || 0) - (b.size || 0), render: (r) => (r.type === 'file' ? fmtSize(r.size) : '') },
  { title: '权限', key: 'mode', width: 80, sorter: (a, b) => String(a.mode).localeCompare(String(b.mode)) },
  { title: '修改时间', key: 'mtime', width: 160, sorter: (a, b) => (a.mtime || 0) - (b.mtime || 0), render: (r) => fmtTime(r.mtime) },
  {
    title: '操作',
    key: 'ops',
    width: 165,
    render(row) {
      const btn = (icon, title, onClick, type = 'default') =>
        h(NButton, { size: 'tiny', quaternary: true, type, title, onClick: (e) => { e.stopPropagation(); onClick(); } },
          { icon: () => h(NIcon, null, { default: () => h(icon) }) });
      const ops = [];
      if (row.type === 'file') {
        ops.push(btn(DownloadOutline, '下载', () => download(row)));
        if ((row.size ?? 0) <= EDIT_MAX) ops.push(btn(DocumentTextOutline, '编辑', () => openEditor(row)));
      }
      ops.push(btn(PencilOutline, '重命名', () => rename(row)));
      ops.push(btn(TrashOutline, '删除', () => remove(row), 'error'));
      return h(NSpace, { size: 2 }, { default: () => ops });
    }
  }
];

function rowProps(row) {
  return {
    style: 'cursor: pointer',
    onDblclick() {
      if (row.type === 'dir' || row.type === 'link') go(row.path);
    },
    onContextmenu(e) {
      e.preventDefault();
      e.stopPropagation();
      onRowContextMenu(e, row);
    }
  };
}

function typeText(t) {
  return { dir: '目录', link: '软链接', file: '文件' }[t] || t;
}

// 是否为可解压的压缩包（按扩展名判断）
function isArchive(name) {
  const n = (name || '').toLowerCase();
  return (
    n.endsWith('.zip') ||
    n.endsWith('.tar') ||
    n.endsWith('.tar.gz') ||
    n.endsWith('.tgz') ||
    n.endsWith('.tar.bz2') ||
    n.endsWith('.tbz2') ||
    n.endsWith('.tar.xz') ||
    n.endsWith('.txz')
  );
}

function onRowContextMenu(e, row) {
  ctxRow = row;
  const termItem = {
    label: '在终端打开',
    key: 'openTerm',
    icon: () => h(NIcon, null, { default: () => h(TerminalOutline) })
  };
  const copyItem = {
    label: '复制路径',
    key: 'copyPath',
    icon: () => h(NIcon, null, { default: () => h(CopyOutline) })
  };
  const archivable = row.type === 'file' && isArchive(row.name);
  const fileOps = row.type === 'file'
    ? [
        { label: '下载', key: 'download', icon: () => h(NIcon, null, { default: () => h(DownloadOutline) }) },
        ...(archivable
          ? [{ label: '解压到…', key: 'unzip', icon: () => h(NIcon, null, { default: () => h(ArchiveOutline) }) }]
          : []),
        ...((row.size ?? 0) <= EDIT_MAX
          ? [{ label: '编辑', key: 'edit', icon: () => h(NIcon, null, { default: () => h(DocumentTextOutline) }) }]
          : [])
      ]
    : [{ label: '进入目录', key: 'enter', icon: () => h(NIcon, null, { default: () => h(FolderOutline) }) }];
  ctxOptions.value = [
    termItem,
    ...fileOps,
    { label: '重命名', key: 'rename', icon: () => h(NIcon, null, { default: () => h(PencilOutline) }) },
    { label: '移动', key: 'move', icon: () => h(NIcon, null, { default: () => h(MoveOutline) }) },
    { label: '属性', key: 'props', icon: () => h(NIcon, null, { default: () => h(DocumentOutline) }) },
    copyItem,
    { type: 'divider', key: 'd1' },
    { label: '删除', key: 'remove', icon: () => h(NIcon, { color: '#e24b4a' }, { default: () => h(TrashOutline) }) }
  ];
  ctxShow.value = true;
  ctxX.value = e.clientX;
  ctxY.value = e.clientY;
}

function onCtxSelect(key) {
  ctxShow.value = false;
  if (!ctxRow) return;
  if (key === 'download') download(ctxRow);
  else if (key === 'edit') openEditor(ctxRow);
  else if (key === 'enter') go(ctxRow.path);
  else if (key === 'unzip') openUnzip(ctxRow);
  else if (key === 'rename') rename(ctxRow);
  else if (key === 'move') openMove([ctxRow]);
  else if (key === 'props') { propRow.value = ctxRow; propShow.value = true; }
  else if (key === 'openTerm') openInTerminal(ctxRow);
  else if (key === 'copyPath') copyPath(ctxRow);
  else if (key === 'remove') remove(ctxRow);
}

// 在终端打开并进入该文件/文件夹所在目录
function openInTerminal(row) {
  const p = row.path;
  const dir = row.type === 'dir'
    ? p
    : p.includes('/')
      ? p.slice(0, p.lastIndexOf('/')) || '/'
      : '/';
  emit('open-terminal', { connId: props.connId, path: dir });
}

async function copyPath(row) {
  try {
    await navigator.clipboard.writeText(row.path);
    message.success('路径已复制：' + row.path);
  } catch {
    message.error('复制失败，请检查浏览器剪贴板权限');
  }
}

function onBlankContextMenu(e) {
  blankCtxShow.value = true;
  blankCtxX.value = e.clientX;
  blankCtxY.value = e.clientY;
}

function onBlankCtxSelect(key) {
  blankCtxShow.value = false;
  if (key === 'newFile') newFile();
  else if (key === 'mkdir') mkdir();
  else if (key === 'upload') fileInput.value?.click();
  else if (key === 'openTerm') emit('open-terminal', { connId: props.connId, path: cwd.value });
  else if (key === 'refresh') refresh();
}

function onFilePick(e) {
  const files = e.target.files;
  if (!files || !files.length) return;
  for (const f of files) doUpload({ file: { name: f.name, file: f } });
  e.target.value = '';
}

async function refresh() {
  loading.value = true;
  try {
    const { data } = await api.get(`/api/sftp/${props.connId}/list`, { params: { path: cwd.value } });
    cwd.value = data.path;
    pathInput.value = data.path;
    items.value = data.items;
  } catch (err) {
    message.error(errMsg(err));
  } finally {
    loading.value = false;
  }
}

function go(p) {
  cwd.value = p || '/';
  clearSelection();
  refresh();
}

function goParent() {
  const p = cwd.value;
  if (p === '/' || !p.includes('/')) {
    go('/');
    return;
  }
  const parent = p.slice(0, p.lastIndexOf('/')) || '/';
  go(parent);
}

async function download(row) {
  const url = `/api/sftp/${props.connId}/download?path=${encodeURIComponent(row.path)}&token=${encodeURIComponent(getToken())}`;
  const { data } = await api.get(url, { responseType: 'blob' });
  await saveBlobToDisk(data, row.name);
}

function openEditor(row) {
  editorRow.value = row;
  editorShow.value = true;
}

function mkdir() {
  let name = '';
  dialog.create({
    title: '新建目录',
    content: () => h(NInput, { placeholder: '目录名称', onUpdateValue: (v) => (name = v) }),
    positiveText: '创建',
    negativeText: '取消',
    onPositiveClick: async () => {
      if (!name.trim()) return false;
      try {
        await api.post(`/api/sftp/${props.connId}/mkdir`, { path: joinPath(cwd.value, name.trim()) });
        refresh();
      } catch (err) {
        message.error(errMsg(err));
        return false;
      }
    }
  });
}

function newFile() {
  let name = '';
  dialog.create({
    title: '新建文件',
    content: () => h(NInput, { placeholder: '文件名（如 app.conf）', onUpdateValue: (v) => (name = v) }),
    positiveText: '创建',
    negativeText: '取消',
    onPositiveClick: async () => {
      if (!name.trim()) return false;
      const target = joinPath(cwd.value, name.trim());
      try {
        await api.post(`/api/sftp/${props.connId}/write`, { path: target, content: '' });
        await refresh();
        const row = items.value.find((i) => i.path === target);
        if (row) openEditor(row);
      } catch (err) {
        message.error(errMsg(err));
        return false;
      }
    }
  });
}

function rename(row) {
  let name = row.name;
  dialog.create({
    title: '重命名',
    content: () => h(NInput, { defaultValue: row.name, onUpdateValue: (v) => (name = v) }),
    positiveText: '保存',
    negativeText: '取消',
    onPositiveClick: async () => {
      if (!name.trim() || name === row.name) return;
      try {
        await api.post(`/api/sftp/${props.connId}/rename`, { from: row.path, to: joinPath(cwd.value, name.trim()) });
        refresh();
      } catch (err) {
        message.error(errMsg(err));
        return false;
      }
    }
  });
}

function remove(row) {
  dialog.warning({
    title: '删除',
    content: `确定删除「${row.name}」吗？${row.type === 'dir' ? '（目录将连同内容一并删除，不可恢复）' : ''}`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await api.post(`/api/sftp/${props.connId}/delete`, { path: row.path, isDir: row.type === 'dir' });
        message.success('已删除');
        refresh();
      } catch (err) {
        message.error(errMsg(err));
      }
    }
  });
}

function joinPath(dir, name) {
  return (dir === '/' ? '' : dir) + '/' + name;
}

// 批量操作：删除 / 移动
const moveShow = ref(false);
const moveTarget = ref('');
const moveRows = ref([]);

// 解压弹窗：目标目录（默认当前目录）+ 可选权限
const unzipShow = ref(false);
const unzipTarget = ref('');
const unzipMode = ref('');
const unzipRow = ref(null);

function openMove(rows) {
  if (!rows || !rows.length) return;
  moveRows.value = rows;
  moveTarget.value = cwd.value;
  moveShow.value = true;
}

function moveSelected() {
  const rows = checkedKeys.value
    .map((p) => items.value.find((i) => i.path === p))
    .filter(Boolean);
  openMove(rows);
}

async function confirmMove() {
  const target = moveTarget.value.trim();
  if (!target) {
    message.warning('请输入目标目录');
    return false;
  }
  let ok = 0;
  let fail = 0;
  for (const r of moveRows.value) {
    const name = r.path.split('/').pop();
    const to = (target === '/' ? '' : target) + '/' + name;
    try {
      await api.post(`/api/sftp/${props.connId}/rename`, { from: r.path, to });
      ok++;
    } catch {
      fail++;
    }
  }
  if (ok) message.success(`已移动 ${ok} 项${fail ? `，失败 ${fail} 项` : ''}`);
  else message.error(`移动失败 ${fail} 项`);
  moveShow.value = false;
  clearSelection();
  refresh();
}

// 解压：打开弹窗，默认解压到当前目录
function openUnzip(row) {
  if (!row) return;
  unzipRow.value = row;
  unzipTarget.value = cwd.value;
  unzipMode.value = '';
  unzipShow.value = true;
}

async function confirmUnzip() {
  const row = unzipRow.value;
  if (!row) return;
  const target = unzipTarget.value.trim() || cwd.value;
  // 权限仅允许 3~4 位八进制，空则不强设
  const mode = /^[0-7]{3,4}$/.test(unzipMode.value.trim()) ? unzipMode.value.trim() : '';
  unzipShow.value = false;
  try {
    await sftpUnzip(props.connId, row.path, target, mode);
    message.success(`已解压到 ${target}`);
    refresh();
  } catch (err) {
    message.error(errMsg(err));
  }
}

function batchRemove() {
  const rows = checkedKeys.value
    .map((p) => items.value.find((i) => i.path === p))
    .filter(Boolean);
  if (!rows.length) return;
  dialog.warning({
    title: '删除',
    content: `确定删除选中的 ${rows.length} 项吗？${rows.some((r) => r.type === 'dir') ? '（目录将连同内容一并删除，不可恢复）' : ''}`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      let ok = 0;
      let fail = 0;
      for (const r of rows) {
        try {
          await api.post(`/api/sftp/${props.connId}/delete`, { path: r.path, isDir: r.type === 'dir' });
          ok++;
        } catch {
          fail++;
        }
      }
      if (ok) message.success(`已删除 ${ok} 项${fail ? `，失败 ${fail} 项` : ''}`);
      else message.error(`删除失败 ${fail} 项`);
      clearSelection();
      refresh();
    }
  });
}

// 入口：NUpload / 拖拽 / Tauri 拖拽都走这里。file = { name, file }
function doUpload({ file }) {
  createUpload(file);
}

function createUpload(file) {
  const u = {
    id: ++uploadSeq,
    name: file.name,
    fileRef: file.file,
    percent: 0,
    status: 'uploading',
    controller: null
  };
  uploads.value.push(u);
  startUpload(u);
  return u;
}

async function startUpload(u) {
  u.status = 'uploading';
  u.percent = 0;
  u.controller = new AbortController();
  const fd = new FormData();
  fd.append('file', u.fileRef);
  try {
    await api.post(`/api/sftp/${props.connId}/upload`, fd, {
      params: { path: cwd.value },
      headers: { 'Content-Type': 'multipart/form-data' },
      signal: u.controller.signal,
      onUploadProgress: (e) => {
        if (e.total) u.percent = Math.round((e.loaded / e.total) * 100);
      }
    });
    u.percent = 100;
    u.status = 'success';
    message.success(`${u.name} 上传完成`);
    refresh();
  } catch (err) {
    // 用户主动暂停/移除导致的 abort 不报错、不计入失败
    if (u.status === 'paused' || u.status === 'canceled') return;
    u.status = 'error';
    message.error(`${u.name} 上传失败: ${errMsg(err)}`);
  } finally {
    scheduleClear();
  }
}

function pauseUpload(u) {
  if (u.status !== 'uploading') return;
  u.status = 'paused';
  u.controller?.abort();
}

function resumeUpload(u) {
  if (u.status !== 'paused') return;
  startUpload(u);
}

function removeUpload(u) {
  u.status = 'canceled';
  u.controller?.abort();
  uploads.value = uploads.value.filter((x) => x.id !== u.id);
}

// 全部完成（成功或失败都算完成）后清空列表；暂停项会保留列表直到继续或移除
function scheduleClear() {
  if (!uploads.value.length) return;
  if (uploads.value.every((u) => u.status === 'success' || u.status === 'error')) {
    setTimeout(() => {
      if (uploads.value.length && uploads.value.every((u) => u.status === 'success' || u.status === 'error')) {
        uploads.value = [];
      }
    }, 2500);
  }
}

// 浏览器端拖拽遮罩：仅在拖入的是文件时显示，用计数器避免子元素 dragleave 抖动
function onDragEnter(e) {
  if (e.dataTransfer && Array.from(e.dataTransfer.types || []).includes('Files')) {
    dragDepth++;
    dragActive.value = true;
  }
}
function onDragOver(e) {
  if (e.dataTransfer && Array.from(e.dataTransfer.types || []).includes('Files')) {
    e.preventDefault();
  }
}
function onDragLeave(e) {
  if (e.dataTransfer && Array.from(e.dataTransfer.types || []).includes('Files')) {
    dragDepth = Math.max(0, dragDepth - 1);
    if (dragDepth === 0) dragActive.value = false;
  }
}
function onDrop(e) {
  dragDepth = 0;
  dragActive.value = false;
  const files = e.dataTransfer?.files;
  if (!files || !files.length) return;
  for (const f of files) doUpload({ file: { name: f.name, file: f } });
}

// 判断本面板当前是否可见（激活 tab）。多个文件管理标签常驻挂载，只有可见的那个才应响应桌面端拖入。
function isPaneVisible() {
  // 仅当前激活（可见）的 tab 才响应桌面端拖入；多文件管理标签常驻挂载，
  // 用 Main.vue 传入的 active 精确区分，避免窗口级 tauri://drag-drop 事件被所有面板重复处理。
  return !!props.active;
}

// Tauri 桌面端：原生 HTML5 拖拽被 Tauri 拦截，需监听其 drag-drop 事件拿到本地路径后上传。
// Tauri v2 在拖入过程中还会发 drag-enter / drag-leave，用来控制遮罩显隐。浏览器端仍走上面的 @drop。
let unlistenDragDrop = null;
let unlistenDragEnter = null;
let unlistenDragLeave = null;

async function setupTauriDragDrop() {
  if (typeof window === 'undefined') return;
  if (!('__TAURI_INTERNALS__' in window || '__TAURI__' in window)) return;
  const { listen } = await import('@tauri-apps/api/event');
  const { readFile, stat, readDir } = await import('@tauri-apps/plugin-fs');
  unlistenDragEnter = await listen('tauri://drag-enter', () => { dragActive.value = true; });
  unlistenDragLeave = await listen('tauri://drag-leave', () => { dragActive.value = false; });
  unlistenDragDrop = await listen('tauri://drag-drop', (ev) => {
    dragActive.value = false;
    if (!isPaneVisible()) return; // 仅当前可见（激活）面板处理拖入，避免多文件管理标签重复上传
    const paths = ev.payload?.paths || [];
    if (!paths.length) return;
    for (const p of paths) uploadLocalPath(p, readFile, stat, readDir);
  });
}

// 递归收集本地路径下的文件并通过 doUpload 上传（目录结构拍平，与按钮上传行为一致）。
async function uploadLocalPath(p, readFile, stat, readDir) {
  let isDir = false;
  try {
    const st = await stat(p);
    isDir = !!st.isDirectory;
  } catch (e) {
    // stat 失败（权限/不存在）不致命：当作文件尝试直读，单文件仍可上传
    console.warn('[upload] stat 失败，尝试按文件读取:', p, e);
  }
  if (isDir) {
    let entries = [];
    try {
      entries = await readDir(p);
    } catch (e) {
      message.error(`无法读取目录 ${p}：${errMsg(e)}`);
      return;
    }
    for (const e of entries) {
      if (e.name === '.' || e.name === '..') continue;
      await uploadLocalPath(p.replace(/\/$/, '') + '/' + e.name, readFile, stat, readDir);
    }
    return;
  }
  try {
    const buf = await readFile(p);
    const name = p.split('/').pop();
    const fileObj = new File([buf], name);
    doUpload({ file: { name, file: fileObj } });
  } catch (e) {
    message.error(`读取本地文件失败：${name}（${errMsg(e)}）`);
  }
}

onMounted(() => {
  refresh();
  setupTauriDragDrop();
});

onUnmounted(() => {
  unlistenDragDrop?.();
  unlistenDragEnter?.();
  unlistenDragLeave?.();
});
</script>

<style scoped>
.sftp-pane { flex: 1; min-height: 0; display: flex; flex-direction: column; padding: 4px 0; }
.sftp-root { flex: 1; min-height: 0; display: flex; flex-direction: column; position: relative; }
.sftp-dragging { outline: 2px dashed #2080f0; outline-offset: -2px; border-radius: 8px; }
.sftp-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.sftp-toolbar-left { display: flex; align-items: center; flex: 1; min-width: 0; }
.sftp-toolbar-left :deep(.n-space) { flex: 1; min-width: 0; }
.sftp-toolbar-right { display: flex; align-items: center; margin-left: auto; }
.sftp-spin { flex: 1; min-height: 0; }
.upload-ring { display: inline-flex; vertical-align: middle; line-height: 0; }
.upload-ring-svg { display: block; }
.upload-trigger-wrap { display: inline-flex; }
.upload-pop { width: 320px; max-height: 320px; overflow: auto; }
.upload-pop-head { font-size: 12px; color: var(--text-sub); margin-bottom: 8px; }
.upload-pop-item { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.up-name { font-size: 12px; max-width: 110px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.up-pct { font-size: 12px; color: var(--text-mut); width: 36px; text-align: right; }
.sftp-drop-mask {
  position: absolute;
  inset: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(20, 32, 56, 0.5);
  pointer-events: none;
  animation: sftp-mask-in 0.15s ease-out;
}
@keyframes sftp-mask-in {
  from { opacity: 0; }
  to { opacity: 1; }
}
.sftp-drop-inner {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  animation: sftp-drop-in 0.18s ease-out;
}
@keyframes sftp-drop-in {
  from { transform: scale(0.96); opacity: 0.6; }
  to { transform: scale(1); opacity: 1; }
}
.sftp-drop-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
}
.sftp-drop-text { font-size: 16px; font-weight: 600; color: #fff; }
.sftp-drop-sub { font-size: 12px; color: rgba(255, 255, 255, 0.8); margin-top: -4px; }
.sftp-empty { text-align: center; color: var(--text-sub); font-size: 12px; padding: 30px 0; border: 1px dashed var(--border); border-radius: 8px; margin-top: 8px; }
</style>

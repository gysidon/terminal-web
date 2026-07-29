<template>
  <div class="sftp-pane">
    <div class="sftp-root" @dragover.prevent @drop.prevent="onDrop" @contextmenu.prevent="onBlankContextMenu">
      <div class="sftp-toolbar">
        <n-breadcrumb>
          <n-breadcrumb-item @click="go('/')">/</n-breadcrumb-item>
          <n-breadcrumb-item v-for="(seg, i) in pathSegs" :key="i" @click="go(segPath(i))">{{ seg }}</n-breadcrumb-item>
        </n-breadcrumb>
        <n-space size="small">
          <n-input v-model:value="pathInput" size="small" placeholder="输入路径后回车" style="width: 200px" @keyup.enter="go(pathInput)" />
          <n-button size="small" secondary @click="refresh">
            <template #icon><n-icon><RefreshOutline /></n-icon></template>
          </n-button>
          <n-button size="small" secondary @click="mkdir">
            <template #icon><n-icon><FolderOpenOutline /></n-icon></template>新建目录
          </n-button>
          <n-button size="small" secondary @click="newFile">
            <template #icon><n-icon><DocumentOutline /></n-icon></template>新建文件
          </n-button>
          <n-upload abstract multiple :custom-request="doUpload" :show-file-list="false">
            <n-upload-trigger #="{ handleClick }" abstract>
              <n-button size="small" type="primary" @click="handleClick">
                <template #icon><n-icon><CloudUploadOutline /></n-icon></template>上传
              </n-button>
            </n-upload-trigger>
          </n-upload>
          <template v-if="checkedKeys.length">
            <n-divider vertical />
            <span class="sftp-sel-info">已选 {{ checkedKeys.length }} 项</span>
            <n-button size="small" type="error" @click="batchRemove">
              <template #icon><n-icon><TrashOutline /></n-icon></template>删除
            </n-button>
            <n-button size="small" @click="moveSelected">
              <template #icon><n-icon><MoveOutline /></n-icon></template>移动
            </n-button>
            <n-button size="small" quaternary @click="clearSelection">取消选择</n-button>
          </template>
        </n-space>
      </div>

      <div v-if="uploads.length" class="upload-list">
        <div v-for="u in uploads" :key="u.id" class="upload-item">
          <span class="upload-name">{{ u.name }}</span>
          <n-progress type="line" :percentage="u.percent" :status="u.status" style="flex: 1" />
          <span class="upload-pct">{{ u.percent }}%</span>
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
  </div>
</template>

<script setup>
import { ref, computed, h, onMounted } from 'vue';
import {
  NSpace, NTag, NBreadcrumb, NBreadcrumbItem, NButton, NIcon,
  NInput, NUpload, NUploadTrigger, NDataTable, NSpin, NProgress, NDropdown, NModal, NDivider,
  NDescriptions, NDescriptionsItem, useMessage, useDialog
} from 'naive-ui';
import {
  RefreshOutline, FolderOpenOutline, CloudUploadOutline, FolderOutline,
  DocumentOutline, LinkOutline, DownloadOutline, PencilOutline, TrashOutline, DocumentTextOutline,
  AddOutline, MoveOutline, TerminalOutline, CopyOutline
} from '@vicons/ionicons5';
import { api, errMsg, getToken } from '../api.js';
import FileEditor from './FileEditor.vue';

const props = defineProps({
  connId: { type: Number, required: true },
  connName: { type: String, default: '' }
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

function toggleSelect(path) {
  const i = checkedKeys.value.indexOf(path);
  if (i >= 0) checkedKeys.value = checkedKeys.value.filter((k) => k !== path);
  else checkedKeys.value = [...checkedKeys.value, path];
}
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

const pathSegs = computed(() => cwd.value.split('/').filter(Boolean));
const segPath = (i) => '/' + pathSegs.value.slice(0, i + 1).join('/');

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
        { style: 'display:flex;align-items:center;gap:8px;cursor:pointer', onClick: () => toggleSelect(row.path) },
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
  const fileOps = row.type === 'file'
    ? [
        { label: '下载', key: 'download', icon: () => h(NIcon, null, { default: () => h(DownloadOutline) }) },
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

function download(row) {
  const url = `/api/sftp/${props.connId}/download?path=${encodeURIComponent(row.path)}&token=${encodeURIComponent(getToken())}`;
  const a = document.createElement('a');
  a.href = url;
  a.download = row.name;
  a.click();
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
    content: `确定删除「${row.name}」吗？${row.type === 'dir' ? '（仅能删除空目录）' : ''}`,
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

function batchRemove() {
  const rows = checkedKeys.value
    .map((p) => items.value.find((i) => i.path === p))
    .filter(Boolean);
  if (!rows.length) return;
  dialog.warning({
    title: '删除',
    content: `确定删除选中的 ${rows.length} 项吗？${rows.some((r) => r.type === 'dir') ? '（仅能删除空目录）' : ''}`,
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

async function doUpload({ file }) {
  const u = { id: ++uploadSeq, name: file.name, percent: 0, status: 'info' };
  uploads.value.push(u);
  const fd = new FormData();
  fd.append('file', file.file);
  try {
    await api.post(`/api/sftp/${props.connId}/upload`, fd, {
      params: { path: cwd.value },
      headers: { 'Content-Type': 'multipart/form-data' },
      onUploadProgress: (e) => {
        if (e.total) u.percent = Math.round((e.loaded / e.total) * 100);
      }
    });
    u.percent = 100;
    u.status = 'success';
    message.success(`${file.name} 上传完成`);
    refresh();
  } catch (err) {
    u.status = 'error';
    message.error(`${file.name} 上传失败: ${errMsg(err)}`);
  } finally {
    setTimeout(() => {
      uploads.value = uploads.value.filter((x) => x.id !== u.id);
    }, 3000);
  }
}

function onDrop(e) {
  const files = e.dataTransfer?.files;
  if (!files || !files.length) return;
  for (const f of files) doUpload({ file: { name: f.name, file: f } });
}

onMounted(refresh);
</script>

<style scoped>
.sftp-pane { flex: 1; min-height: 0; display: flex; flex-direction: column; padding: 4px 0; }
.sftp-root { flex: 1; min-height: 0; display: flex; flex-direction: column; }
.sftp-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.sftp-spin { flex: 1; min-height: 0; }
.upload-list { margin-bottom: 10px; }
.upload-item { display: flex; align-items: center; gap: 10px; margin-bottom: 4px; }
.upload-name { font-size: 12px; color: var(--text-sub); max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.upload-pct { font-size: 12px; color: var(--text-mut); width: 38px; text-align: right; }
.sftp-empty { text-align: center; color: var(--text-sub); font-size: 12px; padding: 30px 0; border: 1px dashed var(--border); border-radius: 8px; margin-top: 8px; }
.sftp-sel-info { font-size: 13px; color: var(--text); }
</style>

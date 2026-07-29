<template>
  <div class="process-pane">
    <div class="process-toolbar">
      <n-space size="small">
        <n-button size="small" secondary @click="refresh">
          <template #icon><n-icon><RefreshOutline /></n-icon></template>刷新
        </n-button>
        <n-switch v-model:value="autoRefresh" size="small" /><span class="tip">自动刷新(5s)</span>
      </n-space>
      <span class="count">共 {{ items.length }} 个进程</span>
    </div>

    <n-spin :show="loading" class="process-spin">
      <n-data-table
        size="small"
        :columns="columns"
        :data="items"
        :bordered="false"
        :max-height="tableHeight"
        :row-key="(r) => r.pid"
        :sort-key="sortKey"
        :sort-order="sortOrder"
        @update:sorter="onSorterChange"
      />
    </n-spin>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, h } from 'vue';
import {
  NSpace, NButton, NIcon, NDataTable, NSpin, NSwitch, NTag, useMessage
} from 'naive-ui';
import {
  RefreshOutline, TrashOutline
} from '@vicons/ionicons5';
import { api, processList, processKill, errMsg } from '../api.js';

const props = defineProps({
  connId: { type: Number, required: true },
  connName: { type: String, default: '' }
});
const message = useMessage();

const items = ref([]);
const loading = ref(false);
const autoRefresh = ref(false);
const tableHeight = window.innerHeight - 160;
let timer = null;

// 受控排序状态：点击列头按该列排序，再次点击在 asc/desc 间切换（不回到未排序）
const sortKey = ref(null);
const sortOrder = ref(null);

// 运行时间(etime)解析为秒，用于正确排序：格式 [[dd-]hh:]mm:ss
function elapsedToSec(s) {
  if (!s) return 0;
  s = String(s).trim();
  let days = 0;
  if (s.includes('-')) {
    const [d, rest] = s.split('-');
    days = parseInt(d, 10) || 0;
    s = rest;
  }
  const t = s.split(':').map(Number);
  if (t.length === 3) return days * 86400 + t[0] * 3600 + t[1] * 60 + t[2];
  if (t.length === 2) return days * 86400 + t[0] * 60 + t[1];
  return days * 86400 + (t[0] || 0);
}

function onSorterChange(state) {
  if (!state || !state.columnKey) return;
  if (state.order) {
    sortKey.value = state.columnKey;
    sortOrder.value = state.order;
  } else {
    // 用户要求：再次点击在 asc/desc 间切换，不回到未排序
    sortKey.value = state.columnKey;
    sortOrder.value = sortOrder.value === 'ascend' ? 'descend' : 'ascend';
  }
}

const columns = [
  { title: 'PID', key: 'pid', width: 90, sorter: (a, b) => (a.pid || 0) - (b.pid || 0) },
  { title: '用户', key: 'user', width: 110, sorter: (a, b) => String(a.user || '').localeCompare(String(b.user || '')) },
  {
    title: 'CPU%', key: 'cpu', width: 96,
    sorter: (a, b) => (a.cpu || 0) - (b.cpu || 0),
    render: (r) => h(NTag, { size: 'small', type: r.cpu > 50 ? 'error' : 'default' }, { default: () => r.cpu })
  },
  {
    title: 'MEM%', key: 'mem', width: 96,
    sorter: (a, b) => (a.mem || 0) - (b.mem || 0),
    render: (r) => h(NTag, { size: 'small', type: r.mem > 50 ? 'warning' : 'default' }, { default: () => r.mem })
  },
  { title: '运行时间', key: 'elapsed', width: 140, sorter: (a, b) => elapsedToSec(a.elapsed) - elapsedToSec(b.elapsed) },
  { title: '命令', key: 'command', ellipsis: { tooltip: true }, sorter: (a, b) => String(a.command || '').localeCompare(String(b.command || '')) },
  {
    title: '操作', key: 'ops', width: 90,
    render(row) {
      return h(
        NButton,
        { size: 'tiny', quaternary: true, type: 'error', title: '结束进程', onClick: () => kill(row) },
        { icon: () => h(NIcon, null, { default: () => h(TrashOutline) }) }
      );
    }
  }
];

async function refresh(showLoading = true) {
  if (showLoading) loading.value = true;
  try {
    const { processes } = await processList(props.connId);
    items.value = processes || [];
  } catch (err) {
    message.error(errMsg(err));
  } finally {
    if (showLoading) loading.value = false;
  }
}

async function kill(row) {
  try {
    await processKill(props.connId, row.pid, 'TERM');
    message.success(`已发送 TERM 信号到 PID ${row.pid}`);
    await refresh();
  } catch (err) {
    message.error(errMsg(err));
  }
}

onMounted(() => {
  refresh();
  timer = setInterval(() => { if (autoRefresh.value) refresh(false); }, 5000);
});
onUnmounted(() => clearInterval(timer));
</script>

<style scoped>
.process-pane { display: flex; flex-direction: column; height: 100%; padding: 12px; box-sizing: border-box; }
.process-toolbar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; gap: 12px; }
.process-toolbar .tip { font-size: 12px; color: var(--text-3, #888); }
.process-toolbar .count { font-size: 12px; color: var(--text-3, #888); }
.process-spin { flex: 1; overflow: auto; }
</style>

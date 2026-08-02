<template>
  <n-modal
    :show="show"
    preset="card"
    :title="`编辑文件：${name}`"
    style="width: 82vw; height: 84vh"
    @update:show="(v) => $emit('update:show', v)"
  >
    <div class="editor-wrap">
      <VueMonacoEditor
        v-if="ready"
        v-model:value="code"
        :language="lang"
        theme="vs-dark"
        :options="{ minimap: { enabled: false }, fontSize: 13, automaticLayout: true, scrollBeyondLastLine: false }"
        height="100%"
      />
      <div v-else class="editor-loading">编辑器加载中…</div>
    </div>
    <template #footer>
      <n-space justify="end">
        <n-button @click="$emit('update:show', false)">取消</n-button>
        <n-button type="primary" :loading="saving" @click="save">保存</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup>
import { ref, watch, computed } from 'vue';
import { NModal, NButton, NSpace, useMessage } from 'naive-ui';
import { VueMonacoEditor } from '@guolao/vue-monaco-editor';
import { ensureMonaco } from '../monaco-setup.js';
import { sftpRead, sftpWrite, errMsg } from '../api.js';

const props = defineProps({
  show: Boolean,
  connId: Number,
  path: String,
  name: String
});
const emit = defineEmits(['update:show', 'saved']);
const message = useMessage();
const code = ref('');
const saving = ref(false);
// Monaco 按需加载：首次真正打开文件编辑器时才拉取 monaco-editor（数 MB），避免拖慢首屏
const ready = ref(false);

const langMap = {
  js: 'javascript', ts: 'typescript', json: 'json', html: 'html', htm: 'html', css: 'css',
  vue: 'html', py: 'python', sh: 'shell', bash: 'shell', yml: 'yaml', yaml: 'yaml',
  xml: 'xml', md: 'markdown', sql: 'sql', go: 'go', java: 'java', c: 'c', cpp: 'cpp',
  h: 'cpp', php: 'php', rb: 'ruby', rs: 'rust', kt: 'kotlin', properties: 'ini', ini: 'ini',
  conf: 'ini', log: 'plaintext', txt: 'plaintext'
};
const lang = computed(() => {
  const ext = (props.name?.split('.').pop() || '').toLowerCase();
  return langMap[ext] || 'plaintext';
});

watch(
  () => [props.show, props.path],
  async ([show]) => {
    if (show && props.path) {
      saving.value = false;
      code.value = '';
      try {
        // 先按需拉取并初始化 Monaco，再渲染编辑器（保证 loader.config 在挂载前完成）
        await ensureMonaco();
        ready.value = true;
        const res = await sftpRead(props.connId, props.path);
        code.value = res.content;
      } catch (err) {
        message.error(errMsg(err));
        emit('update:show', false);
      }
    }
  }
);

async function save() {
  saving.value = true;
  try {
    await sftpWrite(props.connId, props.path, code.value);
    message.success('已保存');
    emit('saved');
    emit('update:show', false);
  } catch (err) {
    message.error(errMsg(err));
  } finally {
    saving.value = false;
  }
}
</script>

<style scoped>
.editor-wrap {
  height: calc(84vh - 130px);
  border: 1px solid #2a2a30;
  border-radius: 6px;
  overflow: hidden;
}
.editor-loading {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-sub, #777);
  font-size: 13px;
}
</style>

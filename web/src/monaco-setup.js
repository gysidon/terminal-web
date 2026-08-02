// Monaco 按需加载封装：仅在真正打开文件编辑器时才拉取 monaco-editor（数 MB），
// 避免把它打进首屏入口 chunk。在 FileEditor.vue 中于渲染编辑器前调用 ensureMonaco()。
import { loader } from '@guolao/vue-monaco-editor';

let pending = null;

export function ensureMonaco() {
  if (!pending) {
    pending = import('monaco-editor').then((monaco) => {
      // @guolao/vue-monaco-editor 需要一份已打包的 monaco 实例（等价于 import * as monaco）。
      // 动态 import 得到的命名空间对象与静态命名空间一致，可直接交给 loader.config。
      loader.config({ monaco });
    });
  }
  return pending;
}

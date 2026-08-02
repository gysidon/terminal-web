// 跨组件轻量事件总线，零依赖。
// 用：import { bus } from './events'；bus.on('refresh-tree', fn) / bus.emit('refresh-tree')
const handlers = new Map();

export const bus = {
  on(event, fn) {
    (handlers.get(event) || handlers.set(event, []).get(event)).push(fn);
    return () => bus.off(event, fn);
  },
  emit(event, ...args) {
    (handlers.get(event) || []).forEach((fn) => fn(...args));
  },
  off(event, fn) {
    const list = handlers.get(event);
    if (list) {
      const idx = list.indexOf(fn);
      if (idx !== -1) list.splice(idx, 1);
    }
  }
};

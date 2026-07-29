import svgCaptcha from 'svg-captcha';
import { randomUUID } from 'node:crypto';

// 内存验证码缓存：id -> { text, expires }
// 单实例部署足够；若是多副本负载均衡需要共享存储（如 Redis），当前工具为单容器场景。
const cache = new Map();

const TTL = 5 * 60 * 1000; // 5 分钟有效

// 周期性清理过期验证码，避免内存泄漏
setInterval(() => {
  const now = Date.now();
  for (const [id, v] of cache) {
    if (v.expires <= now) cache.delete(id);
  }
}, 60 * 1000).unref();

export function createCaptcha() {
  const c = svgCaptcha.create({
    size: 4,
    noise: 2,
    color: true,
    background: '#f2f2f2',
    width: 120,
    height: 40,
    fontSize: 40
  });
  const id = randomUUID();
  cache.set(id, { text: c.text.toLowerCase(), expires: Date.now() + TTL });
  return { id, svg: c.data };
}

// 校验通过后立即删除，防止重放
export function verifyCaptcha(id, text) {
  if (!id || !text) return false;
  const entry = cache.get(id);
  if (!entry) return false;
  cache.delete(id);
  if (Date.now() > entry.expires) return false;
  return text.trim().toLowerCase() === entry.text;
}

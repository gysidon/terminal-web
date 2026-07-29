import { db } from './db.js';
import { getSetting } from './settings.js';

// ---------- 客户端 IP ----------
export function getClientIp(req) {
  const xff = req.headers['x-forwarded-for'];
  if (typeof xff === 'string' && xff.length) return xff.split(',')[0].trim();
  if (Array.isArray(xff) && xff.length) return String(xff[0]).trim();
  return req.ip || req.socket?.remoteAddress || 'unknown';
}

// ---------- IP 白名单 ----------
function ipToLong(ip) {
  const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(ip);
  if (!m) return null;
  return ((+m[1] << 24) >>> 0) + (+m[2] << 16) + (+m[3] << 8) + +m[4];
}

function inCidr(ip, cidr) {
  const [net, bitsStr] = cidr.split('/');
  const bits = bitsStr ? parseInt(bitsStr, 10) : 32;
  const ipLong = ipToLong(ip);
  const netLong = ipToLong(net);
  if (ipLong === null || netLong === null || bits < 0 || bits > 32) return false;
  const mask = bits === 0 ? 0 : (0xffffffff << (32 - bits)) >>> 0;
  return (ipLong & mask) === (netLong & mask);
}

// 支持：精确 IP、CIDR（如 10.0.0.0/24）、前缀（如 192.168.1.）
export function ipInWhitelist(ip, listStr) {
  if (!listStr || !listStr.trim()) return true; // 空 = 不限制
  const list = listStr.split(',').map((s) => s.trim()).filter(Boolean);
  for (const item of list) {
    if (item === ip) return true;
    if (item.endsWith('.')) {
      if (ip.startsWith(item)) return true;
    } else if (item.includes('/')) {
      if (inCidr(ip, item)) return true;
    }
  }
  return false;
}

export function isIpAllowed(req) {
  const wl = getSetting('ip_whitelist');
  if (!wl || !wl.trim()) return true;
  return ipInWhitelist(getClientIp(req), wl);
}

// ---------- 登录失败锁定（防爆破，内存态） ----------
const fails = new Map(); // username -> { count, until }

export function getLockUntil(username) {
  const e = fails.get(username);
  if (!e) return 0;
  if (e.until > Date.now()) return e.until;
  // 仅清理真正过期的锁定（until>0 且已过期）；until=0 表示尚未锁定，仅用于计数，不能删，否则失败次数无法累加
  if (e.until > 0) fails.delete(username);
  return 0;
}

export function recordFail(username) {
  const max = getSetting('login_fail_max') || 5;
  const e = fails.get(username) || { count: 0, until: 0 };
  e.count += 1;
  if (e.count >= max) {
    const lockMin = getSetting('login_lock_minutes') || 15;
    e.until = Date.now() + lockMin * 60 * 1000;
  }
  fails.set(username, e);
  return e;
}

export function clearFail(username) {
  fails.delete(username);
}

// ---------- 审计日志 ----------
export function logAudit({ action, username = '', ip = '', success = true, detail = '' }) {
  try {
    db.prepare(
      `INSERT INTO audit_logs (action, username, ip, success, detail, created_at)
       VALUES (?, ?, ?, ?, ?, datetime('now'))`
    ).run(action, username, ip, success ? 1 : 0, detail);
  } catch {
    /* 审计失败不应影响主流程 */
  }
}

export function recentAudit(limit = 100) {
  const n = Math.min(Math.max(parseInt(limit, 10) || 100, 1), 500);
  return db.prepare('SELECT * FROM audit_logs ORDER BY id DESC LIMIT ?').all(n);
}

import { db } from './db.js';

export const DEFAULT_FONT = '"JetBrains Mono", Menlo, Consolas, "Courier New", monospace';

// 设置项 schema：类型 + 默认值。新增设置只需在此登记。
const SCHEMA = {
  captcha_enabled: { type: 'bool', default: true },
  theme: { type: 'str', default: 'dark' },
  font_size: { type: 'int', default: 13 },
  font_family: { type: 'str', default: DEFAULT_FONT },
  login_fail_max: { type: 'int', default: 5 },
  login_lock_minutes: { type: 'int', default: 15 },
  ip_whitelist: { type: 'str', default: '' },
  session_timeout: { type: 'int', default: 0 },
  sso_enabled: { type: 'bool', default: false }
};

export function seedSettings() {
  const stmt = db.prepare('INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)');
  for (const [k, v] of Object.entries(SCHEMA)) {
    stmt.run(k, String(v.default));
  }
}

function coerce(type, raw) {
  if (raw === null || raw === undefined) return raw;
  if (type === 'bool') return raw === '1' || raw === 'true' || raw === true;
  if (type === 'int') return parseInt(raw, 10) || 0;
  return String(raw);
}

export function getAllSettings() {
  const rows = db.prepare('SELECT key, value FROM settings').all();
  const map = {};
  for (const r of rows) map[r.key] = r.value;
  const out = {};
  for (const [k, v] of Object.entries(SCHEMA)) {
    const raw = map[k] !== undefined ? map[k] : v.default;
    out[k] = coerce(v.type, raw);
  }
  return out;
}

export function getSetting(key) {
  const v = SCHEMA[key];
  if (!v) return undefined;
  const row = db.prepare('SELECT value FROM settings WHERE key = ?').get(key);
  return coerce(v.type, row ? row.value : v.default);
}

export function putSettings(input = {}) {
  const stmt = db.prepare(
    'INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value'
  );
  const updated = {};
  for (const [k, v] of Object.entries(SCHEMA)) {
    if (!(k in input)) continue;
    let val = input[k];
    if (v.type === 'bool') val = val ? '1' : '0';
    else if (v.type === 'int') val = String(parseInt(val, 10) || 0);
    else val = String(val ?? '');
    stmt.run(k, val);
    updated[k] = coerce(v.type, val);
  }
  return updated;
}

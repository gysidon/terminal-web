import jwt from 'jsonwebtoken';
import crypto from 'node:crypto';
import { db } from './db.js';
import { JWT_SECRET, hashPassword, verifyPassword } from './crypto.js';
import { getSetting } from './settings.js';

export function isInitialized() {
  const row = db.prepare('SELECT COUNT(*) AS c FROM users').get();
  return row.c > 0;
}

// 仅在显式设置 ADMIN_PASSWORD 环境变量时才自动创建账号（便于 Docker 无人值守部署）；
// 否则不创建固定账号，首次访问由前端引导用户自行设置管理员。
export function ensureAdmin() {
  if (!process.env.ADMIN_PASSWORD) return;
  const username = process.env.ADMIN_USERNAME || 'admin';
  const existing = db.prepare('SELECT id FROM users WHERE username = ?').get(username);
  if (!existing) {
    db.prepare('INSERT INTO users (username, password_hash) VALUES (?, ?)').run(username, hashPassword(process.env.ADMIN_PASSWORD));
    console.log(`[auth] 已通过环境变量创建管理员 ${username}`);
  }
}

// 首次初始化：仅当系统未初始化时允许创建首个管理员账号，并返回登录 token
export function setupAdmin(username, password) {
  if (isInitialized()) return null;
  db.prepare('INSERT INTO users (username, password_hash) VALUES (?, ?)').run(username, hashPassword(password));
  const uid = db.prepare('SELECT id FROM users WHERE username = ?').get(username).id;
  const jti = createSession(uid);
  return jwt.sign({ uid, username, jti }, JWT_SECRET, { expiresIn: '7d' });
}

export function login(username, password) {
  const user = db.prepare('SELECT * FROM users WHERE username = ?').get(username);
  if (!user) return null;
  if (!verifyPassword(password, user.password_hash)) return null;
  const jti = createSession(user.id);
  return jwt.sign({ uid: user.id, username: user.username, jti }, JWT_SECRET, { expiresIn: '7d' });
}

export function changePassword(uid, oldPassword, newPassword) {
  const user = db.prepare('SELECT * FROM users WHERE id = ?').get(uid);
  if (!user || !verifyPassword(oldPassword, user.password_hash)) return false;
  db.prepare('UPDATE users SET password_hash = ? WHERE id = ?').run(hashPassword(newPassword), uid);
  return true;
}

export function verifyToken(token) {
  try {
    return jwt.verify(token, JWT_SECRET);
  } catch {
    return null;
  }
}

// 创建会话并写入 sessions 表；若开启单点登录，先清除该用户所有旧会话
function createSession(uid) {
  if (getSetting('sso_enabled')) {
    db.prepare('DELETE FROM sessions WHERE uid = ?').run(uid);
  }
  const jti = crypto.randomBytes(16).toString('hex');
  db.prepare('INSERT OR REPLACE INTO sessions (jti, uid, created_at, last_activity) VALUES (?, ?, datetime(\'now\'), ?)')
    .run(jti, uid, Date.now());
  return jti;
}

// 校验会话：验证 JWT + 会话存在性 + 无活动超时
export function verifySession(token) {
  const payload = verifyToken(token);
  if (!payload) return null;
  // 向后兼容：升级前签发的旧 token（无 jti）直接放行，7 天内自然过期
  if (!payload.jti) return payload;
  const session = db.prepare('SELECT * FROM sessions WHERE jti = ?').get(payload.jti);
  if (!session) return null; // 会话不存在（被单点登录清除或已登出）
  const timeout = getSetting('session_timeout'); // 分钟，0 = 关闭
  if (timeout && timeout > 0 && Date.now() - session.last_activity > timeout * 60 * 1000) {
    db.prepare('DELETE FROM sessions WHERE jti = ?').run(payload.jti); // 清理过期会话
    return null;
  }
  return payload;
}

// 刷新会话活跃时间（REST 请求每次鉴权成功时调用；终端 WS 仅在收到 input 按键时调用）
export function touchSession(jti) {
  try {
    db.prepare('UPDATE sessions SET last_activity = ? WHERE jti = ?').run(Date.now(), jti);
  } catch {}
}

export function authHook(req, reply, done) {
  const header = req.headers.authorization || '';
  const token = header.startsWith('Bearer ') ? header.slice(7) : (req.query?.token || '');
  const payload = verifySession(token);
  if (!payload) {
    reply.code(401).send({ error: '未登录或登录已过期' });
    return;
  }
  req.user = payload;
  done();
}

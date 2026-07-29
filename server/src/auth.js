import jwt from 'jsonwebtoken';
import { db } from './db.js';
import { JWT_SECRET, hashPassword, verifyPassword } from './crypto.js';

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
  return jwt.sign({ uid, username }, JWT_SECRET, { expiresIn: '7d' });
}

export function login(username, password) {
  const user = db.prepare('SELECT * FROM users WHERE username = ?').get(username);
  if (!user) return null;
  if (!verifyPassword(password, user.password_hash)) return null;
  return jwt.sign({ uid: user.id, username: user.username }, JWT_SECRET, { expiresIn: '7d' });
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

export function authHook(req, reply, done) {
  const header = req.headers.authorization || '';
  const token = header.startsWith('Bearer ') ? header.slice(7) : (req.query?.token || '');
  const payload = verifyToken(token);
  if (!payload) {
    reply.code(401).send({ error: '未登录或登录已过期' });
    return;
  }
  req.user = payload;
  done();
}

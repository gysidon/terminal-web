import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { DATA_DIR } from './db.js';

function loadOrCreateSecret(filename, envName) {
  if (process.env[envName]) return process.env[envName];
  const file = path.join(DATA_DIR, filename);
  if (fs.existsSync(file)) return fs.readFileSync(file, 'utf8').trim();
  const secret = crypto.randomBytes(32).toString('hex');
  fs.writeFileSync(file, secret, { mode: 0o600 });
  return secret;
}

const MASTER_SECRET = loadOrCreateSecret('.enc-secret', 'ENCRYPTION_KEY');
export const JWT_SECRET = loadOrCreateSecret('.jwt-secret', 'JWT_SECRET');

const KEY = crypto.scryptSync(MASTER_SECRET, 'terminal-web-salt', 32);

// encrypt/decrypt 支持传入自定义密钥（key 默认为本机主密钥推导的 KEY），用于跨机备份 re-key
export function encrypt(plain, key = KEY) {
  if (plain === null || plain === undefined || plain === '') return null;
  const iv = crypto.randomBytes(12);
  const cipher = crypto.createCipheriv('aes-256-gcm', key, iv);
  const enc = Buffer.concat([cipher.update(String(plain), 'utf8'), cipher.final()]);
  const tag = cipher.getAuthTag();
  return Buffer.concat([iv, tag, enc]).toString('base64');
}

export function decrypt(payload, key = KEY) {
  if (!payload) return null;
  const buf = Buffer.from(payload, 'base64');
  const iv = buf.subarray(0, 12);
  const tag = buf.subarray(12, 28);
  const data = buf.subarray(28);
  const decipher = crypto.createDecipheriv('aes-256-gcm', key, iv);
  decipher.setAuthTag(tag);
  return Buffer.concat([decipher.update(data), decipher.final()]).toString('utf8');
}

// 用明文主密钥推导 32 字节密钥（备份文件携带的是主密钥字符串，跨机导入时用它对密文解密）
export function deriveKey(secret) {
  return crypto.scryptSync(secret, 'terminal-web-salt', 32);
}

// 返回本机主密钥（写入备份文件，使备份可跨机还原）
export function getMasterSecret() {
  return MASTER_SECRET;
}

export function hashPassword(password) {
  const salt = crypto.randomBytes(16).toString('hex');
  const hash = crypto.scryptSync(password, salt, 64).toString('hex');
  return `${salt}:${hash}`;
}

export function verifyPassword(password, stored) {
  const [salt, hash] = stored.split(':');
  const calc = crypto.scryptSync(password, salt, 64).toString('hex');
  return crypto.timingSafeEqual(Buffer.from(hash, 'hex'), Buffer.from(calc, 'hex'));
}

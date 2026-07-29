#!/usr/bin/env node
// 重置（或新建）用户密码的命令行工具。
//
// 用法：
//   node src/reset-password.js <用户名> <新密码>
//   node src/reset-password.js            # 也可从环境变量读取：RESET_USER / RESET_PASS
//
// Docker 场景（容器已挂载数据卷，DATA_DIR 默认 /data）：
//   docker compose exec terminal-web node src/reset-password.js admin 你的新密码
//   # 或
//   docker exec -it terminal-web node src/reset-password.js admin 你的新密码
import { db } from './db.js';
import { hashPassword } from './crypto.js';

function fail(msg) {
  console.error('✗ ' + msg);
  process.exit(1);
}

const username = (process.argv[2] || process.env.RESET_USER || '').trim();
const password = process.argv[3] || process.env.RESET_PASS || '';

if (!username) fail('请提供用户名（参数1 或 环境变量 RESET_USER）');
if (!password) fail('请提供新密码（参数2 或 环境变量 RESET_PASS）');
if (password.length < 6) fail('密码至少 6 位');

const existing = db.prepare('SELECT id FROM users WHERE username = ?').get(username);
if (existing) {
  db.prepare('UPDATE users SET password_hash = ? WHERE id = ?').run(hashPassword(password), existing.id);
  console.log(`✓ 已重置用户「${username}」的密码`);
} else {
  db.prepare('INSERT INTO users (username, password_hash) VALUES (?, ?)').run(username, hashPassword(password));
  console.log(`✓ 已新建用户「${username}」并设置密码`);
}
process.exit(0);

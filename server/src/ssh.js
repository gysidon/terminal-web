import { Client } from 'ssh2';
import { db } from './db.js';
import { decrypt } from './crypto.js';

const MAX_JUMP_DEPTH = 5;

export function getConnectionById(id) {
  return db.prepare('SELECT * FROM connections WHERE id = ?').get(id);
}

function buildAuthConfig(conn) {
  const cfg = {
    host: conn.host,
    port: conn.port || 22,
    username: conn.username,
    readyTimeout: 15000,
    keepaliveInterval: 20000,
    keepaliveCountMax: 3
  };
  // 优先使用表单传入的明文凭据（测试连接场景）；未设置时回落到数据库加密字段解密
  const password = conn._password !== undefined ? conn._password : (conn.password_enc ? decrypt(conn.password_enc) : undefined);
  const privateKey = conn._private_key !== undefined ? conn._private_key : (conn.private_key_enc ? decrypt(conn.private_key_enc) : undefined);
  const passphrase = conn._passphrase !== undefined ? conn._passphrase : (conn.passphrase_enc ? decrypt(conn.passphrase_enc) : undefined);
  if (conn.auth_type === 'key') {
    cfg.privateKey = privateKey;
    if (passphrase) cfg.passphrase = passphrase;
  } else {
    cfg.password = password;
    cfg.tryKeyboard = true;
  }
  return cfg;
}

function resolveChain(conn) {
  const chain = [conn];
  const seen = new Set([conn.id]);
  let cur = conn;
  while (cur.jump_id) {
    if (chain.length >= MAX_JUMP_DEPTH) throw new Error('跳板层级过深（最多5级）');
    const jump = getConnectionById(cur.jump_id);
    if (!jump) throw new Error('跳板机配置不存在');
    if (jump.id && seen.has(jump.id)) throw new Error('跳板配置存在循环引用');
    seen.add(jump.id);
    chain.unshift(jump);
    cur = jump;
  }
  return chain;
}

function connectOne(cfg) {
  return new Promise((resolve, reject) => {
    const client = new Client();
    let settled = false;
    client.on('ready', () => { settled = true; resolve(client); });
    client.on('error', (err) => { if (!settled) reject(err); });
    client.on('keyboard-interactive', (n, i, il, prompts, finish) => {
      finish(prompts.map(() => cfg.password || ''));
    });
    try {
      client.connect(cfg);
    } catch (err) {
      reject(err);
    }
  });
}

export async function createSshClient(conn) {
  const chain = resolveChain(conn);
  const clients = [];
  try {
    let prevClient = null;
    for (let i = 0; i < chain.length; i++) {
      const cfg = buildAuthConfig(chain[i]);
      if (prevClient) {
        const sock = await new Promise((resolve, reject) => {
          prevClient.forwardOut('127.0.0.1', 0, chain[i].host, chain[i].port || 22, (err, stream) => {
            if (err) reject(new Error(`通过跳板转发到 ${chain[i].host} 失败: ${err.message}`));
            else resolve(stream);
          });
        });
        cfg.sock = sock;
        delete cfg.host;
        delete cfg.port;
      }
      const client = await connectOne(cfg);
      clients.push(client);
      prevClient = client;
    }
    const target = clients[clients.length - 1];
    const originalEnd = target.end.bind(target);
    target.end = () => {
      for (let i = clients.length - 1; i >= 0; i--) {
        try { i === clients.length - 1 ? originalEnd() : clients[i].end(); } catch {}
      }
    };
    return target;
  } catch (err) {
    for (const c of clients) { try { c.end(); } catch {} }
    throw err;
  }
}

export async function testConnection(conn) {
  const client = await createSshClient(conn);
  client.end();
  return true;
}

// 用表单（可能尚未保存）传入的明文配置测试连接。
// 编辑已有连接且凭据字段留空时，自动从已存连接解密补全；支持跳板机（jump 走已存连接）。
export async function testConnectionConfig(cfg) {
  const id = cfg.id ? Number(cfg.id) : null;
  const stored = id ? getConnectionById(id) : null;
  const auth_type = cfg.auth_type || stored?.auth_type || 'password';
  let password = cfg.password;
  let private_key = cfg.private_key;
  let passphrase = cfg.passphrase;
  if (stored) {
    if (!password && stored.password_enc) password = decrypt(stored.password_enc);
    if (!private_key && stored.private_key_enc) private_key = decrypt(stored.private_key_enc);
    if (!passphrase && stored.passphrase_enc) passphrase = decrypt(stored.passphrase_enc);
  }
  const conn = {
    id: id ?? -1,
    host: cfg.host,
    port: cfg.port || stored?.port || 22,
    username: cfg.username,
    auth_type,
    jump_id: cfg.jump_id ?? stored?.jump_id ?? null,
    _password: password || undefined,
    _private_key: private_key || undefined,
    _passphrase: passphrase || undefined
  };
  const client = await createSshClient(conn);
  client.end();
  return true;
}

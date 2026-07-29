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
  if (conn.auth_type === 'key') {
    cfg.privateKey = decrypt(conn.private_key_enc);
    const passphrase = decrypt(conn.passphrase_enc);
    if (passphrase) cfg.passphrase = passphrase;
  } else {
    cfg.password = decrypt(conn.password_enc);
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

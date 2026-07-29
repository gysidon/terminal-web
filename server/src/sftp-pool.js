import { createSshClient, getConnectionById } from './ssh.js';

const IDLE_TIMEOUT = 5 * 60 * 1000;
const pool = new Map();

function scheduleCleanup(connId) {
  const entry = pool.get(connId);
  if (!entry) return;
  clearTimeout(entry.timer);
  entry.timer = setTimeout(() => {
    try { entry.client.end(); } catch {}
    pool.delete(connId);
  }, IDLE_TIMEOUT);
}

export async function getSftp(connId) {
  let entry = pool.get(connId);
  if (entry) {
    scheduleCleanup(connId);
    if (entry.sftp) return entry.sftp;
  } else {
    const conn = getConnectionById(connId);
    if (!conn) throw new Error('连接不存在');
    const client = await createSshClient(conn);
    entry = { client, sftp: null, timer: null };
    client.on('close', () => {
      clearTimeout(entry.timer);
      pool.delete(connId);
    });
    client.on('error', () => {
      clearTimeout(entry.timer);
      pool.delete(connId);
    });
    pool.set(connId, entry);
  }
  const sftp = await new Promise((resolve, reject) => {
    entry.client.sftp((err, s) => (err ? reject(err) : resolve(s)));
  });
  entry.sftp = sftp;
  scheduleCleanup(connId);
  return sftp;
}

// 复用同一 SSH 连接的 client（与 getSftp 共享连接池 entry），用于执行命令（进程管理、延迟探测等）
export async function getClient(connId) {
  const entry = pool.get(connId);
  if (entry) {
    scheduleCleanup(connId);
    return entry.client;
  }
  const conn = getConnectionById(connId);
  if (!conn) throw new Error('连接不存在');
  const client = await createSshClient(conn);
  const newEntry = { client, sftp: null, timer: null };
  client.on('close', () => {
    clearTimeout(newEntry.timer);
    pool.delete(connId);
  });
  client.on('error', () => {
    clearTimeout(newEntry.timer);
    pool.delete(connId);
  });
  pool.set(connId, newEntry);
  scheduleCleanup(connId);
  return client;
}

export function dropSftp(connId) {
  const entry = pool.get(connId);
  if (entry) {
    clearTimeout(entry.timer);
    try { entry.client.end(); } catch {}
    pool.delete(connId);
  }
}

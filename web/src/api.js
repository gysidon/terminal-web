import axios from 'axios';

export const TOKEN_KEY = 'tw_token';

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || '';
}

export function setToken(token) {
  if (token) localStorage.setItem(TOKEN_KEY, token);
  else localStorage.removeItem(TOKEN_KEY);
}

export const api = axios.create({ baseURL: '/' });

api.interceptors.request.use((config) => {
  const token = getToken();
  if (token) config.headers.Authorization = `Bearer ${token}`;
  return config;
});

api.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401) {
      setToken('');
      window.dispatchEvent(new CustomEvent('tw-unauthorized'));
    }
    return Promise.reject(err);
  }
);

export function errMsg(err) {
  return err?.response?.data?.error || err?.message || '请求失败';
}

export async function setupStatus() {
  const { data } = await api.get('/api/setup/status');
  return data.initialized;
}

export async function setup(username, password) {
  const { data } = await api.post('/api/setup', { username, password });
  setToken(data.token);
  return data;
}

export async function getCaptcha() {
  const { data } = await api.get('/api/captcha');
  return data; // { id, svg }
}

export async function sftpRead(connId, path) {
  const { data } = await api.get(`/api/sftp/${connId}/read`, { params: { path } });
  return data;
}

export async function sftpWrite(connId, path, content) {
  const { data } = await api.post(`/api/sftp/${connId}/write`, { path, content });
  return data;
}

export async function processList(connId) {
  const { data } = await api.get(`/api/process/${connId}`);
  return data;
}

export async function processKill(connId, pid, signal = 'TERM') {
  const { data } = await api.post(`/api/process/${connId}/kill`, { pid, signal });
  return data;
}

export async function pingConn(connId) {
  const { data } = await api.get(`/api/connections/${connId}/ping`);
  return data;
}

// 用表单（可能尚未保存）明文配置测试连接
export async function testConnConfig(cfg) {
  const { data } = await api.post('/api/connections/test', cfg);
  return data;
}

export async function sysInfo(connId) {
  const { data } = await api.get(`/api/sysinfo/${connId}`);
  return data;
}

// ===== 备份与恢复 =====
export async function createBackup() {
  const { data } = await api.post('/api/backup/create');
  return data;
}

export async function listBackups() {
  const { data } = await api.get('/api/backup/list');
  return data;
}

// 触发浏览器下载（带 Authorization 头，用 blob 方式）
export async function downloadBackup(id, filename) {
  const { data } = await api.get(`/api/backup/download/${id}`, { responseType: 'blob' });
  const url = URL.createObjectURL(data);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename || `${id}.json`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export async function deleteBackup(id) {
  const { data } = await api.delete(`/api/backup/${id}`);
  return data;
}

// 导入外部备份文件（multipart；不显式设置 Content-Type 以保留 axios 自动带的 boundary）
export async function importBackup(file) {
  const fd = new FormData();
  fd.append('file', file);
  const { data } = await api.post('/api/backup/import', fd);
  return data;
}

export async function restoreBackup(id) {
  const { data } = await api.post(`/api/backup/restore/${id}`);
  return data;
}

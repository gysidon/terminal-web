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

export async function sysInfo(connId) {
  const { data } = await api.get(`/api/sysinfo/${connId}`);
  return data;
}

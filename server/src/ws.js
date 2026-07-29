import { verifySession, touchSession } from './auth.js';
import { createSshClient, getConnectionById } from './ssh.js';

export default async function wsRoutes(app) {
  app.get('/ws/terminal/:connId', { websocket: true }, (socket, req) => {
    const token = req.query?.token;
    const payload = token ? verifySession(token) : null;
    if (!payload) {
      socket.send(JSON.stringify({ type: 'error', message: '未登录或登录已过期' }));
      socket.close();
      return;
    }

    const conn = getConnectionById(Number(req.params.connId));
    if (!conn) {
      socket.send(JSON.stringify({ type: 'error', message: '连接配置不存在' }));
      socket.close();
      return;
    }

    let client = null;
    let stream = null;
    let closed = false;

    const send = (obj) => {
      if (socket.readyState === 1) socket.send(JSON.stringify(obj));
    };

    send({ type: 'status', message: `正在连接 ${conn.username}@${conn.host}:${conn.port} ...` });

    createSshClient(conn)
      .then((c) => {
        if (closed) { c.end(); return; }
        client = c;
        c.shell({ term: 'xterm-256color', cols: 80, rows: 24 }, (err, s) => {
          if (err) {
            send({ type: 'error', message: `打开终端失败: ${err.message}` });
            socket.close();
            return;
          }
          stream = s;
          send({ type: 'connected' });
          s.on('data', (data) => {
            if (socket.readyState === 1) socket.send(JSON.stringify({ type: 'data', data: data.toString('base64') }));
          });
          s.stderr?.on('data', (data) => {
            if (socket.readyState === 1) socket.send(JSON.stringify({ type: 'data', data: data.toString('base64') }));
          });
          s.on('close', () => {
            send({ type: 'exit' });
            socket.close();
          });
        });
        c.on('error', (e) => {
          send({ type: 'error', message: `SSH 错误: ${e.message}` });
        });
        c.on('close', () => {
          send({ type: 'exit' });
          if (socket.readyState === 1) socket.close();
        });
      })
      .catch((err) => {
        send({ type: 'error', message: `连接失败: ${err.message}` });
        socket.close();
      });

    socket.on('message', (raw) => {
      let msg;
      try { msg = JSON.parse(raw.toString()); } catch { return; }
      if (msg.type === 'input' && stream) {
        if (payload.jti) touchSession(payload.jti); // 严格超时：仅真实按键刷新活跃时间
        stream.write(Buffer.from(msg.data, 'base64'));
      } else if (msg.type === 'resize' && stream) {
        stream.setWindow(msg.rows, msg.cols, 0, 0);
      } else if (msg.type === 'ping') {
        send({ type: 'pong' });
      }
    });

    socket.on('close', () => {
      closed = true;
      try { stream?.end(); } catch {}
      try { client?.end(); } catch {}
    });
  });
}

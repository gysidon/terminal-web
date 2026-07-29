import { authHook } from '../auth.js';
import { getClient } from '../sftp-pool.js';

// 解析 `ps` 输出为结构化进程列表
function parsePs(out) {
  const lines = out.split('\n').filter(Boolean);
  if (lines.length < 2) return [];
  const header = lines[0].trim().split(/\s+/);
  const pidIdx = header.indexOf('PID');
  const userIdx = header.indexOf('USER');
  const cpuIdx = header.indexOf('%CPU');
  const memIdx = header.indexOf('%MEM');
  const etimeIdx = header.indexOf('ELAPSED');
  const commIdx = header.indexOf('COMMAND');
  const procs = [];
  for (let i = 1; i < lines.length; i++) {
    const cols = lines[i].trim().split(/\s+/);
    const comm = commIdx >= 0 ? cols.slice(commIdx).join(' ') : cols[cols.length - 1];
    procs.push({
      pid: pidIdx >= 0 ? Number(cols[pidIdx]) : Number(cols[0]),
      user: userIdx >= 0 ? cols[userIdx] : cols[1],
      cpu: cpuIdx >= 0 ? parseFloat(cols[cpuIdx]) || 0 : 0,
      mem: memIdx >= 0 ? parseFloat(cols[memIdx]) || 0 : 0,
      elapsed: etimeIdx >= 0 ? cols[etimeIdx] : '',
      command: comm
    });
  }
  return procs;
}

export default async function (app) {
  app.register(async (secured) => {
    secured.addHook('preHandler', authHook);

    secured.get('/api/process/:connId', async (req, reply) => {
      const connId = Number(req.params.connId);
      try {
        const client = await getClient(connId);
        const out = await new Promise((resolve, reject) => {
          client.exec(
            "ps -eo pid,user,%cpu,%mem,etime,args --sort=-%cpu | head -n 200",
            (err, stream) => {
              if (err) return reject(err);
              let buf = '';
              stream.on('data', (d) => (buf += d.toString('utf8')));
              stream.stderr.on('data', (d) => (buf += d.toString('utf8')));
              stream.on('close', () => resolve(buf));
            }
          );
        });
        return { processes: parsePs(out) };
      } catch (err) {
        return reply.code(400).send({ error: err.message });
      }
    });

    secured.post('/api/process/:connId/kill', async (req, reply) => {
      const connId = Number(req.params.connId);
      const pid = Number(req.body?.pid);
      const signal = req.body?.signal || 'TERM';
      if (!pid) return reply.code(400).send({ error: '缺少 pid' });
      try {
        const client = await getClient(connId);
        await new Promise((resolve, reject) => {
          client.exec(`kill -${signal} ${pid}`, (err, stream) => {
            if (err) return reject(err);
            stream.on('data', () => {});
            stream.stderr.on('data', () => {});
            stream.on('close', (code) => resolve(code));
          });
        });
        return { ok: true };
      } catch (err) {
        return reply.code(400).send({ error: err.message });
      }
    });
  });
}

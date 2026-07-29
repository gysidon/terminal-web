import path from 'node:path/posix';
import { authHook } from '../auth.js';
import { getSftp } from '../sftp-pool.js';

function normalize(p) {
  if (!p || typeof p !== 'string') return '/';
  const n = path.normalize(p);
  return n.startsWith('/') ? n : '/' + n;
}

function statToItem(name, attrs, dir) {
  const isDir = (attrs.mode & 0o170000) === 0o040000;
  const isLink = (attrs.mode & 0o170000) === 0o120000;
  return {
    name,
    path: path.join(dir, name),
    type: isDir ? 'dir' : isLink ? 'link' : 'file',
    size: attrs.size,
    mode: '0' + (attrs.mode & 0o777).toString(8),
    mtime: attrs.mtime * 1000
  };
}

export default async function sftpRoutes(app) {
  app.register(async (secured) => {
    secured.addHook('preHandler', authHook);

    secured.get('/api/sftp/:connId/list', async (req, reply) => {
      const dir = normalize(req.query.path || '/');
      try {
        const sftp = await getSftp(Number(req.params.connId));
        const list = await new Promise((resolve, reject) => {
          sftp.readdir(dir, (err, l) => (err ? reject(err) : resolve(l)));
        });
        const items = list.map((e) => statToItem(e.filename, e.attrs, dir))
          .sort((a, b) => (a.type === 'dir' ? 0 : 1) - (b.type === 'dir' ? 0 : 1) || a.name.localeCompare(b.name));
        return { path: dir, items };
      } catch (err) {
        return reply.code(400).send({ error: err.message });
      }
    });

    secured.get('/api/sftp/:connId/download', async (req, reply) => {
      const file = normalize(req.query.path);
      try {
        const sftp = await getSftp(Number(req.params.connId));
        const stream = sftp.createReadStream(file);
        reply.header('Content-Disposition', `attachment; filename*=UTF-8''${encodeURIComponent(path.basename(file))}`);
        reply.header('Content-Type', 'application/octet-stream');
        return reply.send(stream);
      } catch (err) {
        return reply.code(400).send({ error: err.message });
      }
    });

    secured.get('/api/sftp/:connId/read', async (req, reply) => {
      const file = normalize(req.query.path);
      const MAX = 2 * 1024 * 1024; // 在线编辑仅支持 2MB 以内的文本文件
      try {
        const sftp = await getSftp(Number(req.params.connId));
        const chunks = [];
        let total = 0;
        const content = await new Promise((resolve, reject) => {
          const rs = sftp.createReadStream(file);
          rs.on('data', (c) => {
            total += c.length;
            if (total > MAX) { rs.destroy(); reject(new Error('文件过大，请下载后编辑（上限 2MB）')); }
            else chunks.push(c);
          });
          rs.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
          rs.on('error', reject);
        });
        return { path: file, content };
      } catch (err) {
        return reply.code(400).send({ error: err.message });
      }
    });

    secured.post('/api/sftp/:connId/write', async (req, reply) => {
      const file = normalize(req.body?.path);
      const content = req.body?.content ?? '';
      try {
        const sftp = await getSftp(Number(req.params.connId));
        await new Promise((resolve, reject) => {
          const ws = sftp.createWriteStream(file);
          ws.on('close', resolve);
          ws.on('error', reject);
          ws.end(Buffer.from(content, 'utf8'));
        });
        return { ok: true };
      } catch (err) {
        return reply.code(400).send({ error: err.message });
      }
    });

    secured.post('/api/sftp/:connId/upload', async (req, reply) => {
      const dir = normalize(req.query.path || '/');
      try {
        const sftp = await getSftp(Number(req.params.connId));
        const parts = req.files();
        const uploaded = [];
        for await (const part of parts) {
          const target = path.join(dir, part.filename);
          await new Promise((resolve, reject) => {
            const ws = sftp.createWriteStream(target);
            ws.on('close', resolve);
            ws.on('error', reject);
            part.file.on('error', reject);
            part.file.pipe(ws);
          });
          uploaded.push(target);
        }
        return { ok: true, uploaded };
      } catch (err) {
        return reply.code(400).send({ error: err.message });
      }
    });

    secured.post('/api/sftp/:connId/mkdir', async (req, reply) => {
      const target = normalize(req.body?.path);
      try {
        const sftp = await getSftp(Number(req.params.connId));
        await new Promise((resolve, reject) => sftp.mkdir(target, (err) => (err ? reject(err) : resolve())));
        return { ok: true };
      } catch (err) {
        return reply.code(400).send({ error: err.message });
      }
    });

    secured.post('/api/sftp/:connId/rename', async (req, reply) => {
      const from = normalize(req.body?.from);
      const to = normalize(req.body?.to);
      try {
        const sftp = await getSftp(Number(req.params.connId));
        await new Promise((resolve, reject) => sftp.rename(from, to, (err) => (err ? reject(err) : resolve())));
        return { ok: true };
      } catch (err) {
        return reply.code(400).send({ error: err.message });
      }
    });

    secured.post('/api/sftp/:connId/delete', async (req, reply) => {
      const target = normalize(req.body?.path);
      const isDir = !!req.body?.isDir;
      try {
        const sftp = await getSftp(Number(req.params.connId));
        if (isDir) {
          await new Promise((resolve, reject) => sftp.rmdir(target, (err) => (err ? reject(err) : resolve())));
        } else {
          await new Promise((resolve, reject) => sftp.unlink(target, (err) => (err ? reject(err) : resolve())));
        }
        return { ok: true };
      } catch (err) {
        return reply.code(400).send({ error: err.message });
      }
    });
  });
}

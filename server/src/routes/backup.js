import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { db, BACKUP_DIR } from '../db.js';
import { getMasterSecret, deriveKey, encrypt, decrypt } from '../crypto.js';
import { authHook } from '../auth.js';

// 跨机 re-key：备份密文用备份机主密钥加密；导入时用备份密钥解密再用本机密钥重写。
// 同机（主密钥一致）则原样保留密文。
function rekey(payload, backupSecret) {
  if (!payload) return null;
  if (backupSecret === getMasterSecret()) return payload;
  const plain = decrypt(payload, deriveKey(backupSecret));
  return encrypt(plain);
}

// 事务内替换全部数据（连接/分组/设置），凭据按需 re-key
function applyBackup(obj) {
  const data = obj.data || {};
  const tx = db.transaction(() => {
    db.prepare('DELETE FROM connections').run();
    db.prepare('DELETE FROM folders').run();
    db.prepare('DELETE FROM settings').run();

    const insF = db.prepare('INSERT INTO folders (id, name, parent_id, sort_order, created_at) VALUES (?, ?, ?, ?, ?)');
    for (const f of data.folders || []) {
      insF.run(f.id, f.name, f.parent_id ?? null, f.sort_order ?? 0, f.created_at ?? null);
    }

    const insS = db.prepare('INSERT INTO settings (key, value) VALUES (?, ?)');
    for (const s of data.settings || []) {
      insS.run(s.key, s.value);
    }

    const insC = db.prepare(`INSERT INTO connections
      (id, name, host, port, username, auth_type, password_enc, private_key_enc, passphrase_enc,
       folder_id, jump_id, remark, sort_order, created_at, updated_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`);
    for (const c of data.connections || []) {
      insC.run(
        c.id, c.name, c.host, c.port, c.username, c.auth_type,
        rekey(c.password_enc, obj.secret),
        rekey(c.private_key_enc, obj.secret),
        rekey(c.passphrase_enc, obj.secret),
        c.folder_id ?? null, c.jump_id ?? null, c.remark ?? null,
        c.sort_order ?? 0, c.created_at ?? null, c.updated_at ?? null
      );
    }
  });
  tx();
}

export default async function backupRoutes(app) {
  // 与 api 路由一致，整体挂在登录鉴权下
  app.register(async (secured) => {
    secured.addHook('preHandler', authHook);

    // 创建备份：生成 JSON 落服务端，记录到 backups 表
    secured.post('/api/backup/create', async () => {
      const connections = db.prepare('SELECT * FROM connections').all();
      const folders = db.prepare('SELECT * FROM folders').all();
      const settings = db.prepare('SELECT key, value FROM settings').all();
      const obj = {
        meta: { app: 'terminal-web', version: 1, exportedAt: new Date().toISOString() },
        secret: getMasterSecret(),
        data: { folders, connections, settings }
      };
      const id = crypto.randomBytes(8).toString('hex');
      const stamp = new Date().toISOString().slice(0, 10).replace(/-/g, '');
      const filename = `terminal-web-backup-${stamp}.json`;
      const filePath = path.join(BACKUP_DIR, `${id}.json`);
      fs.writeFileSync(filePath, JSON.stringify(obj, null, 2), { mode: 0o600 });
      const size = fs.statSync(filePath).size;
      db.prepare('INSERT INTO backups (id, filename, created_at, size, meta) VALUES (?, ?, datetime(\'now\'), ?, ?)')
        .run(id, filename, size, JSON.stringify({ connections: connections.length, folders: folders.length }));
      return { id, filename, size };
    });

    // 备份列表
    secured.get('/api/backup/list', async () =>
      db.prepare('SELECT id, filename, created_at, size FROM backups ORDER BY created_at DESC').all()
    );

    // 下载单个备份文件
    secured.get('/api/backup/download/:id', async (req, reply) => {
      const rec = db.prepare('SELECT * FROM backups WHERE id = ?').get(req.params.id);
      if (!rec) return reply.code(404).send({ error: '备份不存在' });
      const filePath = path.join(BACKUP_DIR, `${rec.id}.json`);
      if (!fs.existsSync(filePath)) return reply.code(404).send({ error: '备份文件已丢失' });
      const content = fs.readFileSync(filePath);
      reply.header('Content-Disposition', `attachment; filename="${rec.filename}"`);
      reply.type('application/json');
      reply.send(content);
    });

    // 删除备份（记录 + 文件）
    secured.delete('/api/backup/:id', async (req) => {
      const rec = db.prepare('SELECT * FROM backups WHERE id = ?').get(req.params.id);
      if (rec) {
        db.prepare('DELETE FROM backups WHERE id = ?').run(req.params.id);
        try { fs.unlinkSync(path.join(BACKUP_DIR, `${rec.id}.json`)); } catch {}
      }
      return { ok: true };
    });

    // 从服务端已有备份恢复（替换全部 + re-key）
    secured.post('/api/backup/restore/:id', async (req, reply) => {
      const rec = db.prepare('SELECT * FROM backups WHERE id = ?').get(req.params.id);
      if (!rec) return reply.code(404).send({ error: '备份不存在' });
      const filePath = path.join(BACKUP_DIR, `${rec.id}.json`);
      if (!fs.existsSync(filePath)) return reply.code(404).send({ error: '备份文件已丢失' });
      let obj;
      try { obj = JSON.parse(fs.readFileSync(filePath, 'utf8')); } catch { return reply.code(500).send({ error: '读取备份失败' }); }
      try { applyBackup(obj); } catch (e) { return reply.code(500).send({ error: '恢复失败: ' + e.message }); }
      return { ok: true };
    });

    // 导入外部备份文件（multipart，替换全部 + re-key）
    secured.post('/api/backup/import', async (req, reply) => {
      let file;
      try { file = await req.file(); } catch { return reply.code(400).send({ error: '未收到文件' }); }
      if (!file) return reply.code(400).send({ error: '未收到文件' });
      const content = await file.toBuffer();
      let obj;
      try { obj = JSON.parse(content.toString('utf8')); } catch { return reply.code(400).send({ error: '文件不是合法的备份 JSON' }); }
      if (!obj || !obj.data) return reply.code(400).send({ error: '备份文件格式不正确' });
      try { applyBackup(obj); } catch (e) { return reply.code(500).send({ error: '导入失败: ' + e.message }); }
      return { ok: true };
    });
  });
}

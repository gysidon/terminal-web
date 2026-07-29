import { db } from '../db.js';
import { encrypt } from '../crypto.js';
import { login, changePassword, authHook, isInitialized, setupAdmin } from '../auth.js';
import { createCaptcha, verifyCaptcha } from '../captcha.js';
import { getAllSettings, getSetting, putSettings } from '../settings.js';
import { getClientIp, getLockUntil, recordFail, clearFail, logAudit, recentAudit, isIpAllowed } from '../security.js';
import { testConnection, getConnectionById, testConnectionConfig } from '../ssh.js';
import { dropSftp, getClient } from '../sftp-pool.js';

function sanitizeConnection(row) {
  if (!row) return row;
  const { password_enc, private_key_enc, passphrase_enc, ...rest } = row;
  return {
    ...rest,
    has_password: !!password_enc,
    has_private_key: !!private_key_enc,
    has_passphrase: !!passphrase_enc
  };
}

export default async function apiRoutes(app) {
  app.get('/api/setup/status', async () => ({ initialized: isInitialized() }));

  app.post('/api/setup', async (req, reply) => {
    if (isInitialized()) return reply.code(403).send({ error: '系统已初始化，请直接登录' });
    const { username, password } = req.body || {};
    if (!username?.trim() || !password) return reply.code(400).send({ error: '请输入用户名和密码' });
    if (password.length < 6) return reply.code(400).send({ error: '密码至少 6 位' });
    const token = setupAdmin(username.trim(), password);
    if (!token) return reply.code(403).send({ error: '初始化失败' });
    return { token, username: username.trim() };
  });

  // 公开：获取登录验证码（svg）。仅登录时使用，首次设置页不校验验证码。
  app.get('/api/captcha', async () => createCaptcha());

  // 公开：登录所需的前置设置（非敏感）。前端据此决定是否展示验证码、应用主题。
  app.get('/api/settings/public', async () => {
    const s = getAllSettings();
    return {
      captchaEnabled: s.captcha_enabled,
      theme: s.theme,
      fontFamily: s.font_family,
      fontSize: s.font_size
    };
  });

  app.post('/api/login', async (req, reply) => {
    const { username, password, captchaId, captchaText } = req.body || {};
    if (!username || !password) return reply.code(400).send({ error: '请输入用户名和密码' });

    const ip = getClientIp(req);
    const lockUntil = getLockUntil(username);
    if (lockUntil) {
      const mins = Math.ceil((lockUntil - Date.now()) / 60000);
      logAudit({ action: 'login', username, ip, success: false, detail: '账户已锁定' });
      return reply.code(429).send({ error: `账户已锁定，请于 ${mins} 分钟后再试` });
    }

    const s = getAllSettings();
    if (s.captcha_enabled && !verifyCaptcha(captchaId, captchaText)) {
      return reply.code(400).send({ error: '验证码错误' });
    }

    const token = login(username, password);
    if (!token) {
      recordFail(username);
      logAudit({ action: 'login', username, ip, success: false, detail: '密码错误' });
      return reply.code(401).send({ error: '用户名或密码错误' });
    }
    clearFail(username);
    logAudit({ action: 'login', username, ip, success: true, detail: '' });
    return { token, username };
  });

  app.register(async (secured) => {
    secured.addHook('preHandler', authHook);

    secured.get('/api/settings', async () => getAllSettings());

    secured.put('/api/settings', async (req, reply) => {
      const body = req.body || {};
      const updated = putSettings(body);
      return updated;
    });

    secured.get('/api/audit', async (req) => {
      const limit = Number(req.query?.limit) || 100;
      return recentAudit(limit);
    });

    secured.post('/api/change-password', async (req, reply) => {
      const { oldPassword, newPassword } = req.body || {};
      if (!newPassword || newPassword.length < 6) return reply.code(400).send({ error: '新密码至少6位' });
      const ok = changePassword(req.user.uid, oldPassword, newPassword);
      if (!ok) return reply.code(400).send({ error: '原密码错误' });
      return { ok: true };
    });

    secured.get('/api/folders', async () => {
      return db.prepare('SELECT * FROM folders ORDER BY sort_order, name').all();
    });

    secured.post('/api/folders', async (req, reply) => {
      const { name, parent_id } = req.body || {};
      if (!name?.trim()) return reply.code(400).send({ error: '文件夹名称不能为空' });
      const r = db.prepare('INSERT INTO folders (name, parent_id) VALUES (?, ?)').run(name.trim(), parent_id || null);
      return db.prepare('SELECT * FROM folders WHERE id = ?').get(r.lastInsertRowid);
    });

    secured.put('/api/folders/:id', async (req, reply) => {
      const { name, parent_id } = req.body || {};
      const id = Number(req.params.id);
      if (parent_id === id) return reply.code(400).send({ error: '不能移动到自身' });
      db.prepare('UPDATE folders SET name = COALESCE(?, name), parent_id = ? WHERE id = ?')
        .run(name?.trim() || null, parent_id ?? null, id);
      return db.prepare('SELECT * FROM folders WHERE id = ?').get(id);
    });

    secured.delete('/api/folders/:id', async (req) => {
      db.prepare('DELETE FROM folders WHERE id = ?').run(Number(req.params.id));
      return { ok: true };
    });

    secured.get('/api/connections', async () => {
      return db.prepare('SELECT * FROM connections ORDER BY sort_order, name').all().map(sanitizeConnection);
    });

    secured.post('/api/connections', async (req, reply) => {
      const b = req.body || {};
      if (!b.name?.trim() || !b.host?.trim() || !b.username?.trim()) {
        return reply.code(400).send({ error: '名称、主机、用户名为必填项' });
      }
      let passwordEnc = encrypt(b.password ?? '');
      let keyEnc = encrypt(b.private_key ?? '');
      let passphraseEnc = encrypt(b.passphrase ?? '');
      // 从已有连接复制时，未填写新凭据则复用其加密凭据（凭据不离开服务端、不暴露明文）
      if (b.copy_from != null) {
        const src = getConnectionById(Number(b.copy_from));
        if (!src) return reply.code(404).send({ error: '复制源连接不存在' });
        if (!b.password) passwordEnc = src.password_enc;
        if (!b.private_key) keyEnc = src.private_key_enc;
        if (!b.passphrase) passphraseEnc = src.passphrase_enc;
      }
      const r = db.prepare(`INSERT INTO connections
        (name, host, port, username, auth_type, password_enc, private_key_enc, passphrase_enc, folder_id, jump_id, remark)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
        .run(
          b.name.trim(), b.host.trim(), Number(b.port) || 22, b.username.trim(),
          b.auth_type === 'key' ? 'key' : 'password',
          passwordEnc, keyEnc, passphraseEnc,
          b.folder_id || null, b.jump_id || null, b.remark || null
        );
      return sanitizeConnection(getConnectionById(r.lastInsertRowid));
    });

    secured.put('/api/connections/:id', async (req, reply) => {
      const id = Number(req.params.id);
      const old = getConnectionById(id);
      if (!old) return reply.code(404).send({ error: '连接不存在' });
      const b = req.body || {};
      if (b.jump_id === id) return reply.code(400).send({ error: '跳板机不能选择自己' });
      const passwordEnc = b.password !== undefined ? encrypt(b.password) : old.password_enc;
      const keyEnc = b.private_key !== undefined ? encrypt(b.private_key) : old.private_key_enc;
      const passphraseEnc = b.passphrase !== undefined ? encrypt(b.passphrase) : old.passphrase_enc;
      db.prepare(`UPDATE connections SET
        name = ?, host = ?, port = ?, username = ?, auth_type = ?,
        password_enc = ?, private_key_enc = ?, passphrase_enc = ?,
        folder_id = ?, jump_id = ?, remark = ?, updated_at = datetime('now')
        WHERE id = ?`)
        .run(
          b.name?.trim() || old.name, b.host?.trim() || old.host, Number(b.port) || old.port,
          b.username?.trim() || old.username, b.auth_type === 'key' ? 'key' : 'password',
          passwordEnc, keyEnc, passphraseEnc,
          b.folder_id !== undefined ? (b.folder_id || null) : old.folder_id,
          b.jump_id !== undefined ? (b.jump_id || null) : old.jump_id,
          b.remark !== undefined ? b.remark : old.remark,
          id
        );
      dropSftp(id);
      return sanitizeConnection(getConnectionById(id));
    });

    secured.delete('/api/connections/:id', async (req) => {
      const id = Number(req.params.id);
      dropSftp(id);
      db.prepare('UPDATE connections SET jump_id = NULL WHERE jump_id = ?').run(id);
      db.prepare('DELETE FROM connections WHERE id = ?').run(id);
      return { ok: true };
    });

    secured.post('/api/connections/:id/test', async (req, reply) => {
      const conn = getConnectionById(Number(req.params.id));
      if (!conn) return reply.code(404).send({ error: '连接不存在' });
      try {
        await testConnection(conn);
        return { ok: true };
      } catch (err) {
        return reply.code(400).send({ error: `连接失败: ${err.message}` });
      }
    });

    // 用表单（可能尚未保存）明文配置测试连接，不依赖已落库凭据
    secured.post('/api/connections/test', async (req, reply) => {
      const cfg = req.body || {};
      if (!cfg.host || !cfg.username) return reply.code(400).send({ error: '请填写主机和用户名' });
      try {
        await testConnectionConfig(cfg);
        return { ok: true };
      } catch (err) {
        return reply.code(400).send({ error: `连接失败: ${err.message}` });
      }
    });

    // 探测 SSH 往返延迟（执行一个无副作用的快命令并测量耗时）
    secured.get('/api/connections/:id/ping', async (req, reply) => {
      const connId = Number(req.params.id);
      const conn = getConnectionById(connId);
      if (!conn) return reply.code(404).send({ error: '连接不存在' });
      try {
        const client = await getClient(connId);
        const latency = await new Promise((resolve, reject) => {
          const t0 = Date.now();
          client.exec('true', (err, stream) => {
            if (err) return reject(err);
            stream.on('data', () => {});
            stream.stderr.on('data', () => {});
            stream.on('close', () => resolve(Date.now() - t0));
          });
        });
        return { latency };
      } catch (err) {
        return reply.code(400).send({ error: `探测失败: ${err.message}` });
      }
    });
  });
}

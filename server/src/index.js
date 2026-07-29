import Fastify from 'fastify';
import fastifyStatic from '@fastify/static';
import fastifyWebsocket from '@fastify/websocket';
import fastifyMultipart from '@fastify/multipart';
import path from 'node:path';
import fs from 'node:fs';
import { fileURLToPath } from 'node:url';
import { ensureAdmin } from './auth.js';
import { seedSettings } from './settings.js';
import { isIpAllowed } from './security.js';
import apiRoutes from './routes/api.js';
import sftpRoutes from './routes/sftp.js';
import processRoutes from './routes/process.js';
import sysinfoRoutes from './routes/sysinfo.js';
import wsRoutes from './ws.js';
import backupRoutes from './routes/backup.js';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PORT = Number(process.env.PORT) || 3000;
const HOST = process.env.HOST || '0.0.0.0';

const app = Fastify({ logger: { level: 'info' }, bodyLimit: 2 * 1024 * 1024 * 1024 });

await app.register(fastifyWebsocket);
await app.register(fastifyMultipart, {
  limits: { fileSize: 2 * 1024 * 1024 * 1024 }
});

ensureAdmin();
seedSettings();

// IP 白名单：对所有 API 请求生效（公开接口除外）。未配置则放行全部。
const PUBLIC_PATHS = new Set(['/api/setup/status', '/api/setup', '/api/captcha', '/api/settings/public']);
app.addHook('onRequest', (req, reply, done) => {
  const url = req.url || '';
  const path = url.split('?')[0];
  if (!path.startsWith('/api')) return done(); // 静态资源(Hash 路由)不限制
  if (PUBLIC_PATHS.has(path)) return done();
  if (isIpAllowed(req)) return done();
  reply.code(403).send({ error: '当前 IP 不在允许访问的列表中' });
});


await app.register(apiRoutes);
await app.register(sftpRoutes);
await app.register(processRoutes);
await app.register(sysinfoRoutes);
await app.register(wsRoutes);
await app.register(backupRoutes);

const staticDir = process.env.STATIC_DIR || path.resolve(__dirname, '../public');
if (fs.existsSync(staticDir)) {
  await app.register(fastifyStatic, { root: staticDir });
  app.setNotFoundHandler((req, reply) => {
    if (req.raw.url?.startsWith('/api') || req.raw.url?.startsWith('/ws')) {
      reply.code(404).send({ error: 'Not Found' });
    } else {
      reply.sendFile('index.html');
    }
  });
}

app.listen({ port: PORT, host: HOST }).then(() => {
  console.log(`[terminal-web] listening on http://${HOST}:${PORT}`);
});

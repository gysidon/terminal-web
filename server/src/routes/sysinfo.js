import { authHook } from '../auth.js';
import { getClient } from '../sftp-pool.js';

// 在远端执行命令，带超时；失败/超时返回 null
function execCmd(client, cmd, timeoutMs = 8000) {
  return new Promise((resolve) => {
    let done = false;
    const timer = setTimeout(() => {
      if (!done) {
        done = true;
        resolve(null);
      }
    }, timeoutMs);
    client.exec(cmd, (err, stream) => {
      if (err) {
        clearTimeout(timer);
        return resolve(null);
      }
      let buf = '';
      stream.on('data', (d) => (buf += d.toString('utf8')));
      stream.stderr.on('data', (d) => (buf += d.toString('utf8')));
      stream.on('close', () => {
        if (!done) {
          done = true;
          clearTimeout(timer);
          resolve(buf);
        }
      });
    });
  });
}

function num(s) {
  const n = parseFloat(String(s).replace(/,/g, ''));
  return Number.isFinite(n) ? n : null;
}

// CPU 占用率：采样 /proc/stat 两次（0.6s 间隔）算空闲差得到占用
const CPU_CMD =
  "R1=$(awk '/^cpu /{print $2+$3+$4+$5}' /proc/stat); I1=$(awk '/^cpu /{print $5}' /proc/stat); " +
  "sleep 0.6; " +
  "R2=$(awk '/^cpu /{print $2+$3+$4+$5}' /proc/stat); I2=$(awk '/^cpu /{print $5}' /proc/stat); " +
  "D=$((R2-R1)); DI=$((I2-I1)); if [ \"$D\" -gt 0 ]; then echo $((100*(D-DI)/D)); else echo 0; fi";
const MEM_CMD = "free -b | awk '/^Mem:/{print $2, $3}'";
const LOAD_CMD = "awk '{print $1, $2, $3}' /proc/loadavg; nproc";
// 各分区占用：排除伪文件系统，输出 mount|size(1K)|used(1K)|percent
const DISK_CMD =
  "df -PT 2>/dev/null | awk 'NR>1{ty=$2; " +
  'if(ty=="tmpfs"||ty=="devtmpfs"||ty=="overlay"||ty=="proc"||ty=="sysfs"||ty=="cgroup"||ty=="udev"||ty=="squashfs")next; ' +
  'sz=$3; us=$4; pct=(sz>0)?int(us*100/sz):0; print $7"|"sz"|"us"|"pct}\'';

export default async function (app) {
  app.register(async (secured) => {
    secured.addHook('preHandler', authHook);

    secured.get('/api/sysinfo/:connId', async (req, reply) => {
      const connId = Number(req.params.connId);
      try {
        const client = await getClient(connId);
        const [cpuRaw, memRaw, loadRaw, diskRaw] = await Promise.all([
          execCmd(client, CPU_CMD),
          execCmd(client, MEM_CMD),
          execCmd(client, LOAD_CMD),
          execCmd(client, DISK_CMD)
        ]);

        let cpu = null;
        if (cpuRaw != null) {
          const v = parseInt(cpuRaw.trim(), 10);
          if (!Number.isNaN(v)) cpu = Math.max(0, Math.min(100, v));
        }

        let mem = null;
        let memTotal = null;
        let memUsed = null;
        if (memRaw) {
          const p = memRaw.trim().split(/\s+/);
          const t = num(p[0]);
          const u = num(p[1]);
          if (t && t > 0) {
            mem = Math.round((u / t) * 100);
            memTotal = t;
            memUsed = u;
          }
        }

        let load1 = null;
        let load5 = null;
        let load15 = null;
        let cores = null;
        if (loadRaw) {
          const lines = loadRaw.trim().split('\n').filter(Boolean);
          if (lines.length >= 1) {
            const l = lines[0].split(/\s+/);
            load1 = num(l[0]);
            load5 = num(l[1]);
            load15 = num(l[2]);
          }
          const last = lines[lines.length - 1];
          if (last && !Number.isNaN(Number(last.trim()))) cores = Number(last.trim());
        }

        const disks = [];
        if (diskRaw) {
          for (const line of diskRaw.trim().split('\n')) {
            if (!line) continue;
            const [mount, size, used, pct] = line.split('|');
            if (!mount) continue;
            disks.push({
              mount,
              total: num(size),
              used: num(used),
              percent: pct == null ? null : Number(pct)
            });
          }
        }

        return { cpu, mem, memTotal, memUsed, load1, load5, load15, cores, disks };
      } catch (err) {
        return reply.code(400).send({ error: err.message });
      }
    });
  });
}

# Go 后端重写规格文档（API/WS 契约 + 兼容性约束）

> 版本基线：`f5c917e`（含会话机制、单点登录、备份/恢复功能）
> 目标：`server-go/` 复刻 Node 版全部行为，前端（web/）零改动，数据（data/）零迁移。
> 验收标准：现有前端 + 现有 data 目录，全功能跑通，含老凭据解密、老密码登录。

---

## 1. 运行时约定

| 项 | 约定 |
|---|---|
| 环境变量 | `PORT`（默认 3000）、`HOST`（默认 0.0.0.0）、`DATA_DIR`（默认 `./data`）、`STATIC_DIR`（可选，静态资源目录）、`MASTER_SECRET` / `JWT_SECRET`（可选，覆盖文件密钥） |
| 数据文件 | `$DATA_DIR/terminal-web.db`（SQLite, WAL + foreign_keys=ON）、`$DATA_DIR/.enc-secret`、`$DATA_DIR/.jwt-secret`、`$DATA_DIR/backups/*.json` |
| 静态托管 | `STATIC_DIR` 存在时托管；404 且非 `/api`、`/ws` 前缀时回落 `index.html`（Hash 路由） |
| Body 上限 | 2GB（大文件上传） |
| 错误响应统一格式 | `{ "error": "中文消息" }`，配合 HTTP 状态码 400/401/403/404/429/500 |
| 新增桌面参数 | `--desktop`：监听 127.0.0.1:0，stdout 打印实际端口与一次性 token（阶段四使用，Web/Docker 行为不变） |

## 2. 全局中间件

### 2.1 IP 白名单（onRequest 级）
- 仅对 `/api` 前缀生效；公开路径豁免：`/api/setup/status`、`/api/setup`、`/api/captcha`、`/api/settings/public`
- 客户端 IP：优先 `X-Forwarded-For` 第一段，否则 socket 地址
- 白名单格式（`ip_whitelist` 设置，逗号分隔）：精确 IP / CIDR（`10.0.0.0/24`）/ 前缀（`192.168.1.`）；空 = 不限制
- 拒绝时 `403 {"error":"当前 IP 不在允许访问的列表中"}`

### 2.2 鉴权（受保护路由 preHandler）
- Token 来源：`Authorization: Bearer <jwt>` 或 query `?token=`
- 校验链：**JWT 验签(HS256) → 有 jti 则查 sessions 表存在性 → session_timeout>0 时校验 `now - last_activity` 未超时（超时则删除会话）**
- 无 jti 的老 token 直接放行（向后兼容，7 天自然过期）
- REST 每次鉴权成功后 `touchSession(jti)` 刷新活跃时间；**例外：见 §3 各接口标注**
- 失败：`401 {"error":"未登录或登录已过期"}`

## 3. REST API 契约

### 3.1 公开接口

| 方法/路径 | 请求 | 响应 | 备注 |
|---|---|---|---|
| GET `/api/setup/status` | - | `{initialized: bool}` | users 表 count>0 |
| POST `/api/setup` | `{username, password}` | `{token, username}` | 已初始化→403；密码<6位→400；创建会话并签 7d JWT |
| GET `/api/captcha` | - | `{id, svg}` | SVG 验证码，4 字符，5 分钟 TTL，内存缓存，验证即删（防重放），忽略大小写 |
| GET `/api/settings/public` | - | `{captchaEnabled, theme, fontFamily, fontSize}` | 驼峰命名（与内部 snake_case 不同！） |
| POST `/api/login` | `{username, password, captchaId?, captchaText?}` | `{token, username}` | 锁定→429（含剩余分钟）；captcha_enabled 时先验码→400；失败→401 + recordFail；成功清计数。均写审计 |

登录失败锁定（内存态）：`login_fail_max`（默认5）次失败后锁 `login_lock_minutes`（默认15）分钟。

### 3.2 受保护接口（需鉴权）

**会话/设置/审计**

| 方法/路径 | 请求 | 响应 | 备注 |
|---|---|---|---|
| POST `/api/activity` | - | `{ok:true}` | 用户活动心跳，touchSession。前端后台轮询不调用此接口 |
| GET `/api/settings` | - | 全部设置对象（snake_case，见 §5） | |
| PUT `/api/settings` | 部分设置键值 | 实际更新的键值 | 仅接受 schema 内的键，类型强制转换 |
| GET `/api/audit?limit=100` | - | 审计行数组（倒序，limit 1~500） | |
| POST `/api/change-password` | `{oldPassword, newPassword}` | `{ok:true}` | 新密码<6位→400；原密码错→400 |

**文件夹**

| 方法/路径 | 请求 | 响应 |
|---|---|---|
| GET `/api/folders` | - | 全部行，`ORDER BY sort_order, name` |
| POST `/api/folders` | `{name, parent_id?}` | 新建行 |
| PUT `/api/folders/:id` | `{name?, parent_id?}` | 更新行；parent_id==id→400 |
| DELETE `/api/folders/:id` | - | `{ok:true}`（子文件夹级联删除，连接 folder_id 置 NULL——由外键约束实现） |

**连接**

| 方法/路径 | 请求 | 响应 | 关键行为 |
|---|---|---|---|
| GET `/api/connections` | - | 行数组（脱敏） | 脱敏：去掉 `*_enc` 三字段，换成 `has_password/has_private_key/has_passphrase` 布尔 |
| POST `/api/connections` | `{name,host,port,username,auth_type,password?,private_key?,passphrase?,folder_id?,jump_id?,remark?,copy_from?}` | 新建行（脱敏） | 明文凭据服务端加密入库；`copy_from` 时：同名自动加数字后缀、未填凭据复用源加密串；jump_id 非整数置 NULL |
| PUT `/api/connections/:id` | 同上（无 copy_from） | 更新行（脱敏） | **凭据留空=不修改**；jump_id==id→400；更新后 dropSftp(id) |
| DELETE `/api/connections/:id` | - | `{ok:true}` | 先 dropSftp；引用它作跳板的连接 jump_id 置 NULL |
| POST `/api/connections/:id/test` | - | `{ok:true}` / 400 `连接失败: xxx` | 用落库凭据实连一次 |
| POST `/api/connections/test` | 表单明文配置（可含 id） | 同上 | 有 id 且凭据留空时从库解密补全；支持跳板 |
| GET `/api/connections/:id/ping` | - | `{latency}` ms | 复用池连接 exec `true` 测往返。**前端轮询用，语义上不 touch 会话** |

**SFTP**（全部复用 §6 连接池；错误统一 400 + err.message）

| 方法/路径 | 请求 | 响应 |
|---|---|---|
| GET `/api/sftp/:connId/list?path=` | - | `{path, items:[{name,path,type(dir/link/file),size,mode("0755"格式),mtime(毫秒)}]}`，目录优先 + 名称排序 |
| GET `/api/sftp/:connId/download?path=` | - | 流式下载，`Content-Disposition: attachment; filename*=UTF-8''<enc>` |
| GET `/api/sftp/:connId/read?path=` | - | `{path, content}`；>2MB → 400"文件过大…" |
| POST `/api/sftp/:connId/write` | `{path, content}` | `{ok:true}` |
| POST `/api/sftp/:connId/upload?path=<dir>` | multipart（可多文件） | `{ok:true, uploaded:[paths]}` |
| POST `/api/sftp/:connId/mkdir` | `{path}` | `{ok:true}` |
| POST `/api/sftp/:connId/rename` | `{from, to}` | `{ok:true}` |
| POST `/api/sftp/:connId/delete` | `{path, isDir}` | `{ok:true}`（isDir 用 rmdir，否则 unlink） |

路径处理：POSIX normalize，强制以 `/` 开头。

**进程 / 系统信息**（复用连接池 exec）

| 方法/路径 | 响应 |
|---|---|
| GET `/api/process/:connId` | `{processes:[{pid,user,cpu,mem,elapsed,command}]}`，命令 `ps -eo pid,user,%cpu,%mem,etime,args --sort=-%cpu \| head -n 200`，按表头列名定位解析 |
| POST `/api/process/:connId/kill` `{pid, signal?}` | `{ok:true}`，默认 TERM |
| GET `/api/sysinfo/:connId` | `{cpu, mem, memTotal, memUsed, load1, load5, load15, cores, disks:[{mount,total,used,percent}]}`；四条命令并行执行（原始 shell 命令照抄 Node 版 sysinfo.js），单条超时 8s 返回 null 字段 |

**备份/恢复**

| 方法/路径 | 请求 | 响应 | 关键行为 |
|---|---|---|---|
| POST `/api/backup/create` | - | `{id, filename, size}` | JSON 格式：`{meta:{app,version:1,exportedAt}, secret:<明文主密钥>, data:{folders,connections,settings}}`；文件 `$BACKUP_DIR/<id>.json`，mode 0600；记录入 backups 表 |
| GET `/api/backup/list` | - | `[{id,filename,created_at,size}]` 倒序 | |
| GET `/api/backup/download/:id` | - | 文件下载 | 404 处理 |
| DELETE `/api/backup/:id` | - | `{ok:true}` | 删记录+文件 |
| POST `/api/backup/restore/:id` | - | `{ok:true}` | 事务内 DELETE 全部 connections/folders/settings 再插入；**re-key**：备份 secret ≠ 本机主密钥时，用 `scrypt(备份secret)` 解密再用本机 key 重加密；相同则密文原样保留 |
| POST `/api/backup/import` | multipart 单文件 | `{ok:true}` | 同 restore 的 applyBackup 逻辑 |

## 4. WebSocket 终端协议

路径：`GET /ws/terminal/:connId?token=<jwt>`（升级请求）

服务端 → 客户端（JSON 文本帧）：
| type | 字段 | 时机 |
|---|---|---|
| `status` | `message` | 连接前发 `正在连接 user@host:port ...` |
| `connected` | - | shell 打开成功（term=xterm-256color, 初始 80x24） |
| `data` | `data`(base64) | stdout/stderr 输出 |
| `error` | `message` | 鉴权失败/配置不存在/SSH错误/打开终端失败 |
| `exit` | - | shell 关闭或 SSH 断开，随后关 socket |
| `pong` | - | 回应 ping |

客户端 → 服务端：
| type | 字段 | 行为 |
|---|---|---|
| `input` | `data`(base64) | 写入 shell stdin；**有 jti 时 touchSession（仅真实按键续命）** |
| `resize` | `rows, cols` | setWindow |
| `ping` | - | 回 pong |

socket 关闭时必须回收 stream 与 SSH client（跳板链全部关闭）。

## 5. 设置 Schema（键名/类型/默认值必须一致）

```
captcha_enabled    bool  true
theme              str   "dark"
font_size          int   13
font_family        str   '"JetBrains Mono", Menlo, Consolas, "Courier New", monospace'
login_fail_max     int   5
login_lock_minutes int   15
ip_whitelist       str   ""
session_timeout    int   0        # 分钟，0=关闭
sso_enabled        bool  false    # 开启时登录前清除该用户全部旧会话
```
存储为 TEXT（bool 存 "1"/"0"），读取时按类型转换。启动时 seed 缺失键。

## 6. SSH/SFTP 连接池

- 按 connId 缓存 SSH client（含 sftp 子会话），空闲 5 分钟自动断开
- keepalive：interval 20s，countMax 3；readyTimeout 15s
- 跳板级联：最多 5 级，循环引用检测；通过 `forwardOut` 隧道逐级连接；目标 client 的 end() 需级联关闭整条链
- password 认证需支持 keyboard-interactive 回退（用密码应答所有 prompt）
- 连接配置更新/删除时 drop 对应池条目

## 7. 数据兼容硬约束（生死线）

| 项 | 精确规格 |
|---|---|
| 凭据加密 | AES-256-GCM；key=`scrypt(MASTER_SECRET, "terminal-web-salt", 32)`，参数 **N=16384, r=8, p=1**（Node 默认）；密文=`base64(12B IV ‖ 16B tag ‖ ciphertext)`；空明文存 NULL |
| 密码哈希 | `<hex salt>:<hex scrypt64字节>`；salt 16 字节随机；同上 scrypt 参数；**常数时间比较** |
| JWT | HS256，secret 为 `.jwt-secret` 文件内容（字符串），payload `{uid, username, jti?}`，7d 过期 |
| 密钥文件 | 若不存在则生成 32 字节随机 hex 写入（mode 0600）；环境变量 `MASTER_SECRET`/`JWT_SECRET` 优先 |
| 表结构 | 见 `server/src/db.js`，CREATE TABLE IF NOT EXISTS 语句逐字保留（users/folders/connections/settings/audit_logs/sessions/backups） |
| 备份 re-key | `deriveKey(secret)=scrypt(secret,"terminal-web-salt",32)`，同参数 |

**兼容自检**（Go 版启动前必须通过的验收脚本）：
1. 用 Go 解密现有库中一条真实 `password_enc` == Node 版解密结果；
2. 用 Go verify 现有 `password_hash`（admin/123123）通过；
3. Node 版签发的带 jti token 在 Go 版鉴权通过。

## 8. Go 技术选型（已定）

| 用途 | 库 |
|---|---|
| HTTP/路由 | 标准库 `net/http` + `chi`（轻量，中间件模型贴合现有 hook 结构） |
| WebSocket | `github.com/coder/websocket`（原 nhooyr） |
| SSH/SFTP | `golang.org/x/crypto/ssh` + `github.com/pkg/sftp` |
| SQLite | `modernc.org/sqlite`（纯 Go，免 CGO，交叉编译三平台） |
| JWT | `github.com/golang-jwt/jwt/v5` |
| scrypt | `golang.org/x/crypto/scrypt` |
| 验证码 | `github.com/mojocn/base64Captcha` 或自绘 SVG（输出格式保持 `{id, svg}`） |

目录结构（server-go/）：
```
server-go/
├── main.go            # 启动、静态托管、--desktop 参数
├── internal/
│   ├── db/            # 打开库、建表、迁移
│   ├── config/        # 环境变量、密钥文件
│   ├── crypto/        # encrypt/decrypt/hash/deriveKey
│   ├── auth/          # login/session/jwt/中间件
│   ├── settings/      # schema + 读写
│   ├── security/      # IP 白名单、锁定、审计
│   ├── captcha/       # 验证码
│   ├── sshx/          # 跳板级联、连接池
│   └── routes/        # api/sftp/process/sysinfo/backup/ws
└── go.mod
```

## 9. 桌面模式规格（`--desktop`，阶段四启用，但接口预留在 Go 版中实现）

> 总原则：**数据层完全不分叉**。桌面差异全部是运行时策略，备份文件与服务器版双向互通。

### 9.1 启动流程（免登录 = 自动登录，不是删登录）

1. `--desktop` 启动：**强制监听 `127.0.0.1:0`**（随机端口），忽略 `HOST`/`PORT` 环境变量——写死在代码，不是配置项；
2. 生成一次性 boot token（32 字节随机 hex，内存态，不落盘不写日志），启动完成后向 stdout 打印一行 JSON：`{"event":"ready","port":<实际端口>,"token":"<boot token>"}`；
3. Tauri 壳读取后让 WebView 加载 `http://127.0.0.1:<port>/?dtk=<token>`；
4. 前端检测到 `dtk` → `POST /api/desktop/session {token}` → 换取正常 JWT + sessions 表记录 → 存 localStorage → `history.replaceState` 清掉 URL 参数 → 直接进主界面；
5. 之后所有请求走与 Web 版完全相同的 JWT/会话链路，业务层零特判。

boot token 约束：一次性（换取成功立即作废）、30 秒时效（超时作废）、校验失败返回 401 且不重发。

### 9.2 desktop 专属路由条件注册

- `POST /api/desktop/session` **仅在 desktop 模式下注册**；非 desktop 模式请求返回 404（与不存在路径无差别，不留探测特征）；
- boot token 及其校验逻辑仅在 desktop 分支初始化，线上进程内存中不存在。

### 9.3 安全设施"忽略但不删除"清单

| 设施 | Web/Docker | 桌面模式 |
|---|---|---|
| IP 白名单 | 生效 | 忽略（仅回环监听，物理隔离） |
| 验证码 / 登录失败锁定 | 生效 | 不适用（无登录页） |
| 单点登录（sso_enabled） | 生效 | 忽略 |
| 无活动超时（session_timeout） | 生效 | 忽略（桌面会话不过期） |

以上设置**照常存在库里、照常随备份进出**，仅在 desktop 运行时被旁路 → 导入服务器备份不会把自己锁在客户端外。

### 9.4 首次启动（空库）

desktop 模式且 users 表为空时：自动创建内置 `admin` 用户（32 字节随机密码，不展示），跳过 setup 向导。

### 9.5 前端配合（改动极小）

- `/api/settings/public` 增加 `desktopMode: bool` 字段（Web 版恒为 false）；
- 前端：有 `dtk` 参数走静默换 token；换失败（404/401）静默回落正常登录页；
- `desktopMode=true` 时隐藏"退出登录"按钮与设置页安全类选项（IP 白名单/SSO/超时/验证码）。

### 9.6 备份互通

- 服务器备份 → 桌面导入：走同一 applyBackup（含 re-key），全量原样入库，安全设置进库但运行时被忽略；
- 桌面导出 → 服务器导入：同一格式，无损恢复；
- users 表当前不在备份 data 中（保持 Node 版行为），互导不影响账号体系。

### 9.7 线上安全不变量（硬性要求）

1. `--desktop` 与"强制 127.0.0.1 监听"绑死，公网环境物理不可达；
2. Docker ENTRYPOINT 不含 `--desktop`，且该开关只认命令行参数、不读环境变量；
3. desktop 路由条件注册，线上无新增攻击面；
4. 鉴权判定全部在后端，前端改动不构成信任边界变化。

Backlog（不在本期）：备份文件明文含主密钥为现有设计，后续可加"用户口令加密备份"选项。

## 10. 非目标（本阶段不做）

- 不改任何 API 行为/路径/字段名（即使有更优设计）
- 不动前端、不动 Node 版代码
- reset-password.js 对应的 CLI 工具后续用 `server-go --reset-password` 子命令实现（低优先级）
- 桌面壳集成（阶段四）

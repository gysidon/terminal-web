> 🇺🇸 English: [README.md](README.md)

# Terminal Web

> 一个**极简主义的 SSH / SFTP 管理终端** —— 浏览器打开即用，也可以装成桌面应用。只做最常用的事，没有花里胡哨的东西。

## 项目简介

### 它解决什么问题

运维和开发日常要连的机器越来越多，但真正高频的操作其实就四件事：**连上机器、敲命令、传文件、看进程**。

而现有工具往往走向两个极端：要么是本地客户端，换台电脑就得重新导一遍连接配置、密钥散落各处；要么是功能堆得满满当当的企业级堡垒机，为了连一台自己的小服务器，得先啃一遍权限模型和工单流程。

Terminal Web 想要的是中间那条路：**一个自己部署、一条 docker 命令起来、打开浏览器就能用的 SSH 终端**。连接配置和加密后的凭据都存在你自己的机器上，换设备只要能访问这个地址，所有连接就都在。

### 适合谁

- 有若干台自己的服务器 / VPS / 家庭 NAS，想要一个统一入口的个人开发者
- 需要在团队内共享一套跳板机连接配置的小团队
- 经常换设备办公，不想反复同步 `~/.ssh/config` 和密钥的人
- 想在 iPad、公司电脑等「不方便装客户端」的环境里临时连服务器的场景

### 设计取舍

**做**：连接分组管理、多标签终端、跳板机级联、SFTP 传输与在线编辑、进程管理、延迟探测、凭据加密存储、单文件自托管。

**不做**：多租户与复杂 RBAC、审批工单、会话录像回放、命令审计与拦截、插件市场。这些是堡垒机的领域，硬塞进来只会让「打开就能用」这件事变难。

界面上同样克制：默认暗色主题、没有引导弹窗、没有需要先理解的抽象概念——新建一个连接，双击，就进终端了。

## 三种使用形态

同一份代码，按你的场景挑一种即可，数据格式完全一致，可随时互换。

| 形态 | 适合场景 | 启动方式 | 数据位置 |
|---|---|---|---|
| **Docker 自托管** | 常驻服务器、多设备共享、团队内使用 | `docker compose up -d` | 挂载卷 `./data` |
| **单二进制直跑** | 本机或轻量 VPS，不想装 Docker | `./terminal-web-server` | `$DATA_DIR`（默认 `./data`） |
| **桌面客户端** | 个人电脑上当本地 SSH 工具用，免登录 | 双击应用图标 | 系统应用数据目录 |

Go 服务端是**纯 Go 编译（无 CGO）的单个可执行文件**，前端静态资源可以一起打进镜像或用 `STATIC_DIR` 指过去，部署时不需要 Node、不需要 Python、不需要系统级 SQLite。

## 架构概览

```
┌──────────────┐   HTTPS/WS    ┌──────────────────┐    SSH / SFTP    ┌────────────┐
│  浏览器 / 桌面 │ ────────────▶ │   Go 服务端       │ ───────────────▶ │  远程主机   │
│  Vue 3 + xterm│ ◀──────────── │  chi + WebSocket │ ◀─────────────── │  (可级联)   │
└──────────────┘   JWT 鉴权     └────────┬─────────┘   最多 5 级跳板    └────────────┘
                                        │
                                        ▼
                                 ┌─────────────┐
                                 │  SQLite     │  连接 / 文件夹 / 设置 / 审计日志
                                 │ terminal.db │  凭据经 AES-256-GCM 加密后落库
                                 └─────────────┘
```

- **终端数据流**：浏览器 xterm.js ↔ WebSocket ↔ 服务端 SSH channel，服务端只做转发，不解析、不记录命令内容。
- **跳板机**：连接可指定上一跳，服务端在内存中逐级建立隧道，最多 5 级。
- **SFTP**：独立的连接池复用 SSH 会话，上传走分片 + 进度回传，≤2MB 的文本文件可在 Monaco 编辑器里直接改。
- **桌面端**：Rust 外壳在本地拉起同一个 Go 服务端作为 sidecar（监听 `127.0.0.1` 随机端口），通过一次性 boot token 自动登录，因此**无需输入账号密码**。

## 技术栈

- **前端**：Vue 3 + Vite + Naive UI + xterm.js + Monaco Editor
- **后端**：Go + chi + golang.org/x/crypto/ssh + pkg/sftp + coder/websocket + modernc.org/sqlite（纯 Go 实现，无需 CGO）
- **桌面端**：Tauri 2 + Rust（内置 Go 服务端 sidecar，免登录直接使用）
- **部署**：多阶段 Docker 镜像 / docker-compose

## 功能特性

- 🖥️ **Web SSH 终端**：基于 xterm.js 的多标签终端，支持跳板（堡垒机）级联连接（最多 5 级）。
- 📁 **文件夹分组**：连接可归入文件夹，左侧树形管理，支持搜索。
- 🔐 **多种登录方式**：账号密码、私钥证书（支持上传与粘贴），凭据 AES-256-GCM 加密存储。
- 📂 **SFTP 文件管理**：以主区域 Tab 方式打开，支持上传（带进度条）、下载、重命名、新建、在线编辑（≤2MB 文本）。
- 📊 **进程管理**：以主区域 Tab 方式打开，查看远程进程列表并支持结束进程。
- ⚡ **连接延迟探测**：左侧连接列表实时显示 SSH 往返延迟。
- 🎨 **主题与外观**：暗色 / 亮色主题、字体大小、字体族可配置。
- 🛡️ **安全增强**：登录验证码开关、登录失败锁定、IP 白名单、登录审计日志。
- 💾 **备份恢复**：设置页一键导出 / 导入全量数据。
- 🐳 **Docker 部署**：多阶段构建镜像，数据卷持久化。
- 🖱️ **桌面客户端**：基于 Tauri 的 macOS / Windows 客户端，内置服务端、免登录、自绘标题栏（详见下文）。

## 截图

**登录页**
![登录页](docs/screenshots/login.png)

**Web SSH 终端**
![Web SSH 终端](docs/screenshots/terminal.png)

**SFTP 文件管理**
![SFTP 文件管理](docs/screenshots/file-manager.png)

**进程管理**
![进程管理](docs/screenshots/process-manager.png)

## 快速开始（Docker）

### 1. 构建镜像

```bash
docker build -t terminal-web .
```

### 2. 运行容器

```bash
docker run -d \
  --name terminal-web \
  -p 3000:3000 \
  -v $(pwd)/data:/data \
  -e ADMIN_USERNAME=admin \
  -e ADMIN_PASSWORD=你的管理员密码 \
  terminal-web
```

启动后访问 http://localhost:3000 。首次访问若数据为空会进入**初始化设置页**，按提示创建管理员账号即可（也可通过 `ADMIN_PASSWORD` 环境变量在无人值守场景下自动建账号）。

### 3. 使用 docker-compose

```bash
docker compose up -d --build
```

`docker-compose.yml` 已挂载 `./data:/data` 作为数据卷，并包含 `TZ`、健康检查等配置。请务必修改其中的 `ADMIN_PASSWORD`。

## 环境变量

| 变量 | 必填 | 默认值 | 说明 |
|---|---|---|---|
| `ADMIN_USERNAME` | 否 | `admin` | 管理员用户名（仅在设置 `ADMIN_PASSWORD` 时自动建账号） |
| `ADMIN_PASSWORD` | 否* | — | 管理员密码；设置后首次启动自动创建账号（适合无人值守部署）。不设置则需通过页面初始化设置 |
| `ENCRYPTION_KEY` | 否 | 自动生成 | 凭据 AES 加密密钥（32 字节随机串）。**建议固定并持久化**，丢失或更改会导致已存凭据无法解密 |
| `JWT_SECRET` | 否 | 自动生成 | JWT 签发密钥。**建议固定并持久化**，更改会使所有登录会话失效 |
| `DATA_DIR` | 否 | `/data` | SQLite 数据库与密钥文件存放目录（容器内） |
| `PORT` | 否 | `3000` | 服务监听端口（容器内） |
| `HOST` | 否 | `0.0.0.0` | 服务监听地址（容器内） |
| `STATIC_DIR` | 否 | `/app/public` | 前端静态资源目录（镜像构建时已写入 `web/dist`，一般无需改动） |
| `TZ` | 否 | `Asia/Shanghai` | 容器时区，影响审计日志时间 |

> *：生产环境务必设置 `ADMIN_PASSWORD`。不设置时需在页面完成首次初始化设置，初始化仅发生一次（数据卷保留 `users` 表，更新镜像不会重复建账号）。

## 本地开发

需要 Go 1.25+ 与 Node.js 18+。

```bash
# 后端（终端 1）—— server-go 是独立 Go module，必须在该目录内执行
cd server-go
DATA_DIR=../data PORT=3000 go run .

# 前端（终端 2）
cd web
npm install
npm run dev
```

> ⚠️ `DATA_DIR` 默认值是相对当前工作目录的 `data`。若在 `server-go/` 内启动却不指定 `DATA_DIR=../data`，会在 `server-go/data/` 生成一套全新的空数据库与新的 `.jwt-secret`，导致看不到原有连接、登录态失效。

前端开发服务器默认监听 `http://localhost:5173`（注意是 `localhost`，非 `127.0.0.1`）。生产构建：

```bash
# 1. 构建前端
cd web && npm run build

# 2. 编译并启动后端（在项目根目录执行，相对路径自然正确）
cd .. && go build -C server-go -o ../terminal-web-server .
STATIC_DIR=web/dist PORT=3000 ./terminal-web-server
# 然后访问 http://localhost:3000
```

### 命令行参数

| 参数 | 说明 |
|---|---|
| `-reset-password <用户名> <新密码>` | 重置或新建用户密码（见下文「忘记密码」） |
| `-selfcheck` | 自检：校验凭据解密、管理员密码与设置项，用于排查数据兼容性 |
| `-desktop` | 桌面模式：监听 `127.0.0.1` 随机端口并输出 boot token，仅供桌面客户端内部调用 |

## 桌面客户端（macOS / Windows）

`desktop/` 是基于 Tauri 2 的跨平台客户端：Rust 外壳启动时以 `-desktop` 模式拉起内置的 Go 服务端 sidecar（本地随机端口），通过一次性 boot token 自动换取登录态，**无需输入账号密码**。

与 Web 版的差异：隐藏了「安全」「登录日志」「修改密码」（本机单用户场景无意义）；使用无系统装饰的自绘标题栏（支持拖动、双击缩放、最小化/最大化/关闭）；「设置」入口移到标题栏右侧。

数据默认位置：

| 平台 | 路径 |
|---|---|
| macOS | `~/Library/Application Support/com.terminalweb.desktop/` |
| Windows | `%APPDATA%\com.terminalweb.desktop\` |

可用 `DATA_DIR` 环境变量覆盖，例如指向服务器版的数据目录以复用同一批连接配置。

构建（需先安装 [Tauri CLI](https://v2.tauri.app/reference/cli/)：`cargo install tauri-cli --version "^2"`）：

```bash
# 1. 构建前端（平台无关）
cd web && npm run build

# 2. 编译 Go sidecar 到 desktop/src-tauri/binaries/
cd ../server-go
go build -o ../desktop/src-tauri/binaries/terminal-web-server .        # macOS / Linux
go build -o ../desktop/src-tauri/binaries/terminal-web-server.exe .    # Windows

# 3. 打包
cd ../desktop/src-tauri && cargo tauri build
#   macOS   → target/release/bundle/{macos/TerminalWeb.app, dmg/*.dmg}
#   Windows → target\release\bundle\{nsis\*-setup.exe, msi\*.msi}
```

> ⚠️ Tauri **不支持跨平台交叉编译**：Windows 安装包必须在 Windows 上构建（需 MSVC 工具链与 WebView2 Runtime），macOS 包必须在 macOS 上构建。

更多细节见 [desktop/README.md](desktop/README.md)。

## 安全说明

- **凭据加密**：SSH 密码与私钥使用 AES-256-GCM 加密后存入数据库，密钥来自 `ENCRYPTION_KEY` 或自动生成并落盘的 `.enc-secret`。数据库文件本身泄露时，没有密钥无法解出凭据。
- **鉴权**：所有 API 与 WebSocket 均需 JWT，签发密钥来自 `JWT_SECRET` 或自动生成的 `.jwt-secret`；更换密钥会立即使全部会话失效。
- **登录防护**：可开启验证码、失败次数锁定、IP 白名单，登录行为写入审计日志。
- **不记录命令**：服务端只在浏览器与远程主机之间转发终端字节流，不解析、不落盘命令内容或输出。
- **部署建议**：公网暴露时请置于 HTTPS 反向代理之后，并固定 `ENCRYPTION_KEY` 与 `JWT_SECRET`（写进 compose 或密钥管理，不要依赖自动生成）。

## 忘记密码 / 重置管理员密码

如果忘记了登录密码，无需重装或清空数据库，通过命令行即可重置（也适用于新建首个账号）。

**Docker（推荐，容器已挂载数据卷）：**

```bash
docker compose exec terminal-web /app/server-go -reset-password admin 你的新密码
# 或
docker exec -it terminal-web /app/server-go -reset-password admin 你的新密码
```

**本地开发：**

```bash
cd server-go
DATA_DIR=../data go run . -reset-password admin 你的新密码
# 也可用环境变量传参：RESET_USER=admin RESET_PASS=你的新密码 go run . -reset-password
# 若数据不在 ../data，改为实际的 DATA_DIR 路径
```

说明：

- `<用户名>` 为任意已有账号；若该用户不存在，会自动以该用户名新建账号。
- 新密码至少 6 位（与页面注册 / 修改密码规则一致）。
- 重置后立即生效，用新密码即可登录；原会话令牌会失效，需重新登录。
- 登录页底部也提供了同样的操作提示。

## 数据持久化

所有数据位于 `DATA_DIR`（Docker 下为挂载的 `./data` 卷）：

- `terminal.db` —— SQLite 数据库（用户、文件夹、连接、设置、审计日志）
- `.enc-secret` —— 自动生成的凭据加密密钥（若未设置 `ENCRYPTION_KEY`）
- `.jwt-secret` —— 自动生成的 JWT 签发密钥（若未设置 `JWT_SECRET`）

**备份只需复制整个 `data` 目录**（务必连同两个密钥文件一起，否则凭据无法解密）。设置页也提供了导出 / 导入功能。

Docker、单二进制、桌面端三种形态使用完全相同的库结构与加密格式，同一份数据目录可以直接互换。

## 目录结构

```
.
├── Dockerfile            # 多阶段构建（前端 build → Go 编译 → alpine 运行）
├── docker-compose.yml    # 容器编排
├── server-go/            # 后端：Go + chi + x/crypto/ssh + modernc.org/sqlite
├── desktop/              # 桌面客户端：Tauri 2 + Rust 外壳 + Go sidecar
├── web/                  # 前端：Vue 3 + Vite + Naive UI + xterm.js
└── docs/                 # 截图与设计文档
```

## 开源协议

本项目采用 [Apache License 2.0](LICENSE) 开源协议。

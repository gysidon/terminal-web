# Terminal Web 桌面客户端

基于 **Tauri 2 + Rust** 的跨平台桌面外壳，内置 Go 服务端作为 sidecar，**开箱免登录**。

## 工作原理

```
┌─────────────────────────────────────────────┐
│  Rust 外壳 (Tauri)                           │
│    │ 1. 以 --desktop 模式拉起 Go sidecar      │
│    ▼                                        │
│  Go 服务端（127.0.0.1 随机端口）              │
│    │ 2. stdout 输出 {"event":"ready",        │
│    │    "port":N,"token":"..."}              │
│    ▼                                        │
│  3. 外壳打开 WebView →                       │
│     http://127.0.0.1:N/?boot=<token>        │
│    │                                        │
│    ▼                                        │
│  4. 前端用 boot token 换取 JWT，自动登录       │
└─────────────────────────────────────────────┘
```

- 端口随机、仅监听 `127.0.0.1`，不对外暴露。
- boot token 一次性、30 秒过期，用完即废。
- 应用退出时外壳会 kill 掉 sidecar 进程。

## 与 Web 版的差异

| 项 | Web 版 | 桌面端 |
|---|---|---|
| 登录 | 账号密码 | 免登录（boot token 自动换取） |
| 安全 / 登录日志 / 修改密码 | 有 | 隐藏（本机单用户场景无意义） |
| 标题栏 | 浏览器原生 | 自绘（拖动、双击缩放、最小化/最大化/关闭） |
| 设置入口 | 侧边栏底部 | 标题栏右侧齿轮 |

前端通过 `store.isDesktop`（由 URL 上的 `?boot=` 参数触发）区分两种形态，同一份代码同时服务 Web 与桌面端。

## 数据目录

| 平台 | 默认位置 |
|---|---|
| macOS | `~/Library/Application Support/com.terminalweb.desktop/` |
| Windows | `%APPDATA%\com.terminalweb.desktop\` |

可用 `DATA_DIR` 环境变量覆盖，例如指向服务器版的数据目录以复用同一批连接配置。

## 构建

前置要求：Go 1.25+、Node.js 18+、Rust 稳定版、[Tauri CLI](https://v2.tauri.app/reference/cli/)。

```bash
cargo install tauri-cli --version "^2"
```

### 通用步骤（1、2 两步平台无关）

```bash
# 1. 构建前端
cd web && npm install && npm run build

# 2. 编译 Go sidecar 到 desktop/src-tauri/binaries/
cd ../server-go
go build -o ../desktop/src-tauri/binaries/terminal-web-server .          # macOS / Linux
go build -o ../desktop/src-tauri/binaries/terminal-web-server.exe .      # Windows
```

### macOS

```bash
cd desktop/src-tauri
cargo tauri build          # 产物：target/release/bundle/{macos/TerminalWeb.app, dmg/*.dmg}
```

### Windows

```powershell
cd desktop\src-tauri
cargo tauri build          # 产物：target\release\bundle\{nsis\*-setup.exe, msi\*.msi}
```

> ⚠️ Tauri **不支持跨平台交叉编译**：Windows 安装包必须在 Windows 上构建（需 MSVC 工具链 + WebView2 Runtime），macOS 包必须在 macOS 上构建。

### 调试运行

```bash
cd desktop/src-tauri && cargo run
```

调试模式下 `resource_dir()` 指向 `target/debug`，可用环境变量绕开：

```bash
TERMINAL_WEB_GO_BIN=../../server-go/terminal-web-server \
TERMINAL_WEB_STATIC_DIR=../../web/dist \
cargo run
```

## 目录结构

```
desktop/
└── src-tauri/
    ├── src/main.rs        # 外壳：拉起 sidecar、解析 ready 事件、建窗口
    ├── capabilities/      # Tauri 权限（远程 127.0.0.1 源的窗口 API 授权）
    ├── icons/             # 应用图标（png / ico / icns）
    ├── binaries/          # Go sidecar（构建产物，不入库）
    └── tauri.conf.json    # 打包配置（resources 把 sidecar 与前端打进安装包）
```

> `capabilities/main.json` 不能删：窗口加载的 `http://127.0.0.1:*` 在 Tauri v2 中属于**远程源**，不显式授权的话拖动、最小化、关闭等窗口 API 会被 IPC 安全层静默拒绝。

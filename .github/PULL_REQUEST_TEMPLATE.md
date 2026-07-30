# 桌面客户端（Tauri 2）+ Go 服务端，移除 Node 实现

## 概述
将项目从「Vue3 + Node.js 服务端 + Web」形态，升级为「Vue3 前端 + 纯 Go 服务端 + Tauri 2 桌面客户端」。

- 服务端用 Go 重写（`server-go/`）：单文件二进制、零 CGO 依赖（`modernc.org/sqlite`），功能与 Node 版完全一致——SSH/SFTP 终端、连接管理、登录超时、单点登录、数据备份恢复、忘记密码重置。
- 新增跨平台桌面客户端（`desktop/`，Tauri 2）：内置 Go 服务端作为 sidecar，免登录直接使用，自带最小化 / 最大化 / 关闭与可拖拽标题栏；Windows 下隐藏控制台窗口。
- 删除旧 Node.js 服务端（`server/`）与根 `package-lock.json`。

## 主要变更
- `server-go/`：Go 重写的服务端（chi + x/crypto/ssh + pkg/sftp + coder/websocket + modernc.org/sqlite）。
- `desktop/`：Tauri 2 桌面客户端（原 `spike-tauri/` 重命名）。Rust 拉起 Go sidecar；`#[cfg(windows)]` 处理 `.exe` 后缀与 `CREATE_NO_WINDOW` 抑制黑框；`capabilities/main.json` 授权窗口拖拽 / 最小化 / 最大化 / 关闭。
- `web/`：新增桌面端外壳（标题栏、拖拽、窗口按钮），通过 `isDesktop` 标识隐藏「修改密码 / 设置 / 登录日志 / 安全」等仅 Web 端需要的入口。
- `Dockerfile` / `docker-compose.yml`：镜像切到 Go 服务端。
- `README.md` / `README.zh-CN.md`：补充项目简介、三种使用形态对比、架构概览、安全说明；桌面端覆盖 macOS + Windows。
- `.github/workflows/build-desktop.yml`：GitHub Actions 自动构建桌面安装包（Windows → nsis/msi，macOS → dmg）。
- `.gitignore`：移除 Node 服务端忽略项，补充 Go / 桌面构建产物。

## 使用方式
| 形态 | 说明 |
|---|---|
| Docker | `docker compose up -d`（镜像内为 Go 服务端） |
| 单文件 | 直接运行 `server-go` 二进制 |
| 桌面端 | 安装 dmg / nsis / msi，打开即用，无需登录 |

## 安全
- 密码 AES-256-GCM 加密存储，JWT 登录态，登录失败次数限制。
- 桌面端随机 admin 密码 + 启动一次性 boot token 自动兑换，本地使用免登录。
- 不记录任何终端命令内容。
- 生产环境建议在反代层启用 HTTPS。

## 备注
- **Tauri 不支持交叉编译**：Windows 安装包需在 Windows runner 构建，macOS 包需在 macOS runner 构建（CI 已分别配置矩阵）。
- 当前 CI 产物为**未签名**安装包；如需对外分发，建议补充 Windows 代码签名与 macOS 公证。
- Node 版 `server/` 已从仓库删除（物理文件备份在本地 `/tmp`，未入库）。

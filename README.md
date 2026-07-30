> 🇨🇳 中文文档：[README.zh-CN.md](README.zh-CN.md)

# Terminal Web

> A **minimalist SSH / SFTP management terminal** — open it in a browser, or install it as a desktop app. It does only the most common things, nothing fancy.

## Introduction

### The problem it solves

Ops engineers and developers keep accumulating machines to connect to, yet the truly high-frequency operations boil down to four things: **connect to a machine, run commands, transfer files, check processes**.

Existing tools tend toward two extremes. Native clients mean re-importing your connection profiles on every new computer, with keys scattered everywhere. Enterprise bastion hosts pile on features — to reach one small personal server you first have to digest a permission model and a ticketing workflow.

Terminal Web aims for the middle road: **an SSH terminal you host yourself, bring up with a single docker command, and use straight from a browser**. Connection profiles and encrypted credentials live on your own machine; switch devices and everything is still there as long as you can reach the URL.

### Who it's for

- Individual developers with a handful of servers / VPSes / home NAS boxes who want one unified entry point
- Small teams sharing a common set of jump-host connection profiles
- People who move between devices often and don't want to keep syncing `~/.ssh/config` and keys
- Situations where installing a native client isn't practical — an iPad, a locked-down work machine, and so on

### Design trade-offs

**In scope**: grouped connection management, multi-tab terminals, jump-host cascading, SFTP transfer and inline editing, process management, latency probing, encrypted credential storage, single-file self-hosting.

**Out of scope**: multi-tenancy and complex RBAC, approval workflows, session recording and replay, command auditing and interception, a plugin marketplace. Those belong to bastion hosts; cramming them in would only make "open it and go" harder.

The UI is equally restrained: dark theme by default, no onboarding modals, no abstractions to learn first — create a connection, double-click, and you're in the terminal.

## Three ways to run it

Same codebase, pick whichever fits. The data format is identical across all three, so you can switch at any time.

| Form | Best for | How to start | Data location |
|---|---|---|---|
| **Docker self-hosting** | Always-on server, multi-device, team use | `docker compose up -d` | Mounted volume `./data` |
| **Single binary** | Local machine or lightweight VPS without Docker | `./terminal-web-server` | `$DATA_DIR` (defaults to `./data`) |
| **Desktop client** | A local SSH tool on your own computer, no login | Double-click the app icon | OS application-data directory |

The Go server is a **single executable built as pure Go (no CGO)**. Frontend assets are either baked into the image or pointed at via `STATIC_DIR` — deployment needs no Node, no Python, and no system SQLite.

## Architecture

```
┌──────────────┐   HTTPS/WS    ┌──────────────────┐    SSH / SFTP    ┌────────────┐
│Browser/Desktop│ ────────────▶ │   Go server      │ ───────────────▶ │Remote hosts│
│ Vue 3 + xterm │ ◀──────────── │  chi + WebSocket │ ◀─────────────── │ (cascading)│
└──────────────┘   JWT auth     └────────┬─────────┘  up to 5 hops    └────────────┘
                                         │
                                         ▼
                                  ┌─────────────┐
                                  │  SQLite     │  connections / folders / settings / audit
                                  │ terminal.db │  credentials sealed with AES-256-GCM
                                  └─────────────┘
```

- **Terminal data flow**: browser xterm.js ↔ WebSocket ↔ server-side SSH channel. The server only forwards bytes — it does not parse or record command content.
- **Jump hosts**: a connection can declare a previous hop; the server builds the tunnel chain in memory, up to 5 levels deep.
- **SFTP**: a dedicated pool reuses SSH sessions; uploads are chunked with progress reporting, and text files ≤ 2 MB can be edited inline in Monaco.
- **Desktop**: the Rust shell launches the very same Go server locally as a sidecar (bound to a random port on `127.0.0.1`) and signs in automatically via a one-time boot token, so **no username or password is needed**.

## Tech Stack

- **Frontend**: Vue 3 + Vite + Naive UI + xterm.js + Monaco Editor
- **Backend**: Go + chi + golang.org/x/crypto/ssh + pkg/sftp + coder/websocket + modernc.org/sqlite (pure Go, no CGO required)
- **Desktop**: Tauri 2 + Rust (bundles the Go server as a sidecar, no login required)
- **Deployment**: Multi-stage Docker image / docker-compose

## Features

- 🖥️ **Web SSH terminal**: A multi-tab terminal powered by xterm.js, with jump-host (bastion) cascading connections (up to 5 levels).
- 📁 **Folder grouping**: Connections can be organized into folders, managed via a left-side tree with search support.
- 🔐 **Multiple auth methods**: Password and private-key certificates (upload or paste supported), with credentials encrypted using AES-256-GCM.
- 📂 **SFTP file manager**: Opens as a main-area tab, supporting upload (with progress bar), download, rename, create, and inline editing (text files ≤ 2 MB).
- 📊 **Process manager**: Opens as a main-area tab, showing the remote process list with the ability to terminate processes.
- ⚡ **Connection latency probe**: The left-side connection list shows live SSH round-trip latency.
- 🎨 **Themes & appearance**: Dark / light theme, font size, and font family are all configurable.
- 🛡️ **Security hardening**: Login CAPTCHA toggle, lockout after failed logins, IP allowlist, and login audit logs.
- 💾 **Backup & restore**: One-click full export / import from the settings page.
- 🐳 **Docker deployment**: Multi-stage image build with persistent data volumes.
- 🖱️ **Desktop client**: Tauri-based macOS / Windows client with a built-in server, no login, and a custom title bar (see below).

## Screenshots

**Login page**
![Login page](docs/screenshots/login.png)

**Web SSH terminal**
![Web SSH terminal](docs/screenshots/terminal.png)

**SFTP file manager**
![SFTP file manager](docs/screenshots/file-manager.png)

**Process manager**
![Process manager](docs/screenshots/process-manager.png)

## Quick Start (Docker)

### 1. Build the image

```bash
docker build -t terminal-web .
```

### 2. Run the container

```bash
docker run -d \
  --name terminal-web \
  -p 3000:3000 \
  -v $(pwd)/data:/data \
  -e ADMIN_USERNAME=admin \
  -e ADMIN_PASSWORD=your-admin-password \
  terminal-web
```

Then open http://localhost:3000. On first visit with an empty database you'll land on the **initial setup page** — follow the prompts to create the administrator account (or set `ADMIN_PASSWORD` to create it automatically for unattended deployments).

### 3. Using docker-compose

```bash
docker compose up -d --build
```

`docker-compose.yml` already mounts `./data:/data` as the data volume and includes `TZ`, health checks, and more. Be sure to change `ADMIN_PASSWORD`.

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `ADMIN_USERNAME` | No | `admin` | Administrator username (only used to auto-create the account when `ADMIN_PASSWORD` is set) |
| `ADMIN_PASSWORD` | No* | — | Administrator password; when set, the account is created automatically on first start (ideal for unattended deployments). Otherwise, use the setup page |
| `ENCRYPTION_KEY` | No | Auto-generated | AES key for credentials (32-byte random string). **Should be fixed and persisted** — losing or changing it makes stored credentials undecryptable |
| `JWT_SECRET` | No | Auto-generated | JWT signing secret. **Should be fixed and persisted** — changing it invalidates all sessions |
| `DATA_DIR` | No | `/data` | Directory for the SQLite database and key files (inside the container) |
| `PORT` | No | `3000` | Service listen port (inside the container) |
| `HOST` | No | `0.0.0.0` | Service listen address (inside the container) |
| `STATIC_DIR` | No | `/app/public` | Frontend static assets directory (baked in as `web/dist` at image build time; usually no need to change) |
| `TZ` | No | `Asia/Shanghai` | Container timezone, affects audit log timestamps |

> *: Always set `ADMIN_PASSWORD` in production. Without it you must complete the initial setup in the UI. Setup happens only once (the `users` table persists in the volume, so image updates won't recreate the account).

## Local Development

Requires Go 1.25+ and Node.js 18+.

```bash
# Backend (terminal 1) — server-go is a standalone Go module; run from inside that directory
cd server-go
DATA_DIR=../data PORT=3000 go run .

# Frontend (terminal 2)
cd web
npm install
npm run dev
```

> ⚠️ `DATA_DIR` defaults to `data` relative to the current working directory. Starting inside `server-go/` without `DATA_DIR=../data` creates a brand-new empty database and `.jwt-secret` under `server-go/data/`, so your existing connections disappear and sessions break.

The frontend dev server listens on `http://localhost:5173` by default (note: `localhost`, not `127.0.0.1`). For a production build:

```bash
# 1. Build the frontend
cd web && npm run build

# 2. Compile and start the backend (run from the project root so relative paths line up)
cd .. && go build -C server-go -o ../terminal-web-server .
STATIC_DIR=web/dist PORT=3000 ./terminal-web-server
# Then open http://localhost:3000
```

### Command-line flags

| Flag | Description |
|---|---|
| `-reset-password <username> <new-password>` | Reset or create a user's password (see "Forgot Password" below) |
| `-selfcheck` | Self-check: verifies credential decryption, the admin password, and settings — useful for diagnosing data compatibility |
| `-desktop` | Desktop mode: listens on a random `127.0.0.1` port and emits a boot token; used internally by the desktop client |

## Desktop Client (macOS / Windows)

`desktop/` is a cross-platform client built on Tauri 2. On startup the Rust shell launches the bundled Go server as a sidecar in `-desktop` mode (random local port) and exchanges a one-time boot token for a session, so **no username or password is required**.

Differences from the web version: "Security", "Login logs", and "Change password" are hidden (meaningless for a single-user local setup); the window uses an undecorated custom title bar (drag, double-click to zoom, minimize/maximize/close); the "Settings" entry moves to the right side of the title bar.

Default data location:

| Platform | Path |
|---|---|
| macOS | `~/Library/Application Support/com.terminalweb.desktop/` |
| Windows | `%APPDATA%\com.terminalweb.desktop\` |

Override it with the `DATA_DIR` environment variable — for example, point it at your server's data directory to reuse the same connection profiles.

Building (install the [Tauri CLI](https://v2.tauri.app/reference/cli/) first: `cargo install tauri-cli --version "^2"`):

```bash
# 1. Build the frontend (platform-independent)
cd web && npm run build

# 2. Compile the Go sidecar into desktop/src-tauri/binaries/
cd ../server-go
go build -o ../desktop/src-tauri/binaries/terminal-web-server .        # macOS / Linux
go build -o ../desktop/src-tauri/binaries/terminal-web-server.exe .    # Windows

# 3. Bundle
cd ../desktop/src-tauri && cargo tauri build
#   macOS   → target/release/bundle/{macos/TerminalWeb.app, dmg/*.dmg}
#   Windows → target\release\bundle\{nsis\*-setup.exe, msi\*.msi}
```

> ⚠️ Tauri **does not support cross-compilation**: Windows installers must be built on Windows (MSVC toolchain + WebView2 Runtime), and macOS bundles must be built on macOS.

See [desktop/README.md](desktop/README.md) for details.

## Security Notes

- **Credential encryption**: SSH passwords and private keys are sealed with AES-256-GCM before hitting the database. The key comes from `ENCRYPTION_KEY` or an auto-generated `.enc-secret` file. A leaked database alone cannot reveal credentials.
- **Authentication**: every API and WebSocket endpoint requires a JWT, signed with `JWT_SECRET` or an auto-generated `.jwt-secret`. Rotating the secret invalidates all sessions immediately.
- **Login protection**: optional CAPTCHA, lockout after repeated failures, IP allowlist, and login activity written to an audit log.
- **No command logging**: the server only relays terminal bytes between browser and remote host — command content and output are never parsed or persisted.
- **Deployment advice**: put it behind an HTTPS reverse proxy when exposed publicly, and pin `ENCRYPTION_KEY` and `JWT_SECRET` explicitly (in compose or a secret manager) instead of relying on auto-generation.

## Forgot Password / Reset the Admin Password

If you forget your login password, there's no need to reinstall or wipe the database — reset it from the command line (this also works for creating the very first account).

**Docker (recommended, the container already mounts the data volume):**

```bash
docker compose exec terminal-web /app/server-go -reset-password admin your-new-password
# or
docker exec -it terminal-web /app/server-go -reset-password admin your-new-password
```

**Local development:**

```bash
cd server-go
DATA_DIR=../data go run . -reset-password admin your-new-password
# Environment variables also work: RESET_USER=admin RESET_PASS=your-new-password go run . -reset-password
# If your data is not in ../data, point DATA_DIR at the real path
```

Notes:

- `<username>` can be any existing account; if the user does not exist, it is created with that username.
- The new password must be at least 6 characters (matching the registration / change-password rules in the UI).
- The change takes effect immediately; existing session tokens are invalidated, so you'll need to log in again.
- The same hint is shown at the bottom of the login page.

## Data Persistence

All data lives in `DATA_DIR` (the mounted `./data` volume under Docker):

- `terminal.db` — the SQLite database (users, folders, connections, settings, audit logs)
- `.enc-secret` — the auto-generated credential encryption key (if `ENCRYPTION_KEY` is not set)
- `.jwt-secret` — the auto-generated JWT signing key (if `JWT_SECRET` is not set)

**To back up, simply copy the entire `data` directory** — including both key files, otherwise credentials cannot be decrypted. The settings page also offers export / import.

Docker, single-binary, and desktop deployments share the exact same schema and encryption format, so one data directory can be moved freely between them.

## Directory Structure

```
.
├── Dockerfile            # Multi-stage build (frontend build → Go compile → alpine runtime)
├── docker-compose.yml    # Container orchestration
├── server-go/            # Backend: Go + chi + x/crypto/ssh + modernc.org/sqlite
├── desktop/              # Desktop client: Tauri 2 + Rust shell + Go sidecar
├── web/                  # Frontend: Vue 3 + Vite + Naive UI + xterm.js
└── docs/                 # Screenshots and design documents
```

## License

This project is licensed under the [Apache License 2.0](LICENSE).

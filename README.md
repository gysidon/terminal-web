> 🇨🇳 中文文档：[README.zh-CN.md](README.zh-CN.md)

# Terminal Web

> A **minimalist web-based SSH management terminal** — it does only the most common things, nothing fancy.

## Introduction

The original idea behind Terminal Web is simple: a clean SSH terminal that runs right in your browser.

Many similar tools pile on features, yet day-to-day use boils down to just a few: connect to a machine, run commands, transfer files, and check processes. Terminal Web focuses only on polishing those tasks — a restrained UI, ready to use out of the box, with no convoluted concepts and no forced extra configuration.

It doesn't chase breadth. It just makes the "must-haves" feel right: grouped connection management, multi-tab terminals, SFTP file transfer, process and system monitoring, and one-click Docker self-hosting.

## Tech Stack

- **Frontend**: Vue 3 + Vite + Naive UI + xterm.js + Monaco Editor
- **Backend**: Node.js + Fastify + ssh2 + better-sqlite3
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
- 🐳 **Docker deployment**: Multi-stage image build with persistent data volumes.

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

After starting, open http://localhost:3000. If the data is empty on first visit, you'll be taken to the **initial setup page** — just follow the prompts to create an admin account (you can also set the `ADMIN_PASSWORD` environment variable to auto-create the account for unattended deployments).

### 3. Using docker-compose

```bash
docker compose up -d --build
```

`docker-compose.yml` already mounts `./data:/data` as a data volume and includes `TZ`, health checks, and other settings. Be sure to change the `ADMIN_PASSWORD` inside it.

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `ADMIN_USERNAME` | No | `admin` | Admin username (only used to auto-create the account when `ADMIN_PASSWORD` is set) |
| `ADMIN_PASSWORD` | No* | — | Admin password; if set, the account is auto-created on first start (good for unattended deployments). If not set, use the in-page setup |
| `ENCRYPTION_KEY` | No | Auto-generated | AES encryption key for credentials (32-byte random string). **Recommended to fix and persist** — losing or changing it makes stored credentials undecryptable |
| `JWT_SECRET` | No | Auto-generated | JWT signing secret. **Recommended to fix and persist** — changing it invalidates all login sessions |
| `DATA_DIR` | No | `/data` | Directory for the SQLite database and key files (inside the container) |
| `PORT` | No | `3000` | Service listen port (inside the container) |
| `STATIC_DIR` | No | `/app/public` | Frontend static assets directory (written to `web/dist` at image build time, usually no need to change) |
| `TZ` | No | `Asia/Shanghai` | Container timezone, affects audit log timestamps |

> *: You must set `ADMIN_PASSWORD` in production. If not set, complete the first-time setup in the UI. Setup happens only once (the `users` table is kept in the data volume, so updating the image won't recreate the account).

## Local Development

Requires Node.js 18+.

```bash
# Backend (terminal 1)
cd server
DATA_DIR=./data PORT=3000 ADMIN_PASSWORD=admin123 npm start

# Frontend (terminal 2)
cd web
npm install
npm run dev
```

The frontend dev server listens on `http://localhost:5173` by default (note: `localhost`, not `127.0.0.1`). For a production build:

```bash
cd web && npm run build
cd ../server && STATIC_DIR=/absolute/path/web/dist PORT=3000 npm start
# Then open http://localhost:3000
```

## Forgot Password / Reset Admin Password

If you forget the login password, there's no need to reinstall or wipe the database — you can reset it from the command line (this also works for creating the first account).

**Docker (recommended, container already has the data volume mounted):**

```bash
docker compose exec terminal-web node src/reset-password.js admin your-new-password
# or
docker exec -it terminal-web node src/reset-password.js admin your-new-password
```

**Local development:**

```bash
cd server
npm run reset -- admin your-new-password
# equivalent to node src/reset-password.js admin your-new-password
# if data is not in the default ./data, prefix with DATA_DIR=/path/to/data
```

Notes:

- `<username>` can be any existing account; if it doesn't exist, an account with that username is created automatically.
- The new password must be at least 6 characters (same rule as the in-page register / change-password flow).
- The reset takes effect immediately — log in with the new password; existing session tokens become invalid and you'll need to log in again.
- The same instructions are also shown at the bottom of the login page.

## Data Persistence

All data lives under `DATA_DIR` (the mounted `./data` volume under Docker):

- `terminal.db` — the SQLite database (users, folders, connections, settings, audit logs)
- `.enc-secret` — the auto-generated credential encryption key (if `ENCRYPTION_KEY` is not set)

**To back up, simply copy the entire `data` directory.**

## Directory Structure

```
.
├── Dockerfile            # Multi-stage build (frontend build + backend run)
├── docker-compose.yml    # Container orchestration
├── server/               # Backend: Fastify + ssh2 + better-sqlite3
└── web/                  # Frontend: Vue 3 + Vite + Naive UI + xterm.js
```

## License

This project is licensed under the [Apache License 2.0](LICENSE).

package db

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"terminal-web/server-go/internal/config"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS folders (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  parent_id INTEGER REFERENCES folders(id) ON DELETE CASCADE,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS connections (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  host TEXT NOT NULL,
  port INTEGER NOT NULL DEFAULT 22,
  username TEXT NOT NULL,
  auth_type TEXT NOT NULL DEFAULT 'password',
  password_enc TEXT,
  private_key_enc TEXT,
  passphrase_enc TEXT,
  folder_id INTEGER REFERENCES folders(id) ON DELETE SET NULL,
  jump_id INTEGER REFERENCES connections(id) ON DELETE SET NULL,
  remark TEXT,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  action TEXT NOT NULL,
  username TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT '',
  success INTEGER NOT NULL DEFAULT 0,
  detail TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sessions (
  jti TEXT PRIMARY KEY,
  uid INTEGER NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  last_activity INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS backups (
  id TEXT PRIMARY KEY,
  filename TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  size INTEGER NOT NULL DEFAULT 0,
  meta TEXT
);
`

// Open 打开（或创建）SQLite 库，开启 WAL 与 foreign_keys，执行建表语句。
// 使用 DSN pragma 控制并发；SetMaxOpenConns(1) 避免多连接写锁竞争。
func Open() error {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)",
		filepath.Join(config.DataDir, "terminal-web.db"))
	var err error
	DB, err = sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	DB.SetMaxOpenConns(1)
	if err := DB.Ping(); err != nil {
		return err
	}
	if _, err := DB.Exec(schema); err != nil {
		return err
	}
	return nil
}

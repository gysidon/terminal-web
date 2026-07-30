package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

var (
	DataDir    string
	BackupDir  string
	Port       string
	Host       string
	StaticDir  string
	MasterSecret string
	JwtSecret  string
)

// Init 加载环境变量与密钥文件。密钥文件不存在则随机生成（mode 0600）。
// 环境变量 MASTER_SECRET / JWT_SECRET 优先于文件。
func Init() error {
	DataDir = envOr("DATA_DIR", "data")
	if !filepath.IsAbs(DataDir) {
		// 以进程工作目录为基准；保持与 Node 版 process.cwd() 行为一致
		abs, err := filepath.Abs(DataDir)
		if err != nil {
			return err
		}
		DataDir = abs
	}
	if err := os.MkdirAll(DataDir, 0o755); err != nil {
		return err
	}
	BackupDir = filepath.Join(DataDir, "backups")
	if err := os.MkdirAll(BackupDir, 0o755); err != nil {
		return err
	}

	Port = envOr("PORT", "3000")
	Host = envOr("HOST", "0.0.0.0")
	StaticDir = os.Getenv("STATIC_DIR")

	var err error
	MasterSecret, err = loadOrCreateSecret(".enc-secret", "MASTER_SECRET")
	if err != nil {
		return err
	}
	JwtSecret, err = loadOrCreateSecret(".jwt-secret", "JWT_SECRET")
	if err != nil {
		return err
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func loadOrCreateSecret(filename, envName string) (string, error) {
	if v := os.Getenv(envName); v != "" {
		return v, nil
	}
	file := filepath.Join(DataDir, filename)
	if data, err := os.ReadFile(file); err == nil {
		s := trimNewline(string(data))
		if s != "" {
			return s, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(buf)
	if err := os.WriteFile(file, []byte(secret), 0o600); err != nil {
		return "", fmt.Errorf("写入密钥文件 %s 失败: %w", file, err)
	}
	return secret, nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

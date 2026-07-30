package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"terminal-web/server-go/internal/auth"
	"terminal-web/server-go/internal/config"
	"terminal-web/server-go/internal/crypto"
	"terminal-web/server-go/internal/db"
	"terminal-web/server-go/internal/routes"
	"terminal-web/server-go/internal/security"
	"terminal-web/server-go/internal/settings"
)

const bodyLimit = 2 * 1024 * 1024 * 1024

func main() {
	desktop := flag.Bool("desktop", false, "桌面模式：监听 127.0.0.1:0，输出 boot token")
	selfCheck := flag.Bool("selfcheck", false, "自检：解密凭据/校验 admin 密码/打印设置")
	resetPwd := flag.Bool("reset-password", false, "重置（或新建）用户密码：-reset-password <用户名> <新密码>，也可用环境变量 RESET_USER / RESET_PASS")
	flag.Parse()

	if *selfCheck {
		os.Exit(runSelfCheck())
	}
	if *resetPwd {
		os.Exit(runResetPassword(flag.Args()))
	}

	if err := config.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "配置初始化失败:", err)
		os.Exit(1)
	}
	crypto.Init()
	if err := db.Open(); err != nil {
		fmt.Fprintln(os.Stderr, "数据库打开失败:", err)
		os.Exit(1)
	}
	if err := settings.Seed(); err != nil {
		fmt.Fprintln(os.Stderr, "设置初始化失败:", err)
		os.Exit(1)
	}
	auth.EnsureAdmin()
	auth.DesktopMode = *desktop

	r := chi.NewRouter()
	r.Use(bodyLimitMiddleware)
	if !*desktop {
		r.Use(ipWhitelist)
	}

	routes.RegisterPublic(r)
	r.Group(func(pr chi.Router) {
		pr.Use(auth.Middleware)
		routes.RegisterProtected(pr)
		routes.RegisterSftp(pr)
		routes.RegisterProcess(pr)
		routes.RegisterSysinfo(pr)
		routes.RegisterBackup(pr)
	})
		routes.RegisterWS(r)

	if *desktop {
		ensureDesktopAdmin()
		dh := &routes.DesktopSessionHandler{Token: randomHex(32), CreatedAt: time.Now()}
		dh.Register(r)
		r.NotFound(notFoundHandler(config.StaticDir))

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			fmt.Fprintln(os.Stderr, "监听失败:", err)
			os.Exit(1)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		out, _ := json.Marshal(map[string]interface{}{
			"event": "ready",
			"port":  port,
			"token": dh.Token,
		})
		fmt.Println(string(out))

		srv := &http.Server{Handler: r}
		if err := srv.Serve(ln); err != nil {
			fmt.Fprintln(os.Stderr, "服务异常:", err)
			os.Exit(1)
		}
		return
	}

	r.NotFound(notFoundHandler(config.StaticDir))
	addr := net.JoinHostPort(config.Host, config.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "监听失败:", err)
		os.Exit(1)
	}
	fmt.Printf("[terminal-web] listening on http://%s\n", addr)
	srv := &http.Server{Handler: r}
	if err := srv.Serve(ln); err != nil {
		fmt.Fprintln(os.Stderr, "服务异常:", err)
		os.Exit(1)
	}
}

func bodyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
		next.ServeHTTP(w, r)
	})
}

func ipWhitelist(next http.Handler) http.Handler {
	public := map[string]bool{
		"/api/setup/status":  true,
		"/api/setup":         true,
		"/api/captcha":       true,
		"/api/settings/public": true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if !strings.HasPrefix(p, "/api") {
			next.ServeHTTP(w, r)
			return
		}
		if public[p] {
			next.ServeHTTP(w, r)
			return
		}
		if security.IsIPAllowed(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"当前 IP 不在允许访问的列表中"}`))
	})
}

func notFoundHandler(staticDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/api") || strings.HasPrefix(p, "/ws") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Not Found"}`))
			return
		}
		if staticDir != "" {
			clean := filepath.Clean(p)
			fpath := filepath.Join(staticDir, clean)
			if fi, err := os.Stat(fpath); err == nil && !fi.IsDir() {
				http.ServeFile(w, r, fpath)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not Found"))
	}
}

func ensureDesktopAdmin() {
	var c int
	_ = db.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&c)
	if c > 0 {
		return
	}
	hashed, err := crypto.HashPassword(randomHex(32))
	if err != nil {
		return
	}
	_, _ = db.DB.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", "admin", hashed)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// runResetPassword 重置（或新建）用户密码，行为与 Node 版 src/reset-password.js 对齐：
// 用户不存在则自动新建；新密码至少 6 位；参数缺省时读取环境变量 RESET_USER / RESET_PASS。
func runResetPassword(args []string) int {
	username := ""
	password := ""
	if len(args) > 0 {
		username = strings.TrimSpace(args[0])
	}
	if len(args) > 1 {
		password = args[1]
	}
	if username == "" {
		username = strings.TrimSpace(os.Getenv("RESET_USER"))
	}
	if password == "" {
		password = os.Getenv("RESET_PASS")
	}
	if username == "" {
		fmt.Fprintln(os.Stderr, "✗ 请提供用户名（参数1 或 环境变量 RESET_USER）")
		return 1
	}
	if password == "" {
		fmt.Fprintln(os.Stderr, "✗ 请提供新密码（参数2 或 环境变量 RESET_PASS）")
		return 1
	}
	if len(password) < 6 {
		fmt.Fprintln(os.Stderr, "✗ 密码至少 6 位")
		return 1
	}

	if err := config.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "✗ 配置初始化失败:", err)
		return 1
	}
	crypto.Init()
	if err := db.Open(); err != nil {
		fmt.Fprintln(os.Stderr, "✗ 数据库打开失败:", err)
		return 1
	}

	hashed, err := crypto.HashPassword(password)
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ 密码哈希失败:", err)
		return 1
	}
	var id int64
	err = db.DB.QueryRow("SELECT id FROM users WHERE username = ?", username).Scan(&id)
	if err == nil {
		if _, err := db.DB.Exec("UPDATE users SET password_hash = ? WHERE id = ?", hashed, id); err != nil {
			fmt.Fprintln(os.Stderr, "✗ 更新失败:", err)
			return 1
		}
		fmt.Printf("✓ 已重置用户「%s」的密码\n", username)
	} else if err == sql.ErrNoRows {
		if _, err := db.DB.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", username, hashed); err != nil {
			fmt.Fprintln(os.Stderr, "✗ 新建失败:", err)
			return 1
		}
		fmt.Printf("✓ 已新建用户「%s」并设置密码\n", username)
	} else {
		fmt.Fprintln(os.Stderr, "✗ 查询失败:", err)
		return 1
	}
	return 0
}

func runSelfCheck() int {
	if err := config.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "config init failed:", err)
		return 1
	}
	crypto.Init()
	if err := db.Open(); err != nil {
		fmt.Fprintln(os.Stderr, "db open failed:", err)
		return 1
	}

	// (a) 解密一条 password_enc
	var enc sql.NullString
	_ = db.DB.QueryRow("SELECT password_enc FROM connections WHERE password_enc IS NOT NULL LIMIT 1").Scan(&enc)
	if enc.Valid {
		plain, err := crypto.Decrypt(enc.String, crypto.DefaultKey())
		if err != nil {
			fmt.Println("FAIL: decrypt password_enc:", err)
			return 1
		}
		fmt.Printf("decrypt password_enc OK, plaintext length=%d\n", len(plain))
	} else {
		fmt.Println("skip: no connection with password_enc")
	}

	// (b) 校验 admin 密码 "123123"
	var hash sql.NullString
	_ = db.DB.QueryRow("SELECT password_hash FROM users WHERE username = 'admin'").Scan(&hash)
	if hash.Valid {
		if !crypto.VerifyPassword("123123", hash.String) {
			fmt.Println("FAIL: admin password '123123' verify")
			return 1
		}
		fmt.Println("admin password '123123' verify OK")
	} else {
		fmt.Println("skip: no admin user")
	}

	// (c) 打印全部设置
	rows, err := db.DB.Query("SELECT key, value FROM settings")
	if err != nil {
		fmt.Println("FAIL: query settings:", err)
		return 1
	}
	defer rows.Close()
	fmt.Println("settings:")
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		fmt.Printf("  %s = %s\n", k, v)
	}
	fmt.Println("selfcheck passed")
	return 0
}

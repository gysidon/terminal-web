package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"

	"terminal-web/server-go/internal/config"
	"terminal-web/server-go/internal/crypto"
	"terminal-web/server-go/internal/db"
	"terminal-web/server-go/internal/settings"
)

// DesktopMode 由 main 在桌面模式下置为 true，用于旁路部分安全设施（IP 白名单/超时/SSO/验证码）。
var DesktopMode bool

func IsInitialized() bool {
	var c int
	_ = db.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&c)
	return c > 0
}

// EnsureAdmin 仅当设置了 ADMIN_PASSWORD 环境变量时创建管理员账号（Docker 无人值守）。
func EnsureAdmin() {
	adminPw := getEnv("ADMIN_PASSWORD", "")
	if adminPw == "" {
		return
	}
	username := getEnv("ADMIN_USERNAME", "admin")
	var existing int
	_ = db.DB.QueryRow("SELECT id FROM users WHERE username = ?", username).Scan(&existing)
	if existing == 0 {
		hash, err := crypto.HashPassword(adminPw)
		if err != nil {
			return
		}
		_, _ = db.DB.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", username, hash)
	}
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// SetupAdmin 首次初始化：仅当未初始化时创建首个管理员并返回 7d JWT。
func SetupAdmin(username, password string) (string, error) {
	if IsInitialized() {
		return "", sql.ErrNoRows
	}
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return "", err
	}
	res, err := db.DB.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", username, hash)
	if err != nil {
		return "", err
	}
	uid, _ := res.LastInsertId()
	jti := CreateSession(int(uid))
	return signToken(int(uid), username, jti)
}

func Login(username, password string) (string, error) {
	var (
		id           int
		passwordHash string
	)
	err := db.DB.QueryRow("SELECT id, password_hash FROM users WHERE username = ?", username).Scan(&id, &passwordHash)
	if err != nil {
		return "", err
	}
	if !crypto.VerifyPassword(password, passwordHash) {
		return "", sql.ErrNoRows
	}
	jti := CreateSession(id)
	return signToken(id, username, jti)
}

func ChangePassword(uid int, oldPassword, newPassword string) bool {
	var passwordHash string
	err := db.DB.QueryRow("SELECT password_hash FROM users WHERE id = ?", uid).Scan(&passwordHash)
	if err != nil {
		return false
	}
	if !crypto.VerifyPassword(oldPassword, passwordHash) {
		return false
	}
	newHash, err := crypto.HashPassword(newPassword)
	if err != nil {
		return false
	}
	_, err = db.DB.Exec("UPDATE users SET password_hash = ? WHERE id = ?", newHash, uid)
	return err == nil
}

func signToken(uid int, username, jti string) (string, error) {
	claims := jwt.MapClaims{
		"uid":      uid,
		"username": username,
		"jti":      jti,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.JwtSecret))
}

// CreateSession 创建会话并返回 jti（导出供桌面模式使用）。
func CreateSession(uid int) string {
	if settings.GetBool("sso_enabled") {
		_, _ = db.DB.Exec("DELETE FROM sessions WHERE uid = ?", uid)
	}
	jti := randHex(16)
	_, _ = db.DB.Exec(
		"INSERT OR REPLACE INTO sessions (jti, uid, created_at, last_activity) VALUES (?, ?, datetime('now'), ?)",
		jti, uid, time.Now().UnixMilli())
	return jti
}

// SignToken 为指定用户签发 7d JWT（导出供桌面模式使用）。
func SignToken(uid int, username, jti string) (string, error) {
	return signToken(uid, username, jti)
}

// VerifyToken 验签，返回 claims；失败返回 nil。
func VerifyToken(tokenStr string) jwt.MapClaims {
	if tokenStr == "" {
		return nil
	}
	parsed, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, sql.ErrNoRows
		}
		return []byte(config.JwtSecret), nil
	})
	if err != nil || !parsed.Valid {
		return nil
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil
	}
	return claims
}

// VerifySession 校验 JWT + 会话存在性 + 无活动超时（桌面模式忽略超时）。
func VerifySession(tokenStr string) jwt.MapClaims {
	claims := VerifyToken(tokenStr)
	if claims == nil {
		return nil
	}
	if _, ok := claims["jti"]; !ok {
		// 向后兼容：升级前签发的旧 token（无 jti）直接放行
		return claims
	}
	jti, _ := claims["jti"].(string)
	var (
		uid           int
		lastActivity  int64
	)
	err := db.DB.QueryRow("SELECT uid, last_activity FROM sessions WHERE jti = ?", jti).Scan(&uid, &lastActivity)
	if err != nil {
		return nil
	}
	if !DesktopMode {
		timeout := settings.GetInt("session_timeout")
		if timeout > 0 && time.Now().UnixMilli()-lastActivity > int64(timeout)*60*1000 {
			_, _ = db.DB.Exec("DELETE FROM sessions WHERE jti = ?", jti)
			return nil
		}
	}
	return claims
}

func TouchSession(jti string) {
	if jti == "" {
		return
	}
	_, _ = db.DB.Exec("UPDATE sessions SET last_activity = ? WHERE jti = ?", time.Now().UnixMilli(), jti)
}

// uid/username/jti 辅助
func GetUID(c jwt.MapClaims) int {
	if v, ok := c["uid"].(float64); ok {
		return int(v)
	}
	return 0
}
func GetUsername(c jwt.MapClaims) string {
	if v, ok := c["username"].(string); ok {
		return v
	}
	return ""
}
func GetJTI(c jwt.MapClaims) string {
	if v, ok := c["jti"].(string); ok {
		return v
	}
	return ""
}

type ctxKey string

const userCtxKey ctxKey = "user"

func UserFrom(r *http.Request) jwt.MapClaims {
	if v, ok := r.Context().Value(userCtxKey).(jwt.MapClaims); ok {
		return v
	}
	return nil
}

func contextWithUser(ctx context.Context, claims jwt.MapClaims) context.Context {
	return context.WithValue(ctx, userCtxKey, claims)
}

// Middleware 受保护路由鉴权。失败写 401。
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		claims := VerifySession(token)
		if claims == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"未登录或登录已过期"}`))
			return
		}
		// REST 鉴权成功后刷新会话活跃时间（/api/connections/.../ping 前端轮询用，不 touch）
		if !strings.HasSuffix(r.URL.Path, "/ping") {
			if jti := GetJTI(claims); jti != "" {
				TouchSession(jti)
			}
		}
		ctx := contextWithUser(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return r.URL.Query().Get("token")
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// chi 路由参数辅助（避免直接依赖 chi 于多处）
func Param(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}

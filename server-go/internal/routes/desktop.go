package routes

import (
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"terminal-web/server-go/internal/auth"
	"terminal-web/server-go/internal/db"
)

// DesktopSessionHandler 仅在桌面模式下注册，用于一次性 boot token 换取正常 JWT。
type DesktopSessionHandler struct {
	mu        sync.Mutex
	Token     string
	CreatedAt time.Time
}

// Register 注册 /api/desktop/session。
func (h *DesktopSessionHandler) Register(r chi.Router) {
	r.Post("/api/desktop/session", h.handle)
}

func (h *DesktopSessionHandler) handle(w http.ResponseWriter, req *http.Request) {
	var b struct {
		Token string `json:"token"`
	}
	_ = readJSON(req, &b)

	h.mu.Lock()
	if h.Token == "" {
		h.mu.Unlock()
		writeError(w, 401, "boot token 已失效")
		return
	}
	if time.Since(h.CreatedAt) > 30*time.Second {
		h.Token = "" // 超时作废
		h.mu.Unlock()
		writeError(w, 401, "boot token 已过期")
		return
	}
	if h.Token != b.Token {
		h.mu.Unlock()
		writeError(w, 401, "boot token 无效")
		return
	}
	h.Token = "" // 一次性：兑换成功立即作废
	h.mu.Unlock()

	var uid int
	var username string
	err := db.DB.QueryRow("SELECT id, username FROM users WHERE username = ?", "admin").Scan(&uid, &username)
	if err != nil {
		writeError(w, 500, "未找到管理员账号")
		return
	}
	jti := auth.CreateSession(uid)
	token, err := auth.SignToken(uid, username, jti)
	if err != nil {
		writeError(w, 500, "签发 token 失败")
		return
	}
	writeJSON(w, 200, map[string]string{"token": token, "username": username})
}

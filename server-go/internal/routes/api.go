package routes

import (
	"database/sql"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"terminal-web/server-go/internal/auth"
	"terminal-web/server-go/internal/captcha"
	"terminal-web/server-go/internal/crypto"
	"terminal-web/server-go/internal/db"
	"terminal-web/server-go/internal/security"
	"terminal-web/server-go/internal/settings"
	"terminal-web/server-go/internal/sshx"
)

// sanitizeConn 脱敏：去掉 *_enc 三字段，增加 has_* 布尔。
func sanitizeConn(c *sshx.ConnRow) map[string]interface{} {
	out := map[string]interface{}{
		"id":             c.ID,
		"name":           c.Name,
		"host":           c.Host,
		"port":           c.Port,
		"username":       c.Username,
		"auth_type":      c.AuthType,
		"folder_id":      nil,
		"jump_id":        nil,
		"remark":         nil,
		"sort_order":     c.SortOrder,
		"created_at":     c.CreatedAt,
		"updated_at":     c.UpdatedAt,
		"has_password":   c.PasswordEnc != "",
		"has_private_key": c.PrivateKeyEnc != "",
		"has_passphrase":  c.PassphraseEnc != "",
	}
	if c.FolderID.Valid {
		out["folder_id"] = c.FolderID.Int64
	}
	if c.JumpID.Valid {
		out["jump_id"] = c.JumpID.Int64
	}
	if c.Remark.Valid {
		out["remark"] = c.Remark.String
	}
	return out
}

func getConnRow(id int) (*sshx.ConnRow, error) {
	return sshx.GetConnectionByID(id)
}

// RegisterPublic 注册公开接口（无需登录）。
func RegisterPublic(r chi.Router) {
	r.Get("/api/setup/status", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, 200, map[string]bool{"initialized": auth.IsInitialized()})
	})

	r.Post("/api/setup", func(w http.ResponseWriter, req *http.Request) {
		if auth.IsInitialized() {
			writeError(w, 403, "系统已初始化，请直接登录")
			return
		}
		var b struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		_ = readJSON(req, &b)
		if b.Username == "" || b.Password == "" {
			writeError(w, 400, "请输入用户名和密码")
			return
		}
		if len(b.Password) < 6 {
			writeError(w, 400, "密码至少 6 位")
			return
		}
		token, err := auth.SetupAdmin(b.Username, b.Password)
		if err != nil || token == "" {
			writeError(w, 403, "初始化失败")
			return
		}
		writeJSON(w, 200, map[string]string{"token": token, "username": b.Username})
	})

	r.Get("/api/captcha", func(w http.ResponseWriter, req *http.Request) {
		id, svg := captcha.Create()
		writeJSON(w, 200, map[string]string{"id": id, "svg": svg})
	})

	r.Get("/api/settings/public", func(w http.ResponseWriter, req *http.Request) {
		s := settings.GetAll()
		resp := map[string]interface{}{
			"captchaEnabled": s["captcha_enabled"],
			"theme":          s["theme"],
			"fontFamily":     s["font_family"],
			"fontSize":       s["font_size"],
		}
		if auth.DesktopMode {
			resp["desktopMode"] = true
		} else {
			resp["desktopMode"] = false
		}
		writeJSON(w, 200, resp)
	})

	r.Post("/api/login", func(w http.ResponseWriter, req *http.Request) {
		var b struct {
			Username    string `json:"username"`
			Password    string `json:"password"`
			CaptchaID   string `json:"captchaId"`
			CaptchaText string `json:"captchaText"`
		}
		_ = readJSON(req, &b)
		if b.Username == "" || b.Password == "" {
			writeError(w, 400, "请输入用户名和密码")
			return
		}
		ip := security.GetClientIP(req)
		if lockUntil := security.GetLockUntil(b.Username); lockUntil > 0 {
			mins := int(math.Ceil(float64(lockUntil-nowMs()) / 60000))
			if mins < 1 {
				mins = 1
			}
			security.LogAudit("login", b.Username, ip, false, "账户已锁定")
			writeError(w, 429, "账户已锁定，请于 "+strconv.Itoa(mins)+" 分钟后再试")
			return
		}
		s := settings.GetAll()
		if s["captcha_enabled"] == true && !captcha.Verify(b.CaptchaID, b.CaptchaText) {
			writeError(w, 400, "验证码错误")
			return
		}
		token, err := auth.Login(b.Username, b.Password)
		if err != nil || token == "" {
			security.RecordFail(b.Username)
			security.LogAudit("login", b.Username, ip, false, "密码错误")
			writeError(w, 401, "用户名或密码错误")
			return
		}
		security.ClearFail(b.Username)
		security.LogAudit("login", b.Username, ip, true, "")
		writeJSON(w, 200, map[string]string{"token": token, "username": b.Username})
	})
}

// RegisterProtected 注册需登录的接口。
func RegisterProtected(r chi.Router) {
	r.Post("/api/activity", func(w http.ResponseWriter, req *http.Request) {
		if claims := auth.UserFrom(req); claims != nil {
			if jti := auth.GetJTI(claims); jti != "" {
				auth.TouchSession(jti)
			}
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})

	r.Get("/api/settings", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, 200, settings.GetAll())
	})

	r.Put("/api/settings", func(w http.ResponseWriter, req *http.Request) {
		var body map[string]interface{}
		_ = readJSON(req, &body)
		updated, err := settings.Put(body)
		if err != nil {
			writeError(w, 500, "保存设置失败")
			return
		}
		writeJSON(w, 200, updated)
	})

	r.Get("/api/audit", func(w http.ResponseWriter, req *http.Request) {
		limitStr := req.URL.Query().Get("limit")
		limit := 100
		if limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil {
				limit = n
			}
		}
		rows, err := security.RecentAudit(limit)
		if err != nil {
			writeError(w, 500, "读取审计日志失败")
			return
		}
		writeJSON(w, 200, rows)
	})

	r.Post("/api/change-password", func(w http.ResponseWriter, req *http.Request) {
		var b struct {
			OldPassword string `json:"oldPassword"`
			NewPassword string `json:"newPassword"`
		}
		_ = readJSON(req, &b)
		if b.NewPassword == "" || len(b.NewPassword) < 6 {
			writeError(w, 400, "新密码至少6位")
			return
		}
		claims := auth.UserFrom(req)
		uid := 0
		if claims != nil {
			uid = auth.GetUID(claims)
		}
		if !auth.ChangePassword(uid, b.OldPassword, b.NewPassword) {
			writeError(w, 400, "原密码错误")
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})

	// ---------- 文件夹 ----------
	r.Get("/api/folders", func(w http.ResponseWriter, req *http.Request) {
		rows, err := queryFolders()
		if err != nil {
			writeError(w, 500, "读取文件夹失败")
			return
		}
		writeJSON(w, 200, rows)
	})

	r.Post("/api/folders", func(w http.ResponseWriter, req *http.Request) {
		var b struct {
			Name     string `json:"name"`
			ParentID *int64 `json:"parent_id"`
		}
		_ = readJSON(req, &b)
		if b.Name == "" {
			writeError(w, 400, "文件夹名称不能为空")
			return
		}
		var pid interface{}
		if b.ParentID != nil {
			pid = *b.ParentID
		} else {
			pid = nil
		}
		res, err := db.DB.Exec("INSERT INTO folders (name, parent_id) VALUES (?, ?)", b.Name, pid)
		if err != nil {
			writeError(w, 500, "创建文件夹失败")
			return
		}
		id, _ := res.LastInsertId()
		row, _ := getFolder(int(id))
		writeJSON(w, 200, row)
	})

	r.Put("/api/folders/{id}", func(w http.ResponseWriter, req *http.Request) {
		id, _ := strconv.Atoi(strParam(req, "id"))
		var b struct {
			Name     *string `json:"name"`
			ParentID *int64  `json:"parent_id"`
		}
		_ = readJSON(req, &b)
		if b.ParentID != nil && *b.ParentID == int64(id) {
			writeError(w, 400, "不能移动到自身")
			return
		}
		var name interface{}
		if b.Name != nil {
			name = *b.Name
		} else {
			name = nil
		}
		var pid interface{}
		if b.ParentID != nil {
			pid = *b.ParentID
		} else {
			pid = nil
		}
		_, err := db.DB.Exec("UPDATE folders SET name = COALESCE(?, name), parent_id = ? WHERE id = ?", name, pid, id)
		if err != nil {
			writeError(w, 500, "更新文件夹失败")
			return
		}
		row, _ := getFolder(id)
		writeJSON(w, 200, row)
	})

	r.Delete("/api/folders/{id}", func(w http.ResponseWriter, req *http.Request) {
		id, _ := strconv.Atoi(strParam(req, "id"))
		_, _ = db.DB.Exec("DELETE FROM folders WHERE id = ?", id)
		writeJSON(w, 200, map[string]bool{"ok": true})
	})

	// ---------- 连接 ----------
	r.Get("/api/connections", func(w http.ResponseWriter, req *http.Request) {
		rows, err := db.DB.Query("SELECT id, name, host, port, username, auth_type, password_enc, private_key_enc, passphrase_enc, folder_id, jump_id, remark, sort_order, created_at, updated_at FROM connections ORDER BY sort_order, name")
		if err != nil {
			writeError(w, 500, "读取连接失败")
			return
		}
		defer rows.Close()
		out := []map[string]interface{}{}
		for rows.Next() {
			c, err := scanConn(rows)
			if err != nil {
				continue
			}
			out = append(out, sanitizeConn(c))
		}
		writeJSON(w, 200, out)
	})

	r.Post("/api/connections", func(w http.ResponseWriter, req *http.Request) {
		var b struct {
			Name       string      `json:"name"`
			Host       string      `json:"host"`
			Port       interface{} `json:"port"`
			Username   string      `json:"username"`
			AuthType   string      `json:"auth_type"`
			Password   string      `json:"password"`
			PrivateKey string      `json:"private_key"`
			Passphrase string      `json:"passphrase"`
			FolderID   interface{} `json:"folder_id"`
			JumpID     interface{} `json:"jump_id"`
			Remark     *string     `json:"remark"`
			CopyFrom   interface{} `json:"copy_from"`
		}
		_ = readJSON(req, &b)
		if b.Name == "" || b.Host == "" || b.Username == "" {
			writeError(w, 400, "名称、主机、用户名为必填项")
			return
		}
		name := b.Name
		// 复制场景：同名自动加数字后缀
		if b.CopyFrom != nil {
			names := existingConnNames()
			if contains(names, name) {
				i := 2
				for contains(names, name+strconv.Itoa(i)) {
					i++
				}
				name = name + strconv.Itoa(i)
			}
		}
		passwordEnc, _ := crypto.Encrypt(b.Password, crypto.DefaultKey())
		keyEnc, _ := crypto.Encrypt(b.PrivateKey, crypto.DefaultKey())
		passphraseEnc, _ := crypto.Encrypt(b.Passphrase, crypto.DefaultKey())
		if b.CopyFrom != nil {
			srcID := toInt(b.CopyFrom)
			src, err := getConnRow(srcID)
			if err != nil || src == nil {
				writeError(w, 404, "复制源连接不存在")
				return
			}
			if b.Password == "" {
				passwordEnc = src.PasswordEnc
			}
			if b.PrivateKey == "" {
				keyEnc = src.PrivateKeyEnc
			}
			if b.Passphrase == "" {
				passphraseEnc = src.PassphraseEnc
			}
		}
		jumpID := toIntOrNil(b.JumpID)
		if !isInt(b.JumpID) {
			jumpID = nil
		}
		port := toIntDefault(b.Port, 22)
		folderID := toIntOrNil(b.FolderID)
		var remarkStr string
		if b.Remark != nil {
			remarkStr = *b.Remark
		}
		remark := nullString(remarkStr)
		authType := "password"
		if b.AuthType == "key" {
			authType = "key"
		}
		res, err := db.DB.Exec(`INSERT INTO connections
			(name, host, port, username, auth_type, password_enc, private_key_enc, passphrase_enc, folder_id, jump_id, remark)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			name, b.Host, port, b.Username, authType, passwordEnc, keyEnc, passphraseEnc, folderID, jumpID, remark)
		if err != nil {
			writeError(w, 500, "创建连接失败")
			return
		}
		id, _ := res.LastInsertId()
		c, _ := getConnRow(int(id))
		writeJSON(w, 200, sanitizeConn(c))
	})

	r.Put("/api/connections/{id}", func(w http.ResponseWriter, req *http.Request) {
		id, _ := strconv.Atoi(strParam(req, "id"))
		old, err := getConnRow(id)
		if err != nil || old == nil {
			writeError(w, 404, "连接不存在")
			return
		}
		var b struct {
			Name       string      `json:"name"`
			Host       string      `json:"host"`
			Port       interface{} `json:"port"`
			Username   string      `json:"username"`
			AuthType   string      `json:"auth_type"`
			Password   string      `json:"password"`
			PrivateKey string      `json:"private_key"`
			Passphrase string      `json:"passphrase"`
			FolderID   interface{} `json:"folder_id"`
			JumpID     interface{} `json:"jump_id"`
			Remark     *string     `json:"remark"`
		}
		_ = readJSON(req, &b)
		if isInt(b.JumpID) && toInt(b.JumpID) == id {
			writeError(w, 400, "跳板机不能选择自己")
			return
		}
		passwordEnc := old.PasswordEnc
		if b.Password != "" {
			passwordEnc, _ = crypto.Encrypt(b.Password, crypto.DefaultKey())
		}
		keyEnc := old.PrivateKeyEnc
		if b.PrivateKey != "" {
			keyEnc, _ = crypto.Encrypt(b.PrivateKey, crypto.DefaultKey())
		}
		passphraseEnc := old.PassphraseEnc
		if b.Passphrase != "" {
			passphraseEnc, _ = crypto.Encrypt(b.Passphrase, crypto.DefaultKey())
		}
		jumpID := old.JumpID
		if b.JumpID != nil {
			jumpID = sqlNullInt64(toIntOrNil(b.JumpID))
		}
		folderID := old.FolderID
		if b.FolderID != nil {
			folderID = sqlNullInt64(toIntOrNil(b.FolderID))
		}
		remark := old.Remark
		if b.Remark != nil {
			remark = nullString(*b.Remark)
			if *b.Remark == "" {
				remark = sql.NullString{String: "", Valid: true}
			}
		}
		authType := old.AuthType
		if b.AuthType == "key" {
			authType = "key"
		} else if b.AuthType != "" {
			authType = "password"
		}
		_, err = db.DB.Exec(`UPDATE connections SET
			name = ?, host = ?, port = ?, username = ?, auth_type = ?,
			password_enc = ?, private_key_enc = ?, passphrase_enc = ?,
			folder_id = ?, jump_id = ?, remark = ?, updated_at = datetime('now')
			WHERE id = ?`,
			orDefault(b.Name, old.Name), orDefault(b.Host, old.Host), toIntDefault(b.Port, old.Port),
			orDefault(b.Username, old.Username), authType,
			passwordEnc, keyEnc, passphraseEnc,
			folderID, jumpID, remark, id)
		if err != nil {
			writeError(w, 500, "更新连接失败")
			return
		}
		sshx.DropSftp(id)
		c, _ := getConnRow(id)
		writeJSON(w, 200, sanitizeConn(c))
	})

	r.Delete("/api/connections/{id}", func(w http.ResponseWriter, req *http.Request) {
		id, _ := strconv.Atoi(strParam(req, "id"))
		sshx.DropSftp(id)
		_, _ = db.DB.Exec("UPDATE connections SET jump_id = NULL WHERE jump_id = ?", id)
		_, _ = db.DB.Exec("DELETE FROM connections WHERE id = ?", id)
		writeJSON(w, 200, map[string]bool{"ok": true})
	})

	r.Post("/api/connections/{id}/test", func(w http.ResponseWriter, req *http.Request) {
		id, _ := strconv.Atoi(strParam(req, "id"))
		conn, err := getConnRow(id)
		if err != nil || conn == nil {
			writeError(w, 404, "连接不存在")
			return
		}
		if err := sshx.TestConnection(conn); err != nil {
			writeError(w, 400, "连接失败: "+err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})

	r.Post("/api/connections/test", func(w http.ResponseWriter, req *http.Request) {
		var cfg map[string]interface{}
		_ = readJSON(req, &cfg)
		host, _ := cfg["host"].(string)
		username, _ := cfg["username"].(string)
		if host == "" || username == "" {
			writeError(w, 400, "请填写主机和用户名")
			return
		}
		if err := sshx.TestConnectionConfig(cfg); err != nil {
			writeError(w, 400, "连接失败: "+err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})

	r.Get("/api/connections/{id}/ping", func(w http.ResponseWriter, req *http.Request) {
		id, _ := strconv.Atoi(strParam(req, "id"))
		conn, err := getConnRow(id)
		if err != nil || conn == nil {
			writeError(w, 404, "连接不存在")
			return
		}
		client, err := sshx.GetClient(id)
		if err != nil {
			writeError(w, 400, "探测失败: "+err.Error())
			return
		}
		latency, err := sshx.ExecTrue(client)
		if err != nil {
			writeError(w, 400, "探测失败: "+err.Error())
			return
		}
		writeJSON(w, 200, map[string]int{"latency": latency})
	})
}

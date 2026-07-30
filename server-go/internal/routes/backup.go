package routes

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

	"terminal-web/server-go/internal/config"
	"terminal-web/server-go/internal/crypto"
	"terminal-web/server-go/internal/db"
	"terminal-web/server-go/internal/sshx"
)

func randHexID8() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func connRowToMap(c *sshx.ConnRow) map[string]interface{} {
	m := map[string]interface{}{
		"id":            c.ID,
		"name":          c.Name,
		"host":          c.Host,
		"port":          c.Port,
		"username":      c.Username,
		"auth_type":     c.AuthType,
		"password_enc":  nilOrStr(c.PasswordEnc),
		"private_key_enc": nilOrStr(c.PrivateKeyEnc),
		"passphrase_enc":  nilOrStr(c.PassphraseEnc),
		"folder_id":     nilOrInt(c.FolderID),
		"jump_id":       nilOrInt(c.JumpID),
		"remark":        nilOrStr(c.Remark.String),
		"sort_order":    c.SortOrder,
		"created_at":    nilOrStr(c.CreatedAt),
		"updated_at":    nilOrStr(c.UpdatedAt),
	}
	return m
}

func nilOrStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nilOrInt(n sql.NullInt64) interface{} {
	if !n.Valid {
		return nil
	}
	return n.Int64
}

func getAllConnections() ([]*sshx.ConnRow, error) {
	rows, err := db.DB.Query("SELECT id, name, host, port, username, auth_type, password_enc, private_key_enc, passphrase_enc, folder_id, jump_id, remark, sort_order, created_at, updated_at FROM connections")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*sshx.ConnRow
	for rows.Next() {
		c, err := scanConn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// rekeyPayload 跨机 re-key：备份密文用备份机主密钥加密；导入时用备份密钥解密再用本机密钥重写。同机原样保留。
func rekeyPayload(payload *string, backupSecret string) *string {
	if payload == nil || *payload == "" {
		return nil
	}
	if backupSecret == crypto.GetMasterSecret() {
		return payload
	}
	plain, err := crypto.Decrypt(*payload, crypto.DeriveKey(backupSecret))
	if err != nil {
		return payload
	}
	enc, err := crypto.Encrypt(plain, crypto.DefaultKey())
	if err != nil {
		return payload
	}
	return &enc
}

type backupConn struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	Host          string  `json:"host"`
	Port          int     `json:"port"`
	Username      string  `json:"username"`
	AuthType      string  `json:"auth_type"`
	PasswordEnc   *string `json:"password_enc"`
	PrivateKeyEnc *string `json:"private_key_enc"`
	PassphraseEnc *string `json:"passphrase_enc"`
	FolderID      *int    `json:"folder_id"`
	JumpID        *int    `json:"jump_id"`
	Remark        *string `json:"remark"`
	SortOrder     int     `json:"sort_order"`
	CreatedAt     *string `json:"created_at"`
	UpdatedAt     *string `json:"updated_at"`
}

type backupFolder struct {
	ID        int     `json:"id"`
	Name      string  `json:"name"`
	ParentID  *int    `json:"parent_id"`
	SortOrder int     `json:"sort_order"`
	CreatedAt *string `json:"created_at"`
}

type backupSetting struct {
	Key   string  `json:"key"`
	Value *string `json:"value"`
}

type backupData struct {
	Folders    []backupFolder  `json:"folders"`
	Connections []backupConn    `json:"connections"`
	Settings   []backupSetting `json:"settings"`
}

type backupObj struct {
	Meta  map[string]interface{} `json:"meta"`
	Secret string                `json:"secret"`
	Data  backupData             `json:"data"`
}

func applyBackup(obj *backupObj) error {
	data := obj.Data
	tx, err := db.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM connections"); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM folders"); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM settings"); err != nil {
		return err
	}

	insF, err := tx.Prepare("INSERT INTO folders (id, name, parent_id, sort_order, created_at) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	for _, f := range data.Folders {
		var pid interface{}
		if f.ParentID != nil {
			pid = *f.ParentID
		}
		var cat interface{}
		if f.CreatedAt != nil {
			cat = *f.CreatedAt
		}
		if _, err := insF.Exec(f.ID, f.Name, pid, f.SortOrder, cat); err != nil {
			return err
		}
	}

	insS, err := tx.Prepare("INSERT INTO settings (key, value) VALUES (?, ?)")
	if err != nil {
		return err
	}
	for _, s := range data.Settings {
		var val interface{}
		if s.Value != nil {
			val = *s.Value
		}
		if _, err := insS.Exec(s.Key, val); err != nil {
			return err
		}
	}

	insC, err := tx.Prepare(`INSERT INTO connections
		(id, name, host, port, username, auth_type, password_enc, private_key_enc, passphrase_enc,
		 folder_id, jump_id, remark, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for _, c := range data.Connections {
		var pid, jid, rid interface{}
		if c.FolderID != nil {
			pid = *c.FolderID
		}
		if c.JumpID != nil {
			jid = *c.JumpID
		}
		if c.Remark != nil {
			rid = *c.Remark
		}
		var cat, uat interface{}
		if c.CreatedAt != nil {
			cat = *c.CreatedAt
		}
		if c.UpdatedAt != nil {
			uat = *c.UpdatedAt
		}
		if _, err := insC.Exec(
			c.ID, c.Name, c.Host, c.Port, c.Username, c.AuthType,
			rekeyPayload(c.PasswordEnc, obj.Secret),
			rekeyPayload(c.PrivateKeyEnc, obj.Secret),
			rekeyPayload(c.PassphraseEnc, obj.Secret),
			pid, jid, rid, c.SortOrder, cat, uat); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RegisterBackup 注册备份/恢复接口（受保护）。
func RegisterBackup(r chi.Router) {
	r.Post("/api/backup/create", func(w http.ResponseWriter, req *http.Request) {
		conns, err := getAllConnections()
		if err != nil {
			writeError(w, 500, "读取连接失败")
			return
		}
		foldersRows, err := queryFolders()
		if err != nil {
			writeError(w, 500, "读取文件夹失败")
			return
		}
		settingsRows, err := db.DB.Query("SELECT key, value FROM settings")
		if err != nil {
			writeError(w, 500, "读取设置失败")
			return
		}
		settingsList := []map[string]string{}
		for settingsRows.Next() {
			var k, v string
			if err := settingsRows.Scan(&k, &v); err == nil {
				settingsList = append(settingsList, map[string]string{"key": k, "value": v})
			}
		}
		settingsRows.Close()

		connMaps := make([]map[string]interface{}, 0, len(conns))
		for _, c := range conns {
			connMaps = append(connMaps, connRowToMap(c))
		}

		obj := map[string]interface{}{
			"meta": map[string]interface{}{
				"app":         "terminal-web",
				"version":     1,
				"exportedAt":  time.Now().UTC().Format(time.RFC3339),
			},
			"secret": crypto.GetMasterSecret(),
			"data": map[string]interface{}{
				"folders":    foldersRows,
				"connections": connMaps,
				"settings":    settingsList,
			},
		}
		id := randHexID8()
		stamp := time.Now().Format("20060102")
		filename := "terminal-web-backup-" + stamp + ".json"
		filePath := filepath.Join(config.BackupDir, id+".json")
		content, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			writeError(w, 500, "生成备份失败")
			return
		}
		if err := os.WriteFile(filePath, content, 0o600); err != nil {
			writeError(w, 500, "写入备份文件失败")
			return
		}
		info, _ := os.Stat(filePath)
		size := 0
		if info != nil {
			size = int(info.Size())
		}
		meta, _ := json.Marshal(map[string]int{"connections": len(conns), "folders": len(foldersRows)})
		_, err = db.DB.Exec("INSERT INTO backups (id, filename, created_at, size, meta) VALUES (?, ?, datetime('now'), ?, ?)",
			id, filename, size, string(meta))
		if err != nil {
			writeError(w, 500, "记录备份失败")
			return
		}
		writeJSON(w, 200, map[string]interface{}{"id": id, "filename": filename, "size": size})
	})

	r.Get("/api/backup/list", func(w http.ResponseWriter, req *http.Request) {
		rows, err := db.DB.Query("SELECT id, filename, created_at, size FROM backups ORDER BY created_at DESC")
		if err != nil {
			writeError(w, 500, "读取备份列表失败")
			return
		}
		defer rows.Close()
		out := []map[string]interface{}{}
		for rows.Next() {
			var id, filename string
			var createdAt string
			var size int
			if err := rows.Scan(&id, &filename, &createdAt, &size); err != nil {
				continue
			}
			out = append(out, map[string]interface{}{
				"id": id, "filename": filename, "created_at": createdAt, "size": size,
			})
		}
		writeJSON(w, 200, out)
	})

	r.Get("/api/backup/download/{id}", func(w http.ResponseWriter, req *http.Request) {
		id := strParam(req, "id")
		var filename string
		err := db.DB.QueryRow("SELECT filename FROM backups WHERE id = ?", id).Scan(&filename)
		if err != nil {
			writeError(w, 404, "备份不存在")
			return
		}
		filePath := filepath.Join(config.BackupDir, id+".json")
		if _, err := os.Stat(filePath); err != nil {
			writeError(w, 404, "备份文件已丢失")
			return
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			writeError(w, 404, "备份文件已丢失")
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(content)
	})

	r.Delete("/api/backup/{id}", func(w http.ResponseWriter, req *http.Request) {
		id := strParam(req, "id")
		var filename string
		_ = db.DB.QueryRow("SELECT filename FROM backups WHERE id = ?", id).Scan(&filename)
		_, _ = db.DB.Exec("DELETE FROM backups WHERE id = ?", id)
		_ = os.Remove(filepath.Join(config.BackupDir, id+".json"))
		writeJSON(w, 200, map[string]bool{"ok": true})
	})

	r.Post("/api/backup/restore/{id}", func(w http.ResponseWriter, req *http.Request) {
		id := strParam(req, "id")
		var filename string
		err := db.DB.QueryRow("SELECT filename FROM backups WHERE id = ?", id).Scan(&filename)
		if err != nil {
			writeError(w, 404, "备份不存在")
			return
		}
		filePath := filepath.Join(config.BackupDir, id+".json")
		if _, err := os.Stat(filePath); err != nil {
			writeError(w, 404, "备份文件已丢失")
			return
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			writeError(w, 500, "读取备份失败")
			return
		}
		var obj backupObj
		if err := json.Unmarshal(content, &obj); err != nil {
			writeError(w, 500, "读取备份失败")
			return
		}
		if err := applyBackup(&obj); err != nil {
			writeError(w, 500, "恢复失败: "+err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})

	r.Post("/api/backup/import", func(w http.ResponseWriter, req *http.Request) {
		mr, err := req.MultipartReader()
		if err != nil {
			writeError(w, 400, "未收到文件")
			return
		}
		var content []byte
		for {
			part, perr := mr.NextPart()
			if perr == io.EOF {
				break
			}
			if perr != nil {
				writeError(w, 400, "未收到文件")
				return
			}
			if part.FileName() == "" {
				_ = part.Close()
				continue
			}
			content, err = io.ReadAll(part)
			_ = part.Close()
			if err != nil {
				writeError(w, 400, "未收到文件")
				return
			}
			break
		}
		if len(content) == 0 {
			writeError(w, 400, "未收到文件")
			return
		}
		var obj backupObj
		if err := json.Unmarshal(content, &obj); err != nil {
			writeError(w, 400, "文件不是合法的备份 JSON")
			return
		}
		if obj.Data.Connections == nil && obj.Data.Folders == nil && obj.Data.Settings == nil {
			writeError(w, 400, "备份文件格式不正确")
			return
		}
		if err := applyBackup(&obj); err != nil {
			writeError(w, 500, "导入失败: "+err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
}

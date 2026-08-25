package routes

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"terminal-web/server-go/internal/sshx"
)

func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	n := path.Clean(p)
	if !strings.HasPrefix(n, "/") {
		return "/" + n
	}
	return n
}

func statToItem(name string, attrs *sftp.FileStat, dir string) map[string]interface{} {
	isDir := (attrs.Mode & 0o170000) == 0o040000
	isLink := (attrs.Mode & 0o170000) == 0o120000
	typ := "file"
	if isDir {
		typ = "dir"
	} else if isLink {
		typ = "link"
	}
	return map[string]interface{}{
		"name":  name,
		"path":  path.Join(dir, name),
		"type":  typ,
		"size":  attrs.Size,
		"mode":  "0" + itoaBase(int(attrs.Mode&0o777), 8),
		"mtime": int64(attrs.Mtime) * 1000, // Mtime 为 uint32，必须先扩宽再乘，否则溢出回绕
	}
}

func itoaBase(n, base int) string {
	if n == 0 {
		return "0"
	}
	digits := "0123456789abcdef"
	var b strings.Builder
	for n > 0 {
		b.WriteByte(digits[n%base])
		n /= base
	}
	r := []byte(b.String())
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

func percentEncode(s string) string {
	e := url.QueryEscape(s)
	return strings.ReplaceAll(e, "+", "%20")
}

// shellQuote 将路径安全地包进单引号，转义内部的单引号，避免远端命令注入。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func sortItems(items []map[string]interface{}) {
	sort.SliceStable(items, func(i, j int) bool {
		di := 0
		if items[i]["type"] == "dir" {
			di = 1
		}
		dj := 0
		if items[j]["type"] == "dir" {
			dj = 1
		}
		if di != dj {
			return di > dj
		}
		return strings.Compare(items[i]["name"].(string), items[j]["name"].(string)) < 0
	})
}

// isPermError 判断 SFTP 错误是否为权限拒绝（状态码 3，或消息含 permission）。
func isPermError(err error) bool {
	if err == nil {
		return false
	}
	if se, ok := err.(*sftp.StatusError); ok && se.Code == 3 {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "permission")
}

// randTempName 生成远端 /tmp 下的随机临时文件名（sudo 写入中转用）。
func randTempName() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "/tmp/.tw-write-" + hex.EncodeToString(b) + ".tmp"
}

// runSudoFallback 先尝试免密 sudo（-n），失败再用 -S 从 stdin 喂密码重试。
// tmpl 中的 {{SUDO}} 占位符会被替换为对应的 sudo 前缀。
func runSudoFallback(client *ssh.Client, tmpl, password string) (string, error) {
	if out, err := sshx.RunCommand(client, strings.ReplaceAll(tmpl, "{{SUDO}}", "sudo -n")); err == nil {
		return out, nil
	}
	return sshx.RunCommandStdin(client, strings.ReplaceAll(tmpl, "{{SUDO}}", "sudo -S -p ''"), password+"\n")
}

// sudoWriteStream 先把 r 的内容经 SFTP 写到 /tmp 临时文件，再用 sudo tee 落到目标路径。
// 用于普通 SFTP 写遇权限拒绝时的提权回退（无开关，默认开启）。
func sudoWriteStream(sftpClient *sftp.Client, connID int, target string, r io.Reader) (string, bool) {
	tmp := randTempName()
	tf, terr := sftpClient.Create(tmp)
	if terr != nil {
		return "创建临时文件失败: " + terr.Error(), false
	}
	if _, cerr := io.Copy(tf, r); cerr != nil {
		_ = tf.Close()
		_ = sftpClient.Remove(tmp)
		return "写入临时文件失败: " + cerr.Error(), false
	}
	_ = tf.Close()
	defer func() { _ = sftpClient.Remove(tmp) }()
	sshClient, serr := sshx.GetClient(connID)
	if serr != nil {
		return "获取 SSH 连接失败: " + serr.Error(), false
	}
	pw, perr := sshx.GetLoginPassword(connID)
	if perr != nil {
		return "读取登录凭据失败: " + perr.Error(), false
	}
	tmpl := "cat " + shellQuote(tmp) + " | {{SUDO}} tee " + shellQuote(target) + " > /dev/null"
	if out, e2 := runSudoFallback(sshClient, tmpl, pw); e2 != nil {
		return "sudo 写入失败: " + e2.Error() + " " + out + "（请确认该账号有 sudo 权限，建议配置 NOPASSWD，或改用有写权限的账号连接）", false
	}
	return "", true
}

// sudoShell 以 sudo 回退方式执行一条命令（mkdir/rename/delete 的权限升级）。
func sudoShell(connID int, tmpl string) (string, bool) {
	sshClient, err := sshx.GetClient(connID)
	if err != nil {
		return "获取 SSH 连接失败: " + err.Error(), false
	}
	pw, err := sshx.GetLoginPassword(connID)
	if err != nil {
		return "读取登录凭据失败: " + err.Error(), false
	}
	if out, serr := runSudoFallback(sshClient, tmpl, pw); serr != nil {
		return "sudo 执行失败: " + serr.Error() + " " + out + "（请确认该账号有 sudo 权限，建议配置 NOPASSWD）", false
	}
	return "", true
}

// RegisterSftp 注册 SFTP 接口（受保护）。
func RegisterSftp(r chi.Router) {
	r.Get("/api/sftp/{connId}/list", func(w http.ResponseWriter, req *http.Request) {
		connID, _ := strconv.Atoi(strParam(req, "connId"))
		dir := normalizePath(req.URL.Query().Get("path"))
		client, err := sshx.GetSftp(connID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		entries, err := client.ReadDir(dir)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		items := make([]map[string]interface{}, 0, len(entries))
		for _, e := range entries {
			if attrs, ok := e.Sys().(*sftp.FileStat); ok {
				items = append(items, statToItem(e.Name(), attrs, dir))
			}
		}
		sortItems(items)
		writeJSON(w, 200, map[string]interface{}{"path": dir, "items": items})
	})

	r.Get("/api/sftp/{connId}/download", func(w http.ResponseWriter, req *http.Request) {
		connID, _ := strconv.Atoi(strParam(req, "connId"))
		file := normalizePath(req.URL.Query().Get("path"))
		client, err := sshx.GetSftp(connID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		f, err := client.Open(file)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		defer f.Close()
		w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''`+percentEncode(path.Base(file)))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(200)
		_, _ = io.Copy(w, f)
	})

	r.Get("/api/sftp/{connId}/read", func(w http.ResponseWriter, req *http.Request) {
		connID, _ := strconv.Atoi(strParam(req, "connId"))
		file := normalizePath(req.URL.Query().Get("path"))
		const max = 2 * 1024 * 1024
		client, err := sshx.GetSftp(connID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		f, err := client.Open(file)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		defer f.Close()
		buf := make([]byte, 0, 4096)
		tmp := make([]byte, 32*1024)
		total := 0
		for {
			n, rerr := f.Read(tmp)
			if n > 0 {
				total += n
				if total > max {
					writeError(w, 400, "文件过大，请下载后编辑（上限 2MB）")
					return
				}
				buf = append(buf, tmp[:n]...)
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				writeError(w, 400, rerr.Error())
				return
			}
		}
		writeJSON(w, 200, map[string]interface{}{"path": file, "content": string(buf)})
	})

	r.Post("/api/sftp/{connId}/write", func(w http.ResponseWriter, req *http.Request) {
		connID, _ := strconv.Atoi(strParam(req, "connId"))
		var b struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		_ = readJSON(req, &b)
		file := normalizePath(b.Path)
		client, err := sshx.GetSftp(connID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		f, err := client.Create(file)
		if err != nil {
			if isPermError(err) {
				if msg, ok := sudoWriteStream(client, connID, file, strings.NewReader(b.Content)); ok {
					writeJSON(w, 200, map[string]bool{"ok": true})
					return
				} else {
					writeError(w, 400, "无写权限，且"+msg)
					return
				}
			}
			writeError(w, 400, err.Error())
			return
		}
		defer f.Close()
		if _, err := f.Write([]byte(b.Content)); err != nil {
			if isPermError(err) {
				if msg, ok := sudoWriteStream(client, connID, file, strings.NewReader(b.Content)); ok {
					writeJSON(w, 200, map[string]bool{"ok": true})
					return
				} else {
					writeError(w, 400, "无写权限，且"+msg)
					return
				}
			}
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})

	r.Post("/api/sftp/{connId}/upload", func(w http.ResponseWriter, req *http.Request) {
		connID, _ := strconv.Atoi(strParam(req, "connId"))
		dir := normalizePath(req.URL.Query().Get("path"))
		client, err := sshx.GetSftp(connID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		mr, err := req.MultipartReader()
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		uploaded := []string{}
		for {
			part, perr := mr.NextPart()
			if perr == io.EOF {
				break
			}
			if perr != nil {
				writeError(w, 400, perr.Error())
				return
			}
			if part.FileName() == "" {
				_ = part.Close()
				continue
			}
			target := path.Join(dir, part.FileName())
			f, ferr := client.Create(target)
			if ferr != nil {
				if isPermError(ferr) {
					var buf bytes.Buffer
					if _, rerr := io.Copy(&buf, part); rerr == nil {
						_ = part.Close()
						if msg, ok := sudoWriteStream(client, connID, target, &buf); ok {
							uploaded = append(uploaded, target)
							continue
						} else {
							writeError(w, 400, "无写权限，且"+msg)
							return
						}
					}
				}
				_ = part.Close()
				writeError(w, 400, ferr.Error())
				return
			}
			if _, cerr := io.Copy(f, part); cerr != nil {
				_ = f.Close()
				_ = part.Close()
				writeError(w, 400, cerr.Error())
				return
			}
			_ = f.Close()
			_ = part.Close()
			uploaded = append(uploaded, target)
		}
		writeJSON(w, 200, map[string]interface{}{"ok": true, "uploaded": uploaded})
	})

	r.Post("/api/sftp/{connId}/mkdir", func(w http.ResponseWriter, req *http.Request) {
		connID, _ := strconv.Atoi(strParam(req, "connId"))
		var b struct {
			Path string `json:"path"`
		}
		_ = readJSON(req, &b)
		target := normalizePath(b.Path)
		client, err := sshx.GetSftp(connID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		if err := client.Mkdir(target); err != nil {
			if isPermError(err) {
				conn, cerr := sshx.GetConnectionByID(connID)
				if cerr != nil {
					writeError(w, 400, "无权限创建目录，且获取连接信息失败: "+cerr.Error())
					return
				}
				tmpl := "{{SUDO}} mkdir -p " + shellQuote(target) + " && {{SUDO}} chown " + conn.Username + " " + shellQuote(target)
				if msg, ok := sudoShell(connID, tmpl); ok {
					writeJSON(w, 200, map[string]bool{"ok": true})
					return
				} else {
					writeError(w, 400, "无权限创建目录，且"+msg)
					return
				}
			}
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})

	r.Post("/api/sftp/{connId}/rename", func(w http.ResponseWriter, req *http.Request) {
		connID, _ := strconv.Atoi(strParam(req, "connId"))
		var b struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		_ = readJSON(req, &b)
		from := normalizePath(b.From)
		to := normalizePath(b.To)
		client, err := sshx.GetSftp(connID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		if err := client.Rename(from, to); err != nil {
			if isPermError(err) {
				if msg, ok := sudoShell(connID, "{{SUDO}} mv "+shellQuote(from)+" "+shellQuote(to)); ok {
					writeJSON(w, 200, map[string]bool{"ok": true})
					return
				} else {
					writeError(w, 400, "无权限重命名，且"+msg)
					return
				}
			}
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})

	r.Post("/api/sftp/{connId}/delete", func(w http.ResponseWriter, req *http.Request) {
		connID, _ := strconv.Atoi(strParam(req, "connId"))
		var b struct {
			Path  string `json:"path"`
			IsDir bool   `json:"isDir"`
		}
		_ = readJSON(req, &b)
		target := normalizePath(b.Path)
		// 目录：sftp 的 RemoveDirectory 只能删空目录，改用 rm -rf 递归删除（路径已 shellQuote 转义，防注入）
		if b.IsDir {
			client, err := sshx.GetClient(connID)
			if err != nil {
				writeError(w, 400, err.Error())
				return
			}
			if out, rerr := sshx.RunCommand(client, "rm -rf "+shellQuote(target)); rerr != nil {
				if isPermError(rerr) {
					if msg, ok := sudoShell(connID, "{{SUDO}} rm -rf "+shellQuote(target)); ok {
						writeJSON(w, 200, map[string]bool{"ok": true})
						return
					} else {
						writeError(w, 400, "删除失败（sudo 提权也失败）: "+msg)
						return
					}
				}
				writeError(w, 400, "删除失败: "+rerr.Error()+" "+out)
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		client, err := sshx.GetSftp(connID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		if derr := client.Remove(target); derr != nil {
			if isPermError(derr) {
				if msg, ok := sudoShell(connID, "{{SUDO}} rm -f "+shellQuote(target)); ok {
					writeJSON(w, 200, map[string]bool{"ok": true})
					return
				} else {
					writeError(w, 400, "无权限删除，且"+msg)
					return
				}
			}
			writeError(w, 400, derr.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})

	// 解压：在远端执行 unzip / tar，把压缩包展开到 dest 目录；可选 chmod -R 权限。
	r.Post("/api/sftp/{connId}/unzip", func(w http.ResponseWriter, req *http.Request) {
		connID, _ := strconv.Atoi(strParam(req, "connId"))
		var b struct {
			Path string `json:"path"`
			Dest string `json:"dest"`
			Mode string `json:"mode"`
		}
		_ = readJSON(req, &b)
		src := normalizePath(b.Path)
		dest := normalizePath(b.Dest)
		if dest == "" || dest == "/" {
			writeError(w, 400, "目标目录不合法")
			return
		}
		lower := strings.ToLower(src)
		ext := strings.ToLower(path.Ext(src))
		var cmd string
		switch {
		case ext == ".zip":
			cmd = "unzip -o " + shellQuote(src) + " -d " + shellQuote(dest)
		case strings.HasSuffix(lower, ".tar.gz"), ext == ".tgz":
			cmd = "mkdir -p " + shellQuote(dest) + " && tar -xzf " + shellQuote(src) + " -C " + shellQuote(dest)
		case ext == ".tar":
			cmd = "mkdir -p " + shellQuote(dest) + " && tar -xf " + shellQuote(src) + " -C " + shellQuote(dest)
		case strings.HasSuffix(lower, ".tar.bz2"), ext == ".tbz2":
			cmd = "mkdir -p " + shellQuote(dest) + " && tar -xjf " + shellQuote(src) + " -C " + shellQuote(dest)
		case strings.HasSuffix(lower, ".tar.xz"), ext == ".txz":
			cmd = "mkdir -p " + shellQuote(dest) + " && tar -xJf " + shellQuote(src) + " -C " + shellQuote(dest)
		default:
			writeError(w, 400, "不支持的压缩格式: "+ext)
			return
		}
		client, err := sshx.GetClient(connID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		if out, rerr := sshx.RunCommand(client, cmd); rerr != nil {
			writeError(w, 400, "解压失败: "+rerr.Error()+" "+out)
			return
		}
		// 可选：对解压出的内容递归设置权限（仅允许 3~4 位八进制）
		if b.Mode != "" {
			if ok, _ := regexp.MatchString(`^[0-7]{3,4}$`, b.Mode); ok {
				if out, cerr := sshx.RunCommand(client, "chmod -R "+b.Mode+" "+shellQuote(dest)); cerr != nil {
					writeError(w, 400, "解压成功但设置权限失败: "+cerr.Error()+" "+out)
					return
				}
			}
		}
		writeJSON(w, 200, map[string]interface{}{"ok": true, "dest": dest})
	})
}

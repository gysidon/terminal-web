package routes

import (
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/pkg/sftp"

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
			writeError(w, 400, err.Error())
			return
		}
		defer f.Close()
		if _, err := f.Write([]byte(b.Content)); err != nil {
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
		client, err := sshx.GetSftp(connID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		var derr error
		if b.IsDir {
			derr = client.RemoveDirectory(target)
		} else {
			derr = client.Remove(target)
		}
		if derr != nil {
			writeError(w, 400, derr.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
}

package routes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/ssh"

	"terminal-web/server-go/internal/auth"
	"terminal-web/server-go/internal/sshx"
)

func writeWS(c *websocket.Conn, mu *sync.Mutex, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	_ = c.Write(context.Background(), websocket.MessageText, data)
}

// RegisterWS 注册 WebSocket 终端接口。
func RegisterWS(r chi.Router) {
	r.Get("/ws/terminal/{connId}", func(w http.ResponseWriter, req *http.Request) {
		handleWS(w, req)
	})
}

func handleWS(w http.ResponseWriter, req *http.Request) {
	connID, _ := strconv.Atoi(chi.URLParam(req, "connId"))
	token := req.URL.Query().Get("token")
	claims := auth.VerifySession(token)

	c, err := websocket.Accept(w, req, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	var mu sync.Mutex
	defer c.Close(websocket.StatusNormalClosure, "")

	if claims == nil {
		writeWS(c, &mu, map[string]interface{}{"type": "error", "message": "未登录或登录已过期"})
		return
	}

	connRow, err := sshx.GetConnectionByID(connID)
	if err != nil || connRow == nil {
		writeWS(c, &mu, map[string]interface{}{"type": "error", "message": "连接配置不存在"})
		return
	}

	writeWS(c, &mu, map[string]interface{}{
		"type":    "status",
		"message": fmt.Sprintf("正在连接 %s@%s:%d ...", connRow.Username, connRow.Host, connRow.Port),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, chain, err := sshx.CreateSshClient(connRow)
	if err != nil {
		writeWS(c, &mu, map[string]interface{}{"type": "error", "message": "连接失败: " + err.Error()})
		return
	}
	defer sshx.CloseChain(chain)

	session, err := client.NewSession()
	if err != nil {
		writeWS(c, &mu, map[string]interface{}{"type": "error", "message": "打开终端失败: " + err.Error()})
		return
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		writeWS(c, &mu, map[string]interface{}{"type": "error", "message": "打开终端失败: " + err.Error()})
		return
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		writeWS(c, &mu, map[string]interface{}{"type": "error", "message": "打开终端失败: " + err.Error()})
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		writeWS(c, &mu, map[string]interface{}{"type": "error", "message": "打开终端失败: " + err.Error()})
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		writeWS(c, &mu, map[string]interface{}{"type": "error", "message": "打开终端失败: " + err.Error()})
		return
	}

	if err := session.Shell(); err != nil {
		writeWS(c, &mu, map[string]interface{}{"type": "error", "message": "打开终端失败: " + err.Error()})
		return
	}
	writeWS(c, &mu, map[string]interface{}{"type": "connected"})

	pump := func(rc io.Reader) {
		buf := make([]byte, 32*1024)
		for {
			n, rerr := rc.Read(buf)
			if n > 0 {
				b64 := base64.StdEncoding.EncodeToString(buf[:n])
				writeWS(c, &mu, map[string]interface{}{"type": "data", "data": b64})
			}
			if rerr != nil {
				break
			}
		}
	}
	go pump(stdout)
	go pump(stderr)

	// 终端关闭时通知前端并关闭 socket
	go func() {
		_ = session.Wait()
		writeWS(c, &mu, map[string]interface{}{"type": "exit"})
		cancel()
		_ = c.Close(websocket.StatusNormalClosure, "")
	}()

	// 读循环：input / resize / ping
	for {
		mt, data, rerr := c.Read(ctx)
		if rerr != nil {
			break
		}
		if mt != websocket.MessageText {
			continue
		}
		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg["type"] {
		case "input":
			if s, ok := msg["data"].(string); ok {
				if jti := auth.GetJTI(claims); jti != "" {
					auth.TouchSession(jti)
				}
				if b, derr := base64.StdEncoding.DecodeString(s); derr == nil {
					_, _ = stdin.Write(b)
				}
			}
		case "resize":
			rows, _ := msg["rows"].(float64)
			cols, _ := msg["cols"].(float64)
			_ = session.WindowChange(int(rows), int(cols))
		case "ping":
			writeWS(c, &mu, map[string]interface{}{"type": "pong"})
		}
	}
}

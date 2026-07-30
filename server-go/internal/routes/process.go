package routes

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"terminal-web/server-go/internal/sshx"
)

// parsePs 解析 `ps` 输出为结构化进程列表。
func parsePs(out string) []map[string]interface{} {
	lines := []string{}
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) < 2 {
		return []map[string]interface{}{}
	}
	header := strings.Fields(lines[0])
	pidIdx := indexOf(header, "PID")
	userIdx := indexOf(header, "USER")
	cpuIdx := indexOf(header, "%CPU")
	memIdx := indexOf(header, "%MEM")
	etimeIdx := indexOf(header, "ELAPSED")
	commIdx := indexOf(header, "COMMAND")

	procs := []map[string]interface{}{}
	for i := 1; i < len(lines); i++ {
		cols := strings.Fields(lines[i])
		comm := ""
		if commIdx >= 0 && commIdx < len(cols) {
			comm = strings.Join(cols[commIdx:], " ")
		} else if len(cols) > 0 {
			comm = cols[len(cols)-1]
		}
		pid := 0.0
		if pidIdx >= 0 && pidIdx < len(cols) {
			pid, _ = strconv.ParseFloat(cols[pidIdx], 64)
		} else if len(cols) > 0 {
			pid, _ = strconv.ParseFloat(cols[0], 64)
		}
		user := ""
		if userIdx >= 0 && userIdx < len(cols) {
			user = cols[userIdx]
		} else if len(cols) > 1 {
			user = cols[1]
		}
		cpu := 0.0
		if cpuIdx >= 0 && cpuIdx < len(cols) {
			cpu, _ = strconv.ParseFloat(cols[cpuIdx], 64)
		}
		mem := 0.0
		if memIdx >= 0 && memIdx < len(cols) {
			mem, _ = strconv.ParseFloat(cols[memIdx], 64)
		}
		elapsed := ""
		if etimeIdx >= 0 && etimeIdx < len(cols) {
			elapsed = cols[etimeIdx]
		}
		procs = append(procs, map[string]interface{}{
			"pid":      int(pid),
			"user":     user,
			"cpu":      cpu,
			"mem":      mem,
			"elapsed":  elapsed,
			"command":  comm,
		})
	}
	return procs
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

// RegisterProcess 注册进程管理接口（受保护）。
func RegisterProcess(r chi.Router) {
	r.Get("/api/process/{connId}", func(w http.ResponseWriter, req *http.Request) {
		connID, _ := strconv.Atoi(strParam(req, "connId"))
		client, err := sshx.GetClient(connID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		out, err := sshx.RunCommand(client, "ps -eo pid,user,%cpu,%mem,etime,args --sort=-%cpu | head -n 200")
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]interface{}{"processes": parsePs(out)})
	})

	r.Post("/api/process/{connId}/kill", func(w http.ResponseWriter, req *http.Request) {
		connID, _ := strconv.Atoi(strParam(req, "connId"))
		var b struct {
			Pid    float64 `json:"pid"`
			Signal string   `json:"signal"`
		}
		_ = readJSON(req, &b)
		if b.Pid == 0 {
			writeError(w, 400, "缺少 pid")
			return
		}
		signal := b.Signal
		if signal == "" {
			signal = "TERM"
		}
		client, err := sshx.GetClient(connID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		_, err = sshx.RunCommand(client, "kill -"+signal+" "+strconv.Itoa(int(b.Pid)))
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
}

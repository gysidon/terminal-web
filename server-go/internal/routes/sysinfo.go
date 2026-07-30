package routes

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/ssh"

	"terminal-web/server-go/internal/sshx"
)

// 以下命令与 Node 版 sysinfo.js 逐字保持一致。
const (
	cpuCmd = "R1=$(awk '/^cpu /{print $2+$3+$4+$5}' /proc/stat); I1=$(awk '/^cpu /{print $5}' /proc/stat); " +
		"sleep 0.6; " +
		"R2=$(awk '/^cpu /{print $2+$3+$4+$5}' /proc/stat); I2=$(awk '/^cpu /{print $5}' /proc/stat); " +
		"D=$((R2-R1)); DI=$((I2-I1)); if [ \"$D\" -gt 0 ]; then echo $((100*(D-DI)/D)); else echo 0; fi"
	memCmd  = "free -b | awk '/^Mem:/{print $2, $3}'"
	loadCmd = "awk '{print $1, $2, $3}' /proc/loadavg; nproc"
	diskCmd = "df -PT 2>/dev/null | awk 'NR>1{ty=$2; if(ty==\"tmpfs\"||ty==\"devtmpfs\"||ty==\"overlay\"||ty==\"proc\"||ty==\"sysfs\"||ty==\"cgroup\"||ty==\"udev\"||ty==\"squashfs\")next; sz=$3; us=$4; pct=(sz>0)?int(us*100/sz):0; print $7\"|\"sz\"|\"us\"|\"pct}'"
)

// execCmd 执行命令并带超时；失败/超时返回 ""（对应 Node 的 null 字段）。
func execCmd(client *ssh.Client, cmd string, timeoutMs int) string {
	type result struct {
		out string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := sshx.RunCommand(client, cmd)
		ch <- result{out, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return ""
		}
		return r.out
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		return ""
	}
}

func numParse(s string) *float64 {
	s = strings.ReplaceAll(s, ",", "")
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return nil
	}
	return &n
}

// RegisterSysinfo 注册系统信息接口（受保护）。
func RegisterSysinfo(r chi.Router) {
	r.Get("/api/sysinfo/{connId}", func(w http.ResponseWriter, req *http.Request) {
		connID, _ := strconv.Atoi(strParam(req, "connId"))
		client, err := sshx.GetClient(connID)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}

		cpuRaw := execCmd(client, cpuCmd, 8000)
		memRaw := execCmd(client, memCmd, 8000)
		loadRaw := execCmd(client, loadCmd, 8000)
		diskRaw := execCmd(client, diskCmd, 8000)

		var cpu interface{}
		if cpuRaw != "" {
			if v, e := strconv.Atoi(strings.TrimSpace(cpuRaw)); e == nil {
				vv := v
				if vv < 0 {
					vv = 0
				}
				if vv > 100 {
					vv = 100
				}
				cpu = vv
			}
		}

		var mem, memTotal, memUsed interface{}
		if memRaw != "" {
			p := strings.Fields(memRaw)
			if len(p) >= 2 {
				t := numParse(p[0])
				u := numParse(p[1])
				if t != nil && *t > 0 {
					mem = int(*u / *t * 100)
					memTotal = int(*t)
					memUsed = int(*u)
				}
			}
		}

		var load1, load5, load15, cores interface{}
		if loadRaw != "" {
			lines := []string{}
			for _, l := range strings.Split(loadRaw, "\n") {
				if strings.TrimSpace(l) != "" {
					lines = append(lines, l)
				}
			}
			if len(lines) >= 1 {
				l := strings.Fields(lines[0])
				if len(l) >= 3 {
					load1 = numParse(l[0])
					load5 = numParse(l[1])
					load15 = numParse(l[2])
				}
			}
			last := lines[len(lines)-1]
			if n, e := strconv.Atoi(strings.TrimSpace(last)); e == nil {
				cores = n
			}
		}

		disks := []map[string]interface{}{}
		if diskRaw != "" {
			for _, line := range strings.Split(strings.TrimSpace(diskRaw), "\n") {
				if line == "" {
					continue
				}
				parts := strings.Split(line, "|")
				if len(parts) < 1 || parts[0] == "" {
					continue
				}
				mount := parts[0]
				size := numParse(orEmpty(parts, 1))
				used := numParse(orEmpty(parts, 2))
				var pct interface{}
				if len(parts) >= 4 {
					if n, e := strconv.Atoi(parts[3]); e == nil {
						pct = n
					}
				}
				disks = append(disks, map[string]interface{}{
					"mount":   mount,
					"total":   size,
					"used":    used,
					"percent": pct,
				})
			}
		}

		writeJSON(w, 200, map[string]interface{}{
			"cpu":       cpu,
			"mem":       mem,
			"memTotal":  memTotal,
			"memUsed":   memUsed,
			"load1":     load1,
			"load5":     load5,
			"load15":    load15,
			"cores":     cores,
			"disks":     disks,
		})
	})
}

func orEmpty(parts []string, i int) string {
	if i < len(parts) {
		return parts[i]
	}
	return ""
}

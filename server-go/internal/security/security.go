package security

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"terminal-web/server-go/internal/db"
	"terminal-web/server-go/internal/settings"
)

// ---------- 客户端 IP ----------

func GetClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ---------- IP 白名单 ----------

func ipToLong(ip string) (uint32, bool) {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return 0, false
	}
	var l uint32
	for i := 0; i < 4; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 || n > 255 {
			return 0, false
		}
		l = (l << 8) + uint32(n)
	}
	return l, true
}

func inCidr(ip, cidr string) bool {
	idx := strings.Index(cidr, "/")
	if idx < 0 {
		return false
	}
	netPart := cidr[:idx]
	bitsStr := cidr[idx+1:]
	bits, err := strconv.Atoi(bitsStr)
	if err != nil || bits < 0 || bits > 32 {
		return false
	}
	ipLong, ok1 := ipToLong(ip)
	netLong, ok2 := ipToLong(netPart)
	if !ok1 || !ok2 {
		return false
	}
	var mask uint32
	if bits == 0 {
		mask = 0
	} else {
		mask = (0xffffffff << (32 - bits)) & 0xffffffff
	}
	return (ipLong & mask) == (netLong & mask)
}

// IpInWhitelist 支持精确 IP、CIDR、前缀（以 "." 结尾）。
func IpInWhitelist(ip, listStr string) bool {
	if strings.TrimSpace(listStr) == "" {
		return true
	}
	for _, item := range strings.Split(listStr, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if item == ip {
			return true
		}
		if strings.HasSuffix(item, ".") {
			if strings.HasPrefix(ip, item) {
				return true
			}
		} else if strings.Contains(item, "/") {
			if inCidr(ip, item) {
				return true
			}
		}
	}
	return false
}

func IsIPAllowed(r *http.Request) bool {
	wl := settings.GetString("ip_whitelist")
	if strings.TrimSpace(wl) == "" {
		return true
	}
	return IpInWhitelist(GetClientIP(r), wl)
}

// ---------- 登录失败锁定（内存态） ----------

type lockEntry struct {
	count int
	until int64
}

var (
	failsMu sync.Mutex
	fails   = map[string]*lockEntry{}
)

// GetLockUntil 返回锁定解除的时间戳（毫秒）；未锁定返回 0。
func GetLockUntil(username string) int64 {
	failsMu.Lock()
	defer failsMu.Unlock()
	e, ok := fails[username]
	if !ok {
		return 0
	}
	now := time.Now().UnixMilli()
	if e.until > now {
		return e.until
	}
	if e.until > 0 {
		delete(fails, username)
	}
	return 0
}

func RecordFail(username string) {
	failsMu.Lock()
	defer failsMu.Unlock()
	max := settings.GetInt("login_fail_max")
	if max <= 0 {
		max = 5
	}
	e := fails[username]
	if e == nil {
		e = &lockEntry{}
		fails[username] = e
	}
	e.count++
	if e.count >= max {
		lockMin := settings.GetInt("login_lock_minutes")
		if lockMin <= 0 {
			lockMin = 15
		}
		e.until = time.Now().UnixMilli() + int64(lockMin)*60*1000
	}
}

func ClearFail(username string) {
	failsMu.Lock()
	defer failsMu.Unlock()
	delete(fails, username)
}

// ---------- 审计日志 ----------

func LogAudit(action, username, ip string, success bool, detail string) {
	_, err := db.DB.Exec(
		`INSERT INTO audit_logs (action, username, ip, success, detail, created_at)
		 VALUES (?, ?, ?, ?, ?, datetime('now'))`,
		action, username, ip, boolToInt(success), detail)
	if err != nil {
		// 审计失败不影响主流程
		return
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func RecentAudit(limit int) ([]map[string]interface{}, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := db.DB.Query("SELECT * FROM audit_logs ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]interface{}{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := map[string]interface{}{}
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				m[c] = string(b)
			} else {
				m[c] = vals[i]
			}
		}
		out = append(out, m)
	}
	return out, nil
}

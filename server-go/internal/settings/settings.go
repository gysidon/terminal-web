package settings

import (
	"database/sql"
	"strconv"

	"terminal-web/server-go/internal/db"
)

const defaultFont = `"JetBrains Mono", Menlo, Consolas, "Courier New", monospace`

type schemaEntry struct {
	typ     string
	def     string
}

var schema = map[string]schemaEntry{
	"captcha_enabled":    {typ: "bool", def: "true"},
	"theme":              {typ: "str", def: "dark"},
	"font_size":          {typ: "int", def: "13"},
	"font_family":        {typ: "str", def: defaultFont},
	"login_fail_max":     {typ: "int", def: "5"},
	"login_lock_minutes": {typ: "int", def: "15"},
	"ip_whitelist":       {typ: "str", def: ""},
	"session_timeout":    {typ: "int", def: "0"},
	"sso_enabled":        {typ: "bool", def: "false"},
}

func Seed() error {
	stmt, err := db.DB.Prepare("INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for k, v := range schema {
		if _, err := stmt.Exec(k, v.def); err != nil {
			return err
		}
	}
	return nil
}

func coerce(typ, raw string) interface{} {
	switch typ {
	case "bool":
		return raw == "1" || raw == "true" || raw == "True"
	case "int":
		n, _ := strconv.Atoi(raw)
		return n
	default:
		return raw
	}
}

// GetAll 返回全部设置（snake_case 键，按 schema 类型转换）。
func GetAll() map[string]interface{} {
	rows, err := db.DB.Query("SELECT key, value FROM settings")
	if err != nil {
		return map[string]interface{}{}
	}
	defer rows.Close()
	raw := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			continue
		}
		raw[k] = v
	}
	out := map[string]interface{}{}
	for k, v := range schema {
		val, ok := raw[k]
		if !ok {
			val = v.def
		}
		out[k] = coerce(v.typ, val)
	}
	return out
}

// Get 读取单个设置并类型转换。
func Get(key string) interface{} {
	v, ok := schema[key]
	if !ok {
		return nil
	}
	var raw sql.NullString
	_ = db.DB.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&raw)
	val := v.def
	if raw.Valid {
		val = raw.String
	}
	return coerce(v.typ, val)
}

// GetString / GetInt / GetBool 便捷访问。
func GetString(key string) string {
	if v, ok := Get(key).(string); ok {
		return v
	}
	return ""
}
func GetInt(key string) int {
	if v, ok := Get(key).(int); ok {
		return v
	}
	return 0
}
func GetBool(key string) bool {
	if v, ok := Get(key).(bool); ok {
		return v
	}
	return false
}

// Put 仅更新 schema 内存在的键，按类型强制转换。返回实际更新的键值。
func Put(input map[string]interface{}) (map[string]interface{}, error) {
	stmt, err := db.DB.Prepare("INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	updated := map[string]interface{}{}
	for k, v := range schema {
		iv, ok := input[k]
		if !ok {
			continue
		}
		var val string
		switch v.typ {
		case "bool":
			b, _ := iv.(bool)
			val = "0"
			if b {
				val = "1"
			}
		case "int":
			switch t := iv.(type) {
			case float64:
				val = strconv.Itoa(int(t))
			case int:
				val = strconv.Itoa(t)
			case int64:
				val = strconv.Itoa(int(t))
			default:
				val = strconv.Itoa(0)
			}
		default:
			val = toString(iv)
		}
		if _, err := stmt.Exec(k, val); err != nil {
			return nil, err
		}
		updated[k] = coerce(v.typ, val)
	}
	return updated, nil
}

func toString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return strconv.FormatInt(0, 10) // 不应触发
	}
}

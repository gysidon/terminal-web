package routes

import (
	"database/sql"
	"strconv"
	"time"

	"terminal-web/server-go/internal/db"
	"terminal-web/server-go/internal/sshx"
)

func nowMs() int64 { return time.Now().UnixMilli() }

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func existingConnNames() []string {
	rows, err := db.DB.Query("SELECT name FROM connections")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func queryFolders() ([]map[string]interface{}, error) {
	rows, err := db.DB.Query("SELECT id, name, parent_id, sort_order, created_at FROM folders ORDER BY sort_order, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []string{"id", "name", "parent_id", "sort_order", "created_at"}
	return scanRows(rows, cols)
}

func getFolder(id int) (map[string]interface{}, error) {
	row := db.DB.QueryRow("SELECT id, name, parent_id, sort_order, created_at FROM folders WHERE id = ?", id)
	cols := []string{"id", "name", "parent_id", "sort_order", "created_at"}
	vals := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := row.Scan(ptrs...); err != nil {
		return nil, err
	}
	m := map[string]interface{}{}
	for i, c := range cols {
		m[c] = vals[i]
	}
	return m, nil
}

// scanConn 将一行扫描为 *sshx.ConnRow。
func scanConn(rows *sql.Rows) (*sshx.ConnRow, error) {
	var c sshx.ConnRow
	var pw, pk, pp sql.NullString
	err := rows.Scan(&c.ID, &c.Name, &c.Host, &c.Port, &c.Username, &c.AuthType,
		&pw, &pk, &pp, &c.FolderID, &c.JumpID,
		&c.Remark, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	c.PasswordEnc, c.PrivateKeyEnc, c.PassphraseEnc = pw.String, pk.String, pp.String
	return &c, err
}

func scanRows(rows *sql.Rows, cols []string) ([]map[string]interface{}, error) {
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
			m[c] = normalizeValue(vals[i])
		}
		out = append(out, m)
	}
	return out, nil
}

// normalizeValue 将数据库驱动返回的 []byte 转为 string，避免 JSON 中变成 base64。
func normalizeValue(v interface{}) interface{} {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// ---------- 类型转换辅助 ----------

func toInt(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(t)
		return n
	case nil:
		return 0
	default:
		return 0
	}
}

func toIntDefault(v interface{}, def int) int {
	if v == nil {
		return def
	}
	if f, ok := v.(float64); ok {
		if f == 0 {
			return def
		}
		return int(f)
	}
	n := toInt(v)
	if n == 0 {
		return def
	}
	return n
}

func toIntOrNil(v interface{}) *int64 {
	if v == nil {
		return nil
	}
	if f, ok := v.(float64); ok {
		if f == 0 {
			return nil
		}
		i := int64(f)
		return &i
	}
	if s, ok := v.(string); ok {
		if s == "" {
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil
		}
		i := int64(n)
		return &i
	}
	i := int64(toInt(v))
	return &i
}

func isInt(v interface{}) bool {
	switch t := v.(type) {
	case float64:
		return true
	case int:
		return true
	case int64:
		return true
	case string:
		_, err := strconv.Atoi(t)
		return err == nil
	default:
		return false
	}
}

func sqlNullInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

func nullString(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

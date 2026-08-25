package sshx

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"terminal-web/server-go/internal/crypto"
	"terminal-web/server-go/internal/db"
)

const maxJumpDepth = 5

// ConnRow 对应 connections 表一行。
type ConnRow struct {
	ID             int
	Name           string
	Host           string
	Port           int
	Username       string
	AuthType       string
	PasswordEnc    string
	PrivateKeyEnc  string
	PassphraseEnc  string
	FolderID       sql.NullInt64
	JumpID         sql.NullInt64
	Remark         sql.NullString
	SortOrder      int
	CreatedAt      string
	UpdatedAt      string
}

// authOverrides 用于测试连接场景：优先使用表单明文凭据。
type authOverrides struct {
	password   *string
	privateKey *string
	passphrase *string
}

func GetConnectionByID(id int) (*ConnRow, error) {
	row := db.DB.QueryRow(`SELECT id, name, host, port, username, auth_type, password_enc, private_key_enc,
		passphrase_enc, folder_id, jump_id, remark, sort_order, created_at, updated_at
		FROM connections WHERE id = ?`, id)
	var c ConnRow
	var pw, pk, pp sql.NullString
	err := row.Scan(&c.ID, &c.Name, &c.Host, &c.Port, &c.Username, &c.AuthType,
		&pw, &pk, &pp, &c.FolderID, &c.JumpID,
		&c.Remark, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.PasswordEnc, c.PrivateKeyEnc, c.PassphraseEnc = pw.String, pk.String, pp.String
	return &c, nil
}

func resolveChain(conn *ConnRow) ([]*ConnRow, error) {
	chain := []*ConnRow{conn}
	seen := map[int]bool{conn.ID: true}
	cur := conn
	for cur.JumpID.Valid {
		if len(chain) >= maxJumpDepth {
			return nil, fmt.Errorf("跳板层级过深（最多5级）")
		}
		jid := int(cur.JumpID.Int64)
		next, err := GetConnectionByID(jid)
		if err != nil {
			return nil, fmt.Errorf("跳板机配置不存在")
		}
		if seen[next.ID] {
			return nil, fmt.Errorf("跳板配置存在循环引用")
		}
		seen[next.ID] = true
		chain = append([]*ConnRow{next}, chain...)
		cur = next
	}
	return chain, nil
}

func resolveChainWithOverrides(conn *ConnRow, ov *authOverrides) ([]*chainHop, error) {
	chain := []*chainHop{{conn, ov}}
	seen := map[int]bool{conn.ID: true}
	cur := conn
	for cur.JumpID.Valid {
		if len(chain) >= maxJumpDepth {
			return nil, fmt.Errorf("跳板层级过深（最多5级）")
		}
		next, err := GetConnectionByID(int(cur.JumpID.Int64))
		if err != nil {
			return nil, fmt.Errorf("跳板机配置不存在")
		}
		if seen[next.ID] {
			return nil, fmt.Errorf("跳板配置存在循环引用")
		}
		seen[next.ID] = true
		chain = append([]*chainHop{{next, nil}}, chain...)
		cur = next
	}
	return chain, nil
}

func buildAuthConfig(conn *ConnRow, ov *authOverrides) (*ssh.ClientConfig, error) {
	cfg := &ssh.ClientConfig{
		User:            conn.Username,
		Timeout:         15 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	var password, privateKey, passphrase string
	if ov != nil {
		if ov.password != nil {
			password = *ov.password
		}
		if ov.privateKey != nil {
			privateKey = *ov.privateKey
		}
		if ov.passphrase != nil {
			passphrase = *ov.passphrase
		}
	}
	if ov == nil || ov.password == nil {
		if conn.PasswordEnc != "" {
			if p, err := crypto.Decrypt(conn.PasswordEnc, crypto.DefaultKey()); err == nil {
				password = p
			}
		}
	}
	if ov == nil || ov.privateKey == nil {
		if conn.PrivateKeyEnc != "" {
			if k, err := crypto.Decrypt(conn.PrivateKeyEnc, crypto.DefaultKey()); err == nil {
				privateKey = k
			}
		}
	}
	if ov == nil || ov.passphrase == nil {
		if conn.PassphraseEnc != "" {
			if ph, err := crypto.Decrypt(conn.PassphraseEnc, crypto.DefaultKey()); err == nil {
				passphrase = ph
			}
		}
	}

	auths := []ssh.AuthMethod{}
	if conn.AuthType == "key" {
		signer, err := ssh.ParsePrivateKey([]byte(privateKey))
		if err != nil && passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(privateKey), []byte(passphrase))
		}
		if err != nil {
			return nil, fmt.Errorf("私钥解析失败: %w", err)
		}
		auths = append(auths, ssh.PublicKeys(signer))
	} else {
		pw := password
		auths = append(auths, ssh.Password(pw))
		auths = append(auths, ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range questions {
				answers[i] = pw
			}
			return answers, nil
		}))
	}
	cfg.Auth = auths
	return cfg, nil
}

func startKeepalive(client *ssh.Client) {
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		fail := 0
		for range ticker.C {
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				fail++
				if fail >= 3 {
					_ = client.Close()
					return
				}
			} else {
				fail = 0
			}
		}
	}()
}

// closeChain 级联关闭整条 SSH 链（从目标到跳板）。
func closeChain(chain []*ssh.Client) {
	for i := len(chain) - 1; i >= 0; i-- {
		_ = chain[i].Close()
	}
}

// CloseChain 级联关闭整条 SSH 链（导出，供调用方在退出时清理）。
func CloseChain(chain []*ssh.Client) { closeChain(chain) }

// dialChain 建立级联连接。返回目标 client 与完整链（含跳板）。
func dialChain(chain []*chainHop) (target *ssh.Client, fullChain []*ssh.Client, err error) {
	clients := make([]*ssh.Client, 0, len(chain))
	defer func() {
		if err != nil {
			closeChain(clients)
		}
	}()

	var prev *ssh.Client
	for _, hop := range chain {
		cfg, e := buildAuthConfig(hop.ConnRow, hop.ov)
		if e != nil {
			err = e
			return
		}
		addr := net.JoinHostPort(hop.Host, fmt.Sprintf("%d", hop.Port))
		var client *ssh.Client
		if prev == nil {
			client, e = ssh.Dial("tcp", addr, cfg)
		} else {
			var sock net.Conn
			sock, e = prev.Dial("tcp", addr)
			if e != nil {
				err = fmt.Errorf("通过跳板转发到 %s 失败: %w", hop.Host, e)
				return
			}
			var ncc ssh.Conn
			var chans <-chan ssh.NewChannel
			var reqs <-chan *ssh.Request
			ncc, chans, reqs, e = ssh.NewClientConn(sock, addr, cfg)
			if e != nil {
				_ = sock.Close()
				err = e
				return
			}
			client = ssh.NewClient(ncc, chans, reqs)
		}
		if e != nil {
			err = e
			return
		}
		startKeepalive(client)
		clients = append(clients, client)
		prev = client
	}
	target = clients[len(clients)-1]
	fullChain = clients
	return
}

// CreateSshClient 级联建立 SSH 连接（支持跳板），返回目标 client 与完整链。
func CreateSshClient(conn *ConnRow) (*ssh.Client, []*ssh.Client, error) {
	chain, err := resolveChain(conn)
	if err != nil {
		return nil, nil, err
	}
	hops := make([]*chainHop, len(chain))
	for i, c := range chain {
		hops[i] = &chainHop{c, nil}
	}
	return dialChain(hops)
}

// TestConnection 用落库凭据实连一次（关闭整条链）。
func TestConnection(conn *ConnRow) error {
	client, chain, err := CreateSshClient(conn)
	if err != nil {
		return err
	}
	_ = client
	closeChain(chain)
	return nil
}

// TestConnectionConfig 用表单明文配置测试连接（可能尚未保存）。
func TestConnectionConfig(cfg map[string]interface{}) error {
	id := 0
	if v, ok := cfg["id"]; ok {
		switch t := v.(type) {
		case float64:
			id = int(t)
		case int:
			id = t
		case string:
			fmt.Sscanf(t, "%d", &id)
		}
	}
	var stored *ConnRow
	if id != 0 {
		if s, err := GetConnectionByID(id); err == nil {
			stored = s
		}
	}
	authType := "password"
	var portInt int = 22
	host, _ := cfg["host"].(string)
	username, _ := cfg["username"].(string)
	var jumpID sql.NullInt64
	if stored != nil {
		authType = stored.AuthType
		if stored.Port != 0 {
			portInt = stored.Port
		}
		if stored.JumpID.Valid {
			jumpID = stored.JumpID
		}
	}
	if v, ok := cfg["auth_type"].(string); ok && v != "" {
		authType = v
	}
	if v, ok := cfg["port"].(float64); ok && v != 0 {
		portInt = int(v)
	}
	if v, ok := cfg["port"].(string); ok && v != "" {
		fmt.Sscanf(v, "%d", &portInt)
	}
	if v, ok := cfg["jump_id"]; ok {
		switch t := v.(type) {
		case float64:
			if t != 0 {
				jumpID = sql.NullInt64{Int64: int64(t), Valid: true}
			}
		case int:
			if t != 0 {
				jumpID = sql.NullInt64{Int64: int64(t), Valid: true}
			}
		}
	}

	password := getStr(cfg, "password")
	privateKey := getStr(cfg, "private_key")
	passphrase := getStr(cfg, "passphrase")

	if stored != nil {
		if password == "" && stored.PasswordEnc != "" {
			password, _ = crypto.Decrypt(stored.PasswordEnc, crypto.DefaultKey())
		}
		if privateKey == "" && stored.PrivateKeyEnc != "" {
			privateKey, _ = crypto.Decrypt(stored.PrivateKeyEnc, crypto.DefaultKey())
		}
		if passphrase == "" && stored.PassphraseEnc != "" {
			passphrase, _ = crypto.Decrypt(stored.PassphraseEnc, crypto.DefaultKey())
		}
	}

	conn := &ConnRow{
		ID:     id,
		Host:   host,
		Port:   portInt,
		Username: username,
		AuthType: authType,
		JumpID:  jumpID,
	}
	p := password
	pk := privateKey
	pp := passphrase
	ov := &authOverrides{password: &p, privateKey: &pk, passphrase: &pp}

	chain, err := resolveChainWithOverrides(conn, ov)
	if err != nil {
		return err
	}
	client, full, err := dialChain(chain)
	if err != nil {
		return err
	}
	_ = client
	closeChain(full)
	return nil
}

// chainHop 同时携带覆盖凭据，供 resolveChainWithOverrides 使用。
type chainHop struct {
	*ConnRow
	ov *authOverrides
}

func jumpIDValid(c *ConnRow) bool { return c.JumpID.Valid }

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// OpenSftp 在给定 client 上打开一个 sftp 子会话。
func OpenSftp(client *ssh.Client) (*sftp.Client, error) {
	return sftp.NewClient(client)
}

// ExecTrue 执行无副作用命令并测量往返耗时（毫秒）。
func ExecTrue(client *ssh.Client) (int, error) {
	session, err := client.NewSession()
	if err != nil {
		return 0, err
	}
	defer session.Close()
	t0 := time.Now()
	if err := session.Run("true"); err != nil {
		return 0, err
	}
	return int(time.Since(t0).Milliseconds()), nil
}

// RunCommand 执行命令并返回合并 stdout/stderr 输出。
func RunCommand(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	out, err := session.CombinedOutput(cmd)
	return string(out), err
}

// RunCommandStdin 执行命令并把 stdinStr 通过 stdin 管道喂给远端进程（用于 sudo -S 喂密码）。
func RunCommandStdin(client *ssh.Client, cmd, stdinStr string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	stdin, err := session.StdinPipe()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	session.Stdout = &buf
	session.Stderr = &buf
	if err := session.Start(cmd); err != nil {
		return "", err
	}
	if stdinStr != "" {
		_, _ = io.WriteString(stdin, stdinStr)
	}
	_ = stdin.Close()
	werr := session.Wait()
	return buf.String(), werr
}

// GetLoginPassword 返回解密后的 SSH 登录密码（仅 sudo 回退时使用，不外泄）。
func GetLoginPassword(connID int) (string, error) {
	conn, err := GetConnectionByID(connID)
	if err != nil {
		return "", err
	}
	if conn.PasswordEnc == "" {
		return "", nil
	}
	return crypto.Decrypt(conn.PasswordEnc, crypto.DefaultKey())
}

package sshx

import (
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

const idleTimeout = 5 * time.Minute // 5 分钟空闲回收

type poolEntry struct {
	target *ssh.Client
	chain  []*ssh.Client
	sftp   *sftp.Client
	timer  *time.Timer
}

var (
	poolMu sync.Mutex
	pool   = map[int]*poolEntry{}
)

func scheduleCleanup(connID int) {
	poolMu.Lock()
	e, ok := pool[connID]
	poolMu.Unlock()
	if !ok {
		return
	}
	if e.timer != nil {
		e.timer.Stop()
	}
	e.timer = time.AfterFunc(idleTimeout, func() {
		poolMu.Lock()
		ent := pool[connID]
		poolMu.Unlock()
		if ent == nil {
			return
		}
		closeChain(ent.chain)
		poolMu.Lock()
		delete(pool, connID)
		poolMu.Unlock()
	})
}

// GetClient 返回（或创建）连接池中的目标 SSH client。
func GetClient(connID int) (*ssh.Client, error) {
	poolMu.Lock()
	if e, ok := pool[connID]; ok {
		poolMu.Unlock()
		scheduleCleanup(connID)
		return e.target, nil
	}
	poolMu.Unlock()

	conn, err := GetConnectionByID(connID)
	if err != nil {
		return nil, err
	}
	target, chain, err := CreateSshClient(conn)
	if err != nil {
		return nil, err
	}
	e := &poolEntry{target: target, chain: chain}
	registerPoolEntry(connID, e)
	scheduleCleanup(connID)
	return target, nil
}

// GetSftp 返回（或创建）连接池中的 sftp 子会话。
func GetSftp(connID int) (*sftp.Client, error) {
	poolMu.Lock()
	if e, ok := pool[connID]; ok {
		if e.sftp != nil {
			poolMu.Unlock()
			scheduleCleanup(connID)
			return e.sftp, nil
		}
		poolMu.Unlock()
	} else {
		poolMu.Unlock()
	}

	conn, err := GetConnectionByID(connID)
	if err != nil {
		return nil, err
	}
	target, chain, err := CreateSshClient(conn)
	if err != nil {
		return nil, err
	}
	s, err := OpenSftp(target)
	if err != nil {
		closeChain(chain)
		return nil, err
	}
	e := &poolEntry{target: target, chain: chain, sftp: s}
	registerPoolEntry(connID, e)
	scheduleCleanup(connID)
	return s, nil
}

func registerPoolEntry(connID int, e *poolEntry) {
	poolMu.Lock()
	pool[connID] = e
	poolMu.Unlock()
	go func() {
		_ = e.target.Wait()
		poolMu.Lock()
		if cur := pool[connID]; cur == e {
			if cur.timer != nil {
				cur.timer.Stop()
			}
			delete(pool, connID)
		}
		poolMu.Unlock()
	}()
}

// DropSftp 关闭并移除池中指定连接条目（级联关闭整条链）。
func DropSftp(connID int) {
	poolMu.Lock()
	defer poolMu.Unlock()
	if e, ok := pool[connID]; ok {
		if e.timer != nil {
			e.timer.Stop()
		}
		closeChain(e.chain)
		delete(pool, connID)
	}
}

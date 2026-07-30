package captcha

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

const (
	TTL       = 5 * 60 * 1000 // 5 分钟有效
	charset   = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
	width     = 120
	height    = 40
	fontSize  = 40
)

type entry struct {
	text     string
	expires  int64
}

var (
	mu    sync.Mutex
	cache = map[string]*entry{}
)

// 周期性清理过期验证码，避免内存泄漏
func init() {
	go func() {
		for {
			time.Sleep(60 * time.Second)
			mu.Lock()
			now := time.Now().UnixMilli()
			for id, e := range cache {
				if e.expires <= now {
					delete(cache, id)
				}
			}
			mu.Unlock()
		}
	}()
}

func randInt(n int) int {
	if n <= 0 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

func randChar() byte {
	return charset[randInt(len(charset))]
}

func randColor() string {
	r := randInt(156) + 50
	g := randInt(156) + 50
	b := randInt(156) + 50
	return fmt.Sprintf("rgb(%d,%d,%d)", r, g, b)
}

func randHex3() string {
	return fmt.Sprintf("#%02X%02X%02X", randInt(256), randInt(256), randInt(256))
}

// Create 生成验证码，返回 {id, svg}。
func Create() (string, string) {
	id := hexID()
	var code strings.Builder
	for i := 0; i < 4; i++ {
		code.WriteByte(randChar())
	}
	text := strings.ToLower(code.String())

	mu.Lock()
	cache[id] = &entry{text: text, expires: time.Now().UnixMilli() + TTL}
	mu.Unlock()

	svg := render(text)
	return id, svg
}

func hexID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func render(text string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		width, height, width, height))
	sb.WriteString(`<rect width="100%" height="100%" fill="#f2f2f2"/>`)

	// 干扰线（noise:2 → 至少 2 条）
	for i := 0; i < 2; i++ {
		x1 := randInt(width)
		y1 := randInt(height)
		x2 := randInt(width)
		y2 := randInt(height)
		sb.WriteString(fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-width="1.5" opacity="0.7"/>`,
			x1, y1, x2, y2, randHex3()))
	}
	// 干扰点
	for i := 0; i < 6; i++ {
		cx := randInt(width)
		cy := randInt(height)
		sb.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="1.2" fill="%s"/>`, cx, cy, randHex3()))
	}

	// 字符
	step := width / 5
	for i, c := range text {
		x := step*(i+1) - randInt(6) + 3
		y := height/2 + randInt(10) - 2
		rot := randInt(40) - 20
		col := randColor()
		sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" font-family="Menlo,Consolas,monospace" font-size="%d" fill="%s" text-anchor="middle" transform="rotate(%d %d %d)">%c</text>`,
			x, y, fontSize, col, rot, x, y, c))
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

// Verify 校验（忽略大小写），成功后立即删除防止重放。
func Verify(id, text string) bool {
	if id == "" || text == "" {
		return false
	}
	mu.Lock()
	defer mu.Unlock()
	e, ok := cache[id]
	if !ok {
		return false
	}
	delete(cache, id)
	if time.Now().UnixMilli() > e.expires {
		return false
	}
	return strings.TrimSpace(strings.ToLower(text)) == e.text
}

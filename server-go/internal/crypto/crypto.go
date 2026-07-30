package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"

	"golang.org/x/crypto/scrypt"

	"terminal-web/server-go/internal/config"
)

const (
	scryptN = 16384
	scryptR = 8
	scryptP = 1
	keyLen  = 32
	salt    = "terminal-web-salt"
	ivLen   = 12
	tagLen  = 16
)

var masterKey []byte

func Init() {
	masterKey = deriveKey(config.MasterSecret)
}

func deriveKey(secret string) []byte {
	k, err := scrypt.Key([]byte(secret), []byte(salt), scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		panic(err)
	}
	return k
}

// Encrypt AES-256-GCM。密文格式 base64(IV ‖ tag ‖ ciphertext)，与 Node 版逐字节兼容。
// 空明文返回 nil（库里存 NULL）。
func Encrypt(plain string, key []byte) (string, error) {
	if plain == "" {
		return "", nil
	}
	return encryptBytes([]byte(plain), key)
}

// EncryptStr 与 Encrypt 行为一致，仅返回值便于调用方处理。
func encryptBytes(plain, key []byte) (string, error) {
	iv := make([]byte, ivLen)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	// gcm.Seal 输出为 ciphertext ‖ tag
	sealed := gcm.Seal(nil, iv, plain, nil)
	ct := sealed[:len(sealed)-tagLen]
	tag := sealed[len(sealed)-tagLen:]
	out := make([]byte, 0, ivLen+tagLen+len(ct))
	out = append(out, iv...)
	out = append(out, tag...)
	out = append(out, ct...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt 还原 Encrypt 的密文。key 默认为本机主密钥。
func Decrypt(payload string, key []byte) (string, error) {
	if payload == "" {
		return "", nil
	}
	buf, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", err
	}
	if len(buf) < ivLen+tagLen {
		return "", errors.New("密文长度不足")
	}
	iv := buf[:ivLen]
	tag := buf[ivLen : ivLen+tagLen]
	ct := buf[ivLen+tagLen:]
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	combined := make([]byte, 0, len(ct)+tagLen)
	combined = append(combined, ct...)
	combined = append(combined, tag...)
	plain, err := gcm.Open(nil, iv, combined, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// DefaultKey 返回本机主密钥推导的 32 字节密钥。
func DefaultKey() []byte { return masterKey }

// GetMasterSecret 返回本机主密钥明文（用于备份文件携带）。
func GetMasterSecret() string { return config.MasterSecret }

// DeriveKey 用明文主密钥推导 32 字节密钥（跨机备份 re-key）。
func DeriveKey(secret string) []byte { return deriveKey(secret) }

// HashPassword 返回 "<hex salt>:<hex scrypt64字节>"。
// 兼容关键：Node 版 crypto.scryptSync(password, salt, 64) 的 salt 是
// randomBytes(16).toString('hex') 产出的 **hex 字符串本身**（32 个 ASCII 字符），
// 不做 hex 解码。Go 侧必须同样把 hex 字符串的字节序列当 salt 用。
func HashPassword(password string) (string, error) {
	saltBytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, saltBytes); err != nil {
		return "", err
	}
	saltHex := hex.EncodeToString(saltBytes)
	hash, err := scrypt.Key([]byte(password), []byte(saltHex), scryptN, scryptR, scryptP, 64)
	if err != nil {
		return "", err
	}
	return saltHex + ":" + hex.EncodeToString(hash), nil
}

// VerifyPassword 常数时间比较。salt 为存储串中冒号前的字符串本身（见 HashPassword 注释）。
func VerifyPassword(password, stored string) bool {
	parts := splitTwo(stored, ":")
	if parts[0] == "" || parts[1] == "" {
		return false
	}
	calc, err := scrypt.Key([]byte(password), []byte(parts[0]), scryptN, scryptR, scryptP, 64)
	if err != nil {
		return false
	}
	storedHash, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(calc, storedHash) == 1
}

func splitTwo(s, sep string) [2]string {
	for i := 0; i < len(s); i++ {
		if s[i] == sep[0] {
			return [2]string{s[:i], s[i+1:]}
		}
	}
	return [2]string{s, ""}
}

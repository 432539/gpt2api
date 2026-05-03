// Package crypto 提供 bcrypt 密码哈希、SHA256 + 盐、AES-256-GCM 加密。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword 用 bcrypt cost=12 哈希密码。
func HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), 12)
	return string(h), err
}

// VerifyPassword 校验密码。
func VerifyPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// SHA256Salt 生成 SHA256(plain + salt) hex。
func SHA256Salt(plain, salt string) string {
	h := sha256.New()
	h.Write([]byte(plain))
	h.Write([]byte(salt))
	return hex.EncodeToString(h.Sum(nil))
}

// HMACSHA256 用 hex 输出。
func HMACSHA256(key, payload string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// RandomString 生成长度 n 的 url-safe 随机字符串。
func RandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b)[:n], nil
}

// RandomBytes 生成 n 字节随机数。
func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// AESGCM 用于账号池凭证加密。
type AESGCM struct{ aead cipher.AEAD }

// NewAESGCM key 必须 32 字节（AES-256）。
func NewAESGCM(key []byte) (*AESGCM, error) {
	if len(key) != 32 {
		return nil, errors.New("aes key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AESGCM{aead: g}, nil
}

// Encrypt 输出 nonce(12) + ciphertext + tag(16)。
func (a *AESGCM) Encrypt(plain []byte) ([]byte, error) {
	nonce := make([]byte, a.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	out := a.aead.Seal(nonce, nonce, plain, nil)
	return out, nil
}

// Decrypt 反向。
func (a *AESGCM) Decrypt(data []byte) ([]byte, error) {
	ns := a.aead.NonceSize()
	if len(data) < ns {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := data[:ns], data[ns:]
	return a.aead.Open(nil, nonce, ct, nil)
}

package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// GenerateRandomKey 生成 32 字节随机密钥并 base64 编码（用于 master_key / session_secret）。
func GenerateRandomKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// Encrypt 用 masterKey（base64）以 AES-256-GCM 加密 plaintext，
// 返回 base64(nonce || ciphertext)。
func Encrypt(plaintext, masterKey string) (string, error) {
	aead, err := newAEAD(masterKey)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	sealed := aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt 还原 Encrypt 的输出。密钥不符或密文被篡改时返回错误（GCM 认证失败）。
func Decrypt(cipherB64, masterKey string) (string, error) {
	aead, err := newAEAD(masterKey)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(cipherB64)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	ns := aead.NonceSize()
	if len(raw) < ns+aead.Overhead() {
		return "", fmt.Errorf("ciphertext too short: %d bytes", len(raw))
	}
	plaintext, err := aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

// newAEAD 从 base64 主密钥构建 AES-256-GCM（强制 32 字节密钥）。
func newAEAD(masterKey string) (cipher.AEAD, error) {
	key, err := base64.StdEncoding.DecodeString(masterKey)
	if err != nil {
		return nil, fmt.Errorf("decode master key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes (AES-256), got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

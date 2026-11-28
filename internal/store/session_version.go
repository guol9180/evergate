package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"gorm.io/gorm"
)

// SchemaMetaKeySessionVersion 会话签名密钥版本：改密时轮换，旧 Cookie 立即失效且重启后仍失效。
const SchemaMetaKeySessionVersion = "session_key_version"

// EnsureSessionKeyVersion 返回当前会话密钥版本，缺失时生成并落库。
func EnsureSessionKeyVersion(db *gorm.DB) (string, error) {
	version, err := GetSchemaMeta(db, SchemaMetaKeySessionVersion)
	if err != nil || version != "" {
		return version, err
	}
	fresh, err := newSessionKeyVersion()
	if err != nil {
		return "", err
	}
	if err := SetSchemaMeta(db, SchemaMetaKeySessionVersion, fresh); err != nil {
		// 并发首启时另一进程可能已写入：以库里的为准
		if current, getErr := GetSchemaMeta(db, SchemaMetaKeySessionVersion); getErr == nil && current != "" {
			return current, nil
		}
		return "", err
	}
	return fresh, nil
}

// RotateSessionKeyVersion 在调用方事务内生成并写入新的会话密钥版本，返回新版本供内存中轮换。
func RotateSessionKeyVersion(tx *gorm.DB) (string, error) {
	fresh, err := newSessionKeyVersion()
	if err != nil {
		return "", err
	}
	if err := SetSchemaMeta(tx, SchemaMetaKeySessionVersion, fresh); err != nil {
		return "", fmt.Errorf("rotate session key version: %w", err)
	}
	return fresh, nil
}

func newSessionKeyVersion() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

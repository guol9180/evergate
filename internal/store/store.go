// Package store 管理 SQLite 连接与 GORM 迁移。
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Open 打开（必要时创建）SQLite 数据库，启用 WAL 与外键约束。
func Open(path string) (*gorm.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory %s: %w", dir, err)
		}
	}
	// glebarez/modernc 驱动通过 file: URI 的 _pragma 参数下发 PRAGMA。
	dsn := "file:" + strings.ReplaceAll(filepath.ToSlash(path), " ", "%20") +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		// GORM 自带日志静默：运行日志统一走 slog（见 AGENTS.md 代码风格）。
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	return db, nil
}

// Migrate 对给定模型执行 GORM AutoMigrate（个人项目阶段唯一迁移方式，见 AGENTS.md 禁止事项）。
func Migrate(db *gorm.DB, models ...any) error {
	if len(models) == 0 {
		return nil
	}
	return db.AutoMigrate(models...)
}

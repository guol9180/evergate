// Package store 管理 SQLite 连接与 GORM 迁移。
package store

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Options 是打开数据库时的可选项。
type Options struct {
	// Synchronous 对应 PRAGMA synchronous：留空 / full = FULL（断电安全，默认），
	// normal = NORMAL（WAL 下提交更快，断电可能丢最后几次提交，不损坏库）。
	Synchronous string
}

// Open 打开（必要时创建）SQLite 数据库，启用 WAL 与外键约束（FULL 同步级别）。
func Open(path string) (*gorm.DB, error) {
	return OpenWithOptions(path, Options{})
}

// OpenWithOptions 按选项打开数据库。
func OpenWithOptions(path string, opt Options) (*gorm.DB, error) {
	synchronous, err := normalizeSynchronous(opt.Synchronous)
	if err != nil {
		return nil, err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory %s: %w", dir, err)
		}
	}
	// glebarez/modernc 驱动通过 file: URI 的 _pragma 参数下发 PRAGMA。
	dsn := "file:" + strings.ReplaceAll(filepath.ToSlash(path), " ", "%20") +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(" + synchronous + ")"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		// GORM 自带日志静默：运行日志统一走 slog（见 AGENTS.md 代码风格）。
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
		// 红线：所有时间以 UTC 存储（见 AGENTS.md 代码风格）。
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	return db, nil
}

// normalizeSynchronous 把配置值映射成 PRAGMA 取值；空值保持 FULL。
func normalizeSynchronous(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "full":
		return "FULL", nil
	case "normal":
		return "NORMAL", nil
	default:
		return "", fmt.Errorf("unsupported sqlite synchronous mode %q (allowed: full, normal)", value)
	}
}

// Migrate 对全部表模型执行 GORM AutoMigrate（个人项目阶段唯一迁移方式，见 AGENTS.md 禁止事项）。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(AllModels()...)
}

// MigrateWithBackup 在 AutoMigrate 前按应用版本变化备份 SQLite 主文件，再执行迁移并记录版本（T75）。
// 备份失败只告警不阻断：迁移本身仍是前进方向，丢失备份好于拒绝启动。
func MigrateWithBackup(db *gorm.DB, dbPath, version string, logger *slog.Logger) error {
	if reason, ok := shouldBackupBeforeMigrate(db, dbPath, version); ok {
		if err := backupDatabase(db, dbPath); err != nil {
			if logger != nil {
				logger.Warn("database backup before migration failed",
					slog.String("reason", reason), slog.Any("error", err))
			}
		} else if logger != nil {
			logger.Info("database backed up before migration", slog.String("reason", reason))
		}
	}
	if err := Migrate(db); err != nil {
		return err
	}
	return SetSchemaMeta(db, SchemaMetaKeyAppVersion, version)
}

// AllModels 返回全部表模型，供 AutoMigrate 使用。
func AllModels() []any {
	return []any{
		&Channel{},
		&ChannelKey{},
		&Token{},
		&RequestLog{},
		&UsageDaily{},
		&ModelPrice{},
		&ModelFallback{},
		&Admin{},
		&RuntimeSetting{},
		&AuditLog{},
		&SchemaMeta{},
		&AlertSetting{},
		&AlertRecord{},
	}
}

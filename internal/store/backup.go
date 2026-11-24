package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// SchemaMetaKeyAppVersion 记录最近一次完成迁移的应用版本；版本变化即视为升级，触发备份。
const SchemaMetaKeyAppVersion = "app_version"

// BackupKeepCount 升级前备份保留份数（按文件名中的 UTC 时间戳排序，最旧优先删除）。
const BackupKeepCount = 5

// shouldBackupBeforeMigrate 判断本次迁移前是否需要备份，返回原因。
// 规则：开发态版本不备份（每次本机构建版本都不同）；库文件不存在或尚无任何业务表（全新安装）不备份；
// schema_meta 缺失（来自旧版本）或记录的 app_version 与当前不同则备份。
func shouldBackupBeforeMigrate(db *gorm.DB, dbPath, version string) (string, bool) {
	if IsDevVersion(version) {
		return "", false
	}
	if _, err := os.Stat(dbPath); err != nil {
		return "", false
	}
	// channels 是最早存在的业务表；它不存在说明库刚由 Open 创建，还没有可备份的内容。
	if !db.Migrator().HasTable(&Channel{}) {
		return "", false
	}
	var count int64
	if err := db.Model(&SchemaMeta{}).Count(&count).Error; err != nil {
		// schema_meta 表不存在：库由旧版本创建，无法判断结构差异，保守备份一次。
		return "schema_meta unavailable", true
	}
	previous, err := GetSchemaMeta(db, SchemaMetaKeyAppVersion)
	if err != nil {
		return "schema_meta unreadable", true
	}
	if previous != version {
		return fmt.Sprintf("version %s -> %s", previous, version), true
	}
	return "", false
}

// IsDevVersion 判断是否为开发态版本串（dev / 含 -dirty / 含 -N-g<sha>），与 AGENTS.md 版本规则一致。
func IsDevVersion(version string) bool {
	v := strings.TrimSpace(version)
	if v == "" || v == "dev" || strings.HasPrefix(v, "dev") {
		return true
	}
	return strings.Contains(v, "-dirty") || containsBuildMeta(v)
}

func containsBuildMeta(v string) bool {
	i := strings.LastIndex(v, "-")
	if i < 0 {
		return false
	}
	// 形如 v0.5.5-3-gabc1234：最后一个 '-' 段后紧跟 'g' 与十六进制 sha。
	rest := v[i+1:]
	return len(rest) > 1 && rest[0] == 'g' && !strings.Contains(rest, ".")
}

// GetSchemaMeta 读取元数据键值；不存在时返回空串。
func GetSchemaMeta(db *gorm.DB, key string) (string, error) {
	var meta SchemaMeta
	if err := db.Where("`key` = ?", key).First(&meta).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("read schema meta %s: %w", key, err)
	}
	return meta.Value, nil
}

// SetSchemaMeta upsert 元数据键值。
func SetSchemaMeta(db *gorm.DB, key, value string) error {
	var meta SchemaMeta
	err := db.Where("`key` = ?", key).First(&meta).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return db.Create(&SchemaMeta{Key: key, Value: value}).Error
	case err != nil:
		return fmt.Errorf("lookup schema meta %s: %w", key, err)
	}
	meta.Value = value
	return db.Model(&SchemaMeta{}).Where("`key` = ?", key).Update("value", value).Error
}

// backupDatabase 先把 WAL 内容合并回主文件，再复制主文件到 <dataDir>/backups/evergate-<UTC>.db，并裁剪保留份数。
func backupDatabase(db *gorm.DB, dbPath string) error {
	if err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		return fmt.Errorf("wal checkpoint: %w", err)
	}
	src, err := os.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer src.Close()

	dir := filepath.Join(filepath.Dir(dbPath), "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	name := fmt.Sprintf("evergate-%s.db", time.Now().UTC().Format("20060102T150405Z"))
	dst, err := openUniqueBackup(dir, name)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(dst.Name())
		return fmt.Errorf("copy database: %w", err)
	}
	if err := dst.Sync(); err != nil {
		dst.Close()
		return fmt.Errorf("sync backup: %w", err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("close backup: %w", err)
	}
	return pruneBackups(dir, BackupKeepCount)
}

// openUniqueBackup 以 O_EXCL 创建备份文件；同一秒内多次备份时追加 -1/-2 序号避免冲突。
func openUniqueBackup(dir, name string) (*os.File, error) {
	base := strings.TrimSuffix(name, ".db")
	for i := 0; i < 100; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d.db", base, i)
		}
		f, err := os.OpenFile(filepath.Join(dir, candidate), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			return f, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
	}
	return nil, errors.New("too many backups within the same second")
}

// pruneBackups 只保留最近 keep 份备份（文件名字典序即时间序）。
func pruneBackups(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("list backups: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "evergate-") && strings.HasSuffix(e.Name(), ".db") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keep {
		if err := os.Remove(filepath.Join(dir, names[0])); err != nil {
			return fmt.Errorf("remove old backup %s: %w", names[0], err)
		}
		names = names[1:]
	}
	return nil
}

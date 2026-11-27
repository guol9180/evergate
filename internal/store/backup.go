package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// SchemaMetaKeyAppVersion 记录最近一次完成迁移的应用版本；版本变化即视为升级，触发备份。
const SchemaMetaKeyAppVersion = "app_version"

// BackupKeepCount 升级前备份保留份数（按文件名中的 UTC 时间戳排序，最旧优先删除）。
const BackupKeepCount = 5

// backupNamePattern 约束备份文件名，防止下载 / 删除接口被路径穿越利用。
var backupNamePattern = regexp.MustCompile(`^evergate-\d{8}T\d{6}Z(-\d+)?\.db$`)

// restorePendingSuffix 恢复暂存文件后缀：上传校验通过后先落盘到这里，下次启动时替换主库。
const restorePendingSuffix = ".restore-pending"

// BackupInfo 是管理台展示用的备份条目。
type BackupInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	Time    time.Time `json:"time"`
	Version string    `json:"version"`
}

// BackupDir 返回备份目录（与主库同级的 backups/）。
func BackupDir(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "backups")
}

// ValidBackupName 判断文件名是否为合法备份名。
func ValidBackupName(name string) bool {
	return backupNamePattern.MatchString(name)
}

// BackupDatabase 立即做一次 WAL checkpoint 并把主库复制到 backups/，返回备份文件名；随后裁剪保留份数。
func BackupDatabase(db *gorm.DB, dbPath string) (string, error) {
	name, err := backupDatabase(db, dbPath)
	return name, err
}

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
func backupDatabase(db *gorm.DB, dbPath string) (string, error) {
	if err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		return "", fmt.Errorf("wal checkpoint: %w", err)
	}
	src, err := os.Open(dbPath)
	if err != nil {
		return "", fmt.Errorf("open database: %w", err)
	}
	defer src.Close()

	dir := filepath.Join(filepath.Dir(dbPath), "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}
	name := fmt.Sprintf("evergate-%s.db", time.Now().UTC().Format("20060102T150405Z"))
	dst, err := openUniqueBackup(dir, name)
	if err != nil {
		return "", fmt.Errorf("create backup file: %w", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(dst.Name())
		return "", fmt.Errorf("copy database: %w", err)
	}
	if err := dst.Sync(); err != nil {
		dst.Close()
		return "", fmt.Errorf("sync backup: %w", err)
	}
	if err := dst.Close(); err != nil {
		return "", fmt.Errorf("close backup: %w", err)
	}
	created := filepath.Base(dst.Name())
	return created, pruneBackups(dir, BackupKeepCount)
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

// ListBackups 返回 backups/ 下的备份（新 → 旧），每条附带大小、修改时间与库内记录的 app_version。
// 读取版本会短暂打开每个备份；打不开的备份标记 version 为空，不阻断列表。
func ListBackups(dbPath string) ([]BackupInfo, error) {
	dir := BackupDir(dbPath)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	out := make([]BackupInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !ValidBackupName(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		version, _ := ProbeBackup(filepath.Join(dir, e.Name()))
		out = append(out, BackupInfo{
			Name: e.Name(), Size: info.Size(), Time: info.ModTime().UTC(), Version: version,
		})
	}
	// 文件名字典序即时间序，倒序得到新 → 旧
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

// BackupPath 校验文件名并返回绝对路径；非法名称返回错误而不是拼路径。
func BackupPath(dbPath, name string) (string, error) {
	if !ValidBackupName(name) {
		return "", fmt.Errorf("invalid backup name %q", name)
	}
	return filepath.Join(BackupDir(dbPath), name), nil
}

// DeleteBackup 删除一份备份；拒绝删除正在被引用的名称以外的任何路径。
func DeleteBackup(dbPath, name string) error {
	path, err := BackupPath(dbPath, name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete backup %s: %w", name, err)
	}
	return nil
}

// ProbeBackup 打开一个备份文件做只读校验：必须能打开、含核心业务表且 schema_meta 可读；返回其 app_version。
func ProbeBackup(path string) (string, error) {
	db, err := OpenWithOptions(path, Options{})
	if err != nil {
		return "", fmt.Errorf("open backup: %w", err)
	}
	sqlDB, _ := db.DB()
	defer func() { _ = sqlDB.Close() }()
	if !db.Migrator().HasTable(&Channel{}) || !db.Migrator().HasTable(&SchemaMeta{}) {
		return "", errors.New("backup is missing core tables (channels / schema_meta)")
	}
	var version string
	if err := db.Raw("SELECT value FROM schema_meta WHERE `key` = ?", SchemaMetaKeyAppVersion).Scan(&version).Error; err != nil {
		return "", fmt.Errorf("read schema_meta: %w", err)
	}
	return version, nil
}

// StageRestore 校验上传的备份文件，先把当前库备份一份（回退点），再把上传文件移到 <dbPath>.restore-pending。
// 真正替换发生在下次启动（ApplyPendingRestore），避免在持有旧句柄时 swap 主库文件。
func StageRestore(db *gorm.DB, dbPath, uploadedPath string) (safetyBackup string, err error) {
	if _, err := ProbeBackup(uploadedPath); err != nil {
		return "", err
	}
	safetyBackup, err = backupDatabase(db, dbPath)
	if err != nil {
		return "", fmt.Errorf("backup current database before restore: %w", err)
	}
	pending := dbPath + restorePendingSuffix
	_ = os.Remove(pending)
	if err := os.Rename(uploadedPath, dbPath+restorePendingSuffix); err != nil {
		return safetyBackup, fmt.Errorf("stage restore file: %w", err)
	}
	// 上传文件被 SQLite 打开时可能留下 -wal / -shm，一并清理
	_ = os.Remove(uploadedPath + "-wal")
	_ = os.Remove(uploadedPath + "-shm")
	return safetyBackup, nil
}

// ApplyPendingRestore 在打开主库之前调用：若存在 .restore-pending，则把当前主库挪到 .pre-restore
// 并用暂存文件替换；返回是否执行了替换。任何一步失败都保留原状并返回错误，由调用方决定是否中止启动。
func ApplyPendingRestore(dbPath string) (bool, error) {
	pending := dbPath + restorePendingSuffix
	if _, err := os.Stat(pending); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat pending restore: %w", err)
	}
	pre := dbPath + ".pre-restore"
	_ = os.Remove(pre)
	if _, err := os.Stat(dbPath); err == nil {
		if err := os.Rename(dbPath, pre); err != nil {
			return false, fmt.Errorf("move current database aside: %w", err)
		}
	}
	// 旧主库的 WAL / SHM 属于被替换的文件，留着会污染新库
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	if err := os.Rename(pending, dbPath); err != nil {
		return false, fmt.Errorf("move restored database into place: %w", err)
	}
	return true, nil
}

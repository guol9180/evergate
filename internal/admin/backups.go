package admin

// 整库备份：创建 / 列表 / 下载 / 删除，以及上传备份并暂存恢复（下次启动生效）。

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/store"
)

// maxRestoreUpload 上传备份的大小上限；单机 SQLite 远小于此，防止误传大文件占满磁盘。
const maxRestoreUpload = 1 << 30

func (a *API) requireDBPath(c *gin.Context) bool {
	if a.opt.DBPath == "" || a.opt.DB == nil {
		fail(c, http.StatusServiceUnavailable, "backup_unavailable", "备份未配置")
		return false
	}
	return true
}

func (a *API) listBackups(c *gin.Context) {
	if !a.requireDBPath(c) {
		return
	}
	items, err := store.ListBackups(a.opt.DBPath)
	if err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	ok(c, gin.H{"items": items, "dir": store.BackupDir(a.opt.DBPath), "restart_pending": a.restorePending()})
}

func (a *API) createBackup(c *gin.Context) {
	if !a.requireDBPath(c) {
		return
	}
	name, err := store.BackupDatabase(a.opt.DB, a.opt.DBPath)
	if err != nil {
		fail(c, http.StatusInternalServerError, "backup_failed", "备份失败")
		return
	}
	info := store.BackupInfo{Name: name, Time: a.now().UTC()}
	if stat, err := os.Stat(filepath.Join(store.BackupDir(a.opt.DBPath), name)); err == nil {
		info.Size = stat.Size()
	}
	info.Version, _ = store.GetSchemaMeta(a.opt.DB, store.SchemaMetaKeyAppVersion)
	ok(c, gin.H{"name": name, "size": info.Size, "version": info.Version, "time": info.Time})
}

func (a *API) downloadBackup(c *gin.Context) {
	if !a.requireDBPath(c) {
		return
	}
	path, err := store.BackupPath(a.opt.DBPath, c.Param("name"))
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "非法备份名")
		return
	}
	if _, err := os.Stat(path); err != nil {
		fail(c, http.StatusNotFound, "backup_not_found", "备份不存在")
		return
	}
	c.FileAttachment(path, c.Param("name"))
}

func (a *API) deleteBackup(c *gin.Context) {
	if !a.requireDBPath(c) {
		return
	}
	name := c.Param("name")
	if err := store.DeleteBackup(a.opt.DBPath, name); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(c, http.StatusNotFound, "backup_not_found", "备份不存在")
			return
		}
		fail(c, http.StatusBadRequest, "invalid_param", "非法备份名")
		return
	}
	ok(c, gin.H{"deleted": true, "name": name})
}

func (a *API) restorePending() bool {
	_, err := os.Stat(a.opt.DBPath + ".restore-pending")
	return err == nil
}

// restoreBackup 接收上传的备份文件：校验 → 自动备份当前库 → 落盘待生效，返回需要重启。
func (a *API) restoreBackup(c *gin.Context) {
	if !a.requireDBPath(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRestoreUpload)
	header, err := c.FormFile("file")
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "请选择备份文件")
		return
	}
	src, err := header.Open()
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "读取上传文件失败")
		return
	}
	defer src.Close()

	if err := os.MkdirAll(a.opt.DataDir, 0o755); err != nil {
		fail(c, http.StatusInternalServerError, "restore_failed", "准备上传目录失败")
		return
	}
	uploaded := filepath.Join(a.opt.DataDir, fmt.Sprintf("restore-upload-%s.db", time.Now().UTC().Format("20060102T150405Z")))
	dst, err := os.OpenFile(uploaded, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		fail(c, http.StatusInternalServerError, "restore_failed", "写入上传文件失败")
		return
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		_ = os.Remove(uploaded)
		fail(c, http.StatusBadRequest, "invalid_param", "保存上传文件失败")
		return
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(uploaded)
		fail(c, http.StatusInternalServerError, "restore_failed", "保存上传文件失败")
		return
	}
	// StageRestore 成功会把上传文件 rename 走；失败时清掉残留
	safety, err := store.StageRestore(a.opt.DB, a.opt.DBPath, uploaded)
	_, statErr := os.Stat(uploaded)
	if statErr == nil {
		_ = os.Remove(uploaded)
		_ = os.Remove(uploaded + "-wal")
		_ = os.Remove(uploaded + "-shm")
	}
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_backup", err.Error())
		return
	}
	ok(c, gin.H{"restart_required": true, "safety_backup": safety, "name": header.Filename})
}

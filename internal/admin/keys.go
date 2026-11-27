package admin

// 上游 key：批量导入、启停、删除。

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"evergate/internal/store"
)

// batchAddKeys 批量导入 key：多行纯文本，每行一把；批内与渠道存量都去重，
// 单事务批量写入，返回新增 / 重复 / 非法计数（不回显任何明文）。
func (a *API) batchAddKeys(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var req struct {
		Keys string `json:"keys" binding:"required"` // 多行纯文本
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误")
		return
	}
	lines := strings.Split(strings.ReplaceAll(req.Keys, "\r\n", "\n"), "\n")
	seen := make(map[string]bool, len(lines))
	plaintexts := make([]string, 0, len(lines))
	duplicates, invalid := 0, 0
	for _, line := range lines {
		key := strings.TrimSpace(line)
		if key == "" {
			continue
		}
		if len(key) > maxKeyChars {
			invalid++
			continue
		}
		if seen[key] {
			duplicates++
			continue
		}
		seen[key] = true
		plaintexts = append(plaintexts, key)
	}
	if len(plaintexts) > maxKeyBatch {
		fail(c, http.StatusBadRequest, "key_batch_too_large", fmt.Sprintf("单次最多导入 %d 把 key，请分批粘贴", maxKeyBatch))
		return
	}
	// 与渠道存量 key 去重：同一把上游 key 重复入库会变成双倍权重，只能解密比对。
	if len(plaintexts) > 0 {
		var existingKeys []*store.ChannelKey
		if err := a.opt.DB.Where("channel_id = ?", ch.ID).Find(&existingKeys).Error; err != nil {
			fail(c, http.StatusInternalServerError, "key_query_failed", "查询已有 key 失败")
			return
		}
		existing := make(map[string]bool, len(existingKeys))
		for _, k := range existingKeys {
			if plain, derr := store.Decrypt(k.KeyCipher, a.opt.MasterKey); derr == nil {
				existing[plain] = true
			}
		}
		kept := plaintexts[:0]
		for _, key := range plaintexts {
			if existing[key] {
				duplicates++
				continue
			}
			kept = append(kept, key)
		}
		plaintexts = kept
	}
	items := make([]*store.ChannelKey, 0, len(plaintexts))
	for _, key := range plaintexts {
		cipher, err := store.Encrypt(key, a.opt.MasterKey)
		if err != nil {
			if a.opt.Logger != nil {
				a.opt.Logger.Warn("encrypt channel key failed", slog.Any("error", err))
			}
			fail(c, http.StatusInternalServerError, "key_encrypt_failed", "加密 key 失败")
			return
		}
		items = append(items, &store.ChannelKey{ChannelID: ch.ID, KeyCipher: cipher, Enabled: true, Weight: 1, State: store.KeyStateActive})
	}
	if len(items) > 0 {
		if err := a.opt.DB.Transaction(func(tx *gorm.DB) error {
			return tx.CreateInBatches(items, 100).Error
		}); err != nil {
			if a.opt.Logger != nil {
				a.opt.Logger.Warn("batch add channel keys failed", slog.Any("error", err))
			}
			fail(c, http.StatusInternalServerError, "key_import_rolled_back", "导入失败，已回滚")
			return
		}
	}
	ok(c, gin.H{"added": len(items), "duplicates": duplicates, "invalid": invalid})
}

func (a *API) updateKey(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "非法 id")
		return
	}
	var key store.ChannelKey
	if err := a.opt.DB.First(&key, id).Error; err != nil {
		fail(c, http.StatusNotFound, "key_not_found", "key 不存在")
		return
	}
	var req struct {
		Enabled  *bool `json:"enabled"`
		Weight   *int  `json:"weight"`
		RPMLimit *int  `json:"rpm_limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误")
		return
	}
	updates := map[string]any{}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
		// 手动重新启用优先于自动状态：重置状态机（任务书 4.5③）
		if *req.Enabled && !key.Enabled {
			a.opt.Pool.Reset(key.ID)
		}
	}
	if req.Weight != nil {
		updates["weight"] = *req.Weight
	}
	if req.RPMLimit != nil {
		if *req.RPMLimit < 0 {
			fail(c, http.StatusBadRequest, "invalid_param", "限流值不能为负数")
			return
		}
		updates["rpm_limit"] = *req.RPMLimit
	}
	if len(updates) > 0 {
		if err := a.opt.DB.Model(&key).Updates(updates).Error; err != nil {
			fail(c, http.StatusInternalServerError, "update_failed", "更新失败")
			return
		}
	}
	ok(c, gin.H{"updated": true})
}

func (a *API) deleteKey(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "非法 id")
		return
	}
	if err := a.opt.DB.Delete(&store.ChannelKey{}, id).Error; err != nil {
		fail(c, http.StatusInternalServerError, "delete_failed", "删除失败")
		return
	}
	if a.opt.Pool != nil {
		a.opt.Pool.ForgetKey(id)
	}
	ok(c, gin.H{"deleted": true})
}

// ---------- 下游令牌 ----------

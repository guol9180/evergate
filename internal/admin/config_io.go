package admin

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"evergate/internal/settings"
	"evergate/internal/store"
	"evergate/internal/usage"
)

// configExportVersion 导出文件结构版本；导入端据此拒绝未知版本。
const configExportVersion = 1

// configChannelEntry 渠道导出项：与 channelReq 同构，便于导入时复用 normalize 校验；绝不含 key。
type configChannelEntry struct {
	Name              string                `json:"name"`
	Type              string                `json:"type"`
	PresetID          string                `json:"preset_id"`
	BaseURL           string                `json:"base_url"`
	Priority          int                   `json:"priority"`
	Weight            int                   `json:"weight"`
	Models            []string              `json:"models"`
	ModelMapping      map[string]string     `json:"model_mapping"`
	Enabled           bool                  `json:"enabled"`
	SupportsResponses bool                  `json:"supports_responses"`
	SupportsAnthropic bool                  `json:"supports_anthropic"`
	SupportsGemini    bool                  `json:"supports_gemini"`
	Timeouts          store.ChannelTimeouts `json:"timeout_config"`
}

type configFallbackEntry struct {
	Model           string   `json:"model"`
	BudgetThreshold int64    `json:"budget_threshold"`
	Fallbacks       []string `json:"fallbacks"`
}

// configPriceEntry 价格覆盖；ChannelName 用于跨实例重映射（id 不稳定）。
type configPriceEntry struct {
	ChannelName            string `json:"channel_name"`
	Model                  string `json:"model"`
	InputMicrosPerMillion  int64  `json:"input_micros_per_million"`
	OutputMicrosPerMillion int64  `json:"output_micros_per_million"`
	// 缓存价仅在导出时已设置才出现；旧版本导出文件没有这两项，导入时不触碰目标实例的现有值。
	CacheReadMicrosPerMillion  *int64 `json:"cache_read_micros_per_million,omitempty"`
	CacheWriteMicrosPerMillion *int64 `json:"cache_write_micros_per_million,omitempty"`
}

func (e configPriceEntry) priceInput() usage.PriceInput {
	return usage.PriceInput{
		Input: e.InputMicrosPerMillion, Output: e.OutputMicrosPerMillion,
		CacheRead: e.CacheReadMicrosPerMillion, CacheWrite: e.CacheWriteMicrosPerMillion,
	}
}

// configTokenEntry 令牌治理字段；KeyHash 永不导出，导入的令牌需用户重新创建明文。
type configTokenEntry struct {
	Name            string   `json:"name"`
	Enabled         bool     `json:"enabled"`
	Quota           int64    `json:"quota"`
	ExpiredAt       *string  `json:"expired_at,omitempty"`
	AllowedModels   []string `json:"allowed_models"`
	AllowedIPs      []string `json:"allowed_ips"`
	DisableFallback bool     `json:"disable_fallback"`
}

type configExport struct {
	Version        int                   `json:"version"`
	ExportedAt     string                `json:"exported_at"`
	Channels       []configChannelEntry  `json:"channels"`
	ModelFallbacks []configFallbackEntry `json:"model_fallbacks"`
	Settings       *settings.Values      `json:"settings"`
	PriceDefault   *configPriceEntry     `json:"price_default"`
	PriceOverrides []configPriceEntry    `json:"price_overrides"`
	Tokens         []configTokenEntry    `json:"tokens"`
	Note           string                `json:"note"`
}

type configImportResult struct {
	Created int64    `json:"created"`
	Updated int64    `json:"updated"`
	Skipped int64    `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

func (a *API) exportConfig(c *gin.Context) {
	var channels []*store.Channel
	if err := a.opt.DB.Order("id asc").Find(&channels).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询渠道失败")
		return
	}
	var fallbacks []*store.ModelFallback
	if err := a.opt.DB.Order("model asc").Find(&fallbacks).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询降级链失败")
		return
	}
	var prices []*store.ModelPrice
	if err := a.opt.DB.Order("channel_id asc, model asc").Find(&prices).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询价格失败")
		return
	}
	var tokens []*store.Token
	if err := a.opt.DB.Order("id asc").Find(&tokens).Error; err != nil {
		fail(c, http.StatusInternalServerError, "查询令牌失败")
		return
	}
	channelName := make(map[int64]string, len(channels))
	out := configExport{
		Version: configExportVersion, ExportedAt: a.now().UTC().Format("2006-01-02T15:04:05Z"),
		Channels:       make([]configChannelEntry, 0, len(channels)),
		ModelFallbacks: make([]configFallbackEntry, 0, len(fallbacks)),
		PriceOverrides: make([]configPriceEntry, 0, len(prices)),
		Tokens:         make([]configTokenEntry, 0, len(tokens)),
		Note:           "Upstream keys and token hashes are never exported. Import tokens to restore governance only; create new token keys afterwards.",
	}
	for _, ch := range channels {
		channelName[ch.ID] = ch.Name
		out.Channels = append(out.Channels, configChannelEntry{
			Name: ch.Name, Type: ch.Type, BaseURL: ch.BaseURL, Priority: ch.Priority, Weight: ch.Weight,
			PresetID: ch.PresetID,
			Models:   ch.Models, ModelMapping: ch.ModelMapping, Enabled: ch.Enabled,
			SupportsResponses: ch.SupportsResponses, SupportsAnthropic: ch.SupportsAnthropic, SupportsGemini: ch.SupportsGemini,
			Timeouts: ch.Timeouts,
		})
	}
	for _, f := range fallbacks {
		out.ModelFallbacks = append(out.ModelFallbacks, configFallbackEntry{Model: f.Model, BudgetThreshold: f.BudgetThreshold, Fallbacks: f.Fallbacks})
	}
	if a.opt.Settings != nil {
		values := a.opt.Settings.Snapshot().Values
		out.Settings = &values
	}
	for _, p := range prices {
		entry := configPriceEntry{
			Model: p.Model, InputMicrosPerMillion: p.InputMicrosPerMillion, OutputMicrosPerMillion: p.OutputMicrosPerMillion,
		}
		if p.CacheReadMicrosPerMillion > 0 {
			entry.CacheReadMicrosPerMillion = &p.CacheReadMicrosPerMillion
		}
		if p.CacheWriteMicrosPerMillion > 0 {
			entry.CacheWriteMicrosPerMillion = &p.CacheWriteMicrosPerMillion
		}
		if p.ChannelID == 0 && p.Model == "" {
			out.PriceDefault = &entry
			continue
		}
		entry.ChannelName = channelName[p.ChannelID]
		out.PriceOverrides = append(out.PriceOverrides, entry)
	}
	for _, t := range tokens {
		entry := configTokenEntry{
			Name: t.Name, Enabled: t.Enabled, Quota: t.Quota, AllowedModels: t.AllowedModels, AllowedIPs: t.AllowedIPs,
			DisableFallback: t.DisableFallback,
		}
		if t.ExpiredAt != nil {
			iso := t.ExpiredAt.UTC().Format("2006-01-02T15:04:05Z")
			entry.ExpiredAt = &iso
		}
		out.Tokens = append(out.Tokens, entry)
	}
	c.Header("Content-Disposition", `attachment; filename="`+fmt.Sprintf("evergate-config-%s.json", a.now().Format("20060102T150405Z"))+`"`)
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": out})
}

// importConfig 按名称幂等 upsert 渠道/令牌，整体替换降级链，upsert 设置与价格；所有条目先校验后落库，任一条目非法则整体拒绝。
func (a *API) importConfig(c *gin.Context) {
	var in configExport
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}
	if in.Version != configExportVersion {
		fail(c, http.StatusBadRequest, fmt.Sprintf("不支持的配置版本 %d", in.Version))
		return
	}
	// 先全量校验，避免半成功。
	reqs := make([]channelReq, 0, len(in.Channels))
	for i, ch := range in.Channels {
		req := channelReq{
			Name: ch.Name, Type: ch.Type, BaseURL: ch.BaseURL,
			PresetID: &ch.PresetID,
			Priority: &ch.Priority, Weight: &ch.Weight, Models: ch.Models, ModelMapping: ch.ModelMapping,
			SupportsResponses: &ch.SupportsResponses, SupportsAnthropic: &ch.SupportsAnthropic, SupportsGemini: &ch.SupportsGemini,
			Timeouts: ch.Timeouts,
		}
		if err := req.normalize(); err != nil {
			fail(c, http.StatusBadRequest, fmt.Sprintf("channels[%d]: %v", i, err))
			return
		}
		if err := a.validateEffectiveChannelTimeouts(req.Timeouts); err != nil {
			fail(c, http.StatusBadRequest, fmt.Sprintf("channels[%d]: %v", i, err))
			return
		}
		reqs = append(reqs, req)
	}
	fallbackRows, err := normalizeModelFallbacks(func() []modelFallbackReq {
		items := make([]modelFallbackReq, 0, len(in.ModelFallbacks))
		for _, f := range in.ModelFallbacks {
			items = append(items, modelFallbackReq(f))
		}
		return items
	}())
	if err != nil {
		fail(c, http.StatusBadRequest, "model_fallbacks: "+err.Error())
		return
	}
	if in.Settings != nil && a.opt.Settings == nil {
		fail(c, http.StatusServiceUnavailable, "系统设置未配置")
		return
	}

	result := configImportResult{}
	// 渠道：按名称 upsert，保留已启用状态与已有 key。
	for i, req := range reqs {
		var existing store.Channel
		err := a.opt.DB.Where("name = ?", req.Name).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			ch := &store.Channel{
				Name: req.Name, Type: req.Type, BaseURL: req.BaseURL,
				PresetID: derefString(req.PresetID, ""),
				Priority: derefInt(req.Priority, 0), Weight: derefInt(req.Weight, 1),
				Models: req.Models, ModelMapping: req.ModelMapping,
				Enabled: in.Channels[i].Enabled, Status: store.ChannelStatusActive,
				SupportsResponses: derefBool(req.SupportsResponses, false), SupportsAnthropic: derefBool(req.SupportsAnthropic, false),
				SupportsGemini: derefBool(req.SupportsGemini, false), Timeouts: req.Timeouts,
			}
			if err := a.opt.DB.Create(ch).Error; err != nil {
				fail(c, http.StatusInternalServerError, "导入渠道失败："+err.Error())
				return
			}
			result.Created++
		case err != nil:
			fail(c, http.StatusInternalServerError, "查询渠道失败")
			return
		default:
			existing.Type, existing.BaseURL = req.Type, req.BaseURL
			existing.PresetID = derefString(req.PresetID, existing.PresetID)
			existing.Priority, existing.Weight = derefInt(req.Priority, existing.Priority), derefInt(req.Weight, existing.Weight)
			existing.Models, existing.ModelMapping, existing.Timeouts = req.Models, req.ModelMapping, req.Timeouts
			existing.SupportsResponses = derefBool(req.SupportsResponses, existing.SupportsResponses)
			existing.SupportsAnthropic = derefBool(req.SupportsAnthropic, existing.SupportsAnthropic)
			existing.SupportsGemini = derefBool(req.SupportsGemini, existing.SupportsGemini)
			if err := a.opt.DB.Save(&existing).Error; err != nil {
				fail(c, http.StatusInternalServerError, "更新渠道失败")
				return
			}
			result.Updated++
		}
	}
	// 降级链：整体替换（与管理台保存语义一致）。
	if in.ModelFallbacks != nil {
		if err := a.opt.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("1 = 1").Delete(&store.ModelFallback{}).Error; err != nil {
				return err
			}
			if len(fallbackRows) == 0 {
				return nil
			}
			return tx.Create(&fallbackRows).Error
		}); err != nil {
			fail(c, http.StatusInternalServerError, "导入降级链失败："+err.Error())
			return
		}
		result.Updated += int64(len(fallbackRows))
	}
	// 设置：复用管理台同一校验与热更新路径。
	if in.Settings != nil {
		if _, err := a.opt.Settings.UpdateValues(*in.Settings); err != nil {
			fail(c, http.StatusBadRequest, "settings: "+err.Error())
			return
		}
		result.Updated++
	}
	// 价格：渠道名 → 本实例 id 重映射；找不到渠道的覆盖跳过。
	if a.opt.Usage != nil {
		var channels []*store.Channel
		if err := a.opt.DB.Find(&channels).Error; err != nil {
			fail(c, http.StatusInternalServerError, "查询渠道失败")
			return
		}
		idByName := make(map[string]int64, len(channels))
		for _, ch := range channels {
			idByName[ch.Name] = ch.ID
		}
		ctx := c.Request.Context()
		if in.PriceDefault != nil {
			if err := a.opt.Usage.SetDefault(ctx, in.PriceDefault.priceInput()); err != nil {
				fail(c, http.StatusBadRequest, "price_default: "+err.Error())
				return
			}
			result.Updated++
		}
		for _, p := range in.PriceOverrides {
			id, found := idByName[strings.TrimSpace(p.ChannelName)]
			if !found {
				result.Skipped++
				result.Errors = append(result.Errors, "price override skipped: channel not found: "+p.ChannelName)
				continue
			}
			if _, err := a.opt.Usage.UpsertOverride(ctx, id, p.Model, p.priceInput()); err != nil {
				fail(c, http.StatusBadRequest, "price_overrides: "+err.Error())
				return
			}
			result.Updated++
		}
	} else if in.PriceDefault != nil || len(in.PriceOverrides) > 0 {
		result.Skipped += int64(len(in.PriceOverrides))
		if in.PriceDefault != nil {
			result.Skipped++
		}
		result.Errors = append(result.Errors, "prices skipped: usage pricing not initialised")
	}
	// 令牌：只恢复治理字段。导出不含明文 key，导入时以随机不可知 key 建为禁用，用户需在管理台重新创建令牌。
	for _, t := range in.Tokens {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			result.Skipped++
			result.Errors = append(result.Errors, "token skipped: empty name")
			continue
		}
		var count int64
		if err := a.opt.DB.Model(&store.Token{}).Where("name = ?", name).Count(&count).Error; err != nil {
			fail(c, http.StatusInternalServerError, "查询令牌失败")
			return
		}
		if count > 0 {
			result.Skipped++
			continue
		}
		allowedModels, allowedIPs, err := normalizeTokenRules(t.AllowedModels, t.AllowedIPs)
		if err != nil {
			fail(c, http.StatusBadRequest, fmt.Sprintf("tokens[%s]: %v", name, err))
			return
		}
		if t.Quota < 0 {
			fail(c, http.StatusBadRequest, fmt.Sprintf("tokens[%s]: 额度不能为负数", name))
			return
		}
		raw := make([]byte, 24)
		if _, err := rand.Read(raw); err != nil {
			fail(c, http.StatusInternalServerError, "生成失败")
			return
		}
		sum := sha256.Sum256(raw)
		token := store.Token{
			Name: name, KeyHash: hex.EncodeToString(sum[:]), Enabled: false, Quota: t.Quota,
			AllowedModels: allowedModels, AllowedIPs: allowedIPs, DisableFallback: t.DisableFallback, Group: "default",
		}
		if t.ExpiredAt != nil {
			if parsed, err := time.Parse(time.RFC3339, *t.ExpiredAt); err == nil {
				utc := parsed.UTC()
				token.ExpiredAt = &utc
			}
		}
		if err := a.opt.DB.Create(&token).Error; err != nil {
			fail(c, http.StatusInternalServerError, "导入令牌失败："+err.Error())
			return
		}
		// Enabled=false 是零值，GORM Create 会回退到列默认值 true；导入令牌必须以禁用态落地，单独强制写一次。
		if err := a.opt.DB.Model(&token).Update("enabled", false).Error; err != nil {
			fail(c, http.StatusInternalServerError, "导入令牌失败："+err.Error())
			return
		}
		token.Enabled = false
		result.Created++
		result.Errors = append(result.Errors, "token imported disabled without key; create a new token in the console: "+name)
	}
	a.reloadPrices()
	ok(c, gin.H{"result": result})
}

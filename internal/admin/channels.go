package admin

// 渠道：CRUD、启停、连通测试、上游模型预览、目录与预设价格。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"evergate/internal/config"
	"evergate/internal/health"
	"evergate/internal/provider"
	"evergate/internal/router"
	"evergate/internal/store"
	"evergate/internal/usage"
)

func (a *API) toChannelDTO(ch *store.Channel, keyCount, availableKeyCount int64, status string) channelDTO {
	return channelDTO{
		ID: ch.ID, Name: ch.Name, Type: ch.Type, BaseURL: ch.BaseURL,
		PresetID: ch.PresetID,
		Priority: ch.Priority, Weight: ch.Weight, Models: ch.Models, ModelMapping: ch.ModelMapping,
		Enabled: ch.Enabled, Status: status, SupportsResponses: ch.SupportsResponses,
		SupportsAnthropic: ch.SupportsAnthropic, SupportsGemini: ch.SupportsGemini,
		AvailableKeyCount: availableKeyCount,
		LastProbeAt:       ch.LastProbeAt, LastProbeResult: ch.LastProbeResult,
		LastProbeStatusCode: ch.LastProbeStatusCode, LastProbeLatencyMS: ch.LastProbeLatencyMS,
		LastProbeErrorType: ch.LastProbeErrorType, LastProbeError: ch.LastProbeError,
		NextProbeAt: ch.NextProbeAt,
		Timeouts:    ch.Timeouts, KeyCount: keyCount, CatalogDiff: ch.CatalogDiff,
		CreatedAt: ch.CreatedAt, UpdatedAt: ch.UpdatedAt,
	}
}

// syncChannelCatalog 立即拉取一次上游目录并返回差异；只更新提示信息，不改写渠道模型列表。
func (a *API) syncChannelCatalog(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	if a.opt.Pool == nil {
		fail(c, http.StatusServiceUnavailable, "catalog_sync_unavailable", "目录同步未启用")
		return
	}
	timeout := 10 * time.Second
	if a.opt.Settings != nil {
		timeout = time.Duration(a.opt.Settings.Snapshot().HealthProbeTimeoutMS) * time.Millisecond
	}
	diff, err := health.SyncChannelCatalog(c.Request.Context(), health.CatalogSyncOptions{
		DB: a.opt.DB, MasterKey: a.opt.MasterKey, Pool: a.opt.Pool, Client: a.opt.ModelListClient,
		Now: a.now, Timeout: func() time.Duration { return timeout }, Logger: a.opt.Logger,
	}, ch)
	if err != nil {
		switch {
		case errors.Is(err, health.ErrNoKeyForCatalog):
			fail(c, http.StatusBadRequest, "channel_no_enabled_key", "渠道没有可用的 key，无法拉取目录")
		case errors.Is(err, context.DeadlineExceeded):
			fail(c, http.StatusGatewayTimeout, "models_fetch_timeout", "拉取模型目录超时")
		default:
			fail(c, http.StatusBadGateway, "models_fetch_failed", "拉取模型目录失败："+err.Error())
		}
		return
	}
	ok(c, gin.H{"catalog_diff": diff})
}

func (a *API) listChannels(c *gin.Context) {
	var channels []*store.Channel
	if err := a.opt.DB.Order("priority asc, id asc").Find(&channels).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	var keys []*store.ChannelKey
	if err := a.opt.DB.Order("id asc").Find(&keys).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	keysByChannel := make(map[int64][]*store.ChannelKey)
	for _, key := range keys {
		keysByChannel[key.ChannelID] = append(keysByChannel[key.ChannelID], key)
	}
	out := make([]channelDTO, 0, len(channels))
	for _, ch := range channels {
		channelKeys := keysByChannel[ch.ID]
		status := health.DeriveStatus(ch, channelKeys, a.opt.Pool.StateMachine().State)
		out = append(out, a.toChannelDTO(ch, int64(len(channelKeys)), int64(health.AvailableKeyCount(channelKeys, a.opt.Pool.StateMachine().State)), status))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if health.StatusRank(out[i].Status) != health.StatusRank(out[j].Status) {
			return health.StatusRank(out[i].Status) < health.StatusRank(out[j].Status)
		}
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	ok(c, gin.H{"items": out})
}

func (r *channelReq) normalize() error {
	r.Name = strings.TrimSpace(r.Name)
	r.Type = strings.TrimSpace(r.Type)
	if r.PresetID != nil {
		*r.PresetID = strings.TrimSpace(*r.PresetID)
	}
	r.BaseURL = strings.TrimSpace(strings.TrimSuffix(r.BaseURL, "/"))
	if r.Name == "" || r.Type == "" {
		return errors.New("name 和 type 不能为空")
	}
	parsed, err := url.Parse(r.BaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("base_url 必须是 http(s) 地址")
	}
	if priority := derefInt(r.Priority, 0); priority < -1000 || priority > 1000 {
		return errors.New("priority 必须在 -1000-1000")
	}
	if weight := derefInt(r.Weight, 1); weight < 1 || weight > 10000 {
		return errors.New("weight 必须在 1-10000")
	}
	if len(r.Models) > 500 {
		return errors.New("models 最多 500 个")
	}
	models := make([]string, 0, len(r.Models))
	modelSet := make(map[string]bool, len(r.Models))
	for i, model := range r.Models {
		model = strings.TrimSpace(model)
		if model == "" || len(model) > 200 {
			return fmt.Errorf("models[%d] 必须非空且不超过 200 字符", i)
		}
		if modelSet[model] {
			continue
		}
		modelSet[model] = true
		models = append(models, model)
	}
	// 允许空模型列表：保存并导入 key 后再「从上游拉取」模型目录（T89）
	if len(r.ModelMapping) > 500 {
		return errors.New("model_mapping 最多 500 项")
	}
	mapping := make(map[string]string, len(r.ModelMapping))
	for publicModel, upstreamModel := range r.ModelMapping {
		publicModel = strings.TrimSpace(publicModel)
		upstreamModel = strings.TrimSpace(upstreamModel)
		if publicModel == "" || upstreamModel == "" || len(publicModel) > 200 || len(upstreamModel) > 200 {
			return errors.New("model_mapping 的键和值必须非空且不超过 200 字符")
		}
		if !modelSet[publicModel] {
			return fmt.Errorf("model_mapping 的公开模型 %q 不在 models 中", publicModel)
		}
		mapping[publicModel] = upstreamModel
	}
	if err := validateChannelTimeouts(r.Timeouts); err != nil {
		return err
	}
	r.Models = models
	r.ModelMapping = mapping
	return nil
}

func validateChannelTimeouts(t store.ChannelTimeouts) error {
	connect, first, overall := 0, 0, 0
	if t.ConnectSec != nil {
		connect = *t.ConnectSec
		if connect < 1 || connect > 300 {
			return errors.New("timeout_config.connect_sec 必须在 1-300")
		}
	}
	if t.FirstTokenSec != nil {
		first = *t.FirstTokenSec
		if first < 1 || first > 3600 {
			return errors.New("timeout_config.first_token_sec 必须在 1-3600")
		}
	}
	if t.OverallSec != nil {
		overall = *t.OverallSec
		if overall < 1 || overall > 86400 {
			return errors.New("timeout_config.overall_sec 必须在 1-86400")
		}
		if (connect > 0 && overall < connect) || (first > 0 && overall < first) {
			return errors.New("timeout_config.overall_sec 不能小于 connect_sec/first_token_sec")
		}
	}
	return nil
}

func (a *API) validateEffectiveChannelTimeouts(t store.ChannelTimeouts) error {
	defaults := config.Default().Defaults
	if a.opt.Settings != nil {
		defaults = a.opt.Settings.Snapshot().ConfigDefaults()
	}
	connect := defaults.ConnectTimeout.Duration
	first := defaults.FirstTokenTimeout.Duration
	overall := defaults.OverallTimeout.Duration
	if t.ConnectSec != nil {
		connect = time.Duration(*t.ConnectSec) * time.Second
	}
	if t.FirstTokenSec != nil {
		first = time.Duration(*t.FirstTokenSec) * time.Second
	}
	if t.OverallSec != nil {
		overall = time.Duration(*t.OverallSec) * time.Second
	}
	if overall < connect || overall < first {
		return errors.New("timeout_config.overall_sec 不能小于有效 connect/first_token 超时")
	}
	return nil
}

func (a *API) createChannel(c *gin.Context) {
	var req channelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误："+err.Error())
		return
	}
	if err := req.normalize(); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	if err := a.validateEffectiveChannelTimeouts(req.Timeouts); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	ch := &store.Channel{
		Name: req.Name, Type: req.Type, BaseURL: req.BaseURL,
		PresetID: derefString(req.PresetID, ""),
		Priority: derefInt(req.Priority, 0), Weight: derefInt(req.Weight, 1),
		Models: req.Models, ModelMapping: req.ModelMapping,
		Enabled: true, Status: store.ChannelStatusActive, SupportsResponses: derefBool(req.SupportsResponses, false),
		SupportsAnthropic: derefBool(req.SupportsAnthropic, false),
		SupportsGemini:    derefBool(req.SupportsGemini, false),
		Timeouts:          req.Timeouts,
	}
	if err := a.opt.DB.Create(ch).Error; err != nil {
		fail(c, http.StatusInternalServerError, "channel_create_failed", "创建失败（名称重复？）")
		return
	}
	a.reloadPrices()
	ok(c, a.toChannelDTO(ch, 0, 0, store.ChannelStatusActive))
}

func (a *API) getChannel(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var keys []*store.ChannelKey
	if err := a.opt.DB.Where("channel_id = ?", ch.ID).Order("id asc").Find(&keys).Error; err != nil {
		fail(c, http.StatusInternalServerError, "key_query_failed", "查询 key 失败")
		return
	}
	keyItems := make([]gin.H, 0, len(keys))
	stateFn := a.opt.Pool.StateMachine().State
	for _, k := range keys {
		remaining := a.opt.Pool.StateMachine().CooldownRemaining(k.ID)
		keyItems = append(keyItems, gin.H{
			"id": k.ID, "enabled": k.Enabled, "weight": k.Weight, "state": stateFn(k.ID),
			"consecutive_fails": k.ConsecutiveFails, "level": k.Level,
			"success_count": k.SuccessCount, "fail_count": k.FailCount,
			"last_error": k.LastError, "cooldown_until": k.CooldownUntil,
			"cooldown_remaining_sec": int64(remaining / time.Second),
			"rpm_limit":              k.RPMLimit, "last_used_at": k.LastUsedAt,
			// 红线：绝不返回 key_cipher / 任何明文（验收 #9）
		})
	}
	status := health.DeriveStatus(ch, keys, a.opt.Pool.StateMachine().State)
	dto := a.toChannelDTO(ch, int64(len(keys)), int64(health.AvailableKeyCount(keys, a.opt.Pool.StateMachine().State)), status)
	ok(c, gin.H{"channel": dto, "keys": keyItems, "prices": a.effectivePrices(ch)})
}

// upstreamModels 把对外模型名换成供应商实际计费的上游模型名（无映射时同名）。
func upstreamModels(ch *store.Channel) []string {
	out := make([]string, 0, len(ch.Models))
	for _, model := range ch.Models {
		if upstream := ch.ModelMapping[model]; upstream != "" {
			out = append(out, upstream)
		} else {
			out = append(out, model)
		}
	}
	return out
}

func (a *API) effectivePrices(ch *store.Channel) []usage.EffectivePrice {
	if a.opt.Usage == nil {
		return nil
	}
	return a.opt.Usage.Effective(ch.ID, upstreamModels(ch))
}

// reloadPrices 渠道的 preset_id 决定内置价格目录能否命中，渠道增删改后刷新计价快照。
func (a *API) reloadPrices() {
	if a.opt.Usage != nil {
		_ = a.opt.Usage.Reload()
	}
}

// applyPresetPrice 把渠道预设目录里的价格一键写成该渠道的模型覆盖（只补缺不覆盖）。
func (a *API) applyPresetPrice(c *gin.Context) {
	if a.opt.Usage == nil {
		fail(c, http.StatusServiceUnavailable, "pricing_not_ready", "计价配置未初始化")
		return
	}
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	if ch.PresetID == "" {
		fail(c, http.StatusBadRequest, "channel_not_preset", "该渠道不是预设渠道，没有可用的预设价格")
		return
	}
	created, err := a.opt.Usage.ApplyPreset(c.Request.Context(), ch.ID, ch.PresetID, upstreamModels(ch))
	if err != nil {
		fail(c, http.StatusInternalServerError, "preset_price_write_failed", "写入预设价格失败")
		return
	}
	ok(c, gin.H{"created": created, "prices": a.effectivePrices(ch)})
}

func (a *API) updateChannel(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var req channelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误："+err.Error())
		return
	}
	if err := req.normalize(); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	if err := a.validateEffectiveChannelTimeouts(req.Timeouts); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", err.Error())
		return
	}
	ch.Name, ch.Type, ch.BaseURL = req.Name, req.Type, req.BaseURL
	ch.PresetID = derefString(req.PresetID, "")
	ch.Priority, ch.Weight = derefInt(req.Priority, ch.Priority), derefInt(req.Weight, ch.Weight)
	ch.Models, ch.ModelMapping, ch.Timeouts = req.Models, req.ModelMapping, req.Timeouts
	ch.SupportsResponses = derefBool(req.SupportsResponses, ch.SupportsResponses)
	ch.SupportsAnthropic = derefBool(req.SupportsAnthropic, ch.SupportsAnthropic)
	ch.SupportsGemini = derefBool(req.SupportsGemini, ch.SupportsGemini)
	if err := a.opt.DB.Save(ch).Error; err != nil {
		fail(c, http.StatusInternalServerError, "update_failed", "更新失败")
		return
	}
	a.reloadPrices()
	var keys []*store.ChannelKey
	if err := a.opt.DB.Where("channel_id = ?", ch.ID).Find(&keys).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	status := health.DeriveStatus(ch, keys, a.opt.Pool.StateMachine().State)
	ok(c, a.toChannelDTO(ch, int64(len(keys)), int64(health.AvailableKeyCount(keys, a.opt.Pool.StateMachine().State)), status))
}

func (a *API) deleteChannel(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var keyIDs []int64
	if err := a.opt.DB.Model(&store.ChannelKey{}).Where("channel_id = ?", ch.ID).Pluck("id", &keyIDs).Error; err != nil {
		fail(c, http.StatusInternalServerError, "delete_failed", "删除失败")
		return
	}
	if err := a.opt.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id = ?", ch.ID).Delete(&store.ChannelKey{}).Error; err != nil {
			return err
		}
		return tx.Delete(ch).Error
	}); err != nil {
		fail(c, http.StatusInternalServerError, "delete_failed", "删除失败")
		return
	}
	// 渠道连同 key 一起删除：同步回收状态机里这些 key 的内存条目
	if a.opt.Pool != nil {
		for _, keyID := range keyIDs {
			a.opt.Pool.ForgetKey(keyID)
		}
	}
	a.reloadPrices()
	ok(c, gin.H{"deleted": true})
}

func (a *API) toggleChannel(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid_param", "参数错误")
		return
	}
	ch.Enabled = derefBool(req.Enabled, !ch.Enabled)
	if err := a.opt.DB.Model(ch).Update("enabled", ch.Enabled).Error; err != nil {
		fail(c, http.StatusInternalServerError, "update_failed", "更新失败")
		return
	}
	ok(c, gin.H{"enabled": ch.Enabled})
}

// testChannel 手动连通测试：与后台健康探测共用协议化模型列表 GET，
// 结果同样驱动 half_open/cooldown 状态机并持久化渠道健康摘要。
func (a *API) testChannel(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	if a.opt.Health == nil {
		fail(c, http.StatusInternalServerError, "prober_not_ready", "健康探测器未初始化")
		return
	}
	var keys []*store.ChannelKey
	if err := a.opt.DB.Where("channel_id = ? AND enabled = ?", ch.ID, true).Order("id asc").Find(&keys).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	timeout, nextInterval := 10*time.Second, 5*time.Minute
	if a.opt.Settings != nil {
		current := a.opt.Settings.Snapshot()
		timeout = time.Duration(current.HealthProbeTimeoutMS) * time.Millisecond
		nextInterval = time.Duration(current.HealthProbeIntervalMS) * time.Millisecond
	}
	result, err := a.opt.Health.ProbeChannel(
		c.Request.Context(), ch, keys, timeout, nextInterval,
	)
	if err != nil {
		if errors.Is(err, health.ErrNoAvailableKey) {
			fail(c, http.StatusBadRequest, "channel_no_key", "渠道没有可用 key")
			return
		}
		if errors.Is(err, health.ErrChannelDisabled) {
			fail(c, http.StatusBadRequest, "channel_disabled", "渠道已停用")
			return
		}
		if errors.Is(err, context.Canceled) {
			fail(c, http.StatusRequestTimeout, "test_cancelled", "测试请求已取消")
			return
		}
		fail(c, http.StatusInternalServerError, "test_failed", "测试失败")
		return
	}
	ok(c, gin.H{
		"ok": result.Result == health.ProbeResultSuccess, "channel_id": result.ChannelID,
		"key_id": result.KeyID, "result": result.Result, "status": result.Status,
		"status_code": result.StatusCode, "latency_ms": result.LatencyMS,
		"error_type": result.ErrorType, "error": result.Error,
		"probed_at": result.ProbedAt, "next_probe_at": result.NextProbeAt,
	})
}

// previewChannelModels 拉取渠道上游模型目录并按现有映射反查对外别名。
// 该操作只读取一把启用 key，不经过 pool，不改变 key 调度状态。
func (a *API) previewChannelModels(c *gin.Context) {
	ch, found := a.loadChannel(c)
	if !found {
		return
	}
	var key store.ChannelKey
	if err := a.opt.DB.Where("channel_id = ? AND enabled = ?", ch.ID, true).
		Order("id asc").First(&key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			fail(c, http.StatusBadRequest, "channel_no_enabled_key", "渠道没有启用的 key，请先导入 key")
			return
		}
		fail(c, http.StatusInternalServerError, "key_query_failed", "查询 key 失败")
		return
	}
	plainKey, err := store.Decrypt(key.KeyCipher, a.opt.MasterKey)
	if err != nil {
		fail(c, http.StatusInternalServerError, "key_decrypt_failed", "解密上游 key 失败")
		return
	}

	timeout := 10 * time.Second
	if a.opt.Settings != nil {
		timeout = time.Duration(a.opt.Settings.Snapshot().HealthProbeTimeoutMS) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()
	result, err := provider.ListModels(ctx, a.opt.ModelListClient, ch, plainKey)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			fail(c, http.StatusGatewayTimeout, "models_fetch_timeout", "拉取模型目录超时")
		case errors.Is(err, context.Canceled):
			fail(c, http.StatusRequestTimeout, "models_fetch_cancelled", "拉取模型目录已取消")
		default:
			fail(c, http.StatusBadGateway, "models_fetch_failed", "拉取模型目录失败："+err.Error())
		}
		return
	}

	aliasesByUpstream := make(map[string][]string)
	for publicModel, upstreamModel := range ch.ModelMapping {
		if publicModel == "" || upstreamModel == "" {
			continue
		}
		aliasesByUpstream[upstreamModel] = append(aliasesByUpstream[upstreamModel], publicModel)
	}
	items := make([]modelPreviewItem, 0, len(result.Models))
	for _, upstreamModel := range result.Models {
		aliases := aliasesByUpstream[upstreamModel]
		var hint *provider.ModelPriceHint
		if price, ok := result.Prices[upstreamModel]; ok {
			hint = &price
		}
		if len(aliases) == 0 {
			items = append(items, modelPreviewItem{PublicModel: upstreamModel, UpstreamModel: upstreamModel, PriceMicros: hint})
			continue
		}
		sort.Strings(aliases)
		for _, publicModel := range aliases {
			items = append(items, modelPreviewItem{PublicModel: publicModel, UpstreamModel: upstreamModel, PriceMicros: hint})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].PublicModel != items[j].PublicModel {
			return items[i].PublicModel < items[j].PublicModel
		}
		return items[i].UpstreamModel < items[j].UpstreamModel
	})
	ok(c, gin.H{
		"items": items, "protocol": result.Protocol,
		"pages": result.Pages, "truncated": result.Truncated,
	})
}

// modelCatalog 为令牌模型选择器提供启用渠道 Models 并集；与下游 /v1/models 共用归一化逻辑。
func (a *API) modelCatalog(c *gin.Context) {
	var channels []*store.Channel
	if err := a.opt.DB.Where("enabled = ?", true).Order("id asc").Find(&channels).Error; err != nil {
		fail(c, http.StatusInternalServerError, "catalog_query_failed", "查询模型目录失败")
		return
	}
	models := router.PublicModelCatalog(channels)
	items := make([]catalogModel, 0, len(models))
	for _, model := range models {
		item := catalogModel{Model: model, Channels: []catalogChannelRef{}}
		for _, ch := range channels {
			if !router.SupportsModel(ch, model) {
				continue
			}
			item.Channels = append(item.Channels, catalogChannelRef{
				ChannelID: ch.ID, ChannelName: ch.Name,
				UpstreamModel: router.UpstreamModel(ch.ModelMapping, model),
			})
		}
		items = append(items, item)
	}
	ok(c, gin.H{"items": items})
}

func (a *API) loadChannel(c *gin.Context) (*store.Channel, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_id", "非法 id")
		return nil, false
	}
	var ch store.Channel
	if err := a.opt.DB.First(&ch, id).Error; err != nil {
		fail(c, http.StatusNotFound, "channel_not_found", "渠道不存在")
		return nil, false
	}
	return &ch, true
}

// ---------- 凭据 ----------

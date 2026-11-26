// Package usage 负责模型价格、请求成本、UTC 日聚合与报表查询。
package usage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"gorm.io/gorm"

	"evergate/internal/store"
)

var ErrPriceOverrideNotFound = errors.New("价格覆盖不存在")

type priceKey struct {
	ChannelID int64
	Model     string
}

const (
	CostStatusPriced   = "priced"
	CostStatusUnpriced = "unpriced"
	CostStatusNoUsage  = "no_usage"
)

type priceSnapshot struct {
	global    *store.ModelPrice
	overrides map[priceKey]store.ModelPrice
	presets   map[int64]string // channel id → preset_id，用于内置目录兜底
}

// Manager 持有价格快照；价格 CRUD 成功后立即重载，请求路径无锁读取。
type Manager struct {
	db      *gorm.DB
	mu      sync.Mutex
	current atomic.Pointer[priceSnapshot]
}

// 价格来源（写入请求日志 price_source，管理台据此标注"预设价格"等）。
const (
	PriceSourceOverride = "override"
	PriceSourceDefault  = "default"
	PriceSourceCatalog  = "catalog"
)

// EffectivePrice 某渠道某模型当前生效的价格及其来源（管理台展示用）。
type EffectivePrice struct {
	Model                      string `json:"model"`
	InputMicrosPerMillion      int64  `json:"input_micros_per_million"`
	OutputMicrosPerMillion     int64  `json:"output_micros_per_million"`
	CacheReadMicrosPerMillion  int64  `json:"cache_read_micros_per_million"`
	CacheWriteMicrosPerMillion int64  `json:"cache_write_micros_per_million"`
	Source                     string `json:"source"` // override / default / catalog / ""（未计价）
	SourceURL                  string `json:"source_url,omitempty"`
}

// Tokens 一次请求的可计费用量。Input 是输入侧总量（普通输入 + 缓存读 + 缓存写），
// 各协议 usage 的字段差异在 relay 归一化时抹平，计价只认这一种形态。
type Tokens struct {
	Input, CacheRead, CacheWrite, Output int64
}

// PriceInput 一组待写入的价格（微美元 / 百万 token）。
// 缓存价为 nil 表示本次未提供：新建落 0，更新保留原值。
type PriceInput struct {
	Input, Output         int64
	CacheRead, CacheWrite *int64
}

func (p PriceInput) validate() error {
	if p.Input < 0 || p.Output < 0 {
		return errors.New("价格不能为负数")
	}
	if (p.CacheRead != nil && *p.CacheRead < 0) || (p.CacheWrite != nil && *p.CacheWrite < 0) {
		return errors.New("缓存价格不能为负数")
	}
	return nil
}

// priceCostMicros 按普通输入、缓存读、缓存写、输出四类分别计价；
// 缓存价未设置（0）时按输入价全价计，宁可高估不低估。
func priceCostMicros(p store.ModelPrice, t Tokens) int64 {
	regular := t.Input - t.CacheRead - t.CacheWrite
	if regular < 0 {
		regular = 0
	}
	cacheReadPrice, cacheWritePrice := p.InputMicrosPerMillion, p.InputMicrosPerMillion
	if p.CacheReadMicrosPerMillion > 0 {
		cacheReadPrice = p.CacheReadMicrosPerMillion
	}
	if p.CacheWriteMicrosPerMillion > 0 {
		cacheWritePrice = p.CacheWriteMicrosPerMillion
	}
	return p.InputMicrosPerMillion*regular + cacheReadPrice*t.CacheRead + cacheWritePrice*t.CacheWrite +
		p.OutputMicrosPerMillion*t.Output
}

func Load(db *gorm.DB) (*Manager, error) {
	if db == nil {
		return nil, errors.New("usage db 不能为空")
	}
	m := &Manager{db: db}
	if err := m.Reload(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) Reload() error {
	if m == nil || m.db == nil {
		return errors.New("usage manager 未初始化")
	}
	var rows []store.ModelPrice
	if err := m.db.Order("channel_id asc, model asc").Find(&rows).Error; err != nil {
		return err
	}
	snapshot := &priceSnapshot{overrides: make(map[priceKey]store.ModelPrice)}
	// 渠道的 preset_id 决定能否命中内置目录；渠道增删改后必须 Reload 才会更新这张表
	var channels []store.Channel
	if err := m.db.Select("id", "preset_id").Find(&channels).Error; err != nil {
		return err
	}
	snapshot.presets = make(map[int64]string, len(channels))
	for _, ch := range channels {
		if ch.PresetID != "" {
			snapshot.presets[ch.ID] = ch.PresetID
		}
	}
	for i := range rows {
		row := rows[i]
		if row.ChannelID == 0 && row.Model == "" {
			snapshot.global = &row
			continue
		}
		snapshot.overrides[priceKey{ChannelID: row.ChannelID, Model: row.Model}] = row
	}
	m.current.Store(snapshot)
	return nil
}

// CalculateWithSource 按回退链计价：渠道+模型覆盖 → 全局默认 → 预设内置目录；返回来源便于日志标注。
func (m *Manager) CalculateWithSource(channelID int64, upstreamModel string, t Tokens) (int64, string) {
	if m == nil || t.Input+t.Output <= 0 {
		return 0, ""
	}
	snapshot := m.current.Load()
	if snapshot == nil {
		return 0, ""
	}
	price := snapshot.global
	source := PriceSourceDefault
	if override, ok := snapshot.overrides[priceKey{ChannelID: channelID, Model: upstreamModel}]; ok {
		price = &override
		source = PriceSourceOverride
	}
	if price == nil {
		if entry, ok := catalogLookup(snapshot.presets[channelID], upstreamModel); ok {
			resolved := entry.price()
			price = &resolved
			source = PriceSourceCatalog
		}
	}
	if price == nil {
		return 0, ""
	}
	total := priceCostMicros(*price, t)
	return (total + 500_000) / 1_000_000, source
}

// Effective 列出某渠道各模型的生效价格（覆盖 → 全局 → 预设目录 → 未计价）。
func (m *Manager) Effective(channelID int64, models []string) []EffectivePrice {
	out := make([]EffectivePrice, 0, len(models))
	if m == nil {
		return out
	}
	snapshot := m.current.Load()
	if snapshot == nil {
		return out
	}
	for _, model := range models {
		if model == "" {
			continue
		}
		item := EffectivePrice{Model: model}
		key := priceKey{ChannelID: channelID, Model: model}
		if row, ok := snapshot.overrides[key]; ok {
			item.InputMicrosPerMillion, item.OutputMicrosPerMillion = row.InputMicrosPerMillion, row.OutputMicrosPerMillion
			item.CacheReadMicrosPerMillion, item.CacheWriteMicrosPerMillion = row.CacheReadMicrosPerMillion, row.CacheWriteMicrosPerMillion
			item.Source = PriceSourceOverride
		} else if snapshot.global != nil {
			item.InputMicrosPerMillion, item.OutputMicrosPerMillion = snapshot.global.InputMicrosPerMillion, snapshot.global.OutputMicrosPerMillion
			item.CacheReadMicrosPerMillion, item.CacheWriteMicrosPerMillion = snapshot.global.CacheReadMicrosPerMillion, snapshot.global.CacheWriteMicrosPerMillion
			item.Source = PriceSourceDefault
		} else {
			if entry, ok := catalogLookup(snapshot.presets[channelID], model); ok {
				entryPrice := entry.price()
				item.InputMicrosPerMillion, item.OutputMicrosPerMillion = entryPrice.InputMicrosPerMillion, entryPrice.OutputMicrosPerMillion
				item.CacheReadMicrosPerMillion, item.CacheWriteMicrosPerMillion = entryPrice.CacheReadMicrosPerMillion, entryPrice.CacheWriteMicrosPerMillion
				item.Source, item.SourceURL = PriceSourceCatalog, entry.SourceURL
			}
		}
		out = append(out, item)
	}
	return out
}

// ApplyPreset 把渠道预设目录中、该渠道模型的未覆盖条目写成渠道+模型覆盖；返回新建条数。
// 只补缺不覆盖：用户已经手填的价格优先于目录。
func (m *Manager) ApplyPreset(ctx context.Context, channelID int64, presetID string, models []string) (int, error) {
	created := 0
	for _, model := range models {
		entry, ok := catalogLookup(presetID, model)
		if !ok {
			continue
		}
		var count int64
		if err := m.db.WithContext(ctx).Model(&store.ModelPrice{}).Where("channel_id = ? AND model = ?", channelID, model).Count(&count).Error; err != nil {
			return created, err
		}
		if count > 0 {
			continue
		}
		if _, err := m.UpsertOverride(ctx, channelID, model, entry.priceInput()); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

func (m *Manager) List(ctx context.Context) (*store.ModelPrice, []store.ModelPrice, error) {
	var rows []store.ModelPrice
	if err := m.db.WithContext(ctx).Order("channel_id asc, model asc").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	var global *store.ModelPrice
	overrides := make([]store.ModelPrice, 0, len(rows))
	for i := range rows {
		row := rows[i]
		if row.ChannelID == 0 && row.Model == "" {
			global = &row
			continue
		}
		overrides = append(overrides, row)
	}
	return global, overrides, nil
}

func (m *Manager) SetDefault(ctx context.Context, price PriceInput) error {
	if err := price.validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := upsertPrice(m.db.WithContext(ctx), 0, "", price); err != nil {
		return err
	}
	return m.Reload()
}

func (m *Manager) UpsertOverride(ctx context.Context, channelID int64, model string, price PriceInput) (store.ModelPrice, error) {
	if channelID <= 0 || model == "" {
		return store.ModelPrice{}, errors.New("渠道+模型覆盖必须指定 channel_id 与 model")
	}
	if err := price.validate(); err != nil {
		return store.ModelPrice{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	row, err := upsertPrice(m.db.WithContext(ctx), channelID, model, price)
	if err != nil {
		return store.ModelPrice{}, err
	}
	if err := m.Reload(); err != nil {
		return store.ModelPrice{}, err
	}
	return row, nil
}

// ValidatePrice 校验一组价格；配置导入在开启事务前复用同一口径。
func ValidatePrice(price PriceInput) error {
	return price.validate()
}

// ImportPrice 是配置导入的一条渠道价格覆盖。
type ImportPrice struct {
	ChannelID int64
	Model     string
	Price     PriceInput
}

// ApplyImport 在调用方事务内写入默认价与覆盖价，不刷新快照；提交成功后由调用方 Reload。
// 任一价格非法或写入失败都返回错误，调用方事务整体回滚。
func (m *Manager) ApplyImport(tx *gorm.DB, def *PriceInput, overrides []ImportPrice) error {
	if def != nil {
		if err := def.validate(); err != nil {
			return err
		}
		if _, err := upsertPrice(tx, 0, "", *def); err != nil {
			return err
		}
	}
	for _, o := range overrides {
		if err := o.Price.validate(); err != nil {
			return fmt.Errorf("%s: %w", o.Model, err)
		}
		if _, err := upsertPrice(tx, o.ChannelID, o.Model, o.Price); err != nil {
			return err
		}
	}
	return nil
}

// upsertPrice 按 (channel_id, model) 收敛写入路径；缓存价为 nil 时保留原值。
func upsertPrice(db *gorm.DB, channelID int64, model string, price PriceInput) (store.ModelPrice, error) {
	var row store.ModelPrice
	result := db.Where("channel_id = ? AND model = ?", channelID, model).First(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		row = store.ModelPrice{
			ChannelID: channelID, Model: model,
			InputMicrosPerMillion: price.Input, OutputMicrosPerMillion: price.Output,
			CacheReadMicrosPerMillion: derefInt64(price.CacheRead), CacheWriteMicrosPerMillion: derefInt64(price.CacheWrite),
		}
		if err := db.Create(&row).Error; err != nil {
			return store.ModelPrice{}, err
		}
	} else if result.Error != nil {
		return store.ModelPrice{}, result.Error
	} else {
		if err := db.Model(&row).Updates(price.updates()).Error; err != nil {
			return store.ModelPrice{}, err
		}
		price.applyTo(&row)
	}
	return row, nil
}

// updates 只写本次提供的列：缓存价为 nil 时不触碰原值。
func (p PriceInput) updates() map[string]any {
	updates := map[string]any{"input_micros_per_million": p.Input, "output_micros_per_million": p.Output}
	if p.CacheRead != nil {
		updates["cache_read_micros_per_million"] = *p.CacheRead
	}
	if p.CacheWrite != nil {
		updates["cache_write_micros_per_million"] = *p.CacheWrite
	}
	return updates
}

func (p PriceInput) applyTo(row *store.ModelPrice) {
	row.InputMicrosPerMillion, row.OutputMicrosPerMillion = p.Input, p.Output
	if p.CacheRead != nil {
		row.CacheReadMicrosPerMillion = *p.CacheRead
	}
	if p.CacheWrite != nil {
		row.CacheWriteMicrosPerMillion = *p.CacheWrite
	}
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func (m *Manager) DeleteOverride(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := m.db.WithContext(ctx).Where("id = ? AND NOT (channel_id = 0 AND model = '')", id).Delete(&store.ModelPrice{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrPriceOverrideNotFound
	}
	return m.Reload()
}

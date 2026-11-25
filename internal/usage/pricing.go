// Package usage 负责模型价格、请求成本、UTC 日聚合与报表查询。
package usage

import (
	"context"
	"errors"
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
	Model                  string `json:"model"`
	InputMicrosPerMillion  int64  `json:"input_micros_per_million"`
	OutputMicrosPerMillion int64  `json:"output_micros_per_million"`
	Source                 string `json:"source"` // override / default / catalog / ""（未计价）
	SourceURL              string `json:"source_url,omitempty"`
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

// Calculate 返回微美元成本；无匹配价格时 priced=false。
func (m *Manager) Calculate(channelID int64, upstreamModel string, inputTokens, outputTokens int64) (int64, bool) {
	cost, source := m.CalculateWithSource(channelID, upstreamModel, inputTokens, outputTokens)
	return cost, source != ""
}

// CalculateWithSource 按回退链计价：渠道+模型覆盖 → 全局默认 → 预设内置目录；返回来源便于日志标注。
func (m *Manager) CalculateWithSource(channelID int64, upstreamModel string, inputTokens, outputTokens int64) (int64, string) {
	if m == nil || inputTokens+outputTokens <= 0 {
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
			price = &store.ModelPrice{InputMicrosPerMillion: entry.InputMicrosPerMillion, OutputMicrosPerMillion: entry.OutputMicrosPerMillion}
			source = PriceSourceCatalog
		}
	}
	if price == nil {
		return 0, ""
	}
	total := price.InputMicrosPerMillion*inputTokens + price.OutputMicrosPerMillion*outputTokens
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
			item.InputMicrosPerMillion, item.OutputMicrosPerMillion, item.Source = row.InputMicrosPerMillion, row.OutputMicrosPerMillion, PriceSourceOverride
		} else if snapshot.global != nil {
			item.InputMicrosPerMillion, item.OutputMicrosPerMillion, item.Source = snapshot.global.InputMicrosPerMillion, snapshot.global.OutputMicrosPerMillion, PriceSourceDefault
		} else {
			if entry, ok := catalogLookup(snapshot.presets[channelID], model); ok {
				item.InputMicrosPerMillion, item.OutputMicrosPerMillion, item.Source, item.SourceURL = entry.InputMicrosPerMillion, entry.OutputMicrosPerMillion, PriceSourceCatalog, entry.SourceURL
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
		if _, err := m.UpsertOverride(ctx, channelID, model, entry.InputMicrosPerMillion, entry.OutputMicrosPerMillion); err != nil {
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

func (m *Manager) SetDefault(ctx context.Context, inputMicros, outputMicros int64) error {
	if inputMicros < 0 || outputMicros < 0 {
		return errors.New("价格不能为负数")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var row store.ModelPrice
	result := m.db.WithContext(ctx).Where("channel_id = ? AND model = ?", 0, "").First(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		row = store.ModelPrice{InputMicrosPerMillion: inputMicros, OutputMicrosPerMillion: outputMicros}
		if err := m.db.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
	} else if result.Error != nil {
		return result.Error
	} else if err := m.db.WithContext(ctx).Model(&row).Updates(map[string]any{
		"input_micros_per_million": inputMicros, "output_micros_per_million": outputMicros,
	}).Error; err != nil {
		return err
	}
	return m.Reload()
}

func (m *Manager) UpsertOverride(ctx context.Context, channelID int64, model string, inputMicros, outputMicros int64) (store.ModelPrice, error) {
	if channelID <= 0 || model == "" {
		return store.ModelPrice{}, errors.New("渠道+模型覆盖必须指定 channel_id 与 model")
	}
	if inputMicros < 0 || outputMicros < 0 {
		return store.ModelPrice{}, errors.New("价格不能为负数")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var row store.ModelPrice
	result := m.db.WithContext(ctx).Where("channel_id = ? AND model = ?", channelID, model).First(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		row = store.ModelPrice{
			ChannelID: channelID, Model: model,
			InputMicrosPerMillion: inputMicros, OutputMicrosPerMillion: outputMicros,
		}
		if err := m.db.WithContext(ctx).Create(&row).Error; err != nil {
			return store.ModelPrice{}, err
		}
	} else if result.Error != nil {
		return store.ModelPrice{}, result.Error
	} else if err := m.db.WithContext(ctx).Model(&row).Updates(map[string]any{
		"input_micros_per_million": inputMicros, "output_micros_per_million": outputMicros,
	}).Error; err != nil {
		return store.ModelPrice{}, err
	}
	row.InputMicrosPerMillion = inputMicros
	row.OutputMicrosPerMillion = outputMicros
	if err := m.Reload(); err != nil {
		return store.ModelPrice{}, err
	}
	return row, nil
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

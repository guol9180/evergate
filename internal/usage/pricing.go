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
}

// Manager 持有价格快照；价格 CRUD 成功后立即重载，请求路径无锁读取。
type Manager struct {
	db      *gorm.DB
	mu      sync.Mutex
	current atomic.Pointer[priceSnapshot]
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
	if m == nil || inputTokens+outputTokens <= 0 {
		return 0, false
	}
	snapshot := m.current.Load()
	if snapshot == nil {
		return 0, false
	}
	price := snapshot.global
	if override, ok := snapshot.overrides[priceKey{ChannelID: channelID, Model: upstreamModel}]; ok {
		price = &override
	}
	if price == nil {
		return 0, false
	}
	total := price.InputMicrosPerMillion*inputTokens + price.OutputMicrosPerMillion*outputTokens
	return (total + 500_000) / 1_000_000, true
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

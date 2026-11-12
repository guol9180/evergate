// Package settings 管理可安全热更新的运行时配置。
// evergate.yml 只负责首次种子；此后 SQLite 单例行是唯一事实源。
package settings

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"evergate/internal/config"
	"evergate/internal/store"
)

const (
	SourceYAML     = "evergate.yml"
	SourceDatabase = "database"
	settingRowID   = int64(1)
)

// Settings 是请求路径读取的无共享快照。
// Values 是可经管理台更新的数值字段；Source 与时间元数据由服务端维护。
type Values struct {
	ConnectTimeoutMS       int64 `json:"connect_timeout_ms"`
	FirstTokenTimeoutMS    int64 `json:"first_token_timeout_ms"`
	OverallTimeoutMS       int64 `json:"overall_timeout_ms"`
	RetryChannels          int   `json:"retry_channels"`
	RetryKeysPerChannel    int   `json:"retry_keys_per_channel"`
	RetryTotalMS           int64 `json:"retry_total_ms"`
	CooldownFailThreshold  int   `json:"cooldown_fail_threshold"`
	CooldownBaseMS         int64 `json:"cooldown_base_ms"`
	CooldownMaxMS          int64 `json:"cooldown_max_ms"`
	LogRetentionDays       int   `json:"log_retention_days"`
	HealthProbeIntervalMS  int64 `json:"health_probe_interval_ms"`
	HealthProbeConcurrency int   `json:"health_probe_concurrency"`
	HealthProbeTimeoutMS   int64 `json:"health_probe_timeout_ms"`
}

type Settings struct {
	Values
	Source        string    `json:"source"`
	InitializedAt time.Time `json:"initialized_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Manager 负责加载、校验与原子发布配置快照。
// 请求路径只调用 Snapshot，不访问数据库，避免热更新造成数据竞争。
type Manager struct {
	db          *gorm.DB
	now         func() time.Time
	current     atomic.Pointer[Settings]
	updateMu    sync.Mutex
	subMu       sync.Mutex
	subscribers map[chan struct{}]struct{}
}

func Default() Settings {
	cfg := config.Default()
	var initializedAt time.Time
	return FromConfig(cfg, initializedAt)
}

func FromConfig(cfg config.Config, initializedAt time.Time) Settings {
	d := cfg.Defaults
	return Settings{
		Values: Values{
			ConnectTimeoutMS:       d.ConnectTimeout.Duration.Milliseconds(),
			FirstTokenTimeoutMS:    d.FirstTokenTimeout.Duration.Milliseconds(),
			OverallTimeoutMS:       d.OverallTimeout.Duration.Milliseconds(),
			RetryChannels:          d.RetryBudget.Channels,
			RetryKeysPerChannel:    d.RetryBudget.KeysPerChannel,
			RetryTotalMS:           d.RetryBudget.Total.Duration.Milliseconds(),
			CooldownFailThreshold:  d.Cooldown.FailThreshold,
			CooldownBaseMS:         d.Cooldown.Base.Duration.Milliseconds(),
			CooldownMaxMS:          d.Cooldown.Max.Duration.Milliseconds(),
			LogRetentionDays:       cfg.Storage.LogRetentionDays,
			HealthProbeIntervalMS:  5 * time.Minute.Milliseconds(),
			HealthProbeConcurrency: 2,
			HealthProbeTimeoutMS:   10 * time.Second.Milliseconds(),
		},
		Source:        SourceYAML,
		InitializedAt: initializedAt,
	}
}

func Load(db *gorm.DB, cfg config.Config, now func() time.Time) (*Manager, error) {
	if db == nil {
		return nil, errors.New("settings db 不能为空")
	}
	if now == nil {
		now = time.Now
	}
	m := &Manager{db: db, now: now, subscribers: make(map[chan struct{}]struct{})}

	var row store.RuntimeSetting
	err := db.First(&row, "id = ?", settingRowID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		next := FromConfig(cfg, now().UTC())
		if err := next.Validate(); err != nil {
			return nil, fmt.Errorf("YAML 运行时设置非法: %w", err)
		}
		row = toRow(next)
		row.ID = settingRowID
		if err := db.Create(&row).Error; err != nil {
			return nil, fmt.Errorf("初始化运行时设置失败: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("读取运行时设置失败: %w", err)
	}

	next := fromRow(row)
	if next.Source == "" {
		next.Source = SourceDatabase
	}
	if err := next.Validate(); err != nil {
		return nil, fmt.Errorf("数据库运行时设置非法: %w", err)
	}
	m.current.Store(&next)
	return m, nil
}

func (m *Manager) Snapshot() Settings {
	if m == nil {
		return Default()
	}
	if v := m.current.Load(); v != nil {
		return *v
	}
	return Default()
}

// Update 先完整校验，再事务更新 SQLite，最后原子发布快照；校验失败不会产生部分更新。
func (m *Manager) Update(next Settings) (Settings, error) {
	m.updateMu.Lock()
	defer m.updateMu.Unlock()

	current := m.Snapshot()
	next.Source = SourceDatabase
	next.InitializedAt = current.InitializedAt
	next.UpdatedAt = m.now().UTC()
	if err := next.Validate(); err != nil {
		return current, err
	}

	row := toRow(next)
	row.ID = settingRowID
	err := m.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&store.RuntimeSetting{}).Where("id = ?", settingRowID).Updates(toUpdates(row))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		return current, fmt.Errorf("保存运行时设置失败: %w", err)
	}
	m.current.Store(&next)
	m.notify()
	return next, nil
}

// UpdateValues 供管理台只提交可编辑字段；来源与时间元数据保持服务端权威。
func (m *Manager) UpdateValues(values Values) (Settings, error) {
	next := m.Snapshot()
	next.Values = values
	return m.Update(next)
}

// Subscribe 返回设置变更信号。后台任务可据此立即重算周期；通道容量为 1，
// 发布方非阻塞，慢消费者不会拖垮管理台保存路径。
func (m *Manager) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	m.subMu.Lock()
	if m.subscribers == nil {
		m.subscribers = make(map[chan struct{}]struct{})
	}
	m.subscribers[ch] = struct{}{}
	m.subMu.Unlock()
	return ch, func() {
		m.subMu.Lock()
		delete(m.subscribers, ch)
		m.subMu.Unlock()
	}
}

func (m *Manager) notify() {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	for ch := range m.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (s Settings) ConfigDefaults() config.Defaults {
	return config.Defaults{
		ConnectTimeout:    config.Duration{Duration: time.Duration(s.ConnectTimeoutMS) * time.Millisecond},
		FirstTokenTimeout: config.Duration{Duration: time.Duration(s.FirstTokenTimeoutMS) * time.Millisecond},
		OverallTimeout:    config.Duration{Duration: time.Duration(s.OverallTimeoutMS) * time.Millisecond},
		RetryBudget: config.RetryBudget{
			Channels: s.RetryChannels, KeysPerChannel: s.RetryKeysPerChannel,
			Total: config.Duration{Duration: time.Duration(s.RetryTotalMS) * time.Millisecond},
		},
		Cooldown: config.Cooldown{
			FailThreshold: s.CooldownFailThreshold,
			Base:          config.Duration{Duration: time.Duration(s.CooldownBaseMS) * time.Millisecond},
			Max:           config.Duration{Duration: time.Duration(s.CooldownMaxMS) * time.Millisecond},
		},
	}
}

func (s Settings) Validate() error {
	connect := time.Duration(s.ConnectTimeoutMS) * time.Millisecond
	first := time.Duration(s.FirstTokenTimeoutMS) * time.Millisecond
	overall := time.Duration(s.OverallTimeoutMS) * time.Millisecond
	retryTotal := time.Duration(s.RetryTotalMS) * time.Millisecond
	cooldownBase := time.Duration(s.CooldownBaseMS) * time.Millisecond
	cooldownMax := time.Duration(s.CooldownMaxMS) * time.Millisecond
	healthInterval := time.Duration(s.HealthProbeIntervalMS) * time.Millisecond
	healthTimeout := time.Duration(s.HealthProbeTimeoutMS) * time.Millisecond

	if connect < 100*time.Millisecond || connect > 5*time.Minute {
		return fmt.Errorf("连接超时必须在 100ms-5m 之间，当前 %s", connect)
	}
	if first < 100*time.Millisecond || first > 10*time.Minute {
		return fmt.Errorf("首 token 超时必须在 100ms-10m 之间，当前 %s", first)
	}
	if overall < time.Second || overall > 24*time.Hour || overall < connect || overall < first {
		return fmt.Errorf("整体超时必须在 1s-24h 且不小于连接/首 token 超时，当前 %s", overall)
	}
	if s.RetryChannels < 1 || s.RetryChannels > 10 {
		return fmt.Errorf("重试渠道数必须在 1-10 之间，当前 %d", s.RetryChannels)
	}
	if s.RetryKeysPerChannel < 1 || s.RetryKeysPerChannel > 10 {
		return fmt.Errorf("每渠道重试 key 数必须在 1-10 之间，当前 %d", s.RetryKeysPerChannel)
	}
	if retryTotal < time.Second || retryTotal > 15*time.Minute {
		return fmt.Errorf("重试总时长必须在 1s-15m 之间，当前 %s", retryTotal)
	}
	if s.CooldownFailThreshold < 1 || s.CooldownFailThreshold > 100 {
		return fmt.Errorf("cooldown 阈值必须在 1-100 之间，当前 %d", s.CooldownFailThreshold)
	}
	if cooldownBase < time.Second || cooldownBase > time.Hour {
		return fmt.Errorf("cooldown 基础时长必须在 1s-1h 之间，当前 %s", cooldownBase)
	}
	if cooldownMax < cooldownBase || cooldownMax > 24*time.Hour {
		return fmt.Errorf("cooldown 上限必须不小于基础时长且不超过 24h，当前 %s", cooldownMax)
	}
	if s.LogRetentionDays < 1 || s.LogRetentionDays > 3650 {
		return fmt.Errorf("日志保留天数必须在 1-3650 之间，当前 %d", s.LogRetentionDays)
	}
	if healthInterval < 10*time.Second || healthInterval > 24*time.Hour {
		return fmt.Errorf("健康探测间隔必须在 10s-24h 之间，当前 %s", healthInterval)
	}
	if s.HealthProbeConcurrency < 1 || s.HealthProbeConcurrency > 32 {
		return fmt.Errorf("健康探测并发必须在 1-32 之间，当前 %d", s.HealthProbeConcurrency)
	}
	if healthTimeout < time.Second || healthTimeout > 2*time.Minute {
		return fmt.Errorf("健康探测超时必须在 1s-2m 之间，当前 %s", healthTimeout)
	}
	return nil
}

func toRow(s Settings) store.RuntimeSetting {
	return store.RuntimeSetting{
		ConnectTimeoutMS: s.ConnectTimeoutMS, FirstTokenTimeoutMS: s.FirstTokenTimeoutMS,
		OverallTimeoutMS: s.OverallTimeoutMS, RetryChannels: s.RetryChannels,
		RetryKeysPerChannel: s.RetryKeysPerChannel, RetryTotalMS: s.RetryTotalMS,
		CooldownFailThreshold: s.CooldownFailThreshold, CooldownBaseMS: s.CooldownBaseMS,
		CooldownMaxMS: s.CooldownMaxMS, LogRetentionDays: s.LogRetentionDays,
		HealthProbeIntervalMS: s.HealthProbeIntervalMS, HealthProbeConcurrency: s.HealthProbeConcurrency,
		HealthProbeTimeoutMS: s.HealthProbeTimeoutMS, Source: s.Source,
		InitializedAt: s.InitializedAt, UpdatedAt: s.UpdatedAt,
	}
}

func fromRow(r store.RuntimeSetting) Settings {
	return Settings{
		Values: Values{
			ConnectTimeoutMS: r.ConnectTimeoutMS, FirstTokenTimeoutMS: r.FirstTokenTimeoutMS,
			OverallTimeoutMS: r.OverallTimeoutMS, RetryChannels: r.RetryChannels,
			RetryKeysPerChannel: r.RetryKeysPerChannel, RetryTotalMS: r.RetryTotalMS,
			CooldownFailThreshold: r.CooldownFailThreshold, CooldownBaseMS: r.CooldownBaseMS,
			CooldownMaxMS: r.CooldownMaxMS, LogRetentionDays: r.LogRetentionDays,
			HealthProbeIntervalMS: r.HealthProbeIntervalMS, HealthProbeConcurrency: r.HealthProbeConcurrency,
			HealthProbeTimeoutMS: r.HealthProbeTimeoutMS,
		},
		Source:        r.Source,
		InitializedAt: r.InitializedAt, UpdatedAt: r.UpdatedAt,
	}
}

func toUpdates(r store.RuntimeSetting) map[string]any {
	return map[string]any{
		"connect_timeout_ms": r.ConnectTimeoutMS, "first_token_timeout_ms": r.FirstTokenTimeoutMS,
		"overall_timeout_ms": r.OverallTimeoutMS, "retry_channels": r.RetryChannels,
		"retry_keys_per_channel": r.RetryKeysPerChannel, "retry_total_ms": r.RetryTotalMS,
		"cooldown_fail_threshold": r.CooldownFailThreshold, "cooldown_base_ms": r.CooldownBaseMS,
		"cooldown_max_ms": r.CooldownMaxMS, "log_retention_days": r.LogRetentionDays,
		"health_probe_interval_ms": r.HealthProbeIntervalMS, "health_probe_concurrency": r.HealthProbeConcurrency,
		"health_probe_timeout_ms": r.HealthProbeTimeoutMS, "source": r.Source,
		"initialized_at": r.InitializedAt, "updated_at": r.UpdatedAt,
	}
}

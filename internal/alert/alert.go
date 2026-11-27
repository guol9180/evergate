// Package alert 把运行事件异步推送到可配置的 Webhook（通用 JSON / 企业微信 / 飞书 / Telegram / Bark）。
// 发送完全异步且失败只记日志，绝不阻塞转发路径；负载只含事件与对象标识，不含任何密钥。
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"evergate/internal/store"
)

const (
	PresetGeneric  = "generic"
	PresetWeCom    = "wecom"
	PresetFeishu   = "feishu"
	PresetTelegram = "telegram"
	PresetBark     = "bark"

	// EventChannelDown / EventChannelRecovered / EventKeyDisabled / EventTokenQuotaLow /
	// EventTokenQuotaExhausted / EventUpdateFailed 是当前支持的事件。
	EventChannelDown         = "channel_down"
	EventChannelRecovered    = "channel_recovered"
	EventKeyDisabled         = "key_disabled"
	EventTokenQuotaLow       = "token_quota_low"
	EventTokenQuotaExhausted = "token_quota_exhausted"
	EventUpdateFailed        = "update_failed"
	// EventTest 只用于设置页的测试发送。
	EventTest = "test"

	settingRowID  = int64(1)
	recordKeep    = 200
	dedupWindow   = 5 * time.Minute
	sendTimeout   = 10 * time.Second
	maxConcurrent = 4
)

// Events 返回全部支持订阅的事件。
func Events() []string {
	return []string{
		EventChannelDown, EventChannelRecovered, EventKeyDisabled,
		EventTokenQuotaLow, EventTokenQuotaExhausted, EventUpdateFailed,
	}
}

// ValidPreset 判断预设是否受支持。
func ValidPreset(preset string) bool {
	switch preset {
	case PresetGeneric, PresetWeCom, PresetFeishu, PresetTelegram, PresetBark:
		return true
	}
	return false
}

// Manager 负责配置读写、异步发送与发送记录。
type Manager struct {
	db       *gorm.DB
	client   *http.Client
	now      func() time.Time
	logger   *slog.Logger
	sem      chan struct{}
	mu       sync.Mutex
	lastSent map[string]time.Time
}

func New(db *gorm.DB, logger *slog.Logger, now func() time.Time) *Manager {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Manager{
		db:       db,
		client:   &http.Client{Timeout: sendTimeout},
		now:      now,
		logger:   logger,
		sem:      make(chan struct{}, maxConcurrent),
		lastSent: make(map[string]time.Time),
	}
}

// Setting 读取当前告警配置；未配置时返回默认值（关闭）。
func (m *Manager) Setting() (store.AlertSetting, error) {
	setting := store.AlertSetting{ID: settingRowID, Preset: PresetGeneric}
	if m.db == nil {
		return setting, nil
	}
	err := m.db.First(&setting, settingRowID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.AlertSetting{ID: settingRowID, Preset: PresetGeneric}, nil
	}
	return setting, err
}

// Save 校验并保存配置（单行 upsert）。
func (m *Manager) Save(setting store.AlertSetting) (store.AlertSetting, error) {
	if !ValidPreset(setting.Preset) {
		return setting, fmt.Errorf("unsupported preset %q", setting.Preset)
	}
	if setting.Enabled && strings.TrimSpace(setting.URL) == "" {
		return setting, errors.New("url is required when alerts are enabled")
	}
	if setting.URL != "" && !strings.HasPrefix(setting.URL, "http://") && !strings.HasPrefix(setting.URL, "https://") {
		return setting, errors.New("url must start with http:// or https://")
	}
	events := make([]string, 0, len(setting.Events))
	seen := make(map[string]bool, len(setting.Events))
	for _, event := range setting.Events {
		if !validEvent(event) {
			return setting, fmt.Errorf("unsupported event %q", event)
		}
		if !seen[event] {
			seen[event] = true
			events = append(events, event)
		}
	}
	setting.ID = settingRowID
	setting.Events = events
	setting.URL = strings.TrimSpace(setting.URL)
	setting.UpdatedAt = m.now()
	if m.db != nil {
		if err := m.db.Save(&setting).Error; err != nil {
			return setting, err
		}
	}
	return setting, nil
}

func validEvent(event string) bool {
	for _, candidate := range Events() {
		if candidate == event {
			return true
		}
	}
	return false
}

// Recent 返回最近的发送记录（新 → 旧）。
func (m *Manager) Recent(limit int) ([]store.AlertRecord, error) {
	if m.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > recordKeep {
		limit = 20
	}
	var records []store.AlertRecord
	err := m.db.Order("id desc").Limit(limit).Find(&records).Error
	return records, err
}

// Notify 异步发送事件；未启用 / 未订阅 / 冷却期内直接跳过。调用方不会被阻塞。
func (m *Manager) Notify(event, summary string, details map[string]any) {
	if m == nil {
		return
	}
	go m.deliver(event, summary, details, false)
}

// SendTest 同步发送一条测试告警（忽略启用状态与事件订阅），返回落库的记录。
func (m *Manager) SendTest() (store.AlertRecord, error) {
	return m.deliver(EventTest, "test alert", map[string]any{"source": "settings page"}, true)
}

func (m *Manager) deliver(event, summary string, details map[string]any, force bool) (store.AlertRecord, error) {
	record := store.AlertRecord{CreatedAt: m.now(), Event: event, Message: summary}
	setting, err := m.Setting()
	if err != nil {
		return record, err
	}
	record.Preset = setting.Preset
	if !force {
		if !setting.Enabled || setting.URL == "" || !subscribed(setting.Events, event) {
			return record, nil
		}
	}
	if setting.URL == "" {
		return record, errors.New("alert webhook url is empty")
	}
	if !force && !m.acquireDedup(event+"|"+summary) {
		return record, nil
	}

	m.sem <- struct{}{}
	defer func() { <-m.sem }()

	text := BuildText(event, summary, details, m.now())
	payload, err := BuildPayload(setting.Preset, text, event, details)
	if err != nil {
		record.Result, record.Error = "failed", err.Error()
		m.persist(&record)
		return record, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, setting.URL, bytes.NewReader(payload))
	if err != nil {
		record.Result, record.Error = "failed", err.Error()
		m.persist(&record)
		return record, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		record.Result, record.Error = "failed", err.Error()
		m.persist(&record)
		m.warn(event, err)
		return record, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	record.StatusCode = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		record.Result = "failed"
		record.Error = fmt.Sprintf("webhook returned HTTP %d", resp.StatusCode)
		m.persist(&record)
		return record, errors.New(record.Error)
	}
	if force {
		record.Result = "test"
	} else {
		record.Result = "sent"
	}
	m.persist(&record)
	return record, nil
}

func subscribed(events []string, event string) bool {
	for _, candidate := range events {
		if candidate == event {
			return true
		}
	}
	return false
}

// acquireDedup 同一事件 + 摘要在一个冷却窗口内只发一次，避免故障期间刷屏。
func (m *Manager) acquireDedup(key string) bool {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if last, ok := m.lastSent[key]; ok && now.Sub(last) < dedupWindow {
		return false
	}
	if len(m.lastSent) > 512 {
		for k, at := range m.lastSent {
			if now.Sub(at) >= dedupWindow {
				delete(m.lastSent, k)
			}
		}
	}
	m.lastSent[key] = now
	return true
}

func (m *Manager) persist(record *store.AlertRecord) {
	if m.db == nil {
		return
	}
	if err := m.db.Create(record).Error; err != nil {
		m.warn("alert record", err)
		return
	}
	var oldest struct{ ID int64 }
	if err := m.db.Model(&store.AlertRecord{}).Select("id").Order("id desc").Offset(recordKeep).Limit(1).Scan(&oldest).Error; err == nil && oldest.ID > 0 {
		_ = m.db.Where("id <= ?", oldest.ID).Delete(&store.AlertRecord{}).Error
	}
}

func (m *Manager) warn(msg string, err error) {
	if m.logger != nil {
		m.logger.Warn("alert delivery failed", slog.String("stage", msg), slog.Any("error", err))
	}
}

// BuildText 生成人类可读的告警正文（英文，便于跨团队阅读）。
func BuildText(event, summary string, details map[string]any, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[EverGate] %s", event)
	if summary != "" {
		fmt.Fprintf(&b, ": %s", summary)
	}
	for _, key := range sortedKeys(details) {
		fmt.Fprintf(&b, "\n%s: %v", key, details[key])
	}
	fmt.Fprintf(&b, "\ntime: %s", now.UTC().Format(time.RFC3339))
	return b.String()
}

func sortedKeys(details map[string]any) []string {
	keys := make([]string, 0, len(details))
	for key := range details {
		keys = append(keys, key)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// BuildPayload 按预设生成请求体；纯函数便于表驱动测试。
func BuildPayload(preset, text, event string, details map[string]any) ([]byte, error) {
	switch preset {
	case PresetWeCom:
		return json.Marshal(map[string]any{
			"msgtype": "text",
			"text":    map[string]any{"content": text},
		})
	case PresetFeishu:
		return json.Marshal(map[string]any{
			"msg_type": "text",
			"content":  map[string]any{"text": text},
		})
	case PresetTelegram:
		return json.Marshal(map[string]any{"text": text})
	case PresetBark:
		title := "[EverGate] " + event
		return json.Marshal(map[string]any{"title": title, "body": text})
	case PresetGeneric, "":
		return json.Marshal(map[string]any{
			"event":   event,
			"message": text,
			"details": details,
		})
	default:
		return nil, fmt.Errorf("unsupported preset %q", preset)
	}
}

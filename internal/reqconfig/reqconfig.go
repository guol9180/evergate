// Package reqconfig 解析请求级 X-EverGate-Config 覆盖配置（任务 T24）。
//
// 设计约束：
//   - 严格白名单：只允许超时、重试预算、候选渠道与亲和开关，未知字段直接报错；
//   - 不接受任何鉴权、密钥、模型白名单或上游 key 相关字段，权限治理不可被请求覆盖；
//   - 数值先做安全上下限校验/收敛，超上限直接拒绝，避免单请求耗尽资源；
//   - 摘要只输出生效字段，可用于请求日志，绝不包含原始 header。
package reqconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	// MaxHeaderBytes 单个 X-EverGate-Config 的上限，防止超大 header 攻击。
	MaxHeaderBytes = 4096
	// MaxChannelIDs 候选渠道列表上限。
	MaxChannelIDs = 16
	// MaxSummaryBytes 写入 request_logs 的摘要上限。
	MaxSummaryBytes = 512

	minConnect    = 1 * time.Second
	maxConnect    = 60 * time.Second
	minFirstToken = 1 * time.Second
	maxFirstToken = 300 * time.Second
	minOverall    = 5 * time.Second
	maxOverall    = 1800 * time.Second
	minRetryTotal = 5 * time.Second
	maxRetryTotal = 600 * time.Second
	maxRetryChan  = 10
	maxRetryKeys  = 10
	summaryIDs    = 4
)

// Config 是解析后的单请求覆盖；nil 字段表示未覆盖。
type Config struct {
	Timeouts        *Timeouts
	Retry           *Retry
	ChannelIDs      []int64
	SessionAffinity *bool
}

// Timeouts 是单请求超时覆盖（已 clamp 到安全下限，超上限在解析期拒绝）。
type Timeouts struct {
	Connect    *time.Duration
	FirstToken *time.Duration
	Overall    *time.Duration
}

// Retry 是单请求重试预算覆盖。
type Retry struct {
	Channels       *int
	KeysPerChannel *int
	Total          *time.Duration
}

// Error 带字段路径的校验错误（path 为空表示整段 header）。
type Error struct {
	Path    string
	Message string
}

func (e *Error) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return e.Path + ": " + e.Message
}

func errf(path, format string, args ...any) error {
	return &Error{Path: path, Message: fmt.Sprintf(format, args...)}
}

// Parse 解析 header；空字符串返回空 Config。任何非法输入均返回 *Error。
func Parse(raw string) (*Config, error) {
	cfg := &Config{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return cfg, nil
	}
	if len(raw) > MaxHeaderBytes {
		return nil, errf("", "配置超过 %d 字节上限", MaxHeaderBytes)
	}
	top, err := object(json.RawMessage(raw), "")
	if err != nil {
		return nil, err
	}
	for key, value := range top {
		switch key {
		case "timeouts":
			timeouts, err := parseTimeouts(value)
			if err != nil {
				return nil, err
			}
			cfg.Timeouts = timeouts
		case "retry":
			retry, err := parseRetry(value)
			if err != nil {
				return nil, err
			}
			cfg.Retry = retry
		case "channels":
			ids, err := parseChannels(value)
			if err != nil {
				return nil, err
			}
			cfg.ChannelIDs = ids
		case "session_affinity":
			v, err := boolValue(value, "session_affinity")
			if err != nil {
				return nil, err
			}
			cfg.SessionAffinity = &v
		default:
			return nil, errf(key, "未知字段（仅允许 timeouts/retry/channels/session_affinity）")
		}
	}
	return cfg, nil
}

func parseTimeouts(raw json.RawMessage) (*Timeouts, error) {
	obj, err := object(raw, "timeouts")
	if err != nil {
		return nil, err
	}
	out := &Timeouts{}
	for key, value := range obj {
		path := "timeouts." + key
		switch key {
		case "connect":
			d, err := durationValue(value, path, minConnect, maxConnect)
			if err != nil {
				return nil, err
			}
			out.Connect = &d
		case "first_token":
			d, err := durationValue(value, path, minFirstToken, maxFirstToken)
			if err != nil {
				return nil, err
			}
			out.FirstToken = &d
		case "overall":
			d, err := durationValue(value, path, minOverall, maxOverall)
			if err != nil {
				return nil, err
			}
			out.Overall = &d
		default:
			return nil, errf(path, "未知字段（仅允许 connect/first_token/overall）")
		}
	}
	if out.Overall != nil && out.FirstToken != nil && *out.Overall < *out.FirstToken {
		return nil, errf("timeouts.overall", "必须 >= timeouts.first_token")
	}
	return out, nil
}

func parseRetry(raw json.RawMessage) (*Retry, error) {
	obj, err := object(raw, "retry")
	if err != nil {
		return nil, err
	}
	out := &Retry{}
	for key, value := range obj {
		path := "retry." + key
		switch key {
		case "channels":
			n, err := intValue(value, path)
			if err != nil {
				return nil, err
			}
			if n < 1 || n > maxRetryChan {
				return nil, errf(path, "必须在 1-%d", maxRetryChan)
			}
			out.Channels = &n
		case "keys_per_channel":
			n, err := intValue(value, path)
			if err != nil {
				return nil, err
			}
			if n < 1 || n > maxRetryKeys {
				return nil, errf(path, "必须在 1-%d", maxRetryKeys)
			}
			out.KeysPerChannel = &n
		case "total":
			d, err := durationValue(value, path, minRetryTotal, maxRetryTotal)
			if err != nil {
				return nil, err
			}
			out.Total = &d
		default:
			return nil, errf(path, "未知字段（仅允许 channels/keys_per_channel/total）")
		}
	}
	return out, nil
}

func parseChannels(raw json.RawMessage) ([]int64, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, errf("channels", "必须是整数数组")
	}
	if len(values) == 0 {
		return nil, errf("channels", "不能为空")
	}
	if len(values) > MaxChannelIDs {
		return nil, errf("channels", "最多 %d 个渠道", MaxChannelIDs)
	}
	seen := make(map[int64]bool, len(values))
	out := make([]int64, 0, len(values))
	for i, item := range values {
		path := fmt.Sprintf("channels[%d]", i)
		id, err := int64Value(item)
		if err != nil {
			return nil, errf(path, "必须是整数")
		}
		if id <= 0 {
			return nil, errf(path, "必须是正整数渠道 id")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

func object(raw json.RawMessage, path string) (map[string]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&obj); err != nil {
		return nil, errf(path, "必须是 JSON 对象")
	}
	if obj == nil {
		return nil, errf(path, "必须是 JSON 对象")
	}
	if err := ensureEOF(dec); err != nil {
		return nil, errf(path, "JSON 之后存在多余内容")
	}
	return obj, nil
}

func ensureEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("extra data")
	}
	return nil
}

func durationValue(raw json.RawMessage, path string, min, max time.Duration) (time.Duration, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, errf(path, "必须是时长字符串，如 \"30s\"")
	}
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil {
		return 0, errf(path, "非法时长 %q", s)
	}
	if d <= 0 {
		return 0, errf(path, "必须 > 0")
	}
	if d > max {
		return 0, errf(path, "超过上限 %s", max)
	}
	if d < min {
		return min, nil // 安全下限收敛，避免请求设置过小超时导致误切换
	}
	return d, nil
}

func intValue(raw json.RawMessage, path string) (int, error) {
	n, err := int64Value(raw)
	if err != nil {
		return 0, errf(path, "必须是整数")
	}
	return int(n), nil
}

// int64Value 只接受 JSON 数字字面量；json.Number 会把带引号的 "2" 也解析成功，
// 因此这里显式检查首字符，保证类型错误能按字段路径拒绝。
func int64Value(raw json.RawMessage) (int64, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	if s[0] != '-' && (s[0] < '0' || s[0] > '9') {
		return 0, fmt.Errorf("not a number")
	}
	return strconv.ParseInt(s, 10, 64)
}

func boolValue(raw json.RawMessage, path string) (bool, error) {
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		return false, errf(path, "必须是布尔值")
	}
	return v, nil
}

// Summary 返回脱敏后的生效摘要（JSON 字符串，<= MaxSummaryBytes）；
// 未携带任何覆盖时返回空串。
func (c *Config) Summary() string {
	if c == nil || (c.Timeouts == nil && c.Retry == nil && len(c.ChannelIDs) == 0 && c.SessionAffinity == nil) {
		return ""
	}
	type timeoutsSummary struct {
		ConnectMS    *int64 `json:"connect_ms,omitempty"`
		FirstTokenMS *int64 `json:"first_token_ms,omitempty"`
		OverallMS    *int64 `json:"overall_ms,omitempty"`
	}
	type retrySummary struct {
		Channels       *int   `json:"channels,omitempty"`
		KeysPerChannel *int   `json:"keys_per_channel,omitempty"`
		TotalMS        *int64 `json:"total_ms,omitempty"`
	}
	type summary struct {
		Timeouts        *timeoutsSummary `json:"timeouts,omitempty"`
		Retry           *retrySummary    `json:"retry,omitempty"`
		Channels        []int64          `json:"channels,omitempty"`
		ChannelsCount   int              `json:"channels_count,omitempty"`
		ChannelsElided  bool             `json:"channels_elided,omitempty"`
		SessionAffinity *bool            `json:"session_affinity,omitempty"`
	}
	out := summary{ChannelsCount: len(c.ChannelIDs), SessionAffinity: c.SessionAffinity}
	if len(c.ChannelIDs) > summaryIDs {
		out.Channels = c.ChannelIDs[:summaryIDs]
		out.ChannelsElided = true
	} else {
		out.Channels = c.ChannelIDs
	}
	if c.Timeouts != nil {
		ts := &timeoutsSummary{}
		if c.Timeouts.Connect != nil {
			ms := c.Timeouts.Connect.Milliseconds()
			ts.ConnectMS = &ms
		}
		if c.Timeouts.FirstToken != nil {
			ms := c.Timeouts.FirstToken.Milliseconds()
			ts.FirstTokenMS = &ms
		}
		if c.Timeouts.Overall != nil {
			ms := c.Timeouts.Overall.Milliseconds()
			ts.OverallMS = &ms
		}
		out.Timeouts = ts
	}
	if c.Retry != nil {
		rs := &retrySummary{Channels: c.Retry.Channels, KeysPerChannel: c.Retry.KeysPerChannel}
		if c.Retry.Total != nil {
			ms := c.Retry.Total.Milliseconds()
			rs.TotalMS = &ms
		}
		out.Retry = rs
	}
	data, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	if len(data) > MaxSummaryBytes {
		out.Channels = nil
		out.ChannelsElided = true
		data, _ = json.Marshal(out)
	}
	return string(data)
}

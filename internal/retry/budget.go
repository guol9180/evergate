package retry

import "time"

// Budget 单请求重试预算（任务书 4.5⑤：默认 3 渠道 × 每渠道 2 key，总时长上限 60s）。
type Budget struct {
	MaxChannels       int
	MaxKeysPerChannel int
	Deadline          time.Time // 零值表示不限总时长
}

// NewBudget 构造重试预算；total <= 0 表示不限总时长。
func NewBudget(maxChannels, maxKeysPerChannel int, total time.Duration, now time.Time) Budget {
	b := Budget{MaxChannels: maxChannels, MaxKeysPerChannel: maxKeysPerChannel}
	if total > 0 {
		b.Deadline = now.Add(total)
	}
	return b
}

// Allow 判断是否还允许发起第 channelAttempt 个渠道的第 keyAttempt 个 key（均从 1 计）。
func (b Budget) Allow(channelAttempt, keyAttempt int, now time.Time) bool {
	if channelAttempt < 1 || keyAttempt < 1 {
		return false
	}
	if b.MaxChannels > 0 && channelAttempt > b.MaxChannels {
		return false
	}
	if b.MaxKeysPerChannel > 0 && keyAttempt > b.MaxKeysPerChannel {
		return false
	}
	if !b.Deadline.IsZero() && now.After(b.Deadline) {
		return false
	}
	return true
}

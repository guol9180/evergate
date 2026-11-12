package health

import (
	"evergate/internal/store"
)

// AvailableKeyCount 返回启用且当前可参与调度的 key 数。
// half_open 仍是受控探测态，但比 cooldown/disabled 更接近可恢复。
func AvailableKeyCount(keys []*store.ChannelKey, stateFn func(int64) string) int {
	available := 0
	for _, key := range keys {
		if !key.Enabled {
			continue
		}
		switch stateFn(key.ID) {
		case store.KeyStateActive, store.KeyStateHalfOpen:
			available++
		}
	}
	return available
}

// DeriveStatus 将实时 key 状态与最近轻量探测结果合成渠道状态。
// down 优先；存在不可用 key 或最近探测失败为 degraded；
// /models 路径不支持时为 inconclusive，不用一次不可判定的探测否定整个渠道。
func DeriveStatus(ch *store.Channel, keys []*store.ChannelKey, stateFn func(int64) string) string {
	if ch == nil || !ch.Enabled || AvailableKeyCount(keys, stateFn) == 0 {
		return store.ChannelStatusDown
	}
	if ch.LastProbeResult == ProbeResultInconclusive {
		return store.ChannelStatusInconclusive
	}
	available := AvailableKeyCount(keys, stateFn)
	enabled := 0
	for _, key := range keys {
		if key.Enabled {
			enabled++
		}
	}
	if enabled > available || ch.LastProbeResult == ProbeResultFailure {
		return store.ChannelStatusDegraded
	}
	return store.ChannelStatusActive
}

// StatusRank 供管理列表把异常渠道排在前面。
func StatusRank(status string) int {
	switch status {
	case store.ChannelStatusDown:
		return 0
	case store.ChannelStatusDegraded:
		return 1
	case store.ChannelStatusInconclusive:
		return 2
	default:
		return 3
	}
}

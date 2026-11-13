package health

import (
	"log/slog"
	"strconv"

	"gorm.io/gorm"

	"evergate/internal/metrics"
	"evergate/internal/pool"
	"evergate/internal/store"
)

// ChannelMetricSamples 在每次 /metrics 抓取时输出渠道健康与 key 状态 gauges。
// 只读 DB 与内存状态机，不触发上游请求；查询失败返回已采集部分并告警。
func ChannelMetricSamples(db *gorm.DB, p *pool.Manager) []metrics.Sample {
	if db == nil {
		return nil
	}
	var channels []*store.Channel
	if err := db.Order("id asc").Find(&channels).Error; err != nil {
		slog.Warn("metrics: query channels failed", slog.Any("error", err))
		return nil
	}
	var keys []*store.ChannelKey
	if err := db.Order("id asc").Find(&keys).Error; err != nil {
		slog.Warn("metrics: query channel keys failed", slog.Any("error", err))
		return nil
	}
	keysByChannel := make(map[int64][]*store.ChannelKey, len(channels))
	for _, key := range keys {
		keysByChannel[key.ChannelID] = append(keysByChannel[key.ChannelID], key)
	}
	stateFn := func(int64) string { return store.KeyStateActive }
	if p != nil {
		stateFn = p.StateMachine().State
	}

	samples := make([]metrics.Sample, 0, len(channels)*5)
	for _, ch := range channels {
		channelKeys := keysByChannel[ch.ID]
		channelLabel := strconv.FormatInt(ch.ID, 10)
		samples = append(samples, metrics.Sample{
			Name: "evergate_channel_status", Help: "渠道聚合健康状态（1 表示当前状态）",
			Labels: map[string]string{"channel": channelLabel, "status": DeriveStatus(ch, channelKeys, stateFn)},
			Value:  1,
		})
		counts := map[string]int{
			store.KeyStateActive: 0, store.KeyStateCooldown: 0,
			store.KeyStateHalfOpen: 0, store.KeyStateDisabled: 0,
		}
		for _, key := range channelKeys {
			state := stateFn(key.ID)
			if _, ok := counts[state]; ok {
				counts[state]++
			}
		}
		for _, state := range []string{store.KeyStateActive, store.KeyStateCooldown, store.KeyStateHalfOpen, store.KeyStateDisabled} {
			samples = append(samples, metrics.Sample{
				Name: "evergate_channel_keys", Help: "渠道内 key 数量，按调度状态分组",
				Labels: map[string]string{"channel": channelLabel, "state": state},
				Value:  float64(counts[state]),
			})
		}
	}
	return samples
}

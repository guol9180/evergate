package admin

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"evergate/internal/health"
	"evergate/internal/store"
)

// testAll 并发上限与整体超时：探测只调用上游模型列表 GET，不触发推理。
const (
	testAllConcurrency = 4
	testAllTimeout     = 30 * time.Second
)

type testAllItem struct {
	ChannelID int64  `json:"channel_id"`
	Name      string `json:"name"`
	Ok        bool   `json:"ok"`
	Result    string `json:"result"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

// testAllChannels 对全部启用渠道并发执行与健康探测相同的模型列表 GET，用于一键摸底与按延迟排序。
func (a *API) testAllChannels(c *gin.Context) {
	if a.opt.Health == nil {
		fail(c, http.StatusInternalServerError, "prober_not_ready", "健康探测器未初始化")
		return
	}
	var channels []*store.Channel
	if err := a.opt.DB.Where("enabled = ?", true).Order("priority asc, id asc").Find(&channels).Error; err != nil {
		fail(c, http.StatusInternalServerError, "query_failed", "查询失败")
		return
	}
	timeout, nextInterval := 10*time.Second, 5*time.Minute
	if a.opt.Settings != nil {
		current := a.opt.Settings.Snapshot()
		timeout = time.Duration(current.HealthProbeTimeoutMS) * time.Millisecond
		nextInterval = time.Duration(current.HealthProbeIntervalMS) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), testAllTimeout)
	defer cancel()

	items := make([]testAllItem, len(channels))
	sem := make(chan struct{}, testAllConcurrency)
	var wg sync.WaitGroup
	for i, ch := range channels {
		wg.Add(1)
		go func(i int, ch *store.Channel) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				items[i] = testAllItem{ChannelID: ch.ID, Name: ch.Name, Result: "skipped", Error: "整体超时"}
				return
			}
			items[i] = a.probeOneForTestAll(ctx, ch, timeout, nextInterval)
		}(i, ch)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		<-done
	}
	ok(c, gin.H{"items": items, "timeout_sec": int(testAllTimeout.Seconds())})
}

// probeOneForTestAll 复用单渠道测试的探测逻辑，把错误折叠为可展示的字段而不是中止整个批次。
func (a *API) probeOneForTestAll(ctx context.Context, ch *store.Channel, timeout, nextInterval time.Duration) testAllItem {
	item := testAllItem{ChannelID: ch.ID, Name: ch.Name}
	var keys []*store.ChannelKey
	if err := a.opt.DB.WithContext(ctx).Where("channel_id = ? AND enabled = ?", ch.ID, true).Order("id asc").Find(&keys).Error; err != nil {
		item.Result, item.Error = "error", "查询 key 失败"
		return item
	}
	result, err := a.opt.Health.ProbeChannel(ctx, ch, keys, timeout, nextInterval)
	if err != nil {
		item.Result = "error"
		switch {
		case err == health.ErrNoAvailableKey:
			item.Error = "渠道没有可用 key"
		case err == health.ErrChannelDisabled:
			item.Error = "渠道已停用"
		case err == context.Canceled, err == context.DeadlineExceeded:
			item.Error = "测试超时或已取消"
		default:
			item.Error = "测试失败"
		}
		return item
	}
	item.Ok = result.Result == health.ProbeResultSuccess
	item.Result, item.LatencyMS, item.Error = result.Result, result.LatencyMS, result.Error
	return item
}

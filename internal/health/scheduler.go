package health

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"evergate/internal/pool"
	"evergate/internal/settings"
	"evergate/internal/store"
)

// SchedulerOptions 装配周期探测任务。WaitFunc 供测试注入“到达周期/配置变更”事件。
type SchedulerOptions struct {
	DB        *gorm.DB
	MasterKey string
	Pool      *pool.Manager
	Settings  *settings.Manager
	Client    HTTPClient
	Logger    *slog.Logger
	Now       func() time.Time
	WaitFunc  func(context.Context, time.Duration, <-chan struct{}) bool
}

type scheduler struct {
	prober *Prober
	opt    SchedulerOptions
}

// StartProbes 启动周期健康探测。设置变更会废弃旧 timer 并按新间隔重建；
// 返回的 stop 会取消未完成 HTTP 请求并等待本轮 goroutine 退出。
func StartProbes(opt SchedulerOptions) (stop func()) {
	if opt.DB == nil || opt.Pool == nil || opt.Settings == nil {
		return func() {}
	}
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	wait := opt.WaitFunc
	if wait == nil {
		wait = timerWait
	}
	opt.WaitFunc = nil
	s := &scheduler{prober: NewProber(ProberOptions{
		DB: opt.DB, MasterKey: opt.MasterKey, Pool: opt.Pool,
		Client: opt.Client, Now: opt.Now,
	}), opt: opt}

	ctx, cancel := context.WithCancel(context.Background())
	changes, unsubscribe := opt.Settings.Subscribe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runOnce(ctx)
		for {
			interval := time.Duration(opt.Settings.Snapshot().HealthProbeIntervalMS) * time.Millisecond
			fired := wait(ctx, interval, changes)
			if ctx.Err() != nil {
				return
			}
			if fired {
				s.runOnce(ctx)
			}
		}
	}()
	return func() {
		cancel()
		<-done
		unsubscribe()
	}
}

func (s *scheduler) runOnce(ctx context.Context) {
	snapshot := s.opt.Settings.Snapshot()
	var channels []*store.Channel
	if err := s.opt.DB.Where("enabled = ?", true).Order("id asc").Find(&channels).Error; err != nil {
		s.opt.Logger.Error("load probe channels failed", slog.Any("error", err))
		return
	}
	if len(channels) == 0 {
		return
	}
	keys := make([]*store.ChannelKey, 0)
	if err := s.opt.DB.Where("enabled = ?", true).Order("id asc").Find(&keys).Error; err != nil {
		s.opt.Logger.Error("load probe keys failed", slog.Any("error", err))
		return
	}
	keysByChannel := make(map[int64][]*store.ChannelKey)
	for _, key := range keys {
		keysByChannel[key.ChannelID] = append(keysByChannel[key.ChannelID], key)
	}

	concurrency := snapshot.HealthProbeConcurrency
	if concurrency < 1 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, ch := range channels {
		channelKeys := keysByChannel[ch.ID]
		if len(channelKeys) == 0 {
			s.markNoKeys(ch)
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		wg.Add(1)
		go func(ch *store.Channel, keys []*store.ChannelKey) {
			defer wg.Done()
			defer func() { <-sem }()
			result, err := s.prober.ProbeChannel(ctx, ch, keys,
				time.Duration(snapshot.HealthProbeTimeoutMS)*time.Millisecond,
				time.Duration(snapshot.HealthProbeIntervalMS)*time.Millisecond,
			)
			if err != nil && ctx.Err() == nil {
				s.opt.Logger.Warn("channel probe skipped",
					slog.Int64("channel_id", ch.ID),
					slog.String("channel", ch.Name),
					slog.Any("error", err),
				)
				return
			}
			if result.Result != ProbeResultSuccess && ctx.Err() == nil {
				s.opt.Logger.Warn("channel probe abnormal",
					slog.Int64("channel_id", ch.ID),
					slog.String("channel", ch.Name),
					slog.String("result", result.Result),
					slog.Int("status_code", result.StatusCode),
					slog.String("error_type", result.ErrorType),
					slog.String("error", result.Error),
				)
			}
		}(ch, channelKeys)
	}
	wg.Wait()
}

func (s *scheduler) markNoKeys(ch *store.Channel) {
	if ch.Status == store.ChannelStatusDown {
		return
	}
	if err := s.opt.DB.Model(&store.Channel{}).Where("id = ?", ch.ID).
		Updates(map[string]any{"status": store.ChannelStatusDown, "next_probe_at": nil}).Error; err != nil {
		s.opt.Logger.Error("mark no-key channel failed", slog.Int64("channel_id", ch.ID), slog.Any("error", err))
	}
}

func timerWait(ctx context.Context, interval time.Duration, changes <-chan struct{}) bool {
	if interval <= 0 {
		interval = time.Millisecond
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-changes:
		return false
	case <-ctx.Done():
		return false
	}
}

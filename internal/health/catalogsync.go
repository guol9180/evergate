package health

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"net/http"

	"evergate/internal/pool"
	"evergate/internal/provider"
	"evergate/internal/router"
	"evergate/internal/store"
)

const (
	// catalogSyncInterval 上游目录同步周期；远低于探测频率，避免对上游造成额外压力。
	catalogSyncInterval = 6 * time.Hour
	// catalogSyncStagger 多渠道错峰，避免同一秒并发拉取所有上游。
	catalogSyncStagger = 2 * time.Second
)

// CatalogSyncOptions 装配目录同步任务。
type CatalogSyncOptions struct {
	DB        *gorm.DB
	MasterKey string
	Pool      *pool.Manager
	Client    HTTPClient
	Now       func() time.Time
	// Timeout 返回单次拉取的超时（跟随管理台健康探测超时设置）。
	Timeout func() time.Duration
	Logger  *slog.Logger
}

// StartCatalogSync 启动周期同步：启动后先跑一轮，之后每 catalogSyncInterval 跑一轮；返回的 stop 等待在途拉取结束。
func StartCatalogSync(opt CatalogSyncOptions) (stop func()) {
	if opt.DB == nil || opt.Pool == nil {
		return func() {}
	}
	if opt.Client == nil {
		opt.Client = http.DefaultClient
	}
	if opt.Now == nil {
		opt.Now = func() time.Time { return time.Now().UTC() }
	}
	if opt.Timeout == nil {
		opt.Timeout = func() time.Duration { return 10 * time.Second }
	}
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		SyncAllCatalogs(ctx, opt)
		ticker := time.NewTicker(catalogSyncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				SyncAllCatalogs(ctx, opt)
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

// SyncAllCatalogs 对全部启用渠道依次同步（错峰）；单渠道失败只记日志，不影响其他渠道。
func SyncAllCatalogs(ctx context.Context, opt CatalogSyncOptions) {
	var channels []*store.Channel
	if err := opt.DB.Where("enabled = ?", true).Order("id asc").Find(&channels).Error; err != nil {
		opt.Logger.Warn("catalog sync: load channels failed", slog.Any("error", err))
		return
	}
	for i, ch := range channels {
		if i > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(catalogSyncStagger):
			}
		}
		if ctx.Err() != nil {
			return
		}
		diff, err := SyncChannelCatalog(ctx, opt, ch)
		if err != nil {
			if errors.Is(err, ErrNoKeyForCatalog) {
				// 还没导入 key 是正常状态，不值得每轮告警
				continue
			}
			// 拉取失败不覆盖已有差异：保留上一次结果，等下一轮再试
			opt.Logger.Warn("catalog sync failed", slog.Int64("channel_id", ch.ID), slog.String("channel", ch.Name), slog.Any("error", err))
			continue
		}
		if len(diff.Added) > 0 || len(diff.Removed) > 0 {
			opt.Logger.Info("catalog drift detected",
				slog.Int64("channel_id", ch.ID), slog.String("channel", ch.Name),
				slog.Int("added", len(diff.Added)), slog.Int("removed", len(diff.Removed)))
		}
	}
}

// ErrNoKeyForCatalog 表示渠道当前没有可用于拉取目录的 key。
var ErrNoKeyForCatalog = errors.New("no available key for catalog sync")

// SyncChannelCatalog 拉取一次上游目录并与渠道模型比较，结果写回 channel.catalog_diff。
// 不驱动 key 状态机（无论成败都释放探测名额），也不改写渠道模型列表。
func SyncChannelCatalog(ctx context.Context, opt CatalogSyncOptions, ch *store.Channel) (store.ChannelCatalogDiff, error) {
	var keys []*store.ChannelKey
	if err := opt.DB.Where("channel_id = ? AND enabled = ?", ch.ID, true).Find(&keys).Error; err != nil {
		return store.ChannelCatalogDiff{}, err
	}
	key := opt.Pool.Acquire(ch.ID, keys)
	if key == nil {
		return store.ChannelCatalogDiff{}, ErrNoKeyForCatalog
	}
	defer opt.Pool.CancelProbe(key.ID)
	plain, err := store.Decrypt(key.KeyCipher, opt.MasterKey)
	if err != nil {
		return store.ChannelCatalogDiff{}, err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, opt.Timeout())
	defer cancel()
	result, err := provider.ListModels(fetchCtx, opt.Client, ch, plain)
	if err != nil {
		return store.ChannelCatalogDiff{}, err
	}
	diff := ComputeCatalogDiff(ch, result.Models, opt.Now().UTC())
	// Update(map) 不会走字段 serializer，这里显式序列化成 JSON 文本写入
	payload, err := json.Marshal(diff)
	if err != nil {
		return diff, err
	}
	if err := opt.DB.Model(&store.Channel{}).Where("id = ?", ch.ID).Update("catalog_diff", string(payload)).Error; err != nil {
		return diff, err
	}
	return diff, nil
}

// ComputeCatalogDiff 按上游模型名比较：渠道的对外模型先经 model_mapping 换成上游名再比。
// Added = 上游有而渠道没有；Removed = 渠道声明但上游目录里已经没有（疑似下线）。
func ComputeCatalogDiff(ch *store.Channel, upstream []string, now time.Time) store.ChannelCatalogDiff {
	declared := make(map[string]bool, len(ch.Models))
	for _, public := range ch.Models {
		declared[router.UpstreamModel(ch.ModelMapping, public)] = true
	}
	seen := make(map[string]bool, len(upstream))
	added := make([]string, 0)
	for _, model := range upstream {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		if !declared[model] {
			added = append(added, model)
		}
	}
	removed := make([]string, 0)
	for model := range declared {
		if !seen[model] {
			removed = append(removed, model)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return store.ChannelCatalogDiff{Added: added, Removed: removed, UpstreamCount: len(seen), CheckedAt: now}
}

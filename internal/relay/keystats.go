package relay

import (
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"evergate/internal/httpx"
	"evergate/internal/store"
)

const (
	// keyStatsInterval 是统计落库周期；进程异常退出最多丢一个周期的计数。
	keyStatsInterval = 500 * time.Millisecond
	// keyStatsFlushThreshold 在突发流量下提前刷盘，避免 pending 无限堆积。
	keyStatsFlushThreshold = 256
)

// keyStatsWriter 把上游 key 的成功 / 失败统计合并为批量 UPDATE 异步落库。
// 调度状态机仍在内存里即时更新，这里只延迟「统计列」的持久化，
// 因此冷却与禁用判定不受刷盘周期影响（任务书：首 token 路径不写库）。
type keyStatsWriter struct {
	db     *gorm.DB
	logger *slog.Logger
	now    func() time.Time

	mu       sync.Mutex
	pending  map[int64]*keyStatsDelta
	flushCh  chan struct{}
	stopCh   chan struct{}
	stopped  chan struct{}
	stopOnce sync.Once
}

type keyStatsDelta struct {
	success   int64
	fail      int64
	lastError string
	lastUsed  time.Time
}

func newKeyStatsWriter(db *gorm.DB, logger *slog.Logger, now func() time.Time) *keyStatsWriter {
	w := &keyStatsWriter{
		db:      db,
		logger:  logger,
		now:     now,
		pending: make(map[int64]*keyStatsDelta),
		flushCh: make(chan struct{}, 1),
		stopCh:  make(chan struct{}),
		stopped: make(chan struct{}),
	}
	go w.loop()
	return w
}

func (w *keyStatsWriter) loop() {
	defer close(w.stopped)
	ticker := time.NewTicker(keyStatsInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.Flush()
		case <-w.flushCh:
			w.Flush()
		case <-w.stopCh:
			w.Flush()
			return
		}
	}
}

// Add 记一次尝试结果；同一 key 的多次结果在刷盘前合并成一次 UPDATE。
func (w *keyStatsWriter) Add(keyID int64, success bool, message string) {
	if len(message) > 300 {
		message = httpx.TruncateRunes(message, 300)
	}
	w.mu.Lock()
	delta := w.pending[keyID]
	if delta == nil {
		delta = &keyStatsDelta{}
		w.pending[keyID] = delta
	}
	if success {
		delta.success++
		delta.lastError = ""
	} else {
		delta.fail++
		delta.lastError = message
	}
	delta.lastUsed = w.now().UTC()
	size := len(w.pending)
	w.mu.Unlock()

	if size >= keyStatsFlushThreshold {
		select {
		case w.flushCh <- struct{}{}:
		default:
		}
	}
}

// Flush 立即落库当前批次；停机与测试断言依赖它。
func (w *keyStatsWriter) Flush() {
	w.mu.Lock()
	batch := w.pending
	w.pending = make(map[int64]*keyStatsDelta, len(batch))
	w.mu.Unlock()
	if len(batch) == 0 || w.db == nil {
		return
	}
	for keyID, delta := range batch {
		updates := map[string]any{
			"last_used_at": delta.lastUsed,
			"last_error":   delta.lastError,
		}
		if delta.success > 0 {
			updates["success_count"] = gorm.Expr("success_count + ?", delta.success)
		}
		if delta.fail > 0 {
			updates["fail_count"] = gorm.Expr("fail_count + ?", delta.fail)
		}
		if err := w.db.Model(&store.ChannelKey{}).Where("id = ?", keyID).Updates(updates).Error; err != nil {
			if w.logger != nil {
				w.logger.Warn("persist key stats failed", slog.Int64("key_id", keyID), slog.Any("error", err))
			}
		}
	}
}

// Close 停止后台协程并做最后一次刷盘（可重复调用）。
func (w *keyStatsWriter) Close() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		<-w.stopped
	})
}
